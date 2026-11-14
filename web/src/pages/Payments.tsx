import * as React from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, ChevronLeft, ChevronRight, FileText, Plus, Printer, Search, Wallet } from 'lucide-react'
import { api, openDocument } from '@/lib/api'
import { date, money, parseDate } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import {
  Badge, Button, Card, Dialog, DialogContent, EmptyState, ErrorState, Field, Input, PageHeader, Select, Skeleton, Table, Td, Textarea, Th,
} from '@/components/ui'
import { usePaymentSheet } from '@/features/payments/PaymentSheet'
import { type Errors, handleError } from '@/features/enrolment/shared'
import type { Account, List, Payment, PaymentList } from '@/api/types'

const PAGE = 50
const methods = ['cash', 'bank', 'mpesa', 'emola'] as const

function monthStart(today: string) {
  return `${today.slice(0, 7)}-01`
}

function useDebounced(v: string, ms = 300) {
  const [out, setOut] = React.useState(v)
  React.useEffect(() => {
    const t = setTimeout(() => setOut(v), ms)
    return () => clearTimeout(t)
  }, [v, ms])
  return out
}

/** Payments (brief 7.9): the cash book — filter by period, method and account; open, reprint or cancel a receipt. */
export function PaymentsPage() {
  const { entity, me, can } = useSession()
  const payment = usePaymentSheet()
  const [params, setParams] = useSearchParams()
  const set = (k: string, v: string) => {
    const next = new URLSearchParams(params)
    if (v) next.set(k, v)
    else next.delete(k)
    if (k !== 'page') next.delete('page')
    setParams(next, { replace: true })
  }
  const from = params.get('from') ?? monthStart(me.today)
  const to = params.get('to') ?? me.today
  const [fromText, setFromText] = React.useState(date(from))
  const [toText, setToText] = React.useState(date(to))
  const method = params.get('method') ?? ''
  const accountId = params.get('account') ?? ''
  const status = params.get('status') ?? ''
  const page = Number(params.get('page') ?? '1')
  const [q, setQ] = React.useState(params.get('q') ?? '')
  const term = useDebounced(q)
  React.useEffect(() => { if (term !== (params.get('q') ?? '')) set('q', term) }, [term]) // eslint-disable-line react-hooks/exhaustive-deps
  const [open, setOpen] = React.useState<string | null>(null)

  const accounts = useQuery({
    queryKey: ['accounts', entity?.id],
    queryFn: () => api.get<List<Account>>(`/entities/${entity!.id}/accounts`),
    enabled: !!entity,
  })
  const list = useQuery({
    queryKey: ['payments', entity?.id ?? 'all', 'list', from, to, method, accountId, status, term, page],
    queryFn: () => api.get<PaymentList>('/payments', {
      from, to, method: method || undefined, receiving_account_id: accountId || undefined, status: status || undefined, q: term || undefined, page, page_size: PAGE,
    }),
    placeholderData: keepPreviousData,
  })

  const applyDate = (k: 'from' | 'to', text: string) => {
    const iso = parseDate(text)
    if (iso) set(k, iso)
    else toast.error('Data no formato dd/mm/aaaa.')
  }
  const quick = (f: string, t: string) => {
    const next = new URLSearchParams(params)
    next.set('from', f)
    next.set('to', t)
    next.delete('page')
    setParams(next, { replace: true })
    setFromText(date(f))
    setToText(date(t))
  }
  const d = list.data
  const pages = d ? Math.max(1, Math.ceil(d.total / PAGE)) : 1

  return (
    <>
      <PageHeader
        title={pt.nav.payments}
        description={entity ? entity.short_name : 'Todas as escolas'}
        actions={can('payments.create') && entity && <Button variant="accent" onClick={() => payment.open()}><Plus className="h-4 w-4" /> {pt.actions.recordPayment}</Button>}
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-5">
        <div className="col-span-2 rounded-card bg-navy p-4 text-white lg:col-span-1">
          <p className="text-xs font-semibold uppercase tracking-wide text-white/60">Recebido</p>
          <p className="mt-1 text-[24px] font-bold tabular-nums">{d ? money(d.sum) : '—'}</p>
          <p className="text-xs text-white/70">{date(from)} a {date(to)}</p>
        </div>
        {methods.map((m) => (
          <button key={m} type="button" onClick={() => set('method', method === m ? '' : m)}
            className={cn('rounded-card border bg-white p-4 text-left shadow-soft hover:border-brand/40', method === m ? 'border-brand ring-2 ring-brand/15' : 'border-line')}>
            <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">{pt.methods[m]}</p>
            <p className="mt-1 text-lg font-bold tabular-nums text-navy">{d ? money(d.by_method[m] ?? 0) : '—'}</p>
          </button>
        ))}
      </div>

      <Card>
        <div className="flex flex-col gap-2 border-b border-line p-3 lg:flex-row lg:flex-wrap lg:items-center">
          <div className="relative lg:w-64">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted" />
            <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Aluno, recibo ou referência" className="pl-9" />
          </div>
          <div className="flex items-center gap-2">
            <Input aria-label="De" value={fromText} onChange={(e) => setFromText(e.target.value)} onBlur={() => applyDate('from', fromText)} onKeyDown={(e) => e.key === 'Enter' && applyDate('from', fromText)} className="w-32" />
            <span className="text-sm text-ink-muted">a</span>
            <Input aria-label="Até" value={toText} onChange={(e) => setToText(e.target.value)} onBlur={() => applyDate('to', toText)} onKeyDown={(e) => e.key === 'Enter' && applyDate('to', toText)} className="w-32" />
          </div>
          <div className="flex gap-1">
            <Button size="sm" variant="ghost" onClick={() => quick(me.today, me.today)}>Hoje</Button>
            <Button size="sm" variant="ghost" onClick={() => quick(monthStart(me.today), me.today)}>Este mês</Button>
            <Button size="sm" variant="ghost" onClick={() => quick(`${me.today.slice(0, 4)}-01-01`, me.today)}>Este ano</Button>
          </div>
          {entity && (
            <Select value={accountId} onChange={(e) => set('account', e.target.value)} className="lg:w-52" aria-label="Conta">
              <option value="">Todas as contas</option>
              {accounts.data?.items.map((a) => <option key={a.id} value={a.id}>{a.label}</option>)}
            </Select>
          )}
          <Select value={status} onChange={(e) => set('status', e.target.value)} className="lg:w-40" aria-label="Estado">
            <option value="">Todos</option>
            <option value="valid">Válidos</option>
            <option value="cancelled">Anulados</option>
          </Select>
        </div>

        {list.isError && <div className="p-4"><ErrorState error={list.error} onRetry={() => list.refetch()} /></div>}
        {!d && !list.isError && <div className="space-y-2 p-4">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-11" />)}</div>}
        {d && d.items.length === 0 && <EmptyState icon={<Wallet className="h-6 w-6" />} title="Sem pagamentos neste período" />}
        {d && d.items.length > 0 && (
          <>
            <ul className="divide-y divide-line md:hidden">
              {d.items.map((p) => (
                <li key={p.id}>
                  <button type="button" onClick={() => setOpen(p.id)} className={cn('flex w-full items-start gap-3 p-4 text-left', p.cancelled_at && 'opacity-60')}>
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-semibold text-navy">{p.pupil_name}</p>
                      <p className="text-xs text-ink-muted">{p.receipt_no} · {date(p.paid_on)} · {pt.methods[p.method]}</p>
                    </div>
                    <div className="text-right">
                      <p className={cn('font-bold tabular-nums', p.cancelled_at ? 'text-ink-muted line-through' : 'text-navy')}>{money(p.amount)}</p>
                      {p.cancelled_at && <Badge tone="bad">Anulado</Badge>}
                    </div>
                  </button>
                </li>
              ))}
            </ul>
            <div className="hidden md:block">
              <Table>
                <thead>
                  <tr><Th>Data</Th><Th>Recibo</Th><Th>Aluno</Th><Th>Método</Th><Th className="hidden xl:table-cell">Referência</Th><Th align="right">Valor</Th><Th className="hidden lg:table-cell">Recebido por</Th></tr>
                </thead>
                <tbody>
                  {d.items.map((p) => (
                    <tr key={p.id} onClick={() => setOpen(p.id)} className={cn('cursor-pointer hover:bg-page/70', p.cancelled_at && 'opacity-60')}>
                      <Td className="whitespace-nowrap">{date(p.paid_on)}</Td>
                      <Td className="whitespace-nowrap font-medium text-navy">{p.receipt_no}{p.cancelled_at && <Badge tone="bad" className="ml-2">Anulado</Badge>}{p.payment_group_id && <Badge tone="violet" className="ml-2">Família</Badge>}</Td>
                      <Td><p className="text-ink">{p.pupil_name}</p><p className="text-xs text-ink-muted">{p.pupil_number}</p></Td>
                      <Td><p>{pt.methods[p.method]}</p>{p.account_label && <p className="text-xs text-ink-muted">{p.account_label}</p>}</Td>
                      <Td className="hidden text-ink-muted xl:table-cell">{p.reference || '—'}</Td>
                      <Td align="right" className={cn('whitespace-nowrap font-semibold', p.cancelled_at && 'line-through')}>{money(p.amount)}</Td>
                      <Td className="hidden text-ink-muted lg:table-cell">{p.received_by_name}</Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </div>
            <div className="flex items-center justify-between border-t border-line px-4 py-3 text-sm text-ink-muted">
              <span>{d.total} {d.total === 1 ? 'pagamento' : 'pagamentos'}</span>
              {pages > 1 && (
                <div className="flex items-center gap-2">
                  <Button size="sm" variant="ghost" disabled={page <= 1} onClick={() => set('page', String(page - 1))} aria-label="Anterior"><ChevronLeft className="h-4 w-4" /></Button>
                  <span>{page} / {pages}</span>
                  <Button size="sm" variant="ghost" disabled={page >= pages} onClick={() => set('page', String(page + 1))} aria-label="Seguinte"><ChevronRight className="h-4 w-4" /></Button>
                </div>
              )}
            </div>
          </>
        )}
      </Card>
      <PaymentDetail id={open} onClose={() => setOpen(null)} />
    </>
  )
}

/** One payment: what it settled, its receipt, and cancellation with a reason (the receipt number is kept). */
export function PaymentDetail({ id, onClose }: { id: string | null; onClose: () => void }) {
  const { can, entity } = useSession()
  const qc = useQueryClient()
  const [cancelling, setCancelling] = React.useState(false)
  const [reason, setReason] = React.useState('')
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)
  React.useEffect(() => { setCancelling(false); setReason(''); setErrors({}) }, [id])
  const q = useQuery({ queryKey: ['payment', id], queryFn: () => api.get<Payment>(`/payments/${id}`), enabled: !!id })
  const p = q.data

  async function cancel() {
    if (reason.trim().length < 5) return setErrors({ reason: 'Indique o motivo.' })
    setSaving(true)
    try {
      await api.post(`/payments/${id}/cancel`, { reason: reason.trim() })
      toast.success('Pagamento anulado. O recibo mantém o número e fica marcado como anulado.')
      void qc.invalidateQueries()
      setCancelling(false)
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={!!id} onOpenChange={(o) => !o && onClose()}>
      <DialogContent side="right" title={p ? `Recibo ${p.receipt_no}` : 'Pagamento'} description={p ? `${p.pupil_name} · ${p.pupil_number}` : undefined}>
        {!p ? <div className="space-y-3 p-5"><Skeleton className="h-20" /><Skeleton className="h-40" /></div> : (
          <div className="space-y-5 p-5">
            {p.cancelled_at && (
              <div className="rounded-lg border border-bad/25 bg-bad-tint p-3 text-sm text-bad">
                Anulado em {date(p.cancelled_at)}{p.cancel_reason && ` — ${p.cancel_reason}`}
              </div>
            )}
            <div className="rounded-card bg-page p-4">
              <p className="text-xs text-ink-muted">Valor</p>
              <p className={cn('text-[26px] font-bold tabular-nums text-navy', p.cancelled_at && 'line-through')}>{money(p.amount)}</p>
              <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                <div><dt className="text-xs text-ink-muted">Data</dt><dd>{date(p.paid_on)}</dd></div>
                <div><dt className="text-xs text-ink-muted">Método</dt><dd>{pt.methods[p.method]}{p.account_label && ` · ${p.account_label}`}</dd></div>
                <div><dt className="text-xs text-ink-muted">Referência</dt><dd>{p.reference || '—'}</dd></div>
                <div><dt className="text-xs text-ink-muted">Recebido por</dt><dd>{p.received_by_name}</dd></div>
                {p.notes && <div className="col-span-2"><dt className="text-xs text-ink-muted">Observações</dt><dd>{p.notes}</dd></div>}
              </dl>
            </div>
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-muted">Liquidou</p>
              <ul className="divide-y divide-line rounded-lg border border-line text-sm">
                {(p.allocations ?? []).map((a) => (
                  <li key={a.charge_id} className={cn('flex items-center justify-between gap-3 px-4 py-2.5', !a.active && 'text-ink-muted line-through')}>
                    <span className="min-w-0 truncate">{a.description}</span>
                    <span className="font-semibold tabular-nums">{money(a.amount)}</span>
                  </li>
                ))}
                {p.credit > 0 && !p.cancelled_at && (
                  <li className="flex items-center justify-between bg-accent-tint px-4 py-2.5"><span>Crédito</span><span className="font-semibold tabular-nums">{money(p.credit)}</span></li>
                )}
                {(p.allocations ?? []).length === 0 && p.credit === 0 && <li className="px-4 py-2.5 text-ink-muted">Nada.</li>}
              </ul>
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              <Button onClick={() => openDocument(`/payments/${p.id}/receipt`).catch((e) => toast.error(e.message))}><FileText className="h-4 w-4" /> Recibo (PDF)</Button>
              <Button variant="secondary" onClick={() => openDocument(`/payments/${p.id}/receipt`, 'html').catch((e) => toast.error(e.message))}><Printer className="h-4 w-4" /> Imprimir</Button>
              <Link to={`/alunos/${p.pupil_id}`} onClick={onClose} className="inline-flex h-10 items-center justify-center rounded-lg border border-line text-sm font-medium text-ink hover:bg-page">Ver ficha do aluno</Link>
              {!p.cancelled_at && can('payments.cancel') && entity?.id === p.entity_id && !cancelling && (
                <Button variant="ghost" className="text-bad hover:bg-bad-tint" onClick={() => setCancelling(true)}><Ban className="h-4 w-4" /> Anular pagamento</Button>
              )}
            </div>
            {cancelling && (
              <div className="space-y-3 rounded-lg border border-bad/25 p-4">
                <p className="text-sm text-ink">O dinheiro sai das cobranças que liquidou e elas voltam a ficar por pagar. O recibo fica marcado como <strong>ANULADO</strong>.</p>
                <Field label="Motivo" error={errors.reason}>
                  <Textarea autoFocus value={reason} onChange={(e) => { setReason(e.target.value); setErrors({}) }} placeholder="Ex.: valor registado em duplicado" />
                </Field>
                <div className="flex justify-end gap-2">
                  <Button variant="secondary" onClick={() => setCancelling(false)}>{pt.actions.cancel}</Button>
                  <Button variant="danger" loading={saving} onClick={cancel}>Anular pagamento</Button>
                </div>
              </div>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
