# Nova Vida SMS

School management system (enrolment + finance) for Nova Vida, Xai-Xai, Mozambique. Two legal entities on one platform: preschool (Jardim Infantil) and primary school (Escola Primária Nova Vida, SU, Lda.).

Read before working:
- `docs/technical-design.md` — architecture, data model, billing rules, API. Source of truth.
- `docs/ui-brief.md` — screens and domain rules from the user's side.
- `docs/ui-prototype-review.md` — design tokens and known prototype issues; `docs/prototype/index.html` is the visual reference.

## Stack
- `api/`: Go, chi, pgx/v5, sqlc, goose, slog. Contract in `api/openapi.yaml`.
- `web/`: React + Vite + TypeScript, Tailwind, shadcn/ui, TanStack Query, react-hook-form + zod.
- `deploy/`: Docker Compose (caddy, api, postgres, gotenberg).

## Rules that must not be broken
- Money is `bigint` centavos in DB, `money.Amount` (int64) in Go, integer centavos in JSON. Never floats.
- Every financial and enrolment query is scoped by entity (`X-Entity-ID`). Pupils and guardians are global.
- Charge status and balances are derived (views / billing package), never stored as truth.
- Payments, receipts and charges are never deleted: cancel with reason, keep the number.
- Document numbers are gap-free per (entity, series, year), assigned inside the same transaction.
- VAT and prices are snapshotted on each charge.
- Every write on financial, enrolment, settings and user data writes an `audit_log` row in the same transaction.
- Billing logic (generation, penalties, allocation) lives in `api/internal/billing` as pure functions with table tests.

## Conventions
- Code, comments, docs, commit messages: English. UI copy: Portuguese (Mozambique), in `web/src/lib/i18n/pt.ts`.
- Display formats: `5.035,00 MT`, dates `dd/mm/yyyy`.
- Update `api/openapi.yaml` in the same change as any endpoint change; regenerate web types.
