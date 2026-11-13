package settings

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
)

// DiscountType: value is basis points when mode is percent (1000 = 10%),
// centavos when mode is fixed.
type DiscountType struct {
	ID               uuid.UUID `json:"id" db:"id"`
	EntityID         uuid.UUID `json:"entity_id" db:"entity_id"`
	Code             string    `json:"code" db:"code"`
	Name             string    `json:"name" db:"name"`
	Mode             string    `json:"mode" db:"mode"`
	Value            int64     `json:"value" db:"value"`
	FeeItemCodes     []string  `json:"fee_item_codes" db:"fee_item_codes"`
	RequiresApproval bool      `json:"requires_approval" db:"requires_approval"`
	Active           bool      `json:"active" db:"active"`
}

const discountCols = `id, entity_id, code, name, mode, value, fee_item_codes, requires_approval, active`

func (h *Handler) listDiscountTypes(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+discountCols+` FROM discount_type WHERE entity_id = ANY($1) ORDER BY name`, auth.EntityScope(r.Context()).EntityIDs)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[DiscountType])
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.List[DiscountType]{Items: nonNil(items), Total: len(items)})
	return nil
}

func loadDiscountType(ctx context.Context, q db.Querier, entityID, id uuid.UUID) (DiscountType, error) {
	rows, err := q.Query(ctx, `SELECT `+discountCols+` FROM discount_type WHERE id = $1 AND entity_id = $2`, id, entityID)
	if err != nil {
		return DiscountType{}, err
	}
	d, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[DiscountType])
	if errors.Is(err, pgx.ErrNoRows) {
		return d, httpx.NotFound()
	}
	return d, err
}

type discountRequest struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	Mode             string   `json:"mode"`
	Value            int64    `json:"value"`
	FeeItemCodes     []string `json:"fee_item_codes"`
	RequiresApproval bool     `json:"requires_approval"`
	Active           *bool    `json:"active"`
}

func (req *discountRequest) validate() error {
	f := httpx.Fields{}
	req.Name = strings.TrimSpace(req.Name)
	f.Add(!codeRe.MatchString(req.Code), "code", "Código: letras minúsculas, números e _.")
	f.Add(req.Name == "", "name", "Obrigatório.")
	switch req.Mode {
	case "percent":
		f.Add(req.Value < 0 || req.Value > 10_000, "value", "Percentagem entre 0 e 100%.")
	case "fixed":
		f.Add(req.Value < 0, "value", "Valor inválido.")
	default:
		f.Add(true, "mode", "Modo inválido.")
	}
	f.Add(len(req.FeeItemCodes) == 0, "fee_item_codes", "Indique pelo menos um item da tabela de preços.")
	for _, c := range req.FeeItemCodes {
		f.Add(!codeRe.MatchString(c), "fee_item_codes", "Código de item inválido.")
	}
	return f.Err()
}

func (h *Handler) createDiscountType(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var req discountRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	var out DiscountType
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(r.Context(), `
			INSERT INTO discount_type (entity_id, code, name, mode, value, fee_item_codes, requires_approval, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			entityID, req.Code, req.Name, req.Mode, req.Value, req.FeeItemCodes, req.RequiresApproval,
			req.Active == nil || *req.Active).Scan(&id)
		if db.IsUniqueViolation(err) {
			return httpx.Validation(httpx.Fields{"code": "Já existe um desconto com este código."})
		}
		if err != nil {
			return err
		}
		if out, err = loadDiscountType(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "create", Table: "discount_type", RecordID: id.String(), After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (h *Handler) updateDiscountType(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req discountRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out DiscountType
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := loadDiscountType(r.Context(), tx, entityID, id)
		if err != nil {
			return err
		}
		req.Code = before.Code
		if err := req.validate(); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE discount_type SET name=$3, mode=$4, value=$5, fee_item_codes=$6, requires_approval=$7, active=$8
			WHERE id=$1 AND entity_id=$2`,
			id, entityID, req.Name, req.Mode, req.Value, req.FeeItemCodes, req.RequiresApproval, req.Active == nil || *req.Active); err != nil {
			return err
		}
		if out, err = loadDiscountType(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "update", Table: "discount_type", RecordID: id.String(), Before: before, After: out})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}
