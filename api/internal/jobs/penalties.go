// Package jobs holds background work: the daily penalty run and its scheduler.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/putodoho/nova-vida-sms/api/internal/audit"
	"github.com/putodoho/nova-vida-sms/api/internal/billing"
	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/ledger"
	"github.com/putodoho/nova-vida-sms/api/internal/money"
)

// PenaltyResult summarises a run.
type PenaltyResult struct {
	Pupils    int          `json:"pupils"`
	Penalties int          `json:"penalties"`
	Amount    money.Amount `json:"amount"`
}

// RunPenalties charges the penalties due on `today` for overdue monthly
// charges of the given entities (nil = all). It is idempotent: running it
// twice on the same day adds nothing. Each pupil is handled in its own
// transaction under the pupil's ledger lock.
func RunPenalties(ctx context.Context, pool *pgxpool.Pool, today time.Time, entityIDs []uuid.UUID) (PenaltyResult, error) {
	var res PenaltyResult
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT c.entity_id, c.pupil_id
		FROM charge c JOIN v_charge_balance b ON b.charge_id = c.id
		WHERE c.kind = 'monthly' AND c.cancelled_at IS NULL AND b.remaining > 0 AND c.due_date < $1
		  AND ($2::uuid[] IS NULL OR c.entity_id = ANY($2))`, today, entityIDs)
	if err != nil {
		return res, err
	}
	type key struct{ entity, pupil uuid.UUID }
	var targets []key
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.entity, &k.pupil); err != nil {
			rows.Close()
			return res, err
		}
		targets = append(targets, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	for _, t := range targets {
		var n int
		var amt money.Amount
		err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
			var err error
			n, amt, err = penaltiesForPupil(ctx, tx, t.entity, t.pupil, today)
			return err
		})
		if err != nil {
			return res, fmt.Errorf("pupil %s: %w", t.pupil, err)
		}
		if n > 0 {
			res.Pupils++
			res.Penalties += n
			res.Amount += amt
		}
	}
	return res, nil
}

func penaltiesForPupil(ctx context.Context, tx pgx.Tx, entityID, pupilID uuid.UUID, today time.Time) (int, money.Amount, error) {
	if err := ledger.LockPupil(ctx, tx, entityID, pupilID); err != nil {
		return 0, 0, err
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.enrolment_id, c.period_month, c.description, c.due_date, b.remaining, l.penalty_rule,
		       COALESCE(array_agg(p.rule_tier) FILTER (WHERE p.id IS NOT NULL), '{}')
		FROM charge c
		JOIN v_charge_balance b ON b.charge_id = c.id
		JOIN enrolment e ON e.id = c.enrolment_id
		JOIN class cl ON cl.id = e.class_id
		JOIN level l ON l.id = cl.level_id
		LEFT JOIN charge p ON p.parent_charge_id = c.id AND p.kind = 'penalty'
		WHERE c.entity_id = $1 AND c.pupil_id = $2 AND c.kind = 'monthly' AND c.cancelled_at IS NULL
		  AND b.remaining > 0 AND c.due_date < $3
		GROUP BY c.id, b.remaining, l.penalty_rule`, entityID, pupilID, today)
	if err != nil {
		return 0, 0, err
	}
	type item struct {
		enrolment uuid.UUID
		period    *int
		draft     billing.PenaltyDraft
	}
	var items []item
	for rows.Next() {
		var c billing.OverdueCharge
		var enrolment uuid.UUID
		var period *int
		var ruleRaw []byte
		var tiers []int32
		if err := rows.Scan(&c.ID, &enrolment, &period, &c.Description, &c.DueDate, &c.Remaining, &ruleRaw, &tiers); err != nil {
			rows.Close()
			return 0, 0, err
		}
		var rule billing.PenaltyRule
		if err := json.Unmarshal(ruleRaw, &rule); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("penalty rule: %w", err)
		}
		c.ExistingTiers = map[int]bool{}
		for _, t := range tiers {
			c.ExistingTiers[int(t)] = true
		}
		for _, d := range billing.Penalties(rule, c, today) {
			items = append(items, item{enrolment, period, d})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	var total money.Amount
	for _, it := range items {
		d := it.draft
		if _, err := tx.Exec(ctx, `
			INSERT INTO charge (entity_id, enrolment_id, pupil_id, kind, parent_charge_id, rule_tier, period_month,
			                    description, due_date, amount_net, amount_vat, amount_gross)
			VALUES ($1, $2, $3, 'penalty', $4, $5, $6, $7, $8, $9, 0, $9)
			ON CONFLICT DO NOTHING`,
			entityID, it.enrolment, pupilID, d.ParentID, d.Tier, it.period, d.Description, d.DueDate, d.Amount); err != nil {
			return 0, 0, err
		}
		total += d.Amount
	}
	if len(items) == 0 {
		return 0, 0, nil
	}
	if err := ledger.ApplyCredit(ctx, tx, entityID, pupilID); err != nil {
		return 0, 0, err
	}
	eid := entityID
	err = audit.Write(ctx, tx, audit.Entry{EntityID: &eid, Action: "penalty", Table: "charge", RecordID: pupilID.String(),
		After: map[string]any{"date": today.Format(time.DateOnly), "penalties": len(items), "amount": total}})
	return len(items), total, err
}

// StartDaily runs fn every day at hour:00 school time, and once at start-up
// to catch up after downtime. A Postgres advisory lock ensures only one API
// instance runs it at a time. It returns when ctx is cancelled.
func StartDaily(ctx context.Context, pool *pgxpool.Pool, loc *time.Location, hour int, name string, fn func(context.Context, time.Time) error) {
	run := func() {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			slog.Error("job: acquire", "job", name, "err", err)
			return
		}
		defer conn.Release()
		var got bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, name).Scan(&got); err != nil || !got {
			return
		}
		defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, name) //nolint:errcheck
		now := time.Now().In(loc)
		y, m, d := now.Date()
		today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		start := time.Now()
		if err := fn(ctx, today); err != nil {
			slog.Error("job failed", "job", name, "err", err)
			return
		}
		slog.Info("job done", "job", name, "ms", time.Since(start).Milliseconds())
	}
	go func() {
		run()
		for {
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc)
			if !next.After(now) {
				next = next.AddDate(0, 0, 1)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				run()
			}
		}
	}()
}
