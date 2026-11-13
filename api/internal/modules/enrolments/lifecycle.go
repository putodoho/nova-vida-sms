package enrolments

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/auth"
	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/httpx"
)

// ---------------------------------------------------------------------------
// Confirm: checks, then charge generation
// ---------------------------------------------------------------------------

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var out *Enrolment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if err := h.confirmTx(r.Context(), tx, entityID, id); err != nil {
			return err
		}
		out, err = h.load(r.Context(), tx, entityID, id, false)
		return err
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h *Handler) confirmTx(ctx context.Context, tx pgx.Tx, entityID, id uuid.UUID) error {
	e, err := h.load(ctx, tx, entityID, id, true)
	if err != nil {
		return err
	}
	if e.Status != "draft" {
		return httpx.Conflict("not_draft", "Esta matrícula já foi confirmada.")
	}
	f := httpx.Fields{}
	f.Add(e.ClassID == nil, "class_id", "Escolha a turma.")
	f.Add(!e.Consents.FeesAgreement, "consents.fees_agreement", "O encarregado tem de aceitar o pagamento das mensalidades.")
	var payers int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM pupil_guardian WHERE pupil_id = $1 AND is_payer`, e.PupilID).Scan(&payers); err != nil {
		return err
	}
	f.Add(payers != 1, "guardians", "Indique o encarregado responsável pelos pagamentos.")
	if err := f.Err(); err != nil {
		return err
	}

	// Capacity: lock the class row so two receptionists cannot fill the last seat twice.
	var capacity, enrolled int
	if err := tx.QueryRow(ctx, `SELECT capacity FROM class WHERE id = $1 FOR UPDATE`, e.ClassID).Scan(&capacity); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM enrolment WHERE class_id = $1 AND status = 'confirmed'`, e.ClassID).Scan(&enrolled); err != nil {
		return err
	}
	if enrolled >= capacity {
		return httpx.Conflict("class_full", "A turma está cheia.")
	}

	if _, err := tx.Exec(ctx, `UPDATE enrolment SET status = 'confirmed', confirmed_at = now() WHERE id = $1`, id); err != nil {
		if db.IsUniqueViolation(err) {
			return httpx.Conflict("already_enrolled", "O aluno já tem uma matrícula confirmada neste ano lectivo.")
		}
		return err
	}

	cd, err := loadCalendar(ctx, tx, entityID, e.SchoolYearID)
	if err != nil {
		return err
	}
	ids := e.OneOffFeeIDs
	for _, s := range e.Services {
		ids = append(ids, s.FeeItemID)
	}
	fees, err := loadFees(ctx, tx, entityID, ids)
	if err != nil {
		return err
	}
	drafts, err := billing.Generate(cd.cal, e.StartMonth, fees, cd.vatMethod, h.today())
	if err != nil {
		return err
	}
	if err := insertCharges(ctx, tx, entityID, e, drafts); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{EntityID: &entityID, Action: "confirm", Table: "enrolment", RecordID: id.String(),
		After: map[string]any{"charges": len(drafts), "class_id": e.ClassID, "start_month": e.StartMonth}})
}

func insertCharges(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, e *Enrolment, drafts []billing.Draft) error {
	userID := auth.CurrentUser(ctx).ID
	for _, d := range drafts {
		var period *int
		if d.PeriodMonth > 0 {
			p := d.PeriodMonth
			period = &p
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO charge (entity_id, enrolment_id, pupil_id, fee_item_id, kind, period_month, description, due_date,
			                    amount_net, amount_vat, amount_gross, vat_rate_bp, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			entityID, e.ID, e.PupilID, d.FeeItemID, d.Kind, period, d.Description, d.DueDate,
			d.Lines.Net, d.Lines.VAT, d.Lines.Gross, d.VATRateBP, userID); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Withdraw
// ---------------------------------------------------------------------------

type withdrawRequest struct {
	Date   string `json:"date"` // last day at school; default today
	Reason string `json:"reason"`
}

// withdraw ends an enrolment. Unpaid charges due after the leaving date are
// cancelled; past debt stays. Charges with any payment are kept.
func (h *Handler) withdraw(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req withdrawRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	req.Reason = strings.TrimSpace(req.Reason)
	date := h.today()
	f := httpx.Fields{}
	if req.Date != "" {
		d, perr := time.Parse(time.DateOnly, req.Date)
		f.Add(perr != nil, "date", "Data inválida.")
		date = d
	}
	f.Add(req.Reason == "", "reason", "Indique o motivo.")
	if err := f.Err(); err != nil {
		return err
	}
	var out *Enrolment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		e, err := h.load(r.Context(), tx, entityID, id, true)
		if err != nil {
			return err
		}
		if e.Status != "confirmed" {
			return httpx.Conflict("not_confirmed", "Só matrículas confirmadas podem ser terminadas.")
		}
		tag, err := tx.Exec(r.Context(), `
			UPDATE charge c SET cancelled_at = now(), cancelled_by = $3, cancel_reason = $4
			WHERE c.enrolment_id = $1 AND c.cancelled_at IS NULL AND c.due_date > $2
			  AND NOT EXISTS (SELECT 1 FROM allocation a WHERE a.charge_id = c.id AND a.active)`,
			id, date, auth.CurrentUser(r.Context()).ID, "Desistência: "+req.Reason)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE enrolment SET status = 'withdrawn', withdrawn_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE contracted_service SET to_month = $2 WHERE enrolment_id = $1 AND to_month IS NULL`,
			id, int(date.Month())); err != nil {
			return err
		}
		if out, err = h.load(r.Context(), tx, entityID, id, false); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "withdraw", Table: "enrolment", RecordID: id.String(),
			After: map[string]any{"date": date.Format(time.DateOnly), "reason": req.Reason, "cancelled_charges": tag.RowsAffected()}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// ---------------------------------------------------------------------------
// Change services on a confirmed enrolment
// ---------------------------------------------------------------------------

type servicesRequest struct {
	MonthlyFeeIDs []uuid.UUID `json:"monthly_fee_ids"` // the full set of optional services wanted from FromMonth
	FromMonth     int         `json:"from_month"`
}

// changeServices adds or removes optional monthly services from a month on.
// Removed services: their unpaid charges from that month are cancelled (a
// charge with a payment blocks the change). Added services: charges are
// generated from that month.
func (h *Handler) changeServices(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var req servicesRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	var out *Enrolment
	err = db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		e, err := h.load(r.Context(), tx, entityID, id, true)
		if err != nil {
			return err
		}
		if e.Status != "confirmed" {
			return httpx.Conflict("not_confirmed", "Só matrículas confirmadas.")
		}
		cd, err := loadCalendar(r.Context(), tx, entityID, e.SchoolYearID)
		if err != nil {
			return err
		}
		if !slices.Contains(cd.cal.BillingMonths, req.FromMonth) || req.FromMonth < e.StartMonth {
			return httpx.Validation(httpx.Fields{"from_month": "Mês inválido."})
		}
		fees, err := applicableFees(r.Context(), tx, entityID, e.SchoolYearID, *e.ClassID)
		if err != nil {
			return err
		}
		want := map[uuid.UUID]bool{}
		for _, fid := range req.MonthlyFeeIDs {
			fee, ok := fees[fid]
			if !ok || fee.Kind != "monthly" {
				return httpx.Validation(httpx.Fields{"monthly_fee_ids": "Serviço mensal inválido para esta turma."})
			}
			want[fid] = true
		}
		current := map[uuid.UUID]Service{}
		for _, s := range e.Services {
			if s.ToMonth == nil {
				current[s.FeeItemID] = s
				if s.Mandatory {
					want[s.FeeItemID] = true // mandatory services cannot be removed
				}
			}
		}
		userID := auth.CurrentUser(r.Context()).ID
		var removed, added []uuid.UUID
		for fid := range current {
			if want[fid] {
				continue
			}
			var blocked bool
			if err := tx.QueryRow(r.Context(), `
				SELECT EXISTS (SELECT 1 FROM charge c JOIN allocation a ON a.charge_id = c.id AND a.active
				               WHERE c.enrolment_id = $1 AND c.fee_item_id = $2 AND c.period_month >= $3 AND c.cancelled_at IS NULL)`,
				id, fid, req.FromMonth).Scan(&blocked); err != nil {
				return err
			}
			if blocked {
				return httpx.Conflict("charges_paid", "Há mensalidades deste serviço já pagas a partir desse mês; anule os pagamentos primeiro.")
			}
			if _, err := tx.Exec(r.Context(), `
				UPDATE charge SET cancelled_at = now(), cancelled_by = $4, cancel_reason = 'Serviço cancelado'
				WHERE enrolment_id = $1 AND fee_item_id = $2 AND period_month >= $3 AND cancelled_at IS NULL`,
				id, fid, req.FromMonth, userID); err != nil {
				return err
			}
			if _, err := tx.Exec(r.Context(), `
				UPDATE contracted_service SET to_month = $3 WHERE enrolment_id = $1 AND fee_item_id = $2 AND to_month IS NULL`,
				id, fid, req.FromMonth-1); err != nil {
				return err
			}
			removed = append(removed, fid)
		}
		for fid := range want {
			if _, ok := current[fid]; ok {
				continue
			}
			added = append(added, fid)
		}
		if len(added) > 0 {
			addFees, err := loadFees(r.Context(), tx, entityID, added)
			if err != nil {
				return err
			}
			drafts, err := billing.Generate(cd.cal, req.FromMonth, addFees, cd.vatMethod, h.today())
			if err != nil {
				return err
			}
			// Skip months that already have a live charge for the same service.
			var keep []billing.Draft
			for _, d := range drafts {
				var exists bool
				if err := tx.QueryRow(r.Context(), `
					SELECT EXISTS (SELECT 1 FROM charge WHERE enrolment_id = $1 AND fee_item_id = $2 AND period_month = $3 AND cancelled_at IS NULL)`,
					id, d.FeeItemID, d.PeriodMonth).Scan(&exists); err != nil {
					return err
				}
				if !exists {
					keep = append(keep, d)
				}
			}
			if err := insertCharges(r.Context(), tx, entityID, e, keep); err != nil {
				return err
			}
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO contracted_service (enrolment_id, fee_item_id, from_month) SELECT $1, unnest($2::uuid[]), $3`,
				id, added, req.FromMonth); err != nil {
				return err
			}
		}
		if out, err = h.load(r.Context(), tx, entityID, id, false); err != nil {
			return err
		}
		return audit.Write(r.Context(), tx, audit.Entry{EntityID: &entityID, Action: "change_services", Table: "enrolment", RecordID: id.String(),
			After: map[string]any{"from_month": req.FromMonth, "added": added, "removed": removed}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// ---------------------------------------------------------------------------
// Renewal (bulk)
// ---------------------------------------------------------------------------

type renewItem struct {
	PupilID uuid.UUID `json:"pupil_id"`
	ClassID uuid.UUID `json:"class_id"`
}

type renewRequest struct {
	SchoolYearID uuid.UUID   `json:"school_year_id"`
	Items        []renewItem `json:"items"`
	Confirm      bool        `json:"confirm"`
}

type renewResult struct {
	PupilID     uuid.UUID  `json:"pupil_id"`
	EnrolmentID *uuid.UUID `json:"enrolment_id"`
	Status      string     `json:"status"` // draft | confirmed | error
	Error       string     `json:"error,omitempty"`
}

// renew creates returning enrolments for the target year. Optional monthly
// services are carried over by fee code; the re-enrolment fee is added when
// the catalogue has one ("re_enrolment"). Each pupil is processed in its own
// transaction, so one failure does not block the rest.
func (h *Handler) renew(w http.ResponseWriter, r *http.Request) error {
	entityID, err := auth.SingleEntity(r.Context())
	if err != nil {
		return err
	}
	var req renewRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if len(req.Items) == 0 || len(req.Items) > 200 {
		return httpx.Validation(httpx.Fields{"items": "Entre 1 e 200 alunos por pedido."})
	}
	results := make([]renewResult, 0, len(req.Items))
	for _, it := range req.Items {
		res := renewResult{PupilID: it.PupilID}
		err := db.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
			// Optional services of the previous confirmed enrolment (any entity), by code.
			var prevCodes []string
			if err := tx.QueryRow(r.Context(), `
				SELECT COALESCE(array_agg(f.code), '{}') FROM contracted_service cs
				JOIN fee_item f ON f.id = cs.fee_item_id
				WHERE cs.to_month IS NULL AND NOT f.mandatory AND cs.enrolment_id = (
					SELECT e.id FROM enrolment e JOIN school_year sy ON sy.id = e.school_year_id
					WHERE e.pupil_id = $1 AND e.status = 'confirmed' AND e.school_year_id <> $2
					ORDER BY sy.year DESC LIMIT 1)`, it.PupilID, req.SchoolYearID).Scan(&prevCodes); err != nil {
				return err
			}
			var monthly, oneOff []uuid.UUID
			rows, err := tx.Query(r.Context(), `
				SELECT f.id, f.kind FROM fee_item f
				WHERE f.entity_id = $1 AND f.school_year_id = $2 AND f.active
				  AND ((f.kind = 'monthly' AND f.code = ANY($3)) OR (f.kind = 'one_off' AND f.code = 're_enrolment'))
				  AND (NOT EXISTS (SELECT 1 FROM fee_item_class fc WHERE fc.fee_item_id = f.id)
				       OR EXISTS (SELECT 1 FROM fee_item_class fc WHERE fc.fee_item_id = f.id AND fc.class_id = $4))`,
				entityID, req.SchoolYearID, prevCodes, it.ClassID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var fid uuid.UUID
				var kind string
				if err := rows.Scan(&fid, &kind); err != nil {
					rows.Close()
					return err
				}
				if kind == "monthly" {
					monthly = append(monthly, fid)
				} else {
					oneOff = append(oneOff, fid)
				}
			}
			rows.Close()
			classID := it.ClassID
			id, err := h.createDraft(r.Context(), tx, entityID, enrolmentRequest{
				PupilID: it.PupilID, SchoolYearID: req.SchoolYearID, ClassID: &classID, Type: "returning",
				MonthlyFeeIDs: monthly, OneOffFeeIDs: oneOff,
				Consents: Consents{FeesAgreement: req.Confirm, UniformBooks: true},
			})
			if err != nil {
				return err
			}
			res.EnrolmentID, res.Status = &id, "draft"
			if req.Confirm {
				if err := h.confirmTx(r.Context(), tx, entityID, id); err != nil {
					return err
				}
				res.Status = "confirmed"
			}
			return nil
		})
		if err != nil {
			if _, ok := err.(*httpx.Error); !ok {
				slog.ErrorContext(r.Context(), "renewal failed", "pupil_id", it.PupilID, "err", err)
			}
			res.EnrolmentID, res.Status, res.Error = nil, "error", errorMessage(err)
		}
		results = append(results, res)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"results": results})
	return nil
}

func errorMessage(err error) string {
	if apiErr, ok := err.(*httpx.Error); ok {
		msg := apiErr.Message
		for _, v := range apiErr.Fields {
			msg += " " + v
		}
		return msg
	}
	return "Erro interno."
}
