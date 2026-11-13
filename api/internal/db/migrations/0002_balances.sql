-- Derived balances. One definition used by every list, account and report.
-- Status (paid / overdue / ...) is derived in Go (billing.Status) because it
-- depends on "today" in the school's time zone.

CREATE VIEW v_charge_balance AS
SELECT
    c.id AS charge_id,
    c.entity_id,
    c.enrolment_id,
    c.pupil_id,
    c.kind,
    c.due_date,
    c.amount_gross,
    c.cancelled_at IS NOT NULL AS cancelled,
    COALESCE(d.total, 0)::bigint AS discount,
    GREATEST(c.amount_gross - COALESCE(d.total, 0), 0)::bigint AS payable,
    COALESCE(a.total, 0)::bigint AS paid,
    CASE WHEN c.cancelled_at IS NOT NULL THEN 0
         ELSE GREATEST(c.amount_gross - COALESCE(d.total, 0) - COALESCE(a.total, 0), 0)
    END::bigint AS remaining
FROM charge c
LEFT JOIN (SELECT charge_id, sum(amount) AS total FROM discount GROUP BY charge_id) d ON d.charge_id = c.id
LEFT JOIN (SELECT charge_id, sum(amount) AS total FROM allocation WHERE active GROUP BY charge_id) a ON a.charge_id = c.id;
