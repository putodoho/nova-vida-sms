// Package pupils manages pupils and their guardians, health records, pick-up
// persons and emergency contacts. Pupils and guardians are global (shared by
// both entities); enrolment and money live in the enrolments package.
package pupils

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/numbering"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	read := auth.Require(auth.PupilsRead)
	write := auth.Require(auth.PupilsWrite)

	r.With(read).Method(http.MethodGet, "/pupils/search", httpx.Handler(h.search))
	r.With(write).Method(http.MethodPost, "/pupils", httpx.Handler(h.create))
	r.With(read).Method(http.MethodGet, "/pupils/{id}", httpx.Handler(h.get))
	r.With(write).Method(http.MethodPut, "/pupils/{id}", httpx.Handler(h.update))
	r.With(write).Method(http.MethodPut, "/pupils/{id}/health", httpx.Handler(h.putHealth))
	r.With(write).Method(http.MethodPut, "/pupils/{id}/guardians", httpx.Handler(h.putGuardians))
	r.With(write).Method(http.MethodPut, "/pupils/{id}/pickup-persons", httpx.Handler(h.putPickup))
	r.With(write).Method(http.MethodPut, "/pupils/{id}/emergency-contacts", httpx.Handler(h.putEmergency))

	r.With(read).Method(http.MethodGet, "/guardians", httpx.Handler(h.searchGuardians))
	r.With(read).Method(http.MethodGet, "/guardians/{id}", httpx.Handler(h.getGuardian))
	r.With(write).Method(http.MethodPost, "/guardians", httpx.Handler(h.createGuardian))
	r.With(write).Method(http.MethodPut, "/guardians/{id}", httpx.Handler(h.updateGuardian))
}

// ---------------------------------------------------------------------------
// Pupil
// ---------------------------------------------------------------------------

type Pupil struct {
	ID                  uuid.UUID `json:"id" db:"id"`
	Number              string    `json:"number" db:"number"`
	FullName            string    `json:"full_name" db:"full_name"`
	Sex                 string    `json:"sex" db:"sex"`
	BirthDate           string    `json:"birth_date" db:"birth_date"`
	Neighbourhood       string    `json:"neighbourhood" db:"neighbourhood"`
	Religion            string    `json:"religion" db:"religion"`
	Orphan              string    `json:"orphan" db:"orphan"`
	PhotoPath           string    `json:"photo_path" db:"photo_path"`
	HomeDisciplineNotes string    `json:"home_discipline_notes" db:"home_discipline_notes"`
	Status              string    `json:"status" db:"status"`
}

const pupilCols = `id, number, full_name, sex, birth_date::text AS birth_date, neighbourhood, religion, orphan,
	photo_path, home_discipline_notes, status`

func loadPupil(ctx context.Context, q db.Querier, id uuid.UUID) (Pupil, error) {
	rows, err := q.Query(ctx, `SELECT `+pupilCols+` FROM pupil WHERE id = $1`, id)
	if err != nil {
		return Pupil{}, err
	}
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Pupil])
	if errors.Is(err, pgx.ErrNoRows) {
		return p, httpx.NotFound()
	}
	return p, err
}

type pupilRequest struct {
	FullName            string `json:"full_name"`
	Sex                 string `json:"sex"`
	BirthDate           string `json:"birth_date"`
	Neighbourhood       string `json:"neighbourhood"`
	Religion            string `json:"religion"`
	Orphan              string `json:"orphan"`
	HomeDisciplineNotes string `json:"home_discipline_notes"`
	Status              string `json:"status"` // update only
}

func (req *pupilRequest) validate(creating bool) error {
	f := httpx.Fields{}
	req.FullName = strings.Join(strings.Fields(req.FullName), " ")
	f.Add(len(req.FullName) < 3, "full_name", "Obrigatório.")
	f.Add(req.Sex != "F" && req.Sex != "M", "sex", "Indique F ou M.")
	bd, err := time.Parse(time.DateOnly, req.BirthDate)
	f.Add(err != nil, "birth_date", "Data inválida (AAAA-MM-DD).")
	f.Add(err == nil && (bd.After(time.Now()) || bd.Year() < 1990), "birth_date", "Data de nascimento improvável.")
	if req.Orphan == "" {
		req.Orphan = "none"
	}
	switch req.Orphan {
	case "none", "father", "mother", "both":
	default:
		f.Add(true, "orphan", "Valor inválido.")
	}
	if creating {
		req.Status = "active"
	}
	f.Add(req.Status != "active" && req.Status != "left", "status", "Estado inválido.")
	return f.Err()
}

type createPupilRequest struct {
	pupilRequest
	Year int `json:"year"` // school year used in the pupil number; default: open year
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	var req createPupilRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(true); err != nil {
		return err
	}
	var out Pupil
	err := db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		year := req.Year
		if year == 0 {
			if err := tx.QueryRow(r.Context(),
				`SELECT year FROM school_year ORDER BY (status = 'open') DESC, year DESC LIMIT 1`).Scan(&year); err != nil {
				return err
			}
		}
		number, err := numbering.Pupil(r.Context(), tx, year)
		if err != nil {
			return err
		}
		var id uuid.UUID
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO pupil (number, full_name, sex, birth_date, neighbourhood, religion, orphan, home_discipline_notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			number, req.FullName, req.Sex, req.BirthDate, strings.TrimSpace(req.Neighbourhood),
			strings.TrimSpace(req.Religion), req.Orphan, strings.TrimSpace(req.HomeDisciplineNotes)).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO health_record (pupil_id) VALUES ($1)`, id); err != nil {
			return err
		}
		if out, err = loadPupil(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "create", Table: "pupil", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req pupilRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if err := req.validate(false); err != nil {
		return err
	}
	var out Pupil
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadPupil(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE pupil SET full_name=$2, sex=$3, birth_date=$4, neighbourhood=$5, religion=$6, orphan=$7,
			       home_discipline_notes=$8, status=$9 WHERE id=$1`,
			id, req.FullName, req.Sex, req.BirthDate, strings.TrimSpace(req.Neighbourhood), strings.TrimSpace(req.Religion),
			req.Orphan, strings.TrimSpace(req.HomeDisciplineNotes), req.Status); err != nil {
			return err
		}
		if out, err = loadPupil(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "pupil", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// SearchHit is a compact pupil row for pickers: name, number and the latest
// enrolment (any entity).
type SearchHit struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	Number     string     `json:"number" db:"number"`
	FullName   string     `json:"full_name" db:"full_name"`
	BirthDate  string     `json:"birth_date" db:"birth_date"`
	Status     string     `json:"status" db:"status"`
	LastYear   *int       `json:"last_year" db:"last_year"`
	LastClass  *string    `json:"last_class" db:"last_class"`
	LastEntity *uuid.UUID `json:"last_entity_id" db:"last_entity_id"`
	PayerName  *string    `json:"payer_name" db:"payer_name"`
	PayerPhone *string    `json:"payer_phone" db:"payer_phone"`
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httpx.JSON(w, http.StatusOK, httpx.List[SearchHit]{Items: []SearchHit{}})
		return nil
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.id, p.number, p.full_name, p.birth_date::text AS birth_date, p.status,
		       le.year AS last_year, le.class_name AS last_class, le.entity_id AS last_entity_id,
		       g.full_name AS payer_name, g.phone1 AS payer_phone
		FROM pupil p
		LEFT JOIN LATERAL (
			SELECT sy.year, c.name AS class_name, e.entity_id
			FROM enrolment e JOIN school_year sy ON sy.id = e.school_year_id LEFT JOIN class c ON c.id = e.class_id
			WHERE e.pupil_id = p.id AND e.status <> 'draft'
			ORDER BY sy.year DESC LIMIT 1) le ON true
		LEFT JOIN pupil_guardian pg ON pg.pupil_id = p.id AND pg.is_payer
		LEFT JOIN guardian g ON g.id = pg.guardian_id
		WHERE p.full_name ILIKE '%' || $1 || '%' OR p.number ILIKE $1 || '%' OR g.phone1 LIKE '%' || $1 || '%'
		ORDER BY p.full_name LIMIT 20`, q)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[SearchHit])
	if err != nil {
		return err
	}
	if items == nil {
		items = []SearchHit{}
	}
	httpx.JSON(w, http.StatusOK, httpx.List[SearchHit]{Items: items, Total: len(items)})
	return nil
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

type Health struct {
	ChronicCondition       string     `json:"chronic_condition" db:"chronic_condition"`
	Allergies              string     `json:"allergies" db:"allergies"`
	DietRestrictions       string     `json:"diet_restrictions" db:"diet_restrictions"`
	Medication             string     `json:"medication" db:"medication"`
	DoctorName             string     `json:"doctor_name" db:"doctor_name"`
	DoctorPhone            string     `json:"doctor_phone" db:"doctor_phone"`
	Notes                  string     `json:"notes" db:"notes"`
	EmergencyCareConsentAt *time.Time `json:"emergency_care_consent_at" db:"emergency_care_consent_at"`
}

type GuardianLink struct {
	GuardianID      uuid.UUID `json:"guardian_id" db:"guardian_id"`
	FullName        string    `json:"full_name" db:"full_name"`
	Phone1          string    `json:"phone1" db:"phone1"`
	Phone2          string    `json:"phone2" db:"phone2"`
	Email           string    `json:"email" db:"email"`
	Neighbourhood   string    `json:"neighbourhood" db:"neighbourhood"`
	Relationship    string    `json:"relationship" db:"relationship"`
	IsPayer         bool      `json:"is_payer" db:"is_payer"`
	ReceivesNotices bool      `json:"receives_notices" db:"receives_notices"`
	MayPickUp       bool      `json:"may_pick_up" db:"may_pick_up"`
	LivesWith       bool      `json:"lives_with" db:"lives_with"`
}

type PickupPerson struct {
	FullName string `json:"full_name" db:"full_name"`
	Phone    string `json:"phone" db:"phone"`
	IDDoc    string `json:"id_doc" db:"id_doc"`
}

type EmergencyContact struct {
	Position     int    `json:"position" db:"position"`
	FullName     string `json:"full_name" db:"full_name"`
	Relationship string `json:"relationship" db:"relationship"`
	Phone        string `json:"phone" db:"phone"`
}

type EnrolmentSummary struct {
	ID         uuid.UUID `json:"id" db:"id"`
	EntityID   uuid.UUID `json:"entity_id" db:"entity_id"`
	EntityName string    `json:"entity_name" db:"entity_name"`
	Year       int       `json:"year" db:"year"`
	ClassName  *string   `json:"class_name" db:"class_name"`
	Type       string    `json:"type" db:"type"`
	Status     string    `json:"status" db:"status"`
}

type Profile struct {
	Pupil
	Health            *Health            `json:"health"` // null without health.read
	HealthAlert       bool               `json:"health_alert"`
	Guardians         []GuardianLink     `json:"guardians"`
	PickupPersons     []PickupPerson     `json:"pickup_persons"`
	EmergencyContacts []EmergencyContact `json:"emergency_contacts"`
	Enrolments        []EnrolmentSummary `json:"enrolments"`
}

func (h *Handler) loadProfile(ctx context.Context, id uuid.UUID) (*Profile, error) {
	p, err := loadPupil(ctx, h.Pool, id)
	if err != nil {
		return nil, err
	}
	out := &Profile{Pupil: p}

	rows, err := h.Pool.Query(ctx, `
		SELECT chronic_condition, allergies, diet_restrictions, medication, doctor_name, doctor_phone, notes, emergency_care_consent_at
		FROM health_record WHERE pupil_id = $1`, id)
	if err != nil {
		return nil, err
	}
	hr, err := pgx.CollectRows(rows, pgx.RowToStructByName[Health])
	if err != nil {
		return nil, err
	}
	if len(hr) == 1 {
		x := hr[0]
		out.HealthAlert = x.ChronicCondition != "" || x.Allergies != "" || x.DietRestrictions != "" || x.Medication != ""
		if auth.CurrentUser(ctx).Can(auth.HealthRead) {
			out.Health = &x
		}
	}

	if out.Guardians, err = collect[GuardianLink](ctx, h.Pool, `
		SELECT g.id AS guardian_id, g.full_name, g.phone1, g.phone2, g.email, g.neighbourhood,
		       pg.relationship, pg.is_payer, pg.receives_notices, pg.may_pick_up, pg.lives_with
		FROM pupil_guardian pg JOIN guardian g ON g.id = pg.guardian_id
		WHERE pg.pupil_id = $1 ORDER BY pg.is_payer DESC, pg.relationship`, id); err != nil {
		return nil, err
	}
	if out.PickupPersons, err = collect[PickupPerson](ctx, h.Pool,
		`SELECT full_name, phone, id_doc FROM pickup_person WHERE pupil_id = $1 ORDER BY full_name`, id); err != nil {
		return nil, err
	}
	if out.EmergencyContacts, err = collect[EmergencyContact](ctx, h.Pool,
		`SELECT position, full_name, relationship, phone FROM emergency_contact WHERE pupil_id = $1 ORDER BY position`, id); err != nil {
		return nil, err
	}
	if out.Enrolments, err = collect[EnrolmentSummary](ctx, h.Pool, `
		SELECT e.id, e.entity_id, en.short_name AS entity_name, sy.year, c.name AS class_name, e.type, e.status
		FROM enrolment e JOIN school_year sy ON sy.id = e.school_year_id JOIN entity en ON en.id = e.entity_id
		LEFT JOIN class c ON c.id = e.class_id
		WHERE e.pupil_id = $1 AND e.entity_id = ANY($2)
		ORDER BY sy.year DESC, e.created_at DESC`, id, auth.CurrentUser(ctx).EntityIDs); err != nil {
		return nil, err
	}
	return out, nil
}

func collect[T any](ctx context.Context, q db.Querier, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []T{}
	}
	return items, nil
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := h.loadProfile(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

func (h *Handler) putHealth(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	if !auth.CurrentUser(r.Context()).Can(auth.HealthRead) {
		return httpx.Forbidden() // writing health data requires seeing it
	}
	var req struct {
		Health
		EmergencyCareConsent bool `json:"emergency_care_consent"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := loadPupil(r.Context(), tx, id); err != nil {
			return err
		}
		var before Health
		_ = tx.QueryRow(r.Context(), `SELECT chronic_condition, allergies, diet_restrictions, medication FROM health_record WHERE pupil_id=$1`, id).
			Scan(&before.ChronicCondition, &before.Allergies, &before.DietRestrictions, &before.Medication)
		t := strings.TrimSpace
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO health_record (pupil_id, chronic_condition, allergies, diet_restrictions, medication, doctor_name, doctor_phone, notes, emergency_care_consent_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CASE WHEN $9 THEN now() END, now())
			ON CONFLICT (pupil_id) DO UPDATE SET
				chronic_condition = EXCLUDED.chronic_condition, allergies = EXCLUDED.allergies,
				diet_restrictions = EXCLUDED.diet_restrictions, medication = EXCLUDED.medication,
				doctor_name = EXCLUDED.doctor_name, doctor_phone = EXCLUDED.doctor_phone, notes = EXCLUDED.notes,
				emergency_care_consent_at = CASE WHEN $9 THEN COALESCE(health_record.emergency_care_consent_at, now()) END,
				updated_at = now()`,
			id, t(req.ChronicCondition), t(req.Allergies), t(req.DietRestrictions), t(req.Medication),
			t(req.DoctorName), t(req.DoctorPhone), t(req.Notes), req.EmergencyCareConsent); err != nil {
			return err
		}
		// Health details stay out of the audit log; record only that they changed.
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "health_record", RecordID: id.String()})
	})
	if err != nil {
		return err
	}
	p, err := h.loadProfile(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

func (h *Handler) putPickup(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req []PickupPerson
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	f := httpx.Fields{}
	f.Add(len(req) > 10, "items", "No máximo 10 pessoas.")
	for _, p := range req {
		f.Add(strings.TrimSpace(p.FullName) == "", "full_name", "Nome obrigatório.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := loadPupil(r.Context(), tx, id); err != nil {
			return err
		}
		before, err := collect[PickupPerson](r.Context(), tx, `SELECT full_name, phone, id_doc FROM pickup_person WHERE pupil_id = $1`, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM pickup_person WHERE pupil_id = $1`, id); err != nil {
			return err
		}
		for _, p := range req {
			if _, err := tx.Exec(r.Context(), `INSERT INTO pickup_person (pupil_id, full_name, phone, id_doc) VALUES ($1, $2, $3, $4)`,
				id, strings.TrimSpace(p.FullName), strings.TrimSpace(p.Phone), strings.TrimSpace(p.IDDoc)); err != nil {
				return err
			}
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "pickup_person", RecordID: id.String(), Before: before, After: req})
	})
	if err != nil {
		return err
	}
	return h.get(w, r)
}

func (h *Handler) putEmergency(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req []EmergencyContact
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	f := httpx.Fields{}
	f.Add(len(req) > 5, "items", "No máximo 5 contactos.")
	for _, c := range req {
		f.Add(strings.TrimSpace(c.FullName) == "", "full_name", "Nome obrigatório.")
		f.Add(strings.TrimSpace(c.Phone) == "", "phone", "Telefone obrigatório.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := loadPupil(r.Context(), tx, id); err != nil {
			return err
		}
		before, err := collect[EmergencyContact](r.Context(), tx, `SELECT position, full_name, relationship, phone FROM emergency_contact WHERE pupil_id = $1`, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM emergency_contact WHERE pupil_id = $1`, id); err != nil {
			return err
		}
		for i, c := range req {
			if _, err := tx.Exec(r.Context(), `INSERT INTO emergency_contact (pupil_id, position, full_name, relationship, phone) VALUES ($1, $2, $3, $4, $5)`,
				id, i+1, strings.TrimSpace(c.FullName), strings.TrimSpace(c.Relationship), strings.TrimSpace(c.Phone)); err != nil {
				return err
			}
		}
		return audit.Write(r.Context(), tx, audit.Entry{Action: "update", Table: "emergency_contact", RecordID: id.String(), Before: before, After: req})
	})
	if err != nil {
		return err
	}
	return h.get(w, r)
}
