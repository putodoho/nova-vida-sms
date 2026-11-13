package pupils

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
)

type Guardian struct {
	ID            uuid.UUID `json:"id" db:"id"`
	FullName      string    `json:"full_name" db:"full_name"`
	Phone1        string    `json:"phone1" db:"phone1"`
	Phone2        string    `json:"phone2" db:"phone2"`
	Email         string    `json:"email" db:"email"`
	Neighbourhood string    `json:"neighbourhood" db:"neighbourhood"`
	IDDoc         string    `json:"id_doc" db:"id_doc"`
}

const guardianCols = `id, full_name, phone1, phone2, email, neighbourhood, id_doc`

type GuardianChild struct {
	PupilID      uuid.UUID `json:"pupil_id" db:"pupil_id"`
	Number       string    `json:"number" db:"number"`
	FullName     string    `json:"full_name" db:"full_name"`
	Relationship string    `json:"relationship" db:"relationship"`
	IsPayer      bool      `json:"is_payer" db:"is_payer"`
}

type GuardianDetail struct {
	Guardian
	Children []GuardianChild `json:"children"`
}

func loadGuardian(ctx context.Context, q db.Querier, id uuid.UUID) (Guardian, error) {
	rows, err := q.Query(ctx, `SELECT `+guardianCols+` FROM guardian WHERE id = $1`, id)
	if err != nil {
		return Guardian{}, err
	}
	g, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Guardian])
	if errors.Is(err, pgx.ErrNoRows) {
		return g, httpx.NotFound()
	}
	return g, err
}

// searchGuardians finds guardians by name or phone, to link siblings to an
// existing guardian instead of creating duplicates.
func (h *Handler) searchGuardians(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httpx.JSON(w, http.StatusOK, httpx.List[Guardian]{Items: []Guardian{}})
		return nil
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, q)
	items, err := collect[Guardian](r.Context(), h.Pool, `
		SELECT `+guardianCols+` FROM guardian
		WHERE full_name ILIKE '%' || $1 || '%'
		   OR ($2 <> '' AND (regexp_replace(phone1, '\D', '', 'g') LIKE '%' || $2 || '%'
		                  OR regexp_replace(phone2, '\D', '', 'g') LIKE '%' || $2 || '%'))
		ORDER BY full_name LIMIT 20`, q, digits)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Guardian]{Items: items, Total: len(items)})
	return nil
}

func (h *Handler) getGuardian(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	g, err := loadGuardian(r.Context(), h.Pool, id)
	if err != nil {
		return err
	}
	children, err := collect[GuardianChild](r.Context(), h.Pool, `
		SELECT p.id AS pupil_id, p.number, p.full_name, pg.relationship, pg.is_payer
		FROM pupil_guardian pg JOIN pupil p ON p.id = pg.pupil_id
		WHERE pg.guardian_id = $1 ORDER BY p.birth_date`, id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, GuardianDetail{Guardian: g, Children: children})
	return nil
}

type guardianRequest struct {
	FullName      string `json:"full_name"`
	Phone1        string `json:"phone1"`
	Phone2        string `json:"phone2"`
	Email         string `json:"email"`
	Neighbourhood string `json:"neighbourhood"`
	IDDoc         string `json:"id_doc"`
}

func (g *guardianRequest) normalise() {
	g.FullName = strings.Join(strings.Fields(g.FullName), " ")
	g.Phone1, g.Phone2 = strings.TrimSpace(g.Phone1), strings.TrimSpace(g.Phone2)
	g.Email, g.Neighbourhood, g.IDDoc = strings.TrimSpace(g.Email), strings.TrimSpace(g.Neighbourhood), strings.TrimSpace(g.IDDoc)
}

func (g *guardianRequest) validate(f httpx.Fields, prefix string) {
	g.normalise()
	f.Add(len(g.FullName) < 3, prefix+"full_name", "Nome obrigatório.")
	f.Add(g.Phone1 == "", prefix+"phone1", "Telefone obrigatório.")
	f.Add(g.Email != "" && !strings.Contains(g.Email, "@"), prefix+"email", "Email inválido.")
}

func insertGuardian(ctx context.Context, tx pgx.Tx, g guardianRequest) (Guardian, error) {
	rows, err := tx.Query(ctx, `
		INSERT INTO guardian (full_name, phone1, phone2, email, neighbourhood, id_doc)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+guardianCols,
		g.FullName, g.Phone1, g.Phone2, g.Email, g.Neighbourhood, g.IDDoc)
	if err != nil {
		return Guardian{}, err
	}
	out, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Guardian])
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{Action: "create", Table: "guardian", RecordID: out.ID.String(), After: out})
}

func (h *Handler) createGuardian(w http.ResponseWriter, r *http.Request) error {
	var req guardianRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	f := httpx.Fields{}
	req.validate(f, "")
	if err := f.Err(); err != nil {
		return err
	}
	var out Guardian
	err := db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = insertGuardian(r.Context(), tx, req)
		return err
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) updateGuardian(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req guardianRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	f := httpx.Fields{}
	req.validate(f, "")
	if err := f.Err(); err != nil {
		return err
	}
	var out Guardian
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadGuardian(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE guardian SET full_name=$2, phone1=$3, phone2=$4, email=$5, neighbourhood=$6, id_doc=$7 WHERE id=$1`,
			id, req.FullName, req.Phone1, req.Phone2, req.Email, req.Neighbourhood, req.IDDoc); err != nil {
			return err
		}
		if out, err = loadGuardian(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "guardian", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// linkRequest links an existing guardian (guardian_id) or creates one inline (guardian).
type linkRequest struct {
	GuardianID      *uuid.UUID       `json:"guardian_id"`
	Guardian        *guardianRequest `json:"guardian"`
	Relationship    string           `json:"relationship"`
	IsPayer         bool             `json:"is_payer"`
	ReceivesNotices bool             `json:"receives_notices"`
	MayPickUp       bool             `json:"may_pick_up"`
	LivesWith       bool             `json:"lives_with"`
}

// putGuardians replaces the pupil's guardian links. Exactly one guardian is
// responsible for payments.
func (h *Handler) putGuardians(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req []linkRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	f := httpx.Fields{}
	f.Add(len(req) == 0, "items", "Indique pelo menos um encarregado.")
	f.Add(len(req) > 4, "items", "No máximo 4 encarregados.")
	payers := 0
	seen := map[uuid.UUID]bool{}
	for i := range req {
		l := &req[i]
		if l.IsPayer {
			payers++
		}
		switch l.Relationship {
		case "mother", "father", "guardian", "other":
		default:
			f.Add(true, "relationship", "Parentesco inválido.")
		}
		switch {
		case l.GuardianID != nil && l.Guardian == nil:
			f.Add(seen[*l.GuardianID], "guardian_id", "Encarregado repetido.")
			seen[*l.GuardianID] = true
		case l.GuardianID == nil && l.Guardian != nil:
			l.Guardian.validate(f, "guardian.")
		default:
			f.Add(true, "guardian", "Indique guardian_id ou os dados de um novo encarregado.")
		}
	}
	f.Add(len(req) > 0 && payers != 1, "is_payer", "Exactamente um encarregado deve ser responsável pelos pagamentos.")
	if err := f.Err(); err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := loadPupil(r.Context(), tx, id); err != nil {
			return err
		}
		before, err := collect[GuardianLink](r.Context(), tx, `
			SELECT g.id AS guardian_id, g.full_name, g.phone1, g.phone2, g.email, g.neighbourhood,
			       pg.relationship, pg.is_payer, pg.receives_notices, pg.may_pick_up, pg.lives_with
			FROM pupil_guardian pg JOIN guardian g ON g.id = pg.guardian_id WHERE pg.pupil_id = $1`, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM pupil_guardian WHERE pupil_id = $1`, id); err != nil {
			return err
		}
		for _, l := range req {
			gid := uuid.Nil
			if l.GuardianID != nil {
				if _, err := loadGuardian(r.Context(), tx, *l.GuardianID); err != nil {
					return httpx.Validation(httpx.Fields{"guardian_id": "Encarregado não encontrado."})
				}
				gid = *l.GuardianID
			} else {
				g, err := insertGuardian(r.Context(), tx, *l.Guardian)
				if err != nil {
					return err
				}
				gid = g.ID
			}
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO pupil_guardian (pupil_id, guardian_id, relationship, is_payer, receives_notices, may_pick_up, lives_with)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				id, gid, l.Relationship, l.IsPayer, l.ReceivesNotices, l.MayPickUp, l.LivesWith); err != nil {
				return err
			}
		}
		after, err := collect[GuardianLink](r.Context(), tx, `
			SELECT g.id AS guardian_id, g.full_name, g.phone1, g.phone2, g.email, g.neighbourhood,
			       pg.relationship, pg.is_payer, pg.receives_notices, pg.may_pick_up, pg.lives_with
			FROM pupil_guardian pg JOIN guardian g ON g.id = pg.guardian_id WHERE pg.pupil_id = $1`, id)
		if err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "pupil_guardian", RecordID: id.String(), Before: before, After: after})
	})
	if err != nil {
		return err
	}
	return h.get(w, r)
}
