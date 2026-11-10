# Nova Vida SMS — Technical Design

Status: draft v1 · 2026-11-10
Inputs: `docs/ui-brief.md`, `docs/ui-prototype-review.md`, scope doc (client answers of 2026-11-01).

## 1. Summary

A single web application serving two legal entities (preschool and primary school) of the Nova Vida school in Xai-Xai. Go REST API + PostgreSQL, React SPA, deployed with Docker Compose on one VPS. The core of the system is a **per-pupil ledger**: charges (fees, one-off items, penalties) on one side, payments allocated to charges on the other, discounts as approved adjustments. Everything financial is scoped to an entity, numbered gap-free, and audited.

## 2. Stack

| Layer | Choice | Why |
| --- | --- | --- |
| API | Go 1.24, `chi` router, `pgx/v5` with hand-written SQL | Plain SQL, no ORM magic around money |
| Migrations | Embedded SQL files + small runner (`internal/db`) | Runs on API start or via CLI |
| Validation | `go-playground/validator` | Request DTOs |
| Logging | `log/slog` (JSON) | Standard library |
| Auth | Server-side sessions in Postgres, opaque token in `HttpOnly; Secure; SameSite=Lax` cookie; `argon2id` passwords | Revocable sessions, no token handling in the browser |
| PDF | HTML templates (`html/template`) → **Gotenberg** container (headless Chromium) | Printables reuse the prototype's HTML/CSS; no PDF layout code in Go |
| Scheduler | In-process (`robfig/cron/v3`) with Postgres advisory lock | Daily penalty run; safe if two API replicas ever run |
| DB | PostgreSQL 16 | |
| Web | React 18 + Vite + TypeScript, Tailwind CSS, shadcn/ui | Matches the prototype and brief |
| Web data | TanStack Query, React Router, react-hook-form + zod | |
| Web misc | `date-fns` (pt locale), Recharts, `openapi-typescript` for API types | |
| Contract | `api/openapi.yaml` (hand-written, source of truth) | Web types generated from it |
| Deploy | Docker Compose: `caddy` (TLS, serves web build, proxies `/api`), `api`, `postgres`, `gotenberg` | One VPS, same origin (no CORS) |

## 3. Repository layout

```
nova-vida-sms/
├── api/
│   ├── cmd/server/main.go          # HTTP server + scheduler
│   ├── cmd/cli/main.go             # migrate, seed, create-admin, import
│   ├── internal/
│   │   ├── config/
│   │   ├── db/                     # pool, tx helper, migration runner
│   │   │   └── migrations/*.sql
│   │   ├── auth/                   # sessions, passwords, RBAC middleware
│   │   ├── httpx/                  # router, errors, pagination, entity scope
│   │   ├── money/                  # Money type, VAT, rounding
│   │   ├── billing/                # charge generation, penalties, allocation (pure logic + service)
│   │   ├── numbering/              # gap-free series
│   │   ├── audit/
│   │   ├── pdf/                    # templates + Gotenberg client
│   │   └── modules/                # one package per area: http handlers + service
│   │       ├── entities/ settings/ years/ classes/ fees/
│   │       ├── pupils/ guardians/ enrolments/
│   │       ├── charges/ payments/ discounts/ documents/ reports/
│   │       └── users/
│   ├── openapi.yaml
│   └── Dockerfile
├── web/
│   ├── src/{app,components,features,lib,api}/
│   └── Dockerfile (build only; output served by Caddy)
├── deploy/{docker-compose.yml,Caddyfile,.env.example}
├── docs/
└── CLAUDE.md
```

## 4. Core concepts

### 4.1 Entities and scope
- `entity` = legal company (preschool, primary). Has NUIT, address, logo, motto, VAT settings, receiving accounts, numbering series.
- **Pupils and guardians are global** (a child moves from Pré to 1.ª classe, i.e. from one company to the other; a guardian pays in both). **Enrolments, charges, payments, discounts, receipts and statements belong to one entity.**
- Every request carries the active entity in header `X-Entity-ID` (or `all` for read-only aggregate views where allowed). Middleware checks the user may access that entity and injects it into the context; every financial query filters by it. Writes never accept `all`.

### 4.2 Money
- Stored as `bigint` **centavos** (1 MT = 100). Go type `money.Amount int64`; never `float64`.
- JSON: integer centavos (`"amount": 503500`). The web formats as `5.035,00 MT`.
- Rounding: half-up to the centavo, applied once per computed line.

### 4.3 VAT
Configured per entity and per fee item: `vat_mode ∈ {none, included, added}`, `vat_rate_bp` (basis points, 500 = 5%), plus `vat_method` per entity:
- `added`: net = price, vat = price × rate, gross = net + vat.
- `included_standard`: gross = price, vat = price × rate / (1 + rate), net = gross − vat → 2.800 → VAT 133,33.
- `included_flat` (current school template): gross = price, vat = price × rate, net = gross − vat → 2.800 → VAT 140,00.
The accountant chooses; default `included_flat` to match current documents. **VAT values are snapshotted on each charge** so later setting changes never alter issued documents.

### 4.4 Ledger
```
charge  (debit)   ← penalty (a charge of kind 'penalty' with parent_charge_id)
   ↑ allocation   ← payment (credit)
   ↑ discount     (approved adjustment, reduces the charge's payable amount)
```
- `charge.payable = amount_gross − Σ approved discounts`
- `charge.paid = Σ active allocations`
- `charge.remaining = payable − paid`
- Status is derived, never stored as truth: `paid` (remaining = 0), `partial` (0 < paid < payable), `overdue` (remaining > 0 and due_date < today), `upcoming` (remaining > 0 and due_date ≥ today), `cancelled` (charge.cancelled_at set).
- Pupil account (per entity, per year): **Em dívida** = Σ remaining of charges due ≤ today; **Total por pagar** = Σ remaining of all non-cancelled charges; **Crédito** = Σ unallocated payment amounts.

### 4.5 Charge generation
- Confirming an enrolment creates, for each contracted monthly service, one charge per billing month of the school year (default Feb–Nov, due day 5), starting from the enrolment's start month; and one charge per selected one-off item (due date from the fee item rule, e.g. 30/01 for preschool first month and books).
- Changing services mid-year: future **unpaid** charges of a removed service are cancelled; charges for an added service are generated from the next billing month. Paid or partially paid charges are never altered.
- Withdrawal: future unpaid charges cancelled; past debt stays.
- Prices come from the fee catalogue of (entity, year, class). Charges snapshot description, unit price and VAT.

### 4.6 Penalties
Rule per level (configurable), evaluated by a **daily job at 01:00** (and on demand) for charges of kind `monthly` that are overdue:
- `percent`: one penalty of `rate × remaining` on day `due + grace_days` (primary: 5%, grace 0).
- `fixed_tiers`: tiers `[{after_day: 5, amount: 100}, {after_day: 10, amount: 200}]`, mode `replace` (total penalty = highest tier reached; the tier-2 penalty is the difference) or `cumulative`. Default `replace` until the client confirms.
- Idempotent: unique `(parent_charge_id, rule_tier)`. If the parent is paid before the job runs, no penalty.
- Waiver: a penalty is cancelled via `penalty waiver` request (permission `penalties.waive`, reason required), audited.

### 4.7 Payment allocation
On payment confirmation, inside one transaction:
1. Lock the pupil's open charges for the entity (`SELECT … FOR UPDATE`).
2. Order: due date ascending; on the same due date, penalties before fees, then one-off items.
3. Allocate until the amount is exhausted. Future (not yet due) charges are included, so advance payments settle the next months.
4. Leftover becomes credit (unallocated remainder on the payment), auto-applied when new charges are generated.
5. Assign receipt number (§4.8), write audit row, commit, then render the receipt PDF asynchronously (cached by receipt id).
- **Guardian payment** for several children: the receptionist enters the split per child (default: oldest due first across children); stored as one payment per child sharing a `payment_group_id` and one receipt listing all.
- **Cancellation** (permission `payments.cancel`, reason required): payment marked cancelled, its allocations set inactive, receipt shown as ANULADO; never deleted. Charge statuses recompute automatically.
- Allocation preview endpoint runs steps 2–4 without writing.

### 4.8 Document numbering
Gap-free per (entity, series, year): `REC-2027-00001`, `EXT-2027-00001`. Table `number_series(entity_id, series, year, next_value)` incremented with `UPDATE … RETURNING` in the same transaction as the document. Cancelled receipts keep their number.

### 4.9 Discounts
- `discount_type` (per entity): name (Social, Staff child, Siblings), mode `percent|fixed`, default value, applies to fee items (e.g. monthly fee only), `requires_approval`.
- `discount_request` on an enrolment: type, value, scope (all remaining months of a service, or specific charges), reason, requested_by → `approved|rejected` by a user with `discounts.approve`. On approval, one `discount` row per affected unpaid charge.

### 4.10 Authorisation
- Permissions are constants in code (`pupils.read`, `pupils.write`, `enrolments.write`, `payments.create`, `payments.cancel`, `charges.read`, `discounts.request`, `discounts.approve`, `penalties.waive`, `documents.generate`, `reports.read`, `health.read`, `settings.manage`, `users.manage`, `audit.read`).
- Roles are data (`role`, `role_permission`); seed roles: Administrador, Director, Recepção/Secretaria, Funcionário. Users get roles and a list of entities they may access.
- Health data visible only with `health.read`; finance columns hidden in class lists without `charges.read`.

### 4.11 Audit
`audit_log(at, user_id, entity_id, action, table, record_id, before jsonb, after jsonb, ip)` written in the same transaction by the service layer for every create/update/cancel on financial, enrolment, settings and user tables.

## 5. Data model

All tables: `id uuid pk default gen_random_uuid()`, `created_at`, `updated_at`; financial tables add `created_by`. Amounts are `bigint` centavos.

| Table | Key columns | Notes |
| --- | --- | --- |
| entity | code, legal_name, short_name, nuit, address, motto, logo_path, vat_method | 2 rows |
| receiving_account | entity_id, kind (cash, bank, mpesa, emola), label, holder, number, nib, active | Many per entity |
| school_year | year, starts_on, ends_on, billing_months int[] (2..11), due_day, status (planning, open, closed) | Shared by both entities |
| level | entity_id, code (preschool, primary, secondary), name, penalty_rule jsonb | |
| class | entity_id, level_id, school_year_id, name (1.ª A, Sala 1), grade (1..6, null for preschool), capacity, room, teacher_staff_id | |
| fee_item | entity_id, school_year_id, code, name, kind (monthly, one_off), amount, vat_mode, vat_rate_bp, due_rule jsonb, mandatory bool | |
| fee_item_class | fee_item_id, class_id | Price scope |
| discount_type | entity_id, name, mode, value, fee_item_codes text[], requires_approval | |
| pupil | number (NV-2027-0042, global), full_name, sex, birth_date, neighbourhood, religion, orphan (none, father, mother, both), photo_path, home_discipline_notes, status | |
| health_record | pupil_id (unique), chronic, allergies, diet, medication, doctor_name, doctor_phone, notes, emergency_care_consent_at | |
| guardian | full_name, phone1, phone2, email, neighbourhood, id_doc | Global |
| pupil_guardian | pupil_id, guardian_id, relationship, is_payer, receives_notices, may_pick_up, lives_with | Exactly one payer per pupil (partial unique index) |
| pickup_person | pupil_id, full_name, phone, id_doc | |
| emergency_contact | pupil_id, position, full_name, relationship, phone | |
| enrolment | entity_id, school_year_id, pupil_id, class_id, type (new, returning), status (draft, confirmed, withdrawn), start_month, consents jsonb, confirmed_at, withdrawn_at | Unique (pupil, year) where status ≠ draft |
| contracted_service | enrolment_id, fee_item_id, from_month, to_month | |
| charge | entity_id, enrolment_id, pupil_id, fee_item_id, kind (monthly, one_off, penalty), parent_charge_id, period_month, description, due_date, amount_net, amount_vat, amount_gross, cancelled_at, cancel_reason, rule_tier | Index (entity_id, pupil_id, due_date) |
| discount_request | entity_id, enrolment_id, discount_type_id, mode, value, scope jsonb, reason, status, requested_by, decided_by, decided_at, decision_note | |
| discount | charge_id, discount_request_id, amount | |
| payment | entity_id, pupil_id, payment_group_id, receipt_no, paid_on, amount, method, receiving_account_id, reference, notes, received_by, cancelled_at, cancelled_by, cancel_reason | Unique (entity_id, receipt_no) |
| allocation | payment_id, charge_id, amount, active | |
| statement | entity_id, pupil_id, school_year_id, number, issued_at, issued_by, pdf_path, snapshot jsonb | |
| number_series | entity_id, series, year, next_value | PK (entity_id, series, year) |
| staff | full_name, function, phone, email, active | |
| app_user | staff_id, login (email or phone), password_hash, active, last_login_at | |
| role, role_permission, user_role, user_entity | | RBAC |
| session | token_hash, user_id, expires_at, ip, user_agent | |
| audit_log | see §4.11 | |
| import_job | kind, file_path, status, report jsonb | Pupil / payment imports |

Derived balances are SQL views (`v_charge_balance`, `v_pupil_account`) so lists and reports use the same definition.

## 6. API (v1)

Base `/api/v1`, JSON, cookie session, header `X-Entity-ID`. Lists: `?page=&page_size=&sort=&q=` + filters, response `{ "items": [...], "total": n }`. Errors: `{ "error": { "code": "validation_failed", "message": "...", "fields": {"amount": "required"} } }` with proper HTTP status.

| Area | Endpoints |
| --- | --- |
| Auth | `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, `POST /auth/password` |
| Settings | `GET/PUT /entities/{id}`, `/entities/{id}/accounts`, `/school-years`, `/levels`, `/classes`, `/fee-items` (+ `POST /fee-items/copy-from-year`), `/discount-types` |
| Pupils | `GET/POST /pupils`, `GET/PATCH /pupils/{id}`, `GET/PUT /pupils/{id}/health`, `/pupils/{id}/guardians`, `/pupils/{id}/pickup-persons`, `/pupils/{id}/emergency-contacts`, `GET /pupils/{id}/account?year=` |
| Guardians | `GET/POST /guardians`, `GET/PATCH /guardians/{id}`, `GET /guardians/{id}/account` |
| Enrolments | `POST /enrolments` (draft), `PATCH /enrolments/{id}`, `POST /enrolments/{id}/confirm`, `POST /enrolments/{id}/withdraw`, `PUT /enrolments/{id}/services`, `POST /enrolments/renew` (bulk) |
| Charges | `GET /charges` (arrears list, aging), `POST /charges/{id}/cancel`, `POST /penalties/{id}/waive`, `POST /penalties/run` (admin) |
| Payments | `POST /payments/preview`, `POST /payments`, `GET /payments`, `GET /payments/{id}`, `POST /payments/{id}/cancel` |
| Discounts | `GET/POST /discount-requests`, `POST /discount-requests/{id}/approve`, `.../reject` |
| Documents | `GET /payments/{id}/receipt.pdf`, `POST /statements` (single or bulk by class) → PDFs, `GET /statements/{id}.pdf`, `GET /enrolments/{id}/form.pdf` |
| Reports | `GET /reports/dashboard`, `/reports/income`, `/reports/arrears`, `/reports/pupils`, `/reports/discounts`, `/reports/penalties`, `/reports/vat` (each `?format=json|xlsx`) |
| Users | `GET/POST /staff`, `GET/POST /users`, `PATCH /users/{id}`, `GET/PUT /roles`, `GET /permissions` |
| Audit | `GET /audit` |
| Import | `POST /imports/pupils`, `POST /imports/payments` (CSV/XLSX templates, dry-run report first) |

## 7. Frontend

- Routes mirror the sidebar; feature folders per area (`features/payments`, …) with API hooks, forms and pages.
- App shell holds the **active entity and year** in a store; the API client adds `X-Entity-ID`; changing entity invalidates all queries.
- Forms: react-hook-form + zod; money input stores centavos; date picker in dd/mm/yyyy.
- Tables: one `DataTable` built on TanStack Table (filters, column visibility, pagination, totals row); on < 768 px it renders cards.
- Permissions from `/auth/me` hide routes and actions; the API enforces them anyway.
- All UI copy in Portuguese, kept in `src/lib/i18n/pt.ts` (no runtime i18n library needed now).

## 8. Deployment and operations

- Docker Compose on one VPS (2 vCPU / 4 GB is enough for ~150 pupils and 10 users). Caddy handles HTTPS.
- Backups: nightly `pg_dump` (compressed) kept 30 days locally and copied off-server (S3-compatible bucket); restore tested before go-live.
- Uploaded files (photos, logos, PDFs) in a volume, included in backups.
- Environments: `dev` (compose locally), `prod`. Seed CLI creates the two entities, 2027 year, classes, fee catalogue and admin user.
- Health endpoint `/api/healthz`; logs to stdout.

## 9. Testing

- `billing` package is pure Go (input: charges, rules, dates; output: penalties/allocations) with table tests for every rule in this document, including the school's statement example (5.300 MT/month, 50.350 MT net per year).
- Integration tests against a real Postgres (compose service) for payment allocation, numbering under concurrency, and cancellation.
- Web: component tests for money input and allocation preview; Playwright smoke test for enrol → pay → receipt.

## 10. Build order

| # | Milestone | Content |
| --- | --- | --- |
| 1 | Foundation | Repo, compose, migrations, config, auth + sessions, RBAC, audit, entity scope, CLI seed |
| 2 | Settings | Entities, accounts, years, levels, classes, fee catalogue, discount types, penalty rules |
| 3 | Pupils & enrolment | Pupils, health, guardians, enrolment wizard API, charge generation, renewal |
| 4 | Payments | Preview, allocation, receipts, numbering, cancellation, guardian group payments |
| 5 | Penalties & discounts | Daily job, waivers, discount requests and approval |
| 6 | Documents & reports | Gotenberg PDFs (receipt, statement, enrolment form), dashboard, reports, Excel export |
| 7 | Web app | Shell + design tokens first, then screens in the order above (can start after milestone 1 against the OpenAPI contract) |
| 8 | Import & go-live | Pupil and payment import from the school's lists, backups, user training |

## 11. Open questions (do not block the build; defaults in brackets)

1. Preschool fixed penalty: tiers cumulative (100 + 200) or replacing (total 200)? [replace]
2. Does "a partir do dia 05" mean late on the 5th itself or from the 6th? [from the 6th: due date is the last day without penalty]
3. VAT calculation method: flat 5% of price (current template) or standard included VAT? [flat, per current template]
4. Pupil enrolled mid-year: billed from the start month only? [yes]
5. Credit left at year end: carried to next year or refunded? [carried]
6. Discount values per type and who approves. [admin approves; values set in settings]
7. Primary enrolment, re-enrolment, uniform and book prices. [entered in settings before go-live]
