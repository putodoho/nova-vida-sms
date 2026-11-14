import * as React from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowLeft, CheckCircle2, FileText, Mail, MapPin, Phone, Users, Wallet } from 'lucide-react'
import { api, openDocument } from '@/lib/api'
import { date, money, moneyInput, parseDate, parseMoney } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Avatar, Badge, Button, Card, Dialog, DialogContent, ErrorState, Field, Input, Select, Skeleton, Textarea } from '@/components/ui'
import { usePaymentSheet } from '@/features/payments/PaymentSheet'
import { type Errors, handleError } from '@/features/enrolment/shared'
import type { Account, GuardianDetail, List, Payment, PupilAccount, PupilProfile } from '@/api/types'

interface Child { id: string; name: string; number: string; relationship: string; isPayer: boolean; className: string | null; entityId: string | null; account?: PupilAccount['summary'] }

/** Guardian profile (brief 7.7): contacts, children, the family balance and one payment split between the children. */
export function GuardianProfilePage() {
  const { id = '' } = useParams()
  const { entity, year, can } = useSession()
  const payment = usePaymentSheet()
  const [family, setFamily] = React.useState(false)
  const g = useQuery({ queryKey: ['guardian', id], queryFn: () => api.get<GuardianDetail>(`/guardians/${id}`) })
  const kids = g.data?.children ?? []
  const profiles = useQueries({
    queries: kids.map((c) => ({ queryKey: ['pupil', c.pupil_id], queryFn: () => api.get<PupilProfile>(`/pupils/${c.pupil_id}`) })),
  })
  const accounts = useQueries({
    queries: kids.map((c) => ({
      queryKey: ['account', c.pupil_id, entity?.id ?? 'all', year?.year],
      queryFn: () => api.get<PupilAccount>(`/pupils/${c.pupil_id}/account`, { year: year?.year }),
      enabled: can('charges.read') && !!year,
    })),
  })

  if (g.isError) return <ErrorState error={g.error} onRetry={() => g.refetch()} />
  if (!g.data) return <div className="space-y-4"><Skeleton className="h-32" /><Skeleton className="h-64" /></div>
  const d = g.data

  const children: Child[] = kids.map((c, i) => {
    const prof = profiles[i]?.data
    const enr = prof?.enrolments.find((e) => e.year === year?.year && e.status === 'confirmed' && (!entity || e.entity_id === entity.id))
    return {
      id: c.pupil_id, name: c.full_name, number: c.number, relationship: c.relationship, isPayer: c.is_payer,
      className: enr?.class_name ?? null, entityId: enr?.entity_id ?? null, account: enr ? accounts[i]?.data?.summary : undefined,
    }
  })
  const enrolled = children.filter((c) => c.entityId)
  const overdue = enrolled.reduce((s, c) => s + (c.account?.overdue ?? 0), 0)
  const outstanding = enrolled.reduce((s, c) => s + (c.account?.outstanding ?? 0), 0)
  const payable = enrolled.filter((c) => c.isPayer && entity && c.entityId === entity.id)
  const canPay = can('payments.create') && !!entity

  return (
    <>
      <Link to="/alunos" className="mb-3 inline-flex items-center gap-1 text-sm font-medium text-ink-muted hover:text-navy"><ArrowLeft className="h-4 w-4" /> {pt.nav.pupils}</Link>
      <Card className="p-5">
        <div className="flex flex-col gap-4 md:flex-row md:items-start">
          <div className="flex min-w-0 flex-1 items-start gap-4">
            <Avatar name={d.full_name} className="h-14 w-14 text-lg" />
            <div className="min-w-0">
              <h1 className="text-[22px] font-bold text-navy">{d.full_name}</h1>
              <p className="text-sm text-ink-muted">Encarregado de {kids.length} {kids.length === 1 ? 'aluno' : 'alunos'}</p>
              <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
                {[d.phone1, d.phone2].filter(Boolean).map((ph) => (
                  <a key={ph} href={`tel:${ph.replace(/\s/g, '')}`} className="inline-flex items-center gap-1.5 text-brand"><Phone className="h-4 w-4" /> {ph}</a>
                ))}
                {d.email && <a href={`mailto:${d.email}`} className="inline-flex items-center gap-1.5 text-brand"><Mail className="h-4 w-4" /> {d.email}</a>}
                {d.neighbourhood && <span className="inline-flex items-center gap-1.5 text-ink-muted"><MapPin className="h-4 w-4" /> {d.neighbourhood}</span>}
              </div>
            </div>
          </div>
          {can('charges.read') && (
            <div className="grid grid-cols-2 gap-2 md:w-80">
              <div className="rounded-lg bg-bad-tint p-3"><p className="text-xs text-ink-muted">{pt.account.overdue}</p><p className="text-lg font-bold tabular-nums text-bad">{money(overdue)}</p></div>
              <div className="rounded-lg bg-page p-3"><p className="text-xs text-ink-muted">{pt.account.outstanding}</p><p className="text-lg font-bold tabular-nums text-navy">{money(outstanding)}</p></div>
            </div>
          )}
        </div>
        {canPay && payable.length >= 2 && (
          <div className="mt-4 flex justify-end border-t border-line pt-4">
            <Button variant="accent" onClick={() => setFamily(true)}><Users className="h-4 w-4" /> Pagamento da família</Button>
          </div>
        )}
      </Card>

      <p className="mb-3 mt-6 text-sm font-semibold text-navy">Educandos · {year?.year}</p>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {children.map((c) => (
          <Card key={c.id} className="p-4">
            <div className="flex items-start gap-3">
              <Avatar name={c.name} />
              <div className="min-w-0 flex-1">
                <Link to={`/alunos/${c.id}`} className="block truncate font-semibold text-navy hover:underline">{c.name}</Link>
                <p className="text-xs text-ink-muted">{c.number} · {c.className ?? 'Sem matrícula neste ano'}</p>
                <div className="mt-1 flex flex-wrap gap-1">
                  <Badge>{pt.relationship[c.relationship]}</Badge>
                  {c.isPayer ? <Badge tone="accent">Paga</Badge> : <Badge tone="neutral">Outro encarregado paga</Badge>}
                </div>
              </div>
            </div>
            {c.account && (
              <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
                <div><p className="text-xs text-ink-muted">{pt.account.overdue}</p><p className={cn('font-semibold tabular-nums', c.account.overdue > 0 ? 'text-bad' : 'text-ok')}>{money(c.account.overdue)}</p></div>
                <div><p className="text-xs text-ink-muted">{pt.account.outstanding}</p><p className="font-semibold tabular-nums text-navy">{money(c.account.outstanding)}</p></div>
              </div>
            )}
            {canPay && c.entityId === entity?.id && (
              <Button size="sm" variant="secondary" className="mt-3 w-full justify-center" onClick={() => payment.open({ id: c.id, name: c.name, number: c.number, className: c.className })}>
                <Wallet className="h-4 w-4" /> {pt.actions.recordPayment}
              </Button>
            )}
          </Card>
        ))}
      </div>
      {family && <FamilyPayment guardian={d.full_name} items={payable} onClose={() => setFamily(false)} />}
    </>
  )
}

const methods = ['cash', 'bank', 'mpesa', 'emola'] as const

/** One amount received from the family, split between the children: one receipt each, same reference. */
function FamilyPayment({ guardian, items, onClose }: { guardian: string; items: Child[]; onClose: () => void }) {
  const { entity, me } = useSession()
  const qc = useQueryClient()
  const [amounts, setAmounts] = React.useState<Record<string, string>>(() =>
    Object.fromEntries(items.map((c) => [c.id, c.account && c.account.overdue > 0 ? moneyInput(c.account.overdue) : ''])),
  )
  const [dateText, setDateText] = React.useState(date(me.today))
  const [method, setMethod] = React.useState<(typeof methods)[number]>('mpesa')
  const [accountId, setAccountId] = React.useState('')
  const [reference, setReference] = React.useState('')
  const [notes, setNotes] = React.useState('')
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)
  const [done, setDone] = React.useState<Payment[] | null>(null)

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

  const splits = items.map((c) => ({ pupil_id: c.id, amount: parseMoney(amounts[c.id] ?? '') ?? 0 })).filter((s) => s.amount > 0)
  const total = splits.reduce((s, x) => s + x.amount, 0)

  async function submit() {
    const e: Errors = {}
    const paidOn = parseDate(dateText)
    if (splits.length < 2) e.splits = 'Indique o valor de pelo menos dois educandos (para um só, use o pagamento normal).'
    if (!paidOn) e.paid_on = 'Data no formato dd/mm/aaaa.'
    if (!accountId) e.receiving_account_id = 'Escolha a conta que recebeu.'
    if (method !== 'cash' && !reference.trim()) e.reference = 'Indique a referência da transacção.'
    if (Object.keys(e).length) return setErrors(e)
    setSaving(true)
    try {
      const r = await api.post<List<Payment>>('/payments/group', {
        paid_on: paidOn, method, receiving_account_id: accountId, reference: reference.trim(), notes: notes.trim(), splits,
      })
      setDone(r.items)
      toast.success(`${r.items.length} recibos emitidos.`)
      void qc.invalidateQueries()
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent side="right" title="Pagamento da família" description={guardian}>
        {done ? (
          <div className="space-y-4 p-5">
            <div className="rounded-card border border-ok/20 bg-ok-tint p-5 text-center">
              <CheckCircle2 className="mx-auto h-10 w-10 text-ok" />
              <p className="mt-2 text-lg font-bold text-navy">{money(done.reduce((s, p) => s + p.amount, 0))} registados</p>
              <p className="text-sm text-ink-muted">Um recibo por educando</p>
            </div>
            <ul className="divide-y divide-line rounded-lg border border-line">
              {done.map((p) => (
                <li key={p.id} className="flex items-center gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-navy">{p.pupil_name}</p>
                    <p className="text-xs text-ink-muted">{p.receipt_no} · {money(p.amount)}</p>
                  </div>
                  <Button size="sm" variant="secondary" onClick={() => openDocument(`/payments/${p.id}/receipt`).catch((x) => toast.error(x.message))}><FileText className="h-4 w-4" /> Recibo</Button>
                </li>
              ))}
            </ul>
            <Button variant="ghost" className="w-full justify-center" onClick={onClose}>{pt.actions.close}</Button>
          </div>
        ) : (
          <div className="space-y-5 p-5">
            <section>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-muted">1. Quanto para cada educando</p>
              <ul className="divide-y divide-line rounded-lg border border-line">
                {items.map((c) => (
                  <li key={c.id} className="flex items-center gap-3 px-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-navy">{c.name}</p>
                      <p className="text-xs text-ink-muted">{c.className} · em dívida {money(c.account?.overdue ?? 0)}</p>
                    </div>
                    <Input inputMode="decimal" aria-label={`Valor para ${c.name}`} value={amounts[c.id] ?? ''} placeholder="0,00" className="w-32 text-right tabular-nums"
                      onChange={(e) => { setAmounts((a) => ({ ...a, [c.id]: e.target.value })); setErrors({}) }}
                      onBlur={() => { const v = parseMoney(amounts[c.id] ?? ''); if (v !== null) setAmounts((a) => ({ ...a, [c.id]: moneyInput(v) })) }} />
                  </li>
                ))}
                <li className="flex items-center justify-between bg-page/60 px-3 py-2.5">
                  <span className="text-sm font-semibold text-navy">Total recebido</span>
                  <span className="text-lg font-bold tabular-nums text-navy">{money(total)}</span>
                </li>
              </ul>
              {errors.splits && <p className="mt-1 text-xs text-bad">{errors.splits}</p>}
            </section>
            <section className="space-y-4">
              <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">2. Pagamento</p>
              <Field label="Data do pagamento" error={errors.paid_on}>
                <Input value={dateText} onChange={(e) => setDateText(e.target.value)} placeholder="dd/mm/aaaa" />
              </Field>
              <Field label="Método">
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
                <Field label={method === 'cash' ? 'Referência (opcional)' : 'Referência da transacção'} error={errors.reference}>
                  <Input value={reference} onChange={(e) => setReference(e.target.value)} invalid={!!errors.reference} />
                </Field>
              </div>
              <Field label="Observações (opcional)"><Textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} /></Field>
            </section>
            <div className="sticky bottom-0 -mx-5 -mb-5 border-t border-line bg-white px-5 py-4">
              <Button className="w-full justify-center" loading={saving} disabled={total <= 0} onClick={submit}>Confirmar {total > 0 ? money(total) : ''}</Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
