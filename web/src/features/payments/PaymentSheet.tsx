// "Registar pagamento": the most frequent action. A side sheet reachable from
// anywhere: pick pupil → see what is owed → amount, method, account, reference →
// live allocation preview → confirm → receipt.
import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, FileText, Printer, Search, X } from 'lucide-react'
import { toast } from 'sonner'
import { api, ApiError, openDocument } from '@/lib/api'
import { date, money, moneyInput, parseDate, parseMoney, todayISO } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Avatar, Badge, Button, Dialog, DialogContent, Field, Input, Select, Spinner, Textarea } from '@/components/ui'
import type { Account, EnrolmentListItem, List, Payment, PaymentPreview } from '@/api/types'

interface PupilRef { id: string; name: string; number?: string; className?: string | null }

const Ctx = React.createContext<{ open: (pupil?: PupilRef) => void } | null>(null)

export function usePaymentSheet() {
  const c = React.useContext(Ctx)
  if (!c) throw new Error('usePaymentSheet outside provider')
  return c
}

export function PaymentSheetProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = React.useState<{ open: boolean; pupil?: PupilRef; key: number }>({ open: false, key: 0 })
  const open = React.useCallback((pupil?: PupilRef) => setState((s) => ({ open: true, pupil, key: s.key + 1 })), [])
  return (
    <Ctx.Provider value={{ open }}>
      {children}
      <Dialog open={state.open} onOpenChange={(o) => setState((s) => ({ ...s, open: o }))}>
        {state.open && (
          <DialogContent side="right" title={pt.actions.recordPayment} description="O valor é aplicado às dívidas mais antigas primeiro.">
            <PaymentForm key={state.key} initialPupil={state.pupil} onDone={() => setState((s) => ({ ...s, open: false }))} onAnother={() => open()} />
          </DialogContent>
        )}
      </Dialog>
    </Ctx.Provider>
  )
}

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = React.useState(value)
  React.useEffect(() => {
    const t = setTimeout(() => setV(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return v
}

function PupilPicker({ onPick }: { onPick: (p: PupilRef) => void }) {
  const [q, setQ] = React.useState('')
  const dq = useDebounced(q, 250)
  const { year } = useSession()
  const { data, isFetching } = useQuery({
    queryKey: ['enrolments', 'picker', dq, year?.year],
    queryFn: () => api.get<List<EnrolmentListItem>>('/enrolments', { q: dq, year: year?.year, status: 'confirmed', page_size: 8 }),
    enabled: dq.trim().length >= 2,
  })
  return (
    <div className="space-y-2">
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted" />
        <Input autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder="Nome, número ou telefone do encarregado" className="pl-9" />
      </div>
      {isFetching && <div className="flex justify-center py-3"><Spinner /></div>}
      {data && data.items.length === 0 && <p className="py-3 text-center text-sm text-ink-muted">Nenhum aluno matriculado encontrado nesta empresa.</p>}
      <div className="space-y-1">
        {data?.items.map((p) => (
          <button
            key={p.enrolment_id}
            onClick={() => onPick({ id: p.pupil_id, name: p.full_name, number: p.number, className: p.class_name })}
            className="flex w-full items-center gap-3 rounded-lg border border-transparent px-3 py-2 text-left hover:border-line hover:bg-page"
          >
            <Avatar name={p.full_name} />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold text-navy">{p.full_name}</p>
              <p className="truncate text-xs text-ink-muted">{p.number} · {p.class_name ?? '—'}{p.payer_phone && ` · ${p.payer_phone}`}</p>
            </div>
            {p.overdue !== undefined && (p.overdue > 0 ? <Badge tone="bad">{money(p.overdue)}</Badge> : <Badge tone="ok">{pt.account.upToDate}</Badge>)}
          </button>
        ))}
      </div>
    </div>
  )
}

const methods = ['cash', 'mpesa', 'emola', 'bank'] as const

function PaymentForm({ initialPupil, onDone, onAnother }: { initialPupil?: PupilRef; onDone: () => void; onAnother: () => void }) {
  const { entity, me } = useSession()
  const qc = useQueryClient()
  const [pupil, setPupil] = React.useState<PupilRef | undefined>(initialPupil)
  const [amountText, setAmountText] = React.useState('')
  const [dateText, setDateText] = React.useState(date(me.today || todayISO()))
  const [method, setMethod] = React.useState<(typeof methods)[number]>('cash')
  const [accountId, setAccountId] = React.useState('')
  const [reference, setReference] = React.useState('')
  const [notes, setNotes] = React.useState('')
  const [errors, setErrors] = React.useState<Record<string, string>>({})
  const [done, setDone] = React.useState<Payment | null>(null)

  const amount = parseMoney(amountText)
  const debouncedAmount = useDebounced(amount ?? 0, 300)

  const accounts = useQuery({
    queryKey: ['accounts', entity?.id],
    queryFn: () => api.get<List<Account>>(`/entities/${entity!.id}/accounts`),
    enabled: !!entity,
  })
  const methodAccounts = (accounts.data?.items ?? []).filter((a) => a.active && a.kind === method)
  React.useEffect(() => {
    setAccountId(methodAccounts.length === 1 ? methodAccounts[0].id : '')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [method, accounts.data])

  const preview = useQuery({
    queryKey: ['payment-preview', pupil?.id, debouncedAmount],
    queryFn: () => api.post<PaymentPreview>('/payments/preview', { pupil_id: pupil!.id, amount: debouncedAmount }),
    enabled: !!pupil,
    placeholderData: (prev) => prev,
  })

  const create = useMutation({
    mutationFn: () => {
      const paidOn = parseDate(dateText)
      const fieldErrors: Record<string, string> = {}
      if (amount === null || amount <= 0) fieldErrors.amount = 'Indique um valor válido.'
      if (!paidOn) fieldErrors.paid_on = 'Data no formato dd/mm/aaaa.'
      if (method !== 'cash' && !reference.trim()) fieldErrors.reference = 'Indique a referência da transacção.'
      if (!accountId) fieldErrors.receiving_account_id = 'Escolha a conta que recebeu.'
      if (Object.keys(fieldErrors).length) throw new ApiError(422, 'validation_failed', 'Dados inválidos.', fieldErrors)
      return api.post<Payment>('/payments', {
        pupil_id: pupil!.id, amount, paid_on: paidOn, method, receiving_account_id: accountId, reference: reference.trim(), notes: notes.trim(),
      })
    },
    onSuccess: (p) => {
      setDone(p)
      setErrors({})
      void qc.invalidateQueries()
      toast.success(`Pagamento registado · recibo ${p.receipt_no}`)
    },
    onError: (e) => {
      if (e instanceof ApiError) {
        setErrors(e.fields)
        if (!Object.keys(e.fields).length) toast.error(e.message)
      } else toast.error(pt.errors.generic)
    },
  })

  if (!entity) {
    return <p className="p-5 text-sm text-ink-muted">Seleccione uma empresa no topo para registar pagamentos.</p>
  }

  if (done) {
    return (
      <div className="space-y-5 p-5">
        <div className="rounded-card border border-ok/20 bg-ok-tint p-5 text-center">
          <CheckCircle2 className="mx-auto h-10 w-10 text-ok" />
          <p className="mt-2 text-lg font-bold text-navy">Pagamento registado</p>
          <p className="text-sm text-ink-muted">Recibo <span className="font-semibold text-navy">{done.receipt_no}</span> · {money(done.amount)}</p>
        </div>
        <dl className="grid grid-cols-2 gap-3 text-sm">
          <div className="rounded-lg bg-page p-3"><dt className="text-xs text-ink-muted">{pt.account.overdue}</dt><dd className="font-semibold text-navy">{money(done.pupil_overdue)}</dd></div>
          <div className="rounded-lg bg-page p-3"><dt className="text-xs text-ink-muted">{pt.account.outstanding}</dt><dd className="font-semibold text-navy">{money(done.pupil_outstanding)}</dd></div>
          {done.credit > 0 && <div className="col-span-2 rounded-lg bg-accent-tint p-3"><dt className="text-xs text-ink-muted">Crédito para pagamentos futuros</dt><dd className="font-semibold text-navy">{money(done.credit)}</dd></div>}
        </dl>
        <div className="grid gap-2 sm:grid-cols-2">
          <Button onClick={() => openDocument(`/payments/${done.id}/receipt`).catch((e) => toast.error(e.message))}><FileText className="h-4 w-4" /> Ver recibo (PDF)</Button>
          <Button variant="secondary" onClick={() => openDocument(`/payments/${done.id}/receipt`, 'html').catch((e) => toast.error(e.message))}><Printer className="h-4 w-4" /> Imprimir</Button>
          <Button variant="secondary" onClick={onAnother}>Registar outro</Button>
          <Button variant="ghost" onClick={onDone}>{pt.actions.close}</Button>
        </div>
      </div>
    )
  }

  const pv = preview.data
  return (
    <form
      className="space-y-5 p-5"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate()
      }}
    >
      <section>
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-muted">1. Aluno</p>
        {pupil ? (
          <div className="flex items-center gap-3 rounded-lg border border-line p-3">
            <Avatar name={pupil.name} />
            <div className="min-w-0 flex-1">
              <p className="truncate font-semibold text-navy">{pupil.name}</p>
              <p className="text-xs text-ink-muted">{[pupil.number, pupil.className].filter(Boolean).join(' · ')}</p>
            </div>
            <Button type="button" variant="ghost" size="icon" onClick={() => setPupil(undefined)} aria-label="Trocar aluno"><X className="h-4 w-4" /></Button>
          </div>
        ) : (
          <PupilPicker onPick={setPupil} />
        )}
        {errors.pupil_id && <p className="mt-1 text-xs text-bad">{errors.pupil_id}</p>}
      </section>

      {pupil && (
        <>
          <section className="grid grid-cols-2 gap-3">
            <button type="button" onClick={() => pv && setAmountText(moneyInput(pv.overdue))} className="rounded-lg border border-line p-3 text-left hover:border-brand/40">
              <p className="text-xs text-ink-muted">{pt.account.overdue}</p>
              <p className={cn('text-lg font-bold tabular-nums', (pv?.overdue ?? 0) > 0 ? 'text-bad' : 'text-ok')}>{pv ? money(pv.overdue) : '…'}</p>
              {(pv?.overdue ?? 0) > 0 && <p className="text-[11px] text-brand">Usar este valor</p>}
            </button>
            <button type="button" onClick={() => pv && setAmountText(moneyInput(pv.outstanding))} className="rounded-lg border border-line p-3 text-left hover:border-brand/40">
              <p className="text-xs text-ink-muted">{pt.account.outstanding}</p>
              <p className="text-lg font-bold tabular-nums text-navy">{pv ? money(pv.outstanding) : '…'}</p>
              {(pv?.outstanding ?? 0) > 0 && <p className="text-[11px] text-brand">Pagar o ano todo</p>}
            </button>
          </section>

          <section className="space-y-4">
            <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">2. Pagamento</p>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Valor recebido (MT)" error={errors.amount} htmlFor="amount">
                <Input id="amount" inputMode="decimal" autoFocus value={amountText} onChange={(e) => setAmountText(e.target.value)}
                  onBlur={() => amount !== null && setAmountText(moneyInput(amount))} placeholder="0,00" invalid={!!errors.amount} className="text-lg font-semibold tabular-nums" />
              </Field>
              <Field label="Data do pagamento" error={errors.paid_on} htmlFor="paid_on">
                <Input id="paid_on" value={dateText} onChange={(e) => setDateText(e.target.value)} placeholder="dd/mm/aaaa" invalid={!!errors.paid_on} />
              </Field>
            </div>
            <Field label="Método" error={errors.method}>
              <div className="grid grid-cols-4 gap-2">
                {methods.map((m) => (
                  <button key={m} type="button" onClick={() => setMethod(m)}
                    className={cn('h-10 rounded-lg border text-sm font-medium', method === m ? 'border-brand bg-brand-tint text-brand' : 'border-line bg-white text-ink hover:bg-page')}>
                    {pt.methods[m]}
                  </button>
                ))}
              </div>
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Conta que recebeu" error={errors.receiving_account_id}>
                <Select value={accountId} onChange={(e) => setAccountId(e.target.value)} invalid={!!errors.receiving_account_id}>
                  <option value="">{methodAccounts.length ? 'Escolha…' : 'Sem contas deste tipo'}</option>
                  {methodAccounts.map((a) => <option key={a.id} value={a.id}>{a.label}{a.number && ` · ${a.number}`}</option>)}
                </Select>
              </Field>
              <Field label={method === 'cash' ? 'Referência (opcional)' : 'Referência da transacção'} error={errors.reference} htmlFor="reference">
                <Input id="reference" value={reference} onChange={(e) => setReference(e.target.value)} placeholder={method === 'mpesa' ? 'Ex.: QK7H2X9' : ''} invalid={!!errors.reference} />
              </Field>
            </div>
            <Field label="Observações (opcional)">
              <Textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} />
            </Field>
          </section>

          <section>
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-muted">3. O que este pagamento liquida</p>
            <div className="rounded-lg border border-line">
              {(amount ?? 0) <= 0 ? (
                <p className="px-4 py-4 text-sm text-ink-muted">Indique o valor para ver a que cobranças é aplicado.</p>
              ) : (
                <ul className="divide-y divide-line text-sm">
                  {pv?.allocations.map((a) => (
                    <li key={a.charge_id} className="flex items-center justify-between gap-3 px-4 py-2.5">
                      <div className="min-w-0">
                        <p className="truncate text-ink">{a.description}</p>
                        <p className="text-xs text-ink-muted">Vence {date(a.due_date)}{a.amount < a.remaining && ` · fica a faltar ${money(a.remaining - a.amount)}`}</p>
                      </div>
                      <span className="font-semibold tabular-nums text-navy">{money(a.amount)}</span>
                    </li>
                  ))}
                  {pv && pv.credit > 0 && (
                    <li className="flex items-center justify-between bg-accent-tint px-4 py-2.5">
                      <span>Crédito para pagamentos futuros</span>
                      <span className="font-semibold tabular-nums">{money(pv.credit)}</span>
                    </li>
                  )}
                </ul>
              )}
            </div>
          </section>

          <div className="sticky bottom-0 -mx-5 -mb-5 border-t border-line bg-white px-5 py-4">
            <Button type="submit" className="w-full" loading={create.isPending} disabled={!amount}>
              Confirmar {amount ? money(amount) : ''}
            </Button>
          </div>
        </>
      )}
    </form>
  )
}
