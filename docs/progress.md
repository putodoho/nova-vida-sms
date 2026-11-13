# Progress

Keep this file current at the end of every work session: what is done, what is next, decisions taken.

## Milestones

| # | Milestone | Status |
| --- | --- | --- |
| 1 | Foundation: schema, auth, RBAC, audit, entity scope, seed, compose | Done (PR #1, #2) |
| 2 | Settings API: years, levels/penalty rules, classes, fee catalogue, discount types, roles and users | Done (PR #3) |
| 3 | Pupils, guardians, enrolment, charge generation, renewal | Done (PR `feat/m3-enrolment`) |
| 4 | Payments: preview, allocation, receipts, numbering, cancellation | Next |
| 5 | Penalties job, waivers, discount requests | |
| 6 | PDFs (Gotenberg), dashboard, reports, Excel export | |
| 7 | Web app (React) | |
| 8 | Import of existing data, backups, go-live | |

## Milestone 1 — done

- Full schema in `api/internal/db/migrations/0001_init.sql` (all tables of the design, incl. ledger).
- Embedded migration runner with advisory lock (`internal/db`).
- Money type in centavos, VAT split for the three methods, formatting `5.035,00 MT`; tests reproduce the client's statement (5.035 MT net/month, 50.350 MT/year).
- Auth: argon2id passwords, server-side sessions (cookie `nv_session`, sliding expiry), login/logout/me/password change.
- RBAC: permissions in code (`internal/auth/permissions.go`), 4 seeded roles, `auth.Require` middleware.
- Entity scope: `X-Entity-ID` middleware; writes require one entity the user can access.
- Audit log written in the same transaction as each change.
- Entities and receiving accounts endpoints (first module using scope + audit).
- Seed for 2027: 2 entities, levels with penalty rules, 10 classes, fee catalogue, discount types, number series, cash account. CLI: `migrate`, `seed`, `create-admin`.
- Docker: API image (distroless), compose with Postgres, Gotenberg, Caddy.
- Integration tests (auth flow, scope, permissions, validation, audit, accounts).

## Milestone 2 — done

- `internal/billing/rules.go`: penalty rule (none / percent / fixed tiers, replace or cumulative) and one-off due rule, validated, with tests.
- Settings endpoints (`internal/modules/settings`): school years (global; months and due day frozen once charges exist), levels with penalty rules, classes (capacity cannot drop below enrolled; delete only without enrolments), fee catalogue with class scope and net/VAT/gross lines, copy catalogue between years, discount types.
- Access endpoints (`internal/modules/access`): staff, users (roles + entities; lower-case logins; only grant entities the actor has; cannot deactivate self or remove the last active admin; deactivation and password reset end sessions), roles (admin role always has every permission; system roles cannot be deleted).
- Every write audited; integration tests for all of the above. OpenAPI 0.2.0.

## Milestone 3 — done

- `billing.Generate`: monthly charges per billing month from the start month, one-off charges on their date (or on enrolment when the date has passed / `on_enrolment`); `billing.Status` derives paid / partial / overdue / upcoming / cancelled. Tests reproduce the client's statement (30 charges, 53.000 MT gross, 2.650 MT VAT, 50.350 MT net).
- Migration `0002_balances.sql`: view `v_charge_balance` (discount, payable, paid, remaining per charge).
- `internal/numbering`: gap-free pupil numbers (`NV-2027-0001`) and document series (used from M4).
- Pupils (`internal/modules/pupils`, global): create/update, quick search, profile (health only with `health.read`; `health_alert` always), health record (audit logs only that it changed), guardian links (exactly one payer; link existing or create inline), pick-up persons, emergency contacts; guardians search by name/phone digits, detail with children.
- Enrolments (`internal/modules/enrolments`, entity-scoped): draft with services and charge preview; confirm (fees consent, one payer, class capacity with row lock, one confirmed enrolment per pupil and year) generates charges; withdraw cancels unpaid charges due after the leaving date; change optional services from a month; bulk renewal into a new year (services carried by fee code, `re_enrolment` fee added, per-pupil result); pupils list with balances; pupil account with Em dívida / Total por pagar / Pago / Crédito.
- API clock (`Deps.Now`, school time zone `Deps.Loc`) so "today" is the school's date; tests run at 10 March 2027.

## Decisions taken during build (differences from the design doc)

- **No sqlc / goose**: queries are hand-written with pgx in each module; migrations use a ~60-line embedded runner. Fewer tools to install; same safety.
- Preschool receipt/statement prefixes are `REC-JI` / `EXT-JI` so the two companies' documents never look alike.
- Preschool fee VAT starts disabled until confirmed; primary monthly fees have 5% VAT included (flat method, as in the current statement).
- Primary one-off fees (enrolment, uniform, books) are not seeded: prices unknown.
- Pupil records are created before the enrolment (wizard step 1), so an abandoned draft leaves a pupil without enrolment; that is harmless and lets the receptionist resume later.
- A partly paid charge past its due date shows as **overdue** (the money is late); `partial` is only for charges not yet due.
- Billing starts at the enrolment's `start_month` chosen by the receptionist; months before it are never charged.
- Bulk renewal with `confirm: true` records the fees consent on behalf of the guardian (the paper renewal form is the evidence).
- Class changes during the year are not supported yet (would need tuition re-pricing); withdraw and re-enrol, or adjust services.

## Notes for whoever continues

- `go.sum` is committed; the Dockerfile requires it. After adding a dependency, run `go mod tidy` on a machine with normal internet access and commit `go.sum`.
- The build sandbox uses a git-ignored `go.work` that maps `golang.org/x/*` and `gopkg.in/*` to their GitHub mirrors; not needed on a normal machine.

## Open questions for the client

See section 11 of `docs/technical-design.md`.
