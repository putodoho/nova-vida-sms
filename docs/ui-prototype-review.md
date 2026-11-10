# UI Prototype Review

Prototype: `docs/prototype/index.html` (static HTML/CSS/JS, no dependencies). Reviewed 2026-11-10 against `docs/ui-brief.md`, by rendering every screen at 1440 px and 390 px.

**Verdict:** approved as the visual and structural reference for the React app. All P1 screens, the enrolment and payment flows, and the three printables are present; no JavaScript errors. The HTML is not ported: the app is rebuilt in React + Tailwind + shadcn/ui using the tokens below. The items in sections 2–4 are fixed during implementation.

## 1. Design tokens to carry over

| Token | Value | Use |
| --- | --- | --- |
| navy | `#173b67` / `#102d50` | Sidebar, headings |
| blue (primary) | `#2463a6`, tint `#eaf2fb` | Buttons, links, active tab |
| yellow (accent) | `#f4c542` | Highlights, preschool tag, chart series |
| ink / muted | `#1c2a3a` / `#6b7888` | Text |
| line / bg | `#e4e9ef` / `#f5f7fa` | Borders, page background |
| green | `#15845a` on `#e7f6ef` | Pago, Em dia, Activo |
| red | `#bf3d45` on `#fff0f0` | Em atraso, debt amounts |
| amber | `#a96d08` on `#fff5df` | Por vencer, Parcial, Pendente |
| purple | `#6d55a8` on `#f1edfa` | e-Mola in charts |
| radius / shadow | `13px` / `0 8px 28px rgba(20,42,68,.08)` | Cards |
| font | Inter, system fallback | All UI; Arial on printables |

## 2. Domain logic (the prototype is inconsistent; the implementation follows the spec)

1. **"Saldo actual" means two things.** The account tab shows 7.865 MT (overdue only) while the rows also list future months unpaid. Show two figures everywhere: **Em dívida** (due and unpaid, incl. penalties) and **Total por pagar no ano** (everything still open). The header badge uses Em dívida.
2. **Penalty rules not applied consistently.** May is partial and overdue but has no penalty; June shows a penalty while "today" is in May. Penalties are computed by the backend per level rule, only after the due date.
3. **Allocation preview labels the penalty "Multa · Maio"** while the penalty sits on June. Preview lines must name the actual charge (month + item).
4. **Charges list mixes per-charge and account balances** (Saldo 7.865 on a 5.300 charge). In that table, Saldo is the charge's own remaining amount.
5. **Entity filter leaks.** With "Primária" selected, preschool pupils (Sofia Bila, Pré) and preschool prices appear in the dashboard, charges and fee catalogue. Every query is scoped to the selected entity.
6. **Fee catalogue:** primary enrolment/re-enrolment rows carry preschool values and "Todas" classes. Each entity has its own catalogue; class scope is a multi-select of real classes.
7. **Dashboard KPIs disagree with the tables** ("Alunos em dívida: 2" vs 4 overdue accounts; aging totals vs rows). Real data fixes this, but the KPI definitions must match the list filters.

## 3. Screens to adjust

- **Record payment:** pupil field is a native select; must be a searchable combobox (name, number, guardian phone) with balance in results. Reference required for BIM/M-Pesa/e-Mola (enforce, not just placeholder). Default date = today.
- **Annual statement printable:** add receipt nº column, VAT columns when VAT is enabled, one-off fees section, payment details (bank NIB, M-Pesa, e-Mola), statement number and issue date, signature line.
- **Settings → Companies:** bank account and mobile wallet are single fields; must be lists (one BIM account + M-Pesa + e-Mola, each with holder name).
- **Settings → VAT per fee item** shown as text ("Não incluído", "Configurar"); make it an editable control (off / included / added).
- **Date inputs** render as mm/dd/yyyy (native input). Use a dd/mm/yyyy date picker.
- **Pupils list:** add class and company filters, and photo when available.
- **Profile header:** health alert strip (allergies, medication, diet) not visible on the pupil shown; confirm it renders when data exists.

## 4. Responsive bugs

- At 390 px the page scrolls horizontally (dashboard tables are wider than the viewport). Tables become card lists or scroll inside their container.
- The entity switcher disappears on mobile; it must stay reachable (top bar or menu).
- The sidebar background stops at the viewport height on long pages.
