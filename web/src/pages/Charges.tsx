import * as React from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AlertTriangle, ArrowRight, Ban, ChevronLeft, ChevronRight, Download, Gavel, Phone, Search, Wallet } from 'lucide-react'
import { api, downloadXlsx } from '@/lib/api'
import { date, money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import {
  Avatar, Badge, Button, Card, ChargeStatus, EmptyState, ErrorState, Input, PageHeader, Select, Skeleton, Table, Td, Th,
} from '@/components/ui'
import { usePaymentSheet } from '@/features/payments/PaymentSheet'
import { ChargeActionDialog, type ChargeRef } from '@/features/charges/ChargeActionDialog'
import type { ChargeList, ClassItem, List, PupilArrears } from '@/api/types'

const PAGE = 50

const buckets = [
  { key: 'd1_30', label: '1–30 dias', min: 1 },
  { key: 'd31_60', label: '31–60 dias', min: 31 },
  { key: 'd61_90', label: '61–90 dias', min: 61 },
  { key: 'over_90', label: 'Mais de 90 dias', min: 91 },
] as const

function daysTone(d: number) {
  return d > 90 ? 'bad' : d > 30 ? 'warn' : 'neutral'
}

function useDebounced(v: string, ms = 300) {
  const [out, setOut] = React.useState(v)
  React.useEffect(() => {
    const t = setTimeout(() => setOut(v), ms)
    return () => clearTimeout(t)
  }, [v, ms])
  return out
}

/** Charges & arrears (brief 7.10): who owes what, since when, and the actions to collect or correct it. */
export function ChargesPage() {
  const { entity, year, can } = useSession()
  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const view = params.get('view') === 'charges' ? 'charges' : 'pupils'
  const set = (k: string, v: string) => {
    const next = new URLSearchParams(params)
    if (v) next.set(k, v)
    else next.delete(k)
    if (k !== 'page') next.delete('page')
    setParams(next, { replace: true })
  }
  const classId = params.get('class') ?? ''
  const minDays = params.get('min') ?? ''
  const [q, setQ] = React.useState(params.get('q') ?? '')
  const term = useDebounced(q)
  React.useEffect(() => { if (term !== (params.get('q') ?? '')) set('q', term) }, [term]) // eslint-disable-line react-hooks/exhaustive-deps

  const classes = useQuery({
    queryKey: ['classes', entity?.id ?? 'all', year?.year],
    queryFn: () => api.get<List<ClassItem>>('/classes', { year: year?.year }),
    enabled: !!year,
  })
  // Aging of everything overdue for the current class filter.
  const aging = useQuery({
    queryKey: ['charges', entity?.id ?? 'all', year?.year, 'aging', classId],
    queryFn: () => api.get<ChargeList>('/charges', { year: year?.year, status: 'overdue', class_id: classId || undefined, page_size: 1 }),
    enabled: !!year,
    placeholderData: keepPreviousData,
  })
  const runPenalties = useMutation({
    mutationFn: () => api.post<{ pupils: number; penalties: number; amount: number }>('/penalties/run'),
    onSuccess: (r) => {
      toast.success(r.penalties ? `${r.penalties} multas lançadas (${money(r.amount)}).` : 'Não há multas novas a lançar.')
      void qc.invalidateQueries()
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : pt.errors.generic),
  })

  const a = aging.data?.aging
  const exportQuery = { year: year?.year, class_id: classId || undefined, min_days: minDays || undefined, q: term || undefined }

  return (
    <>
      <PageHeader
        title={pt.nav.charges}
        description={`Ano lectivo ${year?.year ?? ''}${entity ? ` · ${entity.short_name}` : ' · todas as escolas'}`}
        actions={
          <>
            {can('settings.manage') && (
              <Button variant="secondary" loading={runPenalties.isPending} onClick={() => runPenalties.mutate()}><Gavel className="h-4 w-4" /> Calcular multas</Button>
            )}
            <Button variant="secondary" onClick={() => downloadXlsx('/charges/arrears', exportQuery, `dividas-${year?.year}.xlsx`).catch((e) => toast.error(e.message))}>
              <Download className="h-4 w-4" /> {pt.actions.export}
            </Button>
          </>
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-5">
        <div className="col-span-2 rounded-card bg-navy p-4 text-white lg:col-span-1">
          <p className="text-xs font-semibold uppercase tracking-wide text-white/60">Em dívida</p>
          <p className="mt-1 text-[24px] font-bold tabular-nums">{a ? money(a.total) : '—'}</p>
          <p className="text-xs text-white/70">{a ? `${a.pupils} ${a.pupils === 1 ? 'aluno' : 'alunos'}` : ''}</p>
        </div>
        {buckets.map((b) => {
          const active = minDays === String(b.min)
          return (
            <button
              key={b.key}
              type="button"
              onClick={() => set('min', active ? '' : String(b.min))}
              className={cn(
                'rounded-card border bg-white p-4 text-left shadow-soft transition-colors hover:border-brand/40',
                active ? 'border-brand ring-2 ring-brand/15' : 'border-line',
              )}
              title={`Mostrar dívidas com ${b.min} ou mais dias`}
            >
              <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">{b.label}</p>
              <p className={cn('mt-1 text-lg font-bold tabular-nums', b.min > 90 ? 'text-bad' : 'text-navy')}>{a ? money(a[b.key]) : '—'}</p>
            </button>
          )
        })}
      </div>

      <Card>
        <div className="flex gap-1 overflow-x-auto border-b border-line px-3">
          {([['pupils', 'Por aluno'], ['charges', 'Por cobrança']] as const).map(([k, label]) => (
            <button key={k} onClick={() => set('view', k === 'pupils' ? '' : k)}
              className={cn('whitespace-nowrap border-b-2 px-3 py-3 text-sm font-medium', view === k ? 'border-brand text-brand' : 'border-transparent text-ink-muted hover:text-navy')}>
              {label}
            </button>
          ))}
        </div>
        <div className="flex flex-col gap-2 border-b border-line p-3 md:flex-row md:flex-wrap md:items-center">
          <div className="relative md:w-72">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted" />
            <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={view === 'pupils' ? 'Aluno, nº ou encarregado' : 'Aluno ou nº'} className="pl-9" />
          </div>
          <Select value={classId} onChange={(e) => set('class', e.target.value)} className="md:w-48" aria-label="Turma">
            <option value="">Todas as turmas</option>
            {classes.data?.items.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </Select>
          <Select value={minDays} onChange={(e) => set('min', e.target.value)} className="md:w-48" aria-label="Atraso">
            <option value="">Qualquer atraso</option>
            <option value="31">Mais de 30 dias</option>
            <option value="61">Mais de 60 dias</option>
            <option value="91">Mais de 90 dias</option>
          </Select>
          {view === 'charges' && <ChargeFilters params={params} set={set} months={year?.billing_months ?? []} />}
        </div>
        {view === 'pupils'
          ? <ByPupil classId={classId} minDays={minDays} q={term} />
          : <ByCharge classId={classId} minDays={minDays} q={term} params={params} set={set} />}
      </Card>
    </>
  )
}

function ChargeFilters({ params, set, months }: { params: URLSearchParams; set: (k: string, v: string) => void; months: number[] }) {
  return (
    <>
      <Select value={params.get('status') ?? 'overdue'} onChange={(e) => set('status', e.target.value === 'overdue' ? '' : e.target.value)} className="md:w-40" aria-label="Estado">
        <option value="overdue">Em atraso</option>
        <option value="open">Por pagar</option>
        <option value="paid">Pagas</option>
        <option value="cancelled">Anuladas</option>
        <option value="all">Todas</option>
      </Select>
      <Select value={params.get('kind') ?? ''} onChange={(e) => set('kind', e.target.value)} className="md:w-40" aria-label="Tipo">
        <option value="">Todos os tipos</option>
        {(['monthly', 'one_off', 'penalty'] as const).map((k) => <option key={k} value={k}>{pt.chargeKind[k]}</option>)}
      </Select>
      <Select value={params.get('month') ?? ''} onChange={(e) => set('month', e.target.value)} className="md:w-40" aria-label="Mês">
        <option value="">Todos os meses</option>
        {months.map((m) => <option key={m} value={m}>{pt.months[m]}</option>)}
      </Select>
    </>
  )
}

function ByPupil({ classId, minDays, q }: { classId: string; minDays: string; q: string }) {
  const { entity, year, can } = useSession()
  const payment = usePaymentSheet()
  const list = useQuery({
    queryKey: ['charges', entity?.id ?? 'all', year?.year, 'arrears', classId, minDays, q],
    queryFn: () => api.get<List<PupilArrears>>('/charges/arrears', { year: year?.year, class_id: classId || undefined, min_days: minDays || undefined, q: q || undefined }),
    enabled: !!year,
    placeholderData: keepPreviousData,
  })
  if (list.isError) return <div className="p-4"><ErrorState error={list.error} onRetry={() => list.refetch()} /></div>
  if (!list.data) return <div className="space-y-2 p-4">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-12" />)}</div>
  const items = list.data.items
  if (items.length === 0) return <EmptyState title="Ninguém em dívida" description="Não há dívidas vencidas com estes filtros." />
  const canPay = can('payments.create') && !!entity
  const total = items.reduce((s, x) => s + x.overdue, 0)

  return (
    <>
      {/* Mobile cards */}
      <ul className="divide-y divide-line md:hidden">
        {items.map((x) => (
          <li key={`${x.entity_id}-${x.pupil_id}`} className="p-4">
            <div className="flex items-start gap-3">
              <Avatar name={x.full_name} />
              <div className="min-w-0 flex-1">
                <Link to={`/alunos/${x.pupil_id}`} className="block truncate font-semibold text-navy">{x.full_name}</Link>
                <p className="truncate text-xs text-ink-muted">{x.number} · {x.class_name ?? '—'}</p>
              </div>
              <div className="text-right">
                <p className="font-bold tabular-nums text-bad">{money(x.overdue)}</p>
                <Badge tone={daysTone(x.days_overdue)}>{x.days_overdue} dias</Badge>
              </div>
            </div>
            <div className="mt-3 flex items-center gap-2">
              {x.payer_phone && (
                <a href={`tel:${x.payer_phone.replace(/\s/g, '')}`} className="inline-flex min-w-0 flex-1 items-center gap-1.5 truncate text-sm text-brand">
                  <Phone className="h-4 w-4 shrink-0" /> {x.payer_name} · {x.payer_phone}
                </a>
              )}
              {canPay && <Button size="sm" variant="accent" className="ml-auto" onClick={() => payment.open({ id: x.pupil_id, name: x.full_name, number: x.number, className: x.class_name })}><Wallet className="h-4 w-4" /> Pagar</Button>}
            </div>
          </li>
        ))}
      </ul>

      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>Aluno</Th>
              <Th>Encarregado</Th>
              <Th align="right">Em dívida</Th>
              <Th align="right" className="hidden xl:table-cell">Total por pagar</Th>
              <Th>Desde</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {items.map((x) => (
              <tr key={`${x.entity_id}-${x.pupil_id}`} className="hover:bg-page/70">
                <Td>
                  <Link to={`/alunos/${x.pupil_id}`} className="flex items-center gap-3">
                    <Avatar name={x.full_name} />
                    <div className="min-w-0">
                      <p className="truncate font-semibold text-navy">{x.full_name}</p>
                      <p className="truncate text-xs text-ink-muted">{x.number} · {x.class_name ?? '—'}</p>
                    </div>
                  </Link>
                </Td>
                <Td>
                  <p className="text-ink">{x.payer_name ?? <span className="text-ink-muted">Sem encarregado</span>}</p>
                  <p className="text-xs text-ink-muted">
                    {x.payer_phone && <a href={`tel:${x.payer_phone.replace(/\s/g, '')}`} className="hover:text-brand">{x.payer_phone}</a>}
                    {x.payer_phone2 && <> · {x.payer_phone2}</>}
                  </p>
                </Td>
                <Td align="right">
                  <p className="font-bold text-bad">{money(x.overdue)}</p>
                  <p className="text-xs text-ink-muted">{x.charges} {x.charges === 1 ? 'cobrança' : 'cobranças'}</p>
                </Td>
                <Td align="right" className="hidden xl:table-cell">{money(x.outstanding)}</Td>
                <Td>
                  <p>{date(x.oldest_due)}</p>
                  <Badge tone={daysTone(x.days_overdue)}>{x.days_overdue} dias</Badge>
                </Td>
                <Td align="right">
                  <div className="flex justify-end gap-1">
                    {canPay && (
                      <Button size="sm" variant="secondary" onClick={() => payment.open({ id: x.pupil_id, name: x.full_name, number: x.number, className: x.class_name })}>
                        <Wallet className="h-4 w-4" /> Pagamento
                      </Button>
                    )}
                    <Link to={`/alunos/${x.pupil_id}`} className="inline-flex h-8 items-center rounded-md px-2 text-ink-muted hover:bg-page hover:text-navy" aria-label="Ver conta"><ArrowRight className="h-4 w-4" /></Link>
                  </div>
                </Td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="bg-page/60">
              <Td className="font-semibold text-navy">{items.length} {items.length === 1 ? 'aluno' : 'alunos'}</Td>
              <Td />
              <Td align="right" className="font-bold text-bad">{money(total)}</Td>
              <Td className="hidden xl:table-cell" /><Td /><Td />
            </tr>
          </tfoot>
        </Table>
      </div>
    </>
  )
}

function ByCharge({ classId, minDays, q, params, set }: { classId: string; minDays: string; q: string; params: URLSearchParams; set: (k: string, v: string) => void }) {
  const { entity, year, can } = useSession()
  const [acting, setActing] = React.useState<ChargeRef | null>(null)
  const status = params.get('status') ?? 'overdue'
  const kind = params.get('kind') ?? ''
  const month = params.get('month') ?? ''
  const page = Number(params.get('page') ?? '1')
  const list = useQuery({
    queryKey: ['charges', entity?.id ?? 'all', year?.year, 'list', classId, minDays, q, status, kind, month, page],
    queryFn: () => api.get<ChargeList>('/charges', {
      year: year?.year, class_id: classId || undefined, min_days: minDays || undefined, q: q || undefined,
      status: status === 'all' ? undefined : status, kind: kind || undefined, month: month || undefined, page, page_size: PAGE,
    }),
    enabled: !!year,
    placeholderData: keepPreviousData,
  })
  if (list.isError) return <div className="p-4"><ErrorState error={list.error} onRetry={() => list.refetch()} /></div>
  if (!list.data) return <div className="space-y-2 p-4">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-12" />)}</div>
  const items = list.data.items
  if (items.length === 0) return <EmptyState title="Sem cobranças" description="Nenhuma cobrança corresponde aos filtros." />
  const pages = Math.max(1, Math.ceil(list.data.total / PAGE))

  const action = (c: ChargeList['items'][number]) => {
    if (c.status === 'cancelled' || !entity || c.entity_id !== entity.id) return null
    const allowed = c.kind === 'penalty' ? can('penalties.waive') : can('charges.cancel')
    if (!allowed) return null
    return (
      <Button size="sm" variant="ghost" className="text-ink-muted hover:text-bad" onClick={() => setActing({ ...c, pupil_name: c.pupil_name })}>
        <Ban className="h-4 w-4" /> {c.kind === 'penalty' ? 'Perdoar' : 'Anular'}
      </Button>
    )
  }

  return (
    <>
      <ul className="divide-y divide-line md:hidden">
        {items.map((c) => (
          <li key={c.id} className="p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <Link to={`/alunos/${c.pupil_id}`} className="block truncate font-semibold text-navy">{c.pupil_name}</Link>
                <p className="truncate text-sm text-ink">{c.kind === 'penalty' && <AlertTriangle className="mr-1 inline h-3.5 w-3.5 text-warn" />}{c.description}</p>
                <p className="text-xs text-ink-muted">Vence {date(c.due_date)}{c.days_overdue > 0 && ` · ${c.days_overdue} dias`}</p>
              </div>
              <div className="shrink-0 text-right">
                <p className="font-semibold tabular-nums">{money(c.remaining)}</p>
                <ChargeStatus status={c.status} />
              </div>
            </div>
            {action(c) && <div className="mt-2 flex justify-end">{action(c)}</div>}
          </li>
        ))}
      </ul>
      <div className="hidden md:block">
        <Table>
          <thead>
            <tr>
              <Th>Aluno</Th><Th>Cobrança</Th><Th>Vencimento</Th>
              <Th align="right">Valor</Th><Th align="right" className="hidden lg:table-cell">Pago</Th><Th align="right">Por pagar</Th>
              <Th>Estado</Th><Th />
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id} className={cn('hover:bg-page/70', c.status === 'cancelled' && 'opacity-60')}>
                <Td>
                  <Link to={`/alunos/${c.pupil_id}`} className="font-medium text-navy hover:underline">{c.pupil_name}</Link>
                  <p className="text-xs text-ink-muted">{c.pupil_number} · {c.class_name ?? '—'}</p>
                </Td>
                <Td className={cn(c.kind === 'penalty' && 'text-ink-muted')}>
                  {c.kind === 'penalty' && <AlertTriangle className="mr-1.5 inline h-3.5 w-3.5 text-warn" />}{c.description}
                </Td>
                <Td className="whitespace-nowrap">
                  {date(c.due_date)}
                  {c.days_overdue > 0 && <p className="text-xs text-ink-muted">{c.days_overdue} dias</p>}
                </Td>
                <Td align="right" className="whitespace-nowrap">{money(c.gross - c.discount)}{c.discount > 0 && <p className="text-xs text-ok">−{money(c.discount)}</p>}</Td>
                <Td align="right" className="hidden whitespace-nowrap text-ink-muted lg:table-cell">{money(c.paid)}</Td>
                <Td align="right" className="whitespace-nowrap font-semibold">{money(c.remaining)}</Td>
                <Td><ChargeStatus status={c.status} /></Td>
                <Td align="right">{action(c)}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
      </div>
      <div className="flex items-center justify-between border-t border-line px-4 py-3 text-sm text-ink-muted">
        <span>{list.data.total} {list.data.total === 1 ? 'cobrança' : 'cobranças'}</span>
        {pages > 1 && (
          <div className="flex items-center gap-2">
            <Button size="sm" variant="ghost" disabled={page <= 1} onClick={() => set('page', String(page - 1))} aria-label="Anterior"><ChevronLeft className="h-4 w-4" /></Button>
            <span>{page} / {pages}</span>
            <Button size="sm" variant="ghost" disabled={page >= pages} onClick={() => set('page', String(page + 1))} aria-label="Seguinte"><ChevronRight className="h-4 w-4" /></Button>
          </div>
        )}
      </div>
      <ChargeActionDialog charge={acting} onClose={() => setActing(null)} />
    </>
  )
}
