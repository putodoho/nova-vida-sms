// Package ledger applies payments to charges inside a transaction. The pure
// ordering rules live in billing; this package reads and writes the rows.
package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// LockPupil serialises money movements of one pupil in one entity. Every
// writer (payments, cancellations, credit application, penalties) takes this
// lock first, so balances read afterwards are stable until commit.
func LockPupil(ctx context.Context, tx pgx.Tx, entityID, pupilID uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text), hashtext($2::text))`, entityID, pupilID)
	return err
}

// OpenCharges returns the pupil's non-cancelled charges with money remaining,
// ordered for allocation. Call LockPupil first.
func OpenCharges(ctx context.Context, tx pgx.Tx, entityID, pupilID uuid.UUID) ([]billing.OpenCharge, map[uuid.UUID]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.kind, c.due_date, b.remaining, c.description
		FROM charge c JOIN v_charge_balance b ON b.charge_id = c.id
		WHERE c.entity_id = $1 AND c.pupil_id = $2 AND c.cancelled_at IS NULL AND b.remaining > 0`, entityID, pupilID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []billing.OpenCharge
	desc := map[uuid.UUID]string{}
	for rows.Next() {
		var c billing.OpenCharge
		var due time.Time
		var d string
		if err := rows.Scan(&c.ID, &c.Kind, &due, &c.Remaining, &d); err != nil {
			return nil, nil, err
		}
		c.DueDate = due
		desc[c.ID] = d
		out = append(out, c)
	}
	billing.SortForAllocation(out)
	return out, desc, rows.Err()
}

// Allocate writes allocations of amount from a payment to the pupil's open
// charges and returns them with the unallocated rest.
func Allocate(ctx context.Context, tx pgx.Tx, entityID, pupilID, paymentID uuid.UUID, amount money.Amount) ([]billing.Allocation, money.Amount, error) {
	open, _, err := OpenCharges(ctx, tx, entityID, pupilID)
	if err != nil {
		return nil, 0, err
	}
	allocs, left := billing.Allocate(amount, open)
	for _, a := range allocs {
		if _, err := tx.Exec(ctx, `INSERT INTO allocation (payment_id, charge_id, amount) VALUES ($1, $2, $3)`,
			paymentID, a.ChargeID, a.Amount); err != nil {
			return nil, 0, err
		}
	}
	return allocs, left, nil
}

// ApplyCredit allocates the unallocated part of the pupil's valid payments
// (oldest payment first) to open charges. Call it after new charges are
// created or after a cancellation frees charges.
func ApplyCredit(ctx context.Context, tx pgx.Tx, entityID, pupilID uuid.UUID) error {
	if err := LockPupil(ctx, tx, entityID, pupilID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.amount - COALESCE((SELECT sum(a.amount) FROM allocation a WHERE a.payment_id = p.id AND a.active), 0)
		FROM payment p
		WHERE p.entity_id = $1 AND p.pupil_id = $2 AND p.cancelled_at IS NULL
		ORDER BY p.paid_on, p.created_at`, entityID, pupilID)
	if err != nil {
		return err
	}
	type credit struct {
		id   uuid.UUID
		left money.Amount
	}
	var credits []credit
	for rows.Next() {
		var c credit
		if err := rows.Scan(&c.id, &c.left); err != nil {
			rows.Close()
			return err
		}
		if c.left > 0 {
			credits = append(credits, c)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range credits {
		if _, _, err := Allocate(ctx, tx, entityID, pupilID, c.id, c.left); err != nil {
			return err
		}
	}
	return nil
}
