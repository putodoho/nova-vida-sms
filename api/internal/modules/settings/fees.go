package settings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// ---------------------------------------------------------------------------
// Fee catalogue
// ---------------------------------------------------------------------------

type FeeItem struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	EntityID     uuid.UUID       `json:"entity_id" db:"entity_id"`
	SchoolYearID uuid.UUID       `json:"school_year_id" db:"school_year_id"`
	Code         string          `json:"code" db:"code"`
	Name         string          `json:"name" db:"name"`
	Kind         string          `json:"kind" db:"kind"`
	Amount       money.Amount    `json:"amount" db:"amount"`
	VATMode      string          `json:"vat_mode" db:"vat_mode"`
	VATRateBP    int             `json:"vat_rate_bp" db:"vat_rate_bp"`
	DueRule      json.RawMessage `json:"due_rule" db:"due_rule"`
	Mandatory    bool            `json:"mandatory" db:"mandatory"`
	Active       bool            `json:"active" db:"active"`
	ClassIDs     []uuid.UUID     `json:"class_ids" db:"class_ids"` // empty = every class
	// Price split with the entity's VAT method, for display.
	Lines money.Lines `json:"lines" db:"-"`
}

const feeSelect = `
	SELECT f.id, f.entity_id, f.school_year_id, f.code, f.name, f.kind, f.amount, f.vat_mode, f.vat_rate_bp,
	       f.due_rule, f.mandatory, f.active,
	       COALESCE((SELECT array_agg(fc.class_id ORDER BY fc.class_id) FROM fee_item_class fc WHERE fc.fee_item_id = f.id), '{}') AS class_ids,
	       e.vat_method
	FROM fee_item f JOIN entity e ON e.id = f.entity_id`

type feeRow struct {
	FeeItem
	VATMethod string `db:"vat_method"`
}

func collectFees(rows pgx.Rows) ([]FeeItem, error) {
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[feeRow])
	if err != nil {
		return nil, err
	}
	out := make([]FeeItem, len(raw))
	for i, fr := range raw {
		fi := fr.FeeItem
		fi.Lines, err = money.SplitVAT(fi.Amount, fi.VATMode, fi.VATRateBP, fr.VATMethod)
		if err != nil {
			return nil, err
		}
		out[i] = fi
	}
	return out, nil
}

func (h *Handler) listFees(w http.ResponseWriter, r *http.Request) error {
	yid, err := yearID(r.Context(), h.Pool, r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), feeSelect+`
		WHERE f.entity_id = ANY($1) AND f.school_year_id = $2
		ORDER BY f.kind DESC, f.code`, auth.EntityScope(r.Context()).EntityIDs, yid)
	if err != nil {
		return err
	}
	items, err := collectFees(rows)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[FeeItem]{Items: nonNil(items), Total: len(items)})
	return nil
}

func loadFee(ctx context.Context, q db.Querier, entityID, id uuid.UUID) (FeeItem, error) {
	rows, err := q.Query(ctx, feeSelect+` WHERE f.id = $1 AND f.entity_id = $2`, id, entityID)
	if err != nil {
		return FeeItem{}, err
	}
	items, err := collectFees(rows)
	if err != nil {
		return FeeItem{}, err
	}
	if len(items) == 0 {
		return FeeItem{}, httpx.NotFound()
	}
	return items[0], nil
}

var codeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

type feeRequest struct {
	SchoolYearID uuid.UUID       `json:"school_year_id"`
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Amount       money.Amount    `json:"amount"`
	VATMode      string          `json:"vat_mode"`
	VATRateBP    int             `json:"vat_rate_bp"`
	DueRule      json.RawMessage `json:"due_rule"`
	Mandatory    bool            `json:"mandatory"`
	Active       *bool           `json:"active"`
	ClassIDs     []uuid.UUID     `json:"class_ids"`
}

func (req *feeRequest) validate(ctx context.Context, q db.Querier, entityID uuid.UUID) error {
	f := httpx.Fields{}
	req.Name = strings.TrimSpace(req.Name)
	f.Add(!codeRe.MatchString(req.Code), "code", "Código: letras minúsculas, números e _ (ex.: tuition_4).")
	f.Add(req.Name == "", "name", "Obrigatório.")
	f.Add(req.Kind != "monthly" && req.Kind != "one_off", "kind", "Tipo inválido.")
	f.Add(req.Amount < 0, "amount", "Valor inválido.")
	switch req.VATMode {
	case money.ModeNone:
		req.VATRateBP = 0
	case money.ModeIncluded, money.ModeAdded:
		f.Add(req.VATRateBP <= 0 || req.VATRateBP > 10_000, "vat_rate_bp", "Taxa de IVA inválida.")
	default:
		f.Add(true, "vat_mode", "Modo de IVA inválido.")
	}
	if req.Kind == "monthly" || req.Kind == "one_off" {
		if len(req.DueRule) == 0 {
			req.DueRule = json.RawMessage(`{}`)
		}
		if _, err := billing.ParseDueRule(req.DueRule, req.Kind); err != nil {
			f.Add(true, "due_rule", err.Error())
		}
	}
	var yearOK bool
	var validClasses int
	if err := q.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM school_year WHERE id = $1),
		       (SELECT count(*) FROM class WHERE id = ANY($3) AND entity_id = $2 AND school_year_id = $1)::int`,
		req.SchoolYearID, entityID, req.ClassIDs).Scan(&yearOK, &validClasses); err != nil {
		return err
	}
	f.Add(!yearOK, "school_year_id", "Ano lectivo inválido.")
	f.Add(validClasses != len(uniqueIDs(req.ClassIDs)), "class_ids", "Turmas inválidas para esta empresa e ano.")
	return f.Err()
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := ids[:0:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func setFeeClasses(ctx context.Context, tx pgx.Tx, feeID uuid.UUID, classIDs []uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM fee_item_class WHERE fee_item_id = $1`, feeID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO fee_item_class (fee_item_id, class_id) SELECT $1, unnest($2::uuid[])`, feeID, uniqueIDs(classIDs))
	return err
}

func (h *Handler) createFee(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var req feeRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out FeeItem
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if err := req.validate(r.Context(), tx, entityID); err != nil {
			return err
		}
		active := req.Active == nil || *req.Active
		var id uuid.UUID
		err := tx.QueryRow(r.Context(), `
			INSERT INTO fee_item (entity_id, school_year_id, code, name, kind, amount, vat_mode, vat_rate_bp, due_rule, mandatory, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			entityID, req.SchoolYearID, req.Code, req.Name, req.Kind, req.Amount, req.VATMode, req.VATRateBP,
			req.DueRule, req.Mandatory, active).Scan(&id)
		if db.IsUniqueViolation(err) {
			return httpx.Validation(httpx.Fields{"code": "Já existe um item com este código neste ano."})
		}
		if err != nil {
			return err
		}
		if err := setFeeClasses(r.Context(), tx, id, req.ClassIDs); err != nil {
			return err
		}
		if out, err = loadFee(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "create", Table: "fee_item", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

// updateFee changes catalogue prices. Existing charges keep their snapshot;
// only charges generated afterwards use the new values.
func (h *Handler) updateFee(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req feeRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out FeeItem
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadFee(r.Context(), tx, entityID, id)
		if err != nil {
			return err
		}
		req.SchoolYearID, req.Code = before.SchoolYearID, before.Code // identity of an item is fixed
		if req.Kind != before.Kind {
			return httpx.Validation(httpx.Fields{"kind": "O tipo não pode ser alterado; crie um novo item."})
		}
		if err := req.validate(r.Context(), tx, entityID); err != nil {
			return err
		}
		active := req.Active == nil || *req.Active
		if _, err := tx.Exec(r.Context(), `
			UPDATE fee_item SET name=$3, amount=$4, vat_mode=$5, vat_rate_bp=$6, due_rule=$7, mandatory=$8, active=$9
			WHERE id=$1 AND entity_id=$2`,
			id, entityID, req.Name, req.Amount, req.VATMode, req.VATRateBP, req.DueRule, req.Mandatory, active); err != nil {
			return err
		}
		if err := setFeeClasses(r.Context(), tx, id, req.ClassIDs); err != nil {
			return err
		}
		if out, err = loadFee(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "update", Table: "fee_item", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

type copyRequest struct {
	FromYear int `json:"from_year"`
	ToYear   int `json:"to_year"`
}

type copyResult struct {
	Copied   int      `json:"copied"`
	Skipped  []string `json:"skipped"`  // codes already present in the target year
	Inactive []string `json:"inactive"` // class-scoped items whose classes do not exist in the target year
}

// copyFees copies the entity's catalogue between years. Class scopes are
// matched by class name in the target year; one-off fixed due dates move by
// the difference in years. A class-scoped item none of whose classes exist in
// the target year is copied inactive: without a scope it would apply to every
// class (e.g. a second mandatory tuition for everyone).
func (h *Handler) copyFees(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var req copyRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if req.FromYear == req.ToYear {
		return httpx.Validation(httpx.Fields{"to_year": "Escolha um ano diferente."})
	}
	res := copyResult{Skipped: []string{}, Inactive: []string{}}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var fromID, toID uuid.UUID
		err := tx.QueryRow(r.Context(), `
			SELECT (SELECT id FROM school_year WHERE year = $1), (SELECT id FROM school_year WHERE year = $2)`,
			req.FromYear, req.ToYear).Scan(&fromID, &toID)
		if err != nil {
			return httpx.Validation(httpx.Fields{"from_year": "Os dois anos lectivos têm de existir."})
		}
		rows, err := tx.Query(r.Context(), feeSelect+` WHERE f.entity_id = $1 AND f.school_year_id = $2`, entityID, fromID)
		if err != nil {
			return err
		}
		items, err := collectFees(rows)
		if err != nil {
			return err
		}
		shift := req.ToYear - req.FromYear
		for _, it := range items {
			due := it.DueRule
			if dr, err := billing.ParseDueRule(it.DueRule, it.Kind); err == nil && dr.Date != "" {
				if t, err := time.Parse(time.DateOnly, dr.Date); err == nil {
					dr.Date = t.AddDate(shift, 0, 0).Format(time.DateOnly)
					if due, err = json.Marshal(dr); err != nil {
						return err
					}
				}
			}
			var newID uuid.UUID
			err := tx.QueryRow(r.Context(), `
				INSERT INTO fee_item (entity_id, school_year_id, code, name, kind, amount, vat_mode, vat_rate_bp, due_rule, mandatory, active)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT (entity_id, school_year_id, code) DO NOTHING RETURNING id`,
				entityID, toID, it.Code, it.Name, it.Kind, it.Amount, it.VATMode, it.VATRateBP, due, it.Mandatory, it.Active).Scan(&newID)
			if errors.Is(err, pgx.ErrNoRows) {
				res.Skipped = append(res.Skipped, it.Code)
				continue
			}
			if err != nil {
				return err
			}
			if len(it.ClassIDs) > 0 {
				tag, err := tx.Exec(r.Context(), `
					INSERT INTO fee_item_class (fee_item_id, class_id)
					SELECT $1, tc.id FROM class sc JOIN class tc ON tc.name = sc.name AND tc.entity_id = sc.entity_id
					WHERE sc.id = ANY($2) AND tc.school_year_id = $3`, newID, it.ClassIDs, toID)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					if _, err := tx.Exec(r.Context(), `UPDATE fee_item SET active = false WHERE id = $1`, newID); err != nil {
						return err
					}
					res.Inactive = append(res.Inactive, it.Code)
				}
			}
			res.Copied++
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "copy", Table: "fee_item",
			RecordID: toID.String(), After: map[string]any{"from_year": req.FromYear, "to_year": req.ToYear, "result": res}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, res)
	return nil
}
