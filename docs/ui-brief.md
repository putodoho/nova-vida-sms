# Nova Vida SMS — UI/UX Design Brief

This brief is the input for designing the user interface of the Nova Vida School Management System. Design from it, not from a generic "school ERP" template: the screens, fields and rules below come from the school's real documents and answers.

---

## 1. Product in one paragraph

A web app used by the staff of a small private school in Xai-Xai, Mozambique, to enrol pupils, bill monthly fees, record payments, issue receipts and annual statements, and see who owes what. The school runs two separate companies on one platform: a **preschool** (Jardim Infantil Nova Vida — rooms Sala 1, Sala 2, Sala 3, Pré) and a **primary school** (Escola Primária Nova Vida, SU, Lda. — 1.ª to 4.ª classe, 6 classes, ~116 pupils in 2027). Each company has its own receipts, statements, bank accounts and reports. Internet at the school is stable; this is a normal online web app.

## 2. Language, locale and formats

- **All UI copy in Portuguese (Mozambique).** Labels in this brief are given in English with the Portuguese term where it matters; the designs must show Portuguese.
- Currency: Metical, shown as `5.035,00 MT` (dot thousands, comma decimals, `MT` suffix).
- Dates: `05/02/2027` (dd/mm/yyyy). Months in Portuguese (Fevereiro, Março…).
- School year runs **February to November**: 10 monthly fees, due on the **5th** of each month.

## 3. Users and roles

Roles are configurable (admin can create roles and assign permissions), but design for these defaults:

| Role (PT) | Who | Uses the app for | Device |
| --- | --- | --- | --- |
| Administrador | Finance & admin owner | Everything: settings, prices, discounts approval, reports, users | Desktop |
| Director | School director | Dashboard, reports, approvals, read pupil records | Desktop + phone |
| Recepção / Secretaria | Receptionist | Enrolments, pupil records, recording payments, printing receipts and statements | Desktop (main daily user) |
| Funcionário | Other staff (teachers, supervisors) given limited roles | Class lists, guardian contacts, authorised pick-up list | Phone / tablet |

The receptionist is the heaviest user: optimise for fast keyboard data entry and quick search. The director mostly reads: optimise for a clear dashboard that also works on a phone.

## 4. Brand and visual direction

- Existing identity: **blue and yellow**, an open book with a pencil, sun rays. School motto: "Educando para a Eternidade" (primary) and "Educar nossas crianças para uma Vida Nova" (preschool). Logo files will be supplied.
- Tone: trustworthy, calm, clean — it handles money. Friendly but not childish; the users are adults.
- Blue as primary, yellow as accent only (highlights, totals, key badges); avoid large yellow areas with white text (contrast).
- Must meet WCAG AA contrast. Light mode is required; dark mode is optional.
- Implementation target: **React + Vite + Tailwind CSS + shadcn/ui (Radix)**. Design with components that map to that set (Table, Dialog, Sheet, Tabs, Select, Combobox, Command palette, Badge, Card, Form fields, Toast, Stepper).

## 5. Navigation

Left sidebar (collapsible), top bar with:
- **Entity switcher** (Preschool / Primary / All) — always visible, because every list, report and document belongs to one company. Make the current entity obvious (name + colour tag).
- **School year switcher** (2027, 2028…).
- Global search (Ctrl+K): pupils, guardians, receipts by number.
- Quick action button: **"Registar pagamento"** (Record payment) — reachable from anywhere.
- User menu.

Sidebar items (shown according to permissions):
1. Painel (Dashboard)
2. Alunos (Pupils)
3. Encarregados (Guardians)
4. Matrículas (Enrolments)
5. Pagamentos (Payments)
6. Cobranças e dívidas (Charges & arrears)
7. Descontos (Discounts — approvals)
8. Documentos (Receipts & statements)
9. Turmas (Classes)
10. Relatórios (Reports)
11. Funcionários e utilizadores (Staff & users)
12. Configurações (Settings)
13. Auditoria (Audit log)

## 6. Key domain rules the UI must express

- **Fee catalogue per company, per year, per class.** Examples (2027): Primary 1.ª–3.ª monthly fee 2.800 MT, 4.ª 3.000 MT; after-school (Extraclasse) 1.000 MT; lunch (Almoço) 1.500 MT; Preschool Sala 1 3.300 MT, Sala 2/3/Pré 3.000 MT; one-off: enrolment 3.000, re-enrolment 2.000, uniform 1.000/1.200, books 600–850 MT. All editable in Settings.
- **Contracted services per pupil:** monthly fee is mandatory; after-school and lunch are optional. The pupil's monthly total depends on what is contracted.
- **Charges:** the system generates 10 monthly charges + one-off charges per pupil. Each charge has a status: `Por vencer` (upcoming), `Pago` (paid), `Parcial` (partial), `Em atraso` (overdue), `Anulado` (waived/cancelled).
- **Penalties (Multas):** rule per level — primary 5% of overdue amount; preschool fixed 100 MT after the 5th, 200 MT after the 10th. Penalties can be waived only with a reason and permission.
- **Discounts:** types (social case, staff child, siblings), percentage or fixed, need approval. Show who requested, who approved, why.
- **VAT 5%:** configurable per company and fee item (included in price or added on top). Show VAT lines on statements and receipts when enabled.
- **Payments** are made to the pupil's account, not to one month. A payment pays the oldest open items first (penalties, then fees). Partial and advance payments are normal. Methods: Numerário (cash), Millennium BIM (transfer/deposit), M-Pesa, e-Mola — each with a reference number.
- **Guardians:** one pupil can have several (mother, father, other guardian), each with flags: *responsible for payments*, *receives notices*, *authorised to pick up*. One guardian can have several children, possibly in both companies.

## 7. Screens

Priority **P1** = needed for launch; **P2** = soon after. For each screen, design the default state plus empty, loading and error states where noted.

### 7.1 Login (P1)
Email/phone + password, "forgot password". School logo. Simple.

### 7.2 Dashboard — Painel (P1)
For admin and director. Filtered by entity and year.
- KPI cards: expected this month, received this month, collection rate %, total overdue, number of pupils with overdue payments, active pupils.
- Chart: expected vs received per month (Feb–Nov).
- Payments by method this month (cash, BIM, M-Pesa, e-Mola).
- Table: top overdue accounts (pupil, class, amount overdue, days overdue, guardian phone) with "view" and "record payment" actions.
- Recent payments list.
- Pending approvals (discounts, penalty waivers) count with link.
- Must work well on a phone (director): cards stack, chart simplifies.

### 7.3 Pupils list — Alunos (P1)
Data table: photo/initials, name, number, class/room, company, status (Active, Pending enrolment, Left), balance (badge: Em dia / Em atraso + amount), main guardian + phone.
Filters: company, level, class, status, debt status. Search. Export (Excel). Bulk: print statements for selection.
Action: "Nova matrícula".

### 7.4 Pupil profile (P1) — the most important screen
Header: photo, full name, pupil number, class, company tag, age, status, **current balance** (large, coloured), quick actions: Record payment, Print statement, Edit, Re-enrol.
Alert strip when relevant: allergies / medication / dietary restriction (must be impossible to miss), orphan status, pick-up restrictions.
Tabs:
1. **Resumo** — personal data, class, contracted services and monthly total, main guardian.
2. **Conta corrente** (Account) — the monthly table modelled on the school's statement: Nº, Month, Due date, Amount due, Penalty, Discount, Paid, Balance, Receipt nº, Status badge. Plus a one-off fees section (enrolment, uniform, books). Totals row. Actions per row: waive penalty, apply discount (both permission-gated).
3. **Pagamentos** — list of payments with method, reference, receipt, received by; click opens receipt.
4. **Encarregados** — guardians with role flags, phones, neighbourhood; authorised pick-up persons; emergency contacts (ordered).
5. **Saúde** — chronic conditions, allergies, diet restrictions, medication, doctor + contact, emergency medical consent (yes/no + date).
6. **Documentos** — generated statements, receipts, signed enrolment form (upload).
7. **Histórico** — enrolments per year, class changes, audit of key changes.

### 7.5 Enrolment wizard — Nova matrícula (P1)
Stepper, can save as draft. Steps:
1. **Aluno**: name, sex, date of birth (age auto), neighbourhood, religion (optional), orphan (No / Father / Mother / Both), new or returning pupil, photo.
2. **Saúde**: chronic condition, allergy, diet restriction, medication (each Yes/No + "which?"), extra notes, discipline used at home (preschool form asks this).
3. **Encarregados**: search existing guardian (siblings!) or add new. Mother, father, other guardian. Per guardian: name, 2 phones, neighbourhood, relationship. Flags: responsible for payments (exactly one required), receives notices, authorised to pick up. If parents separated: lives with whom.
4. **Recolha e emergência**: other persons authorised to pick up (name, ID/phone), up to 3 emergency contacts (name, relationship, phone), child's doctor.
5. **Turma e serviços**: company (auto from level), level, class/room with **seats left** shown (max 20), services: monthly fee (fixed), after-school, lunch (toggles) → live **monthly total** and **annual total** preview, discount request (type + reason).
6. **Taxas e consentimentos**: one-off fees to charge now (enrolment/re-enrolment, uniform + size, books) with total; consents checkboxes: image use on social media, agrees to pay the 10 monthly fees, uniform and books required, emergency medical treatment.
7. **Rever e confirmar**: summary of everything, total due now, buttons: Confirm enrolment, Confirm and record payment, Print enrolment form.

### 7.6 Re-enrolment — Renovação (P1)
List of last year's pupils for a class → select many → choose next class, keep services → confirm. Shows who already re-enrolled. Can open each pupil to change services.

### 7.7 Guardian profile — Encarregado (P1)
Contacts, children (in both companies), **combined balance** across children, payments made, "Record payment" for the family (allocate across children).

### 7.8 Record payment — Registar pagamento (P1) — the most frequent action
Modal or side sheet, keyboard friendly:
1. Find pupil or guardian (combobox with balance shown in results).
2. Shows open items (oldest first) with amounts.
3. Enter amount received, date (default today), method, receiving account (e.g. BIM account, M-Pesa number), reference/transaction ID, notes.
4. **Allocation preview**: which items this payment settles, what remains, or credit left over.
5. Confirm → success screen with receipt number, buttons: Print receipt, Download PDF, Record another.
Also: cancel a payment (admin only, with reason; creates a reversal, never deletes).

### 7.9 Payments list — Pagamentos (P1)
Table: date, receipt nº, pupil, guardian, amount, method, account, reference, received by, status (Valid / Cancelled). Filters: date range, method, account, user, company. Totals at the bottom. Export. Used for reconciling M-Pesa / e-Mola / bank statements.

### 7.10 Charges & arrears — Cobranças e dívidas (P1)
Table of charges by month: pupil, class, item, due date, amount, paid, balance, days overdue, penalty, status. Filters: month, class, status, overdue more than N days. Bulk: print statements, export list (for calling guardians — the school uses phone calls and SMS). Aging summary: 1–30, 31–60, 61–90, 90+ days.

### 7.11 Discounts & waivers — Descontos (P1)
Queue of requests: pupil, type, value, months/items, requested by, reason → Approve / Reject (with comment). History tab.

### 7.12 Documents — Documentos (P1)
- Receipts list (search by number).
- Annual statement generation: single pupil or bulk per class → PDF.
- Design the **printables** too (see section 8).

### 7.13 Classes — Turmas (P1)
Cards or table: class name, level, company, teacher, pupils / capacity (e.g. 18/20 bar), room. Class detail: pupil list with balance badges (visible only to finance roles), guardian phones, allergies icons, export/print class list.

### 7.14 Reports — Relatórios (P1)
Each report: filters (company, year, period), on-screen table/chart, export Excel + PDF.
- Income by month and by payment method/account
- Expected vs received
- Arrears and aging
- Pupils per class / level, new vs returning
- Discounts granted
- Penalties charged and waived
- VAT summary

### 7.15 Staff & users — Funcionários e utilizadores (P1)
Staff list (name, function, phone, has system access?). Create user from staff, assign role(s), activate/deactivate, reset password. **Roles editor**: roles as columns, permissions grouped by module as rows, checkboxes.

### 7.16 Settings — Configurações (P1)
Sub-navigation:
- **Empresas** (Companies): name, NUIT, address, logo, motto, bank accounts, mobile wallets, receipt and statement numbering series.
- **Anos lectivos**: year, start/end, billing months, due day.
- **Níveis e turmas**: levels, classes/rooms, capacity, teacher.
- **Tabela de preços** (Fee catalogue): per company + year; items with type (monthly / one-off), class(es), amount, VAT setting; copy from previous year.
- **Multas** (Penalty rules) per level: percentage or fixed tiers by day.
- **Descontos** (Discount types): name, % or fixed, default value, who can approve.
- **IVA**: rate, included vs added, which items.
- **Métodos de pagamento e contas**.

### 7.17 Audit log — Auditoria (P2)
Who, what, when, before/after. Filter by user, entity, record.

### 7.18 Later (P2+, design only if time allows)
Attendance, gate/pick-up log (check authorised persons), SMS reminders, guardian portal, grades and report cards.

## 8. Printable documents (P1)

Design for A4 print and PDF, black-and-white friendly, with company header (logo, name, address, NUIT) and footer motto:
1. **Receipt (Recibo)** — A5 or half A4: number, date, pupil, guardian, items settled (month + service), amount, method + reference, received by, balance after payment.
2. **Annual statement (Extrato anual)** — based on the school's current template: pupil and guardian data, contracted services, annual summary per service (monthly, VAT, net, annual), one-off fees, monthly control table (due, paid date, penalty, discount, paid, balance, receipt nº), totals, payment details (bank, M-Pesa, e-Mola), signature line.
3. **Enrolment form (Ficha de matrícula)** — printable version of the wizard data with consents and signature lines.

## 9. Components to define

Design tokens (colour, type scale, spacing, radius, shadow), then:
- App shell: sidebar, top bar, entity + year switchers, breadcrumbs
- Data table: sorting, filters bar, column chooser, pagination, row actions, bulk selection, totals row, sticky header
- Money display and **money input** (MT formatting while typing)
- Status badges: Por vencer, Pago, Parcial, Em atraso, Anulado; Em dia / Em atraso for accounts
- Company tag (Preschool vs Primary — distinct but not loud)
- Health alert strip
- Stepper (wizard), form sections, inline validation
- Combobox search with rich results (photo, class, balance)
- KPI card, small charts (bar, line)
- Dialogs: confirm, destructive confirm with reason field
- Toasts, empty states (with action), skeleton loaders, permission-denied state

## 10. Sample data for realistic mockups

Use invented names. Example pupil: *Lúcia Mabunda*, 2.ª classe, Primary, services: fee + after-school + lunch = 5.300,00 MT/month; paid Feb–Apr, May partial 3.000,00 MT, June overdue 12 days with 5% penalty 265,00 MT; guardian *Sónia Mabunda* (mother, payer), phone 84 000 0000. Example preschool pupil: *Tiago Cossa*, Sala 2, 3.000,00 MT/month, sibling of a primary pupil.

## 11. What we need back

1. Design tokens + component library (light mode).
2. All **P1** screens at desktop width (1440 px); dashboard, pupil profile and record payment also at mobile width (390 px).
3. The three printables.
4. A clickable flow for: enrol a new pupil → record first payment → print receipt.

Preferred format: **Figma file** (the developer can read Figma directly). Acceptable: React + Tailwind + shadcn/ui code, or HTML. Name screens and components in English; UI copy in Portuguese.
