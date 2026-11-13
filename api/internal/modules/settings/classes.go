package settings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
)

// ---------------------------------------------------------------------------
// Levels and penalty rules
// ---------------------------------------------------------------------------

type Level struct {
	ID          uuid.UUID           `json:"id" db:"id"`
	EntityID    uuid.UUID           `json:"entity_id" db:"entity_id"`
	Code        string              `json:"code" db:"code"`
	Name        string              `json:"name" db:"name"`
	PenaltyRule billing.PenaltyRule `json:"penalty_rule" db:"penalty_rule"`
}

const levelCols = `id, entity_id, code, name, penalty_rule`

func (h *Handler) listLevels(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+levelCols+` FROM level WHERE entity_id = ANY($1) ORDER BY code`, auth.EntityScope(r.Context()).EntityIDs)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[Level])
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Level]{Items: nonNil(items), Total: len(items)})
	return nil
}

type levelRequest struct {
	Name        string          `json:"name"`
	PenaltyRule json.RawMessage `json:"penalty_rule"`
}

func (h *Handler) updateLevel(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req levelRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	f := httpx.Fields{}
	f.Add(strings.TrimSpace(req.Name) == "", "name", "Obrigatório.")
	rule, ruleErr := billing.ParsePenaltyRule(req.PenaltyRule)
	if ruleErr != nil {
		f.Add(true, "penalty_rule", ruleErr.Error())
	}
	if err := f.Err(); err != nil {
		return err
	}
	var out Level
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadLevel(r.Context(), tx, entityID, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE level SET name=$3, penalty_rule=$4 WHERE id=$1 AND entity_id=$2`,
			id, entityID, strings.TrimSpace(req.Name), rule); err != nil {
			return err
		}
		if out, err = loadLevel(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "update", Table: "level", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func loadLevel(ctx context.Context, q db.Querier, entityID, id uuid.UUID) (Level, error) {
	rows, err := q.Query(ctx, `SELECT `+levelCols+` FROM level WHERE id = $1 AND entity_id = $2`, id, entityID)
	if err != nil {
		return Level{}, err
	}
	l, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Level])
	if errors.Is(err, pgx.ErrNoRows) {
		return l, httpx.NotFound()
	}
	return l, err
}

// ---------------------------------------------------------------------------
// Classes
// ---------------------------------------------------------------------------

type Class struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	EntityID       uuid.UUID  `json:"entity_id" db:"entity_id"`
	LevelID        uuid.UUID  `json:"level_id" db:"level_id"`
	SchoolYearID   uuid.UUID  `json:"school_year_id" db:"school_year_id"`
	Name           string     `json:"name" db:"name"`
	Grade          *int       `json:"grade" db:"grade"`
	Capacity       int        `json:"capacity" db:"capacity"`
	Room           string     `json:"room" db:"room"`
	TeacherStaffID *uuid.UUID `json:"teacher_staff_id" db:"teacher_staff_id"`
	TeacherName    *string    `json:"teacher_name" db:"teacher_name"`
	Enrolled       int        `json:"enrolled" db:"enrolled"`
}

const classSelect = `
	SELECT c.id, c.entity_id, c.level_id, c.school_year_id, c.name, c.grade, c.capacity, c.room,
	       c.teacher_staff_id, s.full_name AS teacher_name,
	       (SELECT count(*) FROM enrolment e WHERE e.class_id = c.id AND e.status = 'confirmed')::int AS enrolled
	FROM class c LEFT JOIN staff s ON s.id = c.teacher_staff_id`

func (h *Handler) listClasses(w http.ResponseWriter, r *http.Request) error {
	yid, err := yearID(r.Context(), h.Pool, r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), classSelect+`
		WHERE c.entity_id = ANY($1) AND c.school_year_id = $2
		ORDER BY c.grade NULLS FIRST, c.name`, auth.EntityScope(r.Context()).EntityIDs, yid)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[Class])
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Class]{Items: nonNil(items), Total: len(items)})
	return nil
}

func loadClass(ctx context.Context, q db.Querier, entityID, id uuid.UUID) (Class, error) {
	rows, err := q.Query(ctx, classSelect+` WHERE c.id = $1 AND c.entity_id = $2`, id, entityID)
	if err != nil {
		return Class{}, err
	}
	c, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Class])
	if errors.Is(err, pgx.ErrNoRows) {
		return c, httpx.NotFound()
	}
	return c, err
}

type classRequest struct {
	LevelID        uuid.UUID  `json:"level_id"`
	SchoolYearID   uuid.UUID  `json:"school_year_id"`
	Name           string     `json:"name"`
	Grade          *int       `json:"grade"`
	Capacity       int        `json:"capacity"`
	Room           string     `json:"room"`
	TeacherStaffID *uuid.UUID `json:"teacher_staff_id"`
}

func (req *classRequest) validate(ctx context.Context, q db.Querier, entityID uuid.UUID) error {
	f := httpx.Fields{}
	req.Name = strings.TrimSpace(req.Name)
	f.Add(req.Name == "", "name", "Obrigatório.")
	f.Add(req.Capacity < 1 || req.Capacity > 60, "capacity", "Capacidade entre 1 e 60.")
	f.Add(req.Grade != nil && (*req.Grade < 1 || *req.Grade > 12), "grade", "Classe entre 1 e 12.")
	var levelOK, yearOK, teacherOK bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM level WHERE id = $1 AND entity_id = $2),
		       EXISTS(SELECT 1 FROM school_year WHERE id = $3),
		       $4::uuid IS NULL OR EXISTS(SELECT 1 FROM staff WHERE id = $4 AND active)`,
		req.LevelID, entityID, req.SchoolYearID, req.TeacherStaffID).Scan(&levelOK, &yearOK, &teacherOK); err != nil {
		return err
	}
	f.Add(!levelOK, "level_id", "Nível inválido para esta empresa.")
	f.Add(!yearOK, "school_year_id", "Ano lectivo inválido.")
	f.Add(!teacherOK, "teacher_staff_id", "Professor inválido.")
	return f.Err()
}

func (h *Handler) createClass(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var req classRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out Class
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if err := req.validate(r.Context(), tx, entityID); err != nil {
			return err
		}
		var id uuid.UUID
		err := tx.QueryRow(r.Context(), `
			INSERT INTO class (entity_id, level_id, school_year_id, name, grade, capacity, room, teacher_staff_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			entityID, req.LevelID, req.SchoolYearID, req.Name, req.Grade, req.Capacity, req.Room, req.TeacherStaffID).Scan(&id)
		if db.IsUniqueViolation(err) {
			return httpx.Validation(httpx.Fields{"name": "Já existe uma turma com este nome neste ano."})
		}
		if err != nil {
			return err
		}
		if out, err = loadClass(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "create", Table: "class", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) updateClass(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req classRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out Class
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadClass(r.Context(), tx, entityID, id)
		if err != nil {
			return err
		}
		// A class stays in its year; move pupils between classes instead.
		req.SchoolYearID = before.SchoolYearID
		if err := req.validate(r.Context(), tx, entityID); err != nil {
			return err
		}
		if req.Capacity < before.Enrolled {
			return httpx.Validation(httpx.Fields{"capacity": "A turma já tem mais alunos matriculados."})
		}
		_, err = tx.Exec(r.Context(), `
			UPDATE class SET level_id=$3, name=$4, grade=$5, capacity=$6, room=$7, teacher_staff_id=$8
			WHERE id=$1 AND entity_id=$2`,
			id, entityID, req.LevelID, req.Name, req.Grade, req.Capacity, req.Room, req.TeacherStaffID)
		if db.IsUniqueViolation(err) {
			return httpx.Validation(httpx.Fields{"name": "Já existe uma turma com este nome neste ano."})
		}
		if err != nil {
			return err
		}
		if out, err = loadClass(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "update", Table: "class", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h *Handler) deleteClass(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadClass(r.Context(), tx, entityID, id)
		if err != nil {
			return err
		}
		var used bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM enrolment WHERE class_id = $1)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return httpx.Conflict("class_in_use", "A turma tem matrículas e não pode ser apagada.")
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM class WHERE id = $1 AND entity_id = $2`, id, entityID); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "delete", Table: "class", RecordID: id.String(), Before: before})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
