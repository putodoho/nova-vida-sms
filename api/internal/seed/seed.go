// Package seed loads the reference data for the 2027 school year: the two
// companies, levels with penalty rules, classes, fee catalogue, discount types,
// numbering series and the default roles. It is idempotent: running it again
// adds what is missing and never overwrites values edited in the app.
package seed

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

const Year = 2027

// VAT on monthly fees (confirmed by the school's accountant for the primary
// school). Preschool VAT is not confirmed yet, so it starts disabled there.
const vatRateBP = 500

type entitySeed struct {
	code, legal, short, nuit, address, motto string
	levelCode, levelName, penaltyRule        string
}

var entities = []entitySeed{
	{
		code: "primary", legal: "Escola Primária Nova Vida, SU, Lda.", short: "Primária", nuit: "401785507",
		address: "Paragem Portão – Chinunguine B, Cidade de Xai-Xai, Moçambique", motto: "Educando para a Eternidade",
		levelCode: "primary", levelName: "Ensino Primário",
		penaltyRule: `{"type":"percent","rate_bp":500,"grace_days":0}`,
	},
	{
		code: "preschool", legal: "Jardim Infantil Nova Vida", short: "Pré-escolar", nuit: "",
		address: "Cidade de Xai-Xai, Moçambique", motto: "Educar nossas crianças para uma Vida Nova",
		levelCode: "preschool", levelName: "Educação Infantil",
		penaltyRule: `{"type":"fixed_tiers","mode":"replace","grace_days":0,"tiers":[{"after_day":5,"amount":10000},{"after_day":10,"amount":20000}]}`,
	},
}

type classSeed struct {
	entity, name string
	grade        int // 0 = preschool room
	capacity     int
}

var classes = []classSeed{
	{"primary", "1.ª A", 1, 20}, {"primary", "1.ª B", 1, 20},
	{"primary", "2.ª A", 2, 20}, {"primary", "2.ª B", 2, 20},
	{"primary", "3.ª", 3, 20}, {"primary", "4.ª", 4, 20},
	{"preschool", "Sala 1", 0, 20}, {"preschool", "Sala 2", 0, 20},
	{"preschool", "Sala 3", 0, 20}, {"preschool", "Pré", 0, 20},
}

type feeSeed struct {
	entity, code, name, kind string
	amountMT                 int64
	vat                      bool
	dueRule                  string
	mandatory                bool
	classes                  []string // empty = all classes of the entity
}

// Primary one-off fees (enrolment, uniform, books) differ from the preschool
// and are not known yet; the administrator adds them in Settings.
var fees = []feeSeed{
	{"primary", "tuition_1_3", "Mensalidade 1.ª–3.ª classe", "monthly", 2800, true, `{}`, true, []string{"1.ª A", "1.ª B", "2.ª A", "2.ª B", "3.ª"}},
	{"primary", "tuition_4", "Mensalidade 4.ª classe", "monthly", 3000, true, `{}`, true, []string{"4.ª"}},
	{"primary", "after_school", "Extraclasse", "monthly", 1000, true, `{}`, false, nil},
	{"primary", "lunch", "Almoço", "monthly", 1500, true, `{}`, false, nil},

	{"preschool", "tuition_sala1", "Mensalidade Sala 1", "monthly", 3300, false, `{}`, true, []string{"Sala 1"}},
	{"preschool", "tuition", "Mensalidade Sala 2, Sala 3 e Pré", "monthly", 3000, false, `{}`, true, []string{"Sala 2", "Sala 3", "Pré"}},
	{"preschool", "enrolment", "Inscrição", "one_off", 3000, false, `{"on_enrolment":true}`, false, nil},
	{"preschool", "re_enrolment", "Renovação de inscrição", "one_off", 2000, false, `{"on_enrolment":true}`, false, nil},
	{"preschool", "uniform_small", "Uniforme (3–5 anos)", "one_off", 1000, false, `{"date":"2027-01-30"}`, false, nil},
	{"preschool", "uniform", "Uniforme", "one_off", 1200, false, `{"date":"2027-01-30"}`, false, nil},
	{"preschool", "books_sala1", "Livros Sala 1", "one_off", 600, false, `{"date":"2027-01-30"}`, false, []string{"Sala 1"}},
	{"preschool", "books_sala2", "Livros Sala 2", "one_off", 750, false, `{"date":"2027-01-30"}`, false, []string{"Sala 2"}},
	{"preschool", "books_sala3", "Livros Sala 3", "one_off", 800, false, `{"date":"2027-01-30"}`, false, []string{"Sala 3"}},
	{"preschool", "books_pre", "Livros Pré", "one_off", 850, false, `{"date":"2027-01-30"}`, false, []string{"Pré"}},
}

// Discount values are not confirmed; they start at 0 and are set in Settings.
var discountTypes = []struct{ code, name string }{
	{"social", "Desconto social"},
	{"staff_child", "Filho de colaborador"},
	{"siblings", "Irmãos"},
}

// Run applies the seed in one transaction.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	return db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if err := SeedRoles(ctx, tx); err != nil {
			return err
		}
		yearID, err := upsertYear(ctx, tx)
		if err != nil {
			return err
		}
		for _, e := range entities {
			if err := seedEntity(ctx, tx, e, yearID); err != nil {
				return fmt.Errorf("entity %s: %w", e.code, err)
			}
		}
		return nil
	})
}

// SeedRoles creates the default roles. The administrator role always holds
// every permission; the other roles get their defaults only when first
// created, so later edits made in the roles editor are kept.
func SeedRoles(ctx context.Context, q db.Querier) error {
	for _, r := range auth.DefaultRoles() {
		var id uuid.UUID
		var inserted bool
		err := q.QueryRow(ctx, `
			INSERT INTO role (code, name, description, system) VALUES ($1, $2, $3, true)
			ON CONFLICT (code) DO UPDATE SET system = true
			RETURNING id, (xmax = 0)`, r.Code, r.Name, r.Description).Scan(&id, &inserted)
		if err != nil {
			return err
		}
		if r.Code != "admin" && !inserted {
			continue
		}
		for _, p := range r.Permissions {
			if _, err := q.Exec(ctx, `INSERT INTO role_permission (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, string(p)); err != nil {
				return err
			}
		}
	}
	return nil
}

func upsertYear(ctx context.Context, q db.Querier) (uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `
		INSERT INTO school_year (year, starts_on, ends_on, billing_months, due_day, status)
		VALUES ($1, '2027-02-01', '2027-11-30', '{2,3,4,5,6,7,8,9,10,11}', 5, 'open')
		ON CONFLICT (year) DO UPDATE SET year = EXCLUDED.year
		RETURNING id`, Year).Scan(&id)
	return id, err
}

func seedEntity(ctx context.Context, q db.Querier, e entitySeed, yearID uuid.UUID) error {
	var entityID uuid.UUID
	err := q.QueryRow(ctx, `
		INSERT INTO entity (code, legal_name, short_name, nuit, address, motto, vat_method)
		VALUES ($1, $2, $3, $4, $5, $6, 'included_flat')
		ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
		RETURNING id`, e.code, e.legal, e.short, e.nuit, e.address, e.motto).Scan(&entityID)
	if err != nil {
		return err
	}

	var levelID uuid.UUID
	if err := q.QueryRow(ctx, `
		INSERT INTO level (entity_id, code, name, penalty_rule) VALUES ($1, $2, $3, $4)
		ON CONFLICT (entity_id, code) DO UPDATE SET code = EXCLUDED.code
		RETURNING id`, entityID, e.levelCode, e.levelName, e.penaltyRule).Scan(&levelID); err != nil {
		return err
	}

	classIDs := map[string]uuid.UUID{}
	for _, c := range classes {
		if c.entity != e.code {
			continue
		}
		var grade *int
		if c.grade > 0 {
			g := c.grade
			grade = &g
		}
		var id uuid.UUID
		if err := q.QueryRow(ctx, `
			INSERT INTO class (entity_id, level_id, school_year_id, name, grade, capacity)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (entity_id, school_year_id, name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, entityID, levelID, yearID, c.name, grade, c.capacity).Scan(&id); err != nil {
			return err
		}
		classIDs[c.name] = id
	}

	for _, f := range fees {
		if f.entity != e.code {
			continue
		}
		vatMode, rate := money.ModeNone, 0
		if f.vat {
			vatMode, rate = money.ModeIncluded, vatRateBP
		}
		var feeID uuid.UUID
		var inserted bool
		err := q.QueryRow(ctx, `
			INSERT INTO fee_item (entity_id, school_year_id, code, name, kind, amount, vat_mode, vat_rate_bp, due_rule, mandatory)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (entity_id, school_year_id, code) DO UPDATE SET code = EXCLUDED.code
			RETURNING id, (xmax = 0)`,
			entityID, yearID, f.code, f.name, f.kind, int64(money.MT(f.amountMT)), vatMode, rate, f.dueRule, f.mandatory).
			Scan(&feeID, &inserted)
		if err != nil {
			return fmt.Errorf("fee %s: %w", f.code, err)
		}
		if !inserted {
			continue
		}
		for _, cn := range f.classes {
			cid, ok := classIDs[cn]
			if !ok {
				return fmt.Errorf("fee %s: unknown class %q", f.code, cn)
			}
			if _, err := q.Exec(ctx, `INSERT INTO fee_item_class (fee_item_id, class_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, feeID, cid); err != nil {
				return err
			}
		}
	}

	for _, d := range discountTypes {
		if _, err := q.Exec(ctx, `
			INSERT INTO discount_type (entity_id, code, name, mode, value, fee_item_codes)
			VALUES ($1, $2, $3, 'percent', 0, $4)
			ON CONFLICT (entity_id, code) DO NOTHING`, entityID, d.code, d.name, tuitionCodes(e.code)); err != nil {
			return err
		}
	}

	for _, s := range []struct{ series, prefix string }{{"receipt", "REC"}, {"statement", "EXT"}} {
		prefix := s.prefix
		if e.code == "preschool" {
			prefix += "-JI" // distinct series so receipts of the two companies never look alike
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO number_series (entity_id, series, year, prefix) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, entityID, s.series, Year, prefix); err != nil {
			return err
		}
	}

	if _, err := q.Exec(ctx, `
		INSERT INTO receiving_account (entity_id, kind, label)
		SELECT $1, 'cash', 'Caixa da secretaria'
		WHERE NOT EXISTS (SELECT 1 FROM receiving_account WHERE entity_id = $1 AND kind = 'cash')`, entityID); err != nil {
		return err
	}
	return nil
}

func tuitionCodes(entity string) []string {
	var out []string
	for _, f := range fees {
		if f.entity == entity && strings.HasPrefix(f.code, "tuition") {
			out = append(out, f.code)
		}
	}
	return out
}

// CreateAdmin creates (or resets the password of) an administrator with
// access to every entity.
func CreateAdmin(ctx context.Context, pool *pgxpool.Pool, login, name, password string) (uuid.UUID, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	var userID uuid.UUID
	err = db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if err := SeedRoles(ctx, tx); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO app_user (login, display_name, password_hash) VALUES ($1, $2, $3)
			ON CONFLICT (login) DO UPDATE SET password_hash = EXCLUDED.password_hash, display_name = EXCLUDED.display_name, active = true
			RETURNING id`, strings.TrimSpace(login), name, hash).Scan(&userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_role (user_id, role_id) SELECT $1, id FROM role WHERE code = 'admin'
			ON CONFLICT DO NOTHING`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO user_entity (user_id, entity_id) SELECT $1, id FROM entity
			ON CONFLICT DO NOTHING`, userID)
		return err
	})
	return userID, err
}
