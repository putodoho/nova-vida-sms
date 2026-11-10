# Progress

Keep this file current at the end of every work session: what is done, what is next, decisions taken.

## Milestones

| # | Milestone | Status |
| --- | --- | --- |
| 1 | Foundation: schema, auth, RBAC, audit, entity scope, seed, compose | Done (PR `feat/m1-foundation`) |
| 2 | Settings API: years, levels/penalty rules, classes, fee catalogue, discount types, roles and users | Next |
| 3 | Pupils, guardians, enrolment, charge generation, renewal | |
| 4 | Payments: preview, allocation, receipts, numbering, cancellation | |
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

## Decisions taken during build (differences from the design doc)

- **No sqlc / goose**: queries are hand-written with pgx in each module; migrations use a ~60-line embedded runner. Fewer tools to install; same safety.
- Preschool receipt/statement prefixes are `REC-JI` / `EXT-JI` so the two companies' documents never look alike.
- Preschool fee VAT starts disabled until confirmed; primary monthly fees have 5% VAT included (flat method, as in the current statement).
- Primary one-off fees (enrolment, uniform, books) are not seeded: prices unknown.

## Notes for whoever continues

- `go.sum` is not committed yet: the build sandbox cannot reach proxy.golang.org. Run `go mod tidy` in `api/` once and commit `go.sum`. The Dockerfile runs `go mod tidy` if it is missing.
- The sandbox uses a git-ignored `go.work` that replaces `golang.org/x/*` with their GitHub mirrors; not needed on a normal machine.

## Open questions for the client

See section 11 of `docs/technical-design.md`.
