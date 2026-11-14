# Progress

Keep this file current at the end of every work session: what is done, what is next, decisions taken.

## Milestones

| # | Milestone | Status |
| --- | --- | --- |
| 1 | Foundation: schema, auth, RBAC, audit, entity scope, seed, compose | Done (PR #1, #2) |
| 2 | Settings API: years, levels/penalty rules, classes, fee catalogue, discount types, roles and users | Done (PR #3) |
| 3 | Pupils, guardians, enrolment, charge generation, renewal | Done (PR #4) |
| 4 | Payments: preview, allocation, receipts, numbering, cancellation | Done (PR #5) |
| 5 | Penalties job, waivers, discount requests | Done (PR #6) |
| 6 | PDFs (Gotenberg), dashboard, reports, Excel export | Done (PR #7) |
| 7a | Web app: foundation, shell, login, dashboard, pupils, profile, payments | Done (PR `feat/m7a-web-foundation`) |
| — | Hosting: CI on PRs, deploy on merge (GHCR images → VPS behind Nginx), `docs/deploy-vps.md` | Done (PR `ci/pipeline-vps`) |
| 7b-1 | Web app: enrolment wizard, drafts, renewal, withdrawal, edit pupil data | Done (PR `feat/m7b1-enrolment`) |
| 7b-2 | Web app: arrears and charges, penalty waivers, discount requests and approvals | Done (PR `feat/m7b2-charges`) |
| 7b-3 | Web app: payments list, documents (class statements, receipts), reports with Excel export, guardian profile | Done (PR `feat/m7b3-payments-docs-reports`) |
| 7b-4 | Web app: classes, school years, fee catalogue, penalty rules, discount types, receiving accounts, users and roles | Next |
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

## Milestone 4 — done

- `billing.Allocate`: oldest due first; on the same day penalties, then monthly fees, then one-off items; future charges included (advance payment); remainder is credit. Table tests.
- `internal/ledger`: per-pupil advisory lock for every money movement, allocation writer, `ApplyCredit` (unallocated payments are applied when new charges appear — enrolment confirm, added services — and after a cancellation).
- Payments (`internal/modules/payments`): preview without writing; create (method ↔ receiving account must match, reference required except cash, no future date, max one year back, duplicate reference refused, pupil must be enrolled in the entity); family payment split between children (one receipt each, shared group id); list with filters, sum and totals by method; detail with allocations and pupil balance; cancel with reason (receipt keeps its number).
- Receipt numbers `REC-2027-00001` (primary) and `REC-JI-2027-00001` (preschool), gap-free under concurrency (tested with 10 parallel payments, race detector on). A year's series is created automatically on its first receipt; the receipt year is the payment date's year.

## Milestone 5 — done

- `billing.Penalties`: percent (5% of what is still owed, once, after due + grace) and fixed tiers (replace: total = highest tier reached; cumulative: sum); a tier already charged or waived is never charged again. `billing.DiscountAmount`: percent of gross or fixed, capped at what is unpaid. Table tests for both school rules.
- `internal/jobs`: `RunPenalties` (per pupil, under the ledger lock, idempotent, credit applied to new penalties) and `StartDaily` (01:00 school time + once at start-up, Postgres advisory lock so one instance runs it; `RUN_JOBS=false` disables it).
- Charges (`internal/modules/charges`): arrears list with filters and aging buckets (1–30, 31–60, 61–90, 90+ days); cancel a fee (its penalties go with it, released money re-applied); waive a penalty; run penalties on demand.
- Discount requests (`internal/modules/discounts`): request (default mode/value from the type, from a month), pending requests show what approval would grant, approve/reject with note; types without approval apply at once; only an administrator may decide their own request.
- Penalty charges carry the parent's due date, so a payment settles the penalty right before the fee it belongs to.

## Milestone 6 — done

- `money.Words`: amounts in Portuguese words for receipts ("cinco mil e trinta e cinco meticais").
- `internal/report`: one table format for JSON and Excel; dependency-free XLSX writer (title, frozen header, filter, MT money format, dd/mm/yyyy dates, totals row) — opens in Excel, LibreOffice and openpyxl.
- `internal/pdf`: Gotenberg client (`preferCssPageSize`, page size from CSS `@page`).
- Documents (`internal/modules/documents`, Go `html/template`, styled with the prototype's tokens): receipt (A5, amount in words, ANULADO watermark), annual statement (A4, modelled on the school's template: services with VAT, one-off fees, monthly control with penalties/discounts/receipts/status, totals, payment details), enrolment form (A4, consents, signatures). `?format=html` returns the HTML (preview and browser printing); default is PDF. Statements are numbered (`EXT-2027-00001`) and stored as snapshots; bulk issue per class returns one document.
- Reports (`internal/modules/reports`, `reports.read`): dashboard; income by month/method/account; arrears per pupil; pupils per class; discounts; penalties; VAT — each with `?format=xlsx`.
- Samples rendered with Chromium (same engine as Gotenberg) in `docs/samples/`.

## Milestone 7a — done

- `web/`: Vite + React 18 + TypeScript + Tailwind 3 (tokens from the prototype) + Radix primitives + TanStack Query + Recharts + cmdk + sonner. Self-hosted Inter font.
- `src/lib`: API client (session cookie, `X-Entity-ID`, `ApiError` with field errors), formatters (centavos ↔ `5.035,00 MT`, dd/mm/yyyy), session provider (user, permissions, active entity incl. "Todas" read-only, school year), shared Portuguese labels.
- Screens: login; app shell (sidebar filtered by permissions, entity and year switchers, Ctrl+K pupil search, global "Registar pagamento", mobile drawer and floating button); dashboard; pupils list (filters, balances, mobile cards); pupil profile (balance header, health alert strip, tabs: summary, account, payments, guardians, health, documents, history; issue statement, enrolment form); payment sheet (pupil picker with balances, quick amounts, method ↔ account, reference, live allocation preview, receipt PDF/print).
- Receptionists (no `reports.read`) land on the pupils list.
- API: `/auth/me` returns the school's `today`; `FAKE_TODAY` (non-prod) pins the date for demos/training.
- Docker: `caddy` now builds the web app (`web/Dockerfile`).
- Verified in Chromium at 1440 px and 390 px against the real API with demo data (no horizontal scroll on mobile, no console errors).

## Milestone 7b-3 — done

- `/pagamentos`: received in the period (today / this month / this year / custom dates) with totals per method (click to filter), account and status filters, search by pupil, receipt or reference; paged list (family payments marked). Payment detail sheet: what it settled, receipt PDF / print, cancel with reason (`payments.cancel`; the receipt keeps its number and shows ANULADO).
- `/relatorios`: income (by month, method or account, date range), arrears, pupils per class, discounts, penalties and VAT — one generic table renderer for the API's `report.Table`, the same data as the Excel export button.
- `/documentos`: class statements (one numbered statement per pupil, one PDF), one pupil's statement, enrolment form, and the register of issued statements (reopen as issued).
- `/encarregados/:id` (linked from the pupil's guardians tab): contacts, children with this year's class and balances, family totals, per-child payment and **family payment** (one amount split between the children, one receipt each, shared reference; `POST /payments/group`).
- The bank method label is now just "Banco" (the account carries the bank name).

## Milestone 7b-2 — done

- `/cobrancas` (`src/pages/Charges.tsx`): total overdue and aging buckets (click a bucket to filter), "Por aluno" calling list (payer and phones with `tel:` links, overdue, outstanding, oldest due and days, record payment) and "Por cobrança" (status, type, month, class, days; paged); Excel export of the calling list; "Calcular multas" for `settings.manage`.
- Cancel a fee / waive a penalty with a reason (`src/features/charges/ChargeActionDialog.tsx`), from the charges list and from the pupil's account tab.
- `/descontos`: pending / approved / recused requests with what approval would grant; approve (optional note) or recuse (reason required); "Pedir desconto" dialog (pupil, type with its default, % or MT per month, from month, reason). Types without approval apply at once. The pupil profile has "Pedir desconto" and shows pending/approved discounts above the account.
- API: `GET /charges/arrears` (`charges.read`) — overdue debt per pupil with payer contacts, filters year, class, min days, search; `?format=xlsx`. Reception can export the calling list without `reports.read`. OpenAPI 0.7.0.

## Parked (to agree with the client)

- Receiving accounts with more detail (bank name, instructions, shown on documents, order), payment reference (pupil number) on documents, a short payment notice (aviso de cobrança), proof-of-payment attachments and a "proofs to validate" queue, reminders by SMS/WhatsApp (preferred over email). New payment methods (POS card, cheque) need a code change; new accounts do not. Whether the school needs fiscal invoices.

## Milestone 7b-1 — done

- Enrolment wizard (`/matriculas/nova`, `src/pages/EnrolmentWizard.tsx`, steps in `src/features/enrolment/`): pupil (with duplicate hints that reuse an existing record), health (Yes/No + "which?", doctor; only with `health.read`), guardians (search existing so siblings share the guardian and payer; new ones inline; exactly one payer; contact edits update the guardian), pick-up and emergency contacts, class (seats left) + first billing month + optional services with live monthly/annual totals, one-off fees and consents, review with the charges confirming will create and the amount due now; confirm, or confirm and record payment; print the enrolment form.
- Every step saves on "Seguinte": the pupil exists after step 1, the draft after step 5; the URL carries pupil/enrolment and step, so a reload or a draft opened later resumes (drafts open on the review).
- `/matriculas`: drafts / confirmed / withdrawn with counts; continue or delete a draft (the pupil record stays).
- `/matriculas/renovacao`: pick last year's class (either school, so the last preschool class can move to the 1st grade), see who is already re-enrolled, pick the new class (seats left), confirm now or create drafts; per-pupil result.
- Pupil profile: "Continuar matrícula", "Matricular em <ano>", "Editar dados" (wizard steps 1–4 for that pupil), "Registar desistência" (date + reason).
- API fix: copying the fee catalogue to a new year now copies a class-scoped item **inactive** when none of its classes exist in the target year (it used to lose its scope and apply to every class, e.g. a second mandatory tuition). The response lists them in `inactive`.

## Hosting — done

- `.github/workflows/ci.yml` (pull requests, and first job of Deploy): gofmt, go vet, `go test -p 1 -race` against a Postgres service; web `npm ci` + `npm run build` (includes the type-check).
- `.github/workflows/deploy.yml` (push to main / manual): builds `ghcr.io/putodoho/nova-vida-sms-{api,web}:<sha>` and `:latest`, copies `deploy/docker-compose.vps.yml` + `Caddyfile` to `/opt/nova-vida`, pulls, restarts, seeds (idempotent) and checks `/api/healthz`.
- On the VPS the web container listens only on `127.0.0.1:8088`; the existing Nginx terminates TLS (certbot) for `schoolsys.orisystems.site`. Demo runs with `APP_ENV=demo` and `FAKE_TODAY=2027-05-12`.

## Decisions taken during build (differences from the design doc)

- **No sqlc / goose**: queries are hand-written with pgx in each module; migrations use a ~60-line embedded runner. Fewer tools to install; same safety.
- Preschool receipt/statement prefixes are `REC-JI` / `EXT-JI` so the two companies' documents never look alike.
- Preschool fee VAT starts disabled until confirmed; primary monthly fees have 5% VAT included (flat method, as in the current statement).
- Primary one-off fees (enrolment, uniform, books) are not seeded: prices unknown.
- Pupil records are created before the enrolment (wizard step 1), so an abandoned draft leaves a pupil without enrolment; that is harmless and lets the receptionist resume later.
- A partly paid charge past its due date shows as **overdue** (the money is late); `partial` is only for charges not yet due.
- Billing starts at the enrolment's `start_month` chosen by the receptionist; months before it are never charged.
- Bulk renewal with `confirm: true` records the fees consent on behalf of the guardian (the paper renewal form is the evidence).
- The VAT report lists VAT as charged and discounts separately; whether discounts reduce the VAT base is for the accountant to confirm.
- Penalty rules apply only to monthly fees ("5% sobre o valor mensal em atraso"), never to one-off items.
- Web types are hand-written in `web/src/api/types.ts` (the OpenAPI responses are not fully typed yet).
- Class changes during the year are not supported yet (would need tuition re-pricing); withdraw and re-enrol, or adjust services.

## Notes for whoever continues

- `go.sum` is committed; the Dockerfile requires it. After adding a dependency, run `go mod tidy` on a machine with normal internet access and commit `go.sum`.
- The build sandbox uses a git-ignored `go.work` that maps `golang.org/x/*` and `gopkg.in/*` to their GitHub mirrors; not needed on a normal machine.

## Open questions for the client

See section 11 of `docs/technical-design.md`.
