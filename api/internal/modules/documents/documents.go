// Package documents renders receipts, annual statements and enrolment forms
// as HTML (?format=html, for on-screen preview and browser printing) or PDF
// (default, via Gotenberg). Statements are numbered and stored as snapshots,
// so a re-printed statement is identical to the one handed out.
package documents

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
	"github.com/putodoho/nova-vida-sms/api/internal/numbering"
	"github.com/putodoho/nova-vida-sms/api/internal/pdf"
)

//go:embed templates/*.html
var templateFS embed.FS

var statusLabels = map[string]string{"paid": "Pago", "overdue": "Em atraso", "upcoming": "Por vencer", "partial": "Parcial"}

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"money": func(a money.Amount) string { return a.Format() },
	"status": func(s string) template.HTML {
		if s == "" {
			return ""
		}
		return template.HTML(fmt.Sprintf(`<span class="badge b-%s">%s</span>`, s, statusLabels[s]))
	},
}).ParseFS(templateFS, "templates/*.html"))

type Handler struct {
	Pool     *pgxpool.Pool
	Renderer pdf.Renderer
	Loc      *time.Location
	Now      func() time.Time
}

func (h *Handler) Routes(r chi.Router) {
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/payments/{id}/receipt", httpx.Handler(h.receipt))
	gen := auth.Require(auth.DocumentsGenerate)
	r.With(gen).Method(http.MethodPost, "/statements", httpx.Handler(h.createStatement))
	r.With(gen).Method(http.MethodPost, "/statements/class", httpx.Handler(h.classStatements))
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/statements", httpx.Handler(h.listStatements))
	r.With(auth.Require(auth.ChargesRead)).Method(http.MethodGet, "/statements/{id}", httpx.Handler(h.getStatement))
	r.With(auth.Require(auth.PupilsRead)).Method(http.MethodGet, "/enrolments/{id}/form", httpx.Handler(h.form))
}

func (h *Handler) today() time.Time {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	loc := h.Loc
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// send renders a template and writes HTML or PDF depending on ?format.
func (h *Handler) send(w http.ResponseWriter, r *http.Request, name string, data any, filename string) error {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}
	if r.URL.Query().Get("format") == "html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err := w.Write(buf.Bytes())
		return err
	}
	out, err := h.Renderer.PDF(r.Context(), buf.Bytes())
	if err != nil {
		return &httpx.Error{Status: http.StatusBadGateway, Code: "pdf_unavailable", Message: "Não foi possível gerar o PDF. Tente a pré-visualização."}
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.pdf"`, filename))
	_, err = w.Write(out)
	return err
}

func (h *Handler) receipt(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	rc, err := buildReceipt(r.Context(), h.Pool, auth.EntityScope(r.Context()).EntityIDs, id, h.today())
	if err != nil {
		return err
	}
	return h.send(w, r, "receipt", rc, rc.Number)
}

func (h *Handler) form(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var entityID uuid.UUID
	if err := h.Pool.QueryRow(r.Context(), `SELECT entity_id FROM enrolment WHERE id = $1 AND entity_id = ANY($2)`,
		id, auth.EntityScope(r.Context()).EntityIDs).Scan(&entityID); err != nil {
		return httpx.NotFound()
	}
	f, err := buildEnrolmentForm(r.Context(), h.Pool, entityID, id, auth.CurrentUser(r.Context()).Can(auth.HealthRead), h.today())
	if err != nil {
		return err
	}
	return h.send(w, r, "form", f, "ficha-"+f.PupilNumber)
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

type StatementRef struct {
	ID        uuid.UUID `json:"id"`
	Number    string    `json:"number"`
	PupilID   uuid.UUID `json:"pupil_id"`
	PupilName string    `json:"pupil_name"`
	Year      int       `json:"year"`
	IssuedAt  time.Time `json:"issued_at"`
	IssuedBy  string    `json:"issued_by"`
}

// issue numbers a statement for an enrolment, stores its snapshot and returns it.
func (h *Handler) issue(ctx context.Context, tx pgx.Tx, entityID, enrolmentID uuid.UUID) (*Statement, uuid.UUID, error) {
	var pupilID, yearID uuid.UUID
	var year int
	if err := tx.QueryRow(ctx, `
		SELECT e.pupil_id, e.school_year_id, sy.year FROM enrolment e JOIN school_year sy ON sy.id = e.school_year_id
		WHERE e.id = $1 AND e.entity_id = $2 AND e.status <> 'draft'`, enrolmentID, entityID).Scan(&pupilID, &yearID, &year); err != nil {
		return nil, uuid.Nil, httpx.Validation(httpx.Fields{"pupil_id": "O aluno não tem matrícula confirmada neste ano."})
	}
	number, err := numbering.Document(ctx, tx, entityID, "statement", year)
	if err != nil {
		return nil, uuid.Nil, err
	}
	st, err := buildStatement(ctx, tx, enrolmentID, number, h.today())
	if err != nil {
		return nil, uuid.Nil, err
	}
	snap, err := json.Marshal(st)
	if err != nil {
		return nil, uuid.Nil, err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO statement (entity_id, pupil_id, school_year_id, number, issued_by, snapshot)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		entityID, pupilID, yearID, number, auth.CurrentUser(ctx).ID, snap).Scan(&id); err != nil {
		return nil, uuid.Nil, err
	}
	return st, id, audit.Write(ctx, tx, audit.Entry{EntityID: &entityID, Action: "issue", Table: "statement", RecordID: id.String(),
		After: map[string]any{"number": number, "pupil_id": pupilID, "outstanding": st.Outstanding}})
}

// createStatement issues a statement for {pupil_id, year} and returns its reference.
func (h *Handler) createStatement(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var in struct {
		PupilID uuid.UUID `json:"pupil_id"`
		Year    int       `json:"year"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var ref StatementRef
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var enrolmentID uuid.UUID
		if err := tx.QueryRow(r.Context(), `
			SELECT e.id FROM enrolment e JOIN school_year sy ON sy.id = e.school_year_id
			WHERE e.pupil_id = $1 AND e.entity_id = $2 AND e.status <> 'draft'
			  AND ($3 = 0 OR sy.year = $3) ORDER BY sy.year DESC LIMIT 1`, in.PupilID, entityID, in.Year).Scan(&enrolmentID); err != nil {
			return httpx.Validation(httpx.Fields{"pupil_id": "O aluno não tem matrícula confirmada neste ano."})
		}
		st, id, err := h.issue(r.Context(), tx, entityID, enrolmentID)
		if err != nil {
			return err
		}
		ref = StatementRef{ID: id, Number: st.Number, PupilID: in.PupilID, PupilName: st.PupilName, Year: st.Year, IssuedAt: time.Now(),
			IssuedBy: auth.CurrentUser(r.Context()).DisplayName}
		return nil
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, ref)
	return nil
}

// classStatements issues statements for every confirmed enrolment of a class
// and returns them as one document (one statement per page).
func (h *Handler) classStatements(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var in struct {
		ClassID uuid.UUID `json:"class_id"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var list []*Statement
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT e.id FROM enrolment e JOIN pupil p ON p.id = e.pupil_id
			WHERE e.class_id = $1 AND e.entity_id = $2 AND e.status = 'confirmed' ORDER BY p.full_name`, in.ClassID, entityID)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return httpx.Validation(httpx.Fields{"class_id": "A turma não tem alunos matriculados."})
		}
		for _, id := range ids {
			st, _, err := h.issue(r.Context(), tx, entityID, id)
			if err != nil {
				return err
			}
			list = append(list, st)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return h.send(w, r, "statements", list, "extratos")
}

func (h *Handler) listStatements(w http.ResponseWriter, r *http.Request) error {
	var pupil *uuid.UUID
	if v := r.URL.Query().Get("pupil_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return httpx.BadRequest("pupil_id inválido.")
		}
		pupil = &id
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT s.id, s.number, s.pupil_id, p.full_name, sy.year, s.issued_at, u.display_name
		FROM statement s JOIN pupil p ON p.id = s.pupil_id JOIN school_year sy ON sy.id = s.school_year_id
		JOIN app_user u ON u.id = s.issued_by
		WHERE s.entity_id = ANY($1) AND ($2::uuid IS NULL OR s.pupil_id = $2)
		ORDER BY s.issued_at DESC LIMIT 200`, auth.EntityScope(r.Context()).EntityIDs, pupil)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[StatementRef])
	if err != nil {
		return err
	}
	if items == nil {
		items = []StatementRef{}
	}
	httpx.JSON(w, http.StatusOK, httpx.List[StatementRef]{Items: items, Total: len(items)})
	return nil
}

// getStatement renders a stored statement from its snapshot.
func (h *Handler) getStatement(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var snap []byte
	if err := h.Pool.QueryRow(r.Context(), `SELECT snapshot FROM statement WHERE id = $1 AND entity_id = ANY($2)`,
		id, auth.EntityScope(r.Context()).EntityIDs).Scan(&snap); err != nil {
		return httpx.NotFound()
	}
	var st Statement
	if err := json.Unmarshal(snap, &st); err != nil {
		return err
	}
	return h.send(w, r, "statements", []*Statement{&st}, st.Number)
}
