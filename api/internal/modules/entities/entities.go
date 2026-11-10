// Package entities manages the legal companies and their receiving accounts.
package entities

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	r.Method(http.MethodGet, "/entities", httpx.Handler(h.list))
	r.Method(http.MethodGet, "/entities/{id}", httpx.Handler(h.get))
	r.With(auth.Require(auth.SettingsManage)).Method(http.MethodPut, "/entities/{id}", httpx.Handler(h.update))
	r.Method(http.MethodGet, "/entities/{id}/accounts", httpx.Handler(h.listAccounts))
	r.With(auth.Require(auth.SettingsManage)).Method(http.MethodPost, "/entities/{id}/accounts", httpx.Handler(h.createAccount))
	r.With(auth.Require(auth.SettingsManage)).Method(http.MethodPut, "/entities/{id}/accounts/{accountID}", httpx.Handler(h.updateAccount))
}

type Entity struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	LegalName string    `json:"legal_name"`
	ShortName string    `json:"short_name"`
	NUIT      string    `json:"nuit"`
	Address   string    `json:"address"`
	Motto     string    `json:"motto"`
	LogoPath  string    `json:"logo_path"`
	VATMethod string    `json:"vat_method"`
}

const entityCols = `id, code, legal_name, short_name, nuit, address, motto, logo_path, vat_method`

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+entityCols+` FROM entity WHERE id = ANY($1) ORDER BY code DESC`, auth.EntityScope(r.Context()).EntityIDs)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Entity])
	if err != nil {
		return err
	}
	if items == nil {
		items = []Entity{}
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Entity]{Items: items, Total: len(items)})
	return nil
}

// pathEntity parses {id} and checks it is inside the request scope.
func pathEntity(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.NotFound()
	}
	for _, e := range auth.EntityScope(r.Context()).EntityIDs {
		if e == id {
			return id, nil
		}
	}
	return uuid.Nil, httpx.NotFound()
}

func (h *Handler) load(ctx context.Context, q db.Querier, id uuid.UUID) (Entity, error) {
	rows, err := q.Query(ctx, `SELECT `+entityCols+` FROM entity WHERE id = $1`, id)
	if err != nil {
		return Entity{}, err
	}
	e, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Entity])
	if errors.Is(err, pgx.ErrNoRows) {
		return e, httpx.NotFound()
	}
	return e, err
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := pathEntity(r)
	if err != nil {
		return err
	}
	e, err := h.load(r.Context(), h.Pool, id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, e)
	return nil
}

type updateRequest struct {
	LegalName string `json:"legal_name"`
	ShortName string `json:"short_name"`
	NUIT      string `json:"nuit"`
	Address   string `json:"address"`
	Motto     string `json:"motto"`
	VATMethod string `json:"vat_method"`
}

func (req *updateRequest) validate() error {
	f := map[string]string{}
	req.LegalName, req.ShortName = strings.TrimSpace(req.LegalName), strings.TrimSpace(req.ShortName)
	req.NUIT = strings.TrimSpace(req.NUIT)
	if req.LegalName == "" {
		f["legal_name"] = "Obrigatório."
	}
	if req.ShortName == "" {
		f["short_name"] = "Obrigatório."
	}
	if req.NUIT != "" && (len(req.NUIT) != 9 || strings.Trim(req.NUIT, "0123456789") != "") {
		f["nuit"] = "O NUIT tem 9 dígitos."
	}
	switch req.VATMethod {
	case money.VATAdded, money.VATIncludedStandard, money.VATIncludedFlat:
	default:
		f["vat_method"] = "Método de IVA inválido."
	}
	if len(f) > 0 {
		return httpx.Validation(f)
	}
	return nil
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := pathEntity(r)
	if err != nil {
		return err
	}
	if auth.EntityScope(r.Context()).EntityID != id {
		return httpx.BadRequest("A empresa do pedido não corresponde à empresa seleccionada.")
	}
	var req updateRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	var after Entity
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		before, err := h.load(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE entity SET legal_name=$2, short_name=$3, nuit=$4, address=$5, motto=$6, vat_method=$7
			WHERE id = $1`, id, req.LegalName, req.ShortName, req.NUIT, req.Address, req.Motto, req.VATMethod); err != nil {
			return err
		}
		if after, err = h.load(r.Context(), tx, id); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{
			EntityID: &id, Action: "update", Table: "entity", RecordID: id.String(), Before: before, After: after,
		})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, after)
	return nil
}

type Account struct {
	ID       uuid.UUID `json:"id"`
	EntityID uuid.UUID `json:"entity_id"`
	Kind     string    `json:"kind"`
	Label    string    `json:"label"`
	Holder   string    `json:"holder"`
	Number   string    `json:"number"`
	NIB      string    `json:"nib"`
	Active   bool      `json:"active"`
}

const accountCols = `id, entity_id, kind, label, holder, number, nib, active`

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) error {
	id, err := pathEntity(r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+accountCols+` FROM receiving_account WHERE entity_id = $1 ORDER BY kind, label`, id)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Account])
	if err != nil {
		return err
	}
	if items == nil {
		items = []Account{}
	}
	httpx.JSON(w, http.StatusOK, httpx.List[Account]{Items: items, Total: len(items)})
	return nil
}

type accountRequest struct {
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Holder string `json:"holder"`
	Number string `json:"number"`
	NIB    string `json:"nib"`
	Active *bool  `json:"active"`
}

func (req *accountRequest) validate() error {
	f := map[string]string{}
	switch req.Kind {
	case "cash", "bank", "mpesa", "emola":
	default:
		f["kind"] = "Tipo inválido."
	}
	if strings.TrimSpace(req.Label) == "" {
		f["label"] = "Obrigatório."
	}
	if req.Kind == "bank" && strings.TrimSpace(req.NIB) == "" {
		f["nib"] = "Obrigatório para contas bancárias."
	}
	if (req.Kind == "mpesa" || req.Kind == "emola") && strings.TrimSpace(req.Number) == "" {
		f["number"] = "Obrigatório para carteiras móveis."
	}
	if len(f) > 0 {
		return httpx.Validation(f)
	}
	return nil
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) error {
	id, err := pathEntity(r)
	if err != nil {
		return err
	}
	if auth.EntityScope(r.Context()).EntityID != id {
		return httpx.BadRequest("A empresa do pedido não corresponde à empresa seleccionada.")
	}
	var req accountRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	var acc Account
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			INSERT INTO receiving_account (entity_id, kind, label, holder, number, nib)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+accountCols,
			id, req.Kind, strings.TrimSpace(req.Label), req.Holder, req.Number, req.NIB)
		if err != nil {
			return err
		}
		if acc, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Account]); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &id, Action: "create", Table: "receiving_account", RecordID: acc.ID.String(), After: acc})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, acc)
	return nil
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) error {
	id, err := pathEntity(r)
	if err != nil {
		return err
	}
	if auth.EntityScope(r.Context()).EntityID != id {
		return httpx.BadRequest("A empresa do pedido não corresponde à empresa seleccionada.")
	}
	accID, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		return httpx.NotFound()
	}
	var req accountRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	var after Account
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT `+accountCols+` FROM receiving_account WHERE id=$1 AND entity_id=$2 FOR UPDATE`, accID, id)
		if err != nil {
			return err
		}
		before, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Account])
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		rows, err = tx.Query(r.Context(), `
			UPDATE receiving_account SET kind=$3, label=$4, holder=$5, number=$6, nib=$7, active=$8
			WHERE id=$1 AND entity_id=$2 RETURNING `+accountCols,
			accID, id, req.Kind, strings.TrimSpace(req.Label), req.Holder, req.Number, req.NIB, active)
		if err != nil {
			return err
		}
		if after, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Account]); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &id, Action: "update", Table: "receiving_account", RecordID: accID.String(), Before: before, After: after})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, after)
	return nil
}
