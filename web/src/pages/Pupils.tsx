import * as React from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, HeartPulse, Plus, Search, Users, Wallet } from 'lucide-react'
import { api } from '@/lib/api'
import { age, money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Avatar, Badge, BalanceBadge, Button, Card, EmptyState, ErrorState, Input, PageHeader, Select, Skeleton, Table, Td, Th } from '@/components/ui'
import { usePaymentSheet } from '@/features/payments/PaymentSheet'
import type { ClassItem, EnrolmentListItem, List } from '@/api/types'

const PAGE = 25

export function PupilsPage() {
  const { year, entity, can } = useSession()
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const payment = usePaymentSheet()
  const q = params.get('q') ?? ''
  const classId = params.get('class') ?? ''
  const status = params.get('status') ?? ''
  const debt = params.get('debt') ?? ''
  const page = Number(params.get('page') ?? '1')
  const [search, setSearch] = React.useState(q)

  const set = (k: string, v: string) => {
    const next = new URLSearchParams(params)
    if (v) next.set(k, v)
    else next.delete(k)
    if (k !== 'page') next.delete('page')
    setParams(next, { replace: true })
  }
  React.useEffect(() => {
    const t = setTimeout(() => search !== q && set('q', search), 300)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search])

  const classes = useQuery({
    queryKey: ['classes', entity?.id ?? 'all', year?.year],
    queryFn: () => api.get<List<ClassItem>>('/classes', { year: year?.year }),
    enabled: !!year,
  })
  const list = useQuery({
    queryKey: ['enrolments', entity?.id ?? 'all', year?.year, q, classId, status, debt, page],
    queryFn: () => api.get<List<EnrolmentListItem>>('/enrolments', { year: year?.year, q, class_id: classId, status, debt, page, page_size: PAGE }),
    enabled: !!year,
    placeholderData: keepPreviousData,
  })
  const items = list.data?.items ?? []
  const total = list.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE))
  const showMoney = can('charges.read')

  return (
    <>
      <PageHeader
        title={pt.nav.pupils}
        description={list.data ? `${total} ${total === 1 ? 'aluno' : 'alunos'} · ano lectivo ${year?.year}` : undefined}
        actions={can('enrolments.write') && (
          <Button onClick={() => navigate('/matriculas')}><Plus className="h-4 w-4" /> {pt.actions.newEnrolment}</Button>
        )}
      />

      <Card>
        <div className="flex flex-col gap-3 border-b border-line p-4 md:flex-row md:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted" />
            <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Nome, número ou telefone do encarregado" className="pl-9" />
          </div>
          <div className="grid grid-cols-2 gap-2 md:flex">
            <Select value={classId} onChange={(e) => set('class', e.target.value)} className="md:w-40" aria-label="Turma">
              <option value="">Todas as turmas</option>
              {classes.data?.items.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </Select>
            <Select value={status} onChange={(e) => set('status', e.target.value)} className="md:w-40" aria-label="Estado">
              <option value="">Todos os estados</option>
              <option value="confirmed">Matriculados</option>
              <option value="draft">Rascunhos</option>
              <option value="withdrawn">Desistências</option>
            </Select>
            {showMoney && (
              <button
                onClick={() => set('debt', debt ? '' : 'overdue')}
                className={cn('col-span-2 h-10 rounded-lg border px-3 text-sm font-medium md:col-span-1', debt ? 'border-bad bg-bad-tint text-bad' : 'border-line bg-white text-ink hover:bg-page')}
              >
                Só em atraso
              </button>
            )}
          </div>
        </div>

        {list.isError && <div className="p-4"><ErrorState error={list.error} onRetry={() => list.refetch()} /></div>}
        {list.isLoading && <div className="space-y-2 p-4">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-12" />)}</div>}
        {list.data && items.length === 0 && (
          <EmptyState icon={<Users className="h-6 w-6" />} title="Nenhum aluno encontrado" description={q || classId || status || debt ? 'Experimente limpar os filtros.' : 'As matrículas confirmadas aparecem aqui.'} />
        )}

        {items.length > 0 && (
          <>
            {/* Desktop table */}
            <Table className="hidden md:block">
              <thead>
                <tr>
                  <Th>Aluno</Th>
                  <Th>Turma</Th>
                  <Th>Encarregado</Th>
                  <Th>Estado</Th>
                  {showMoney && <Th align="right">{pt.account.overdue}</Th>}
                  {showMoney && <Th align="right">{pt.account.outstanding}</Th>}
                  <Th />
                </tr>
              </thead>
              <tbody>
                {items.map((p) => (
                  <tr key={p.enrolment_id} className="cursor-pointer hover:bg-page/70" onClick={() => navigate(`/alunos/${p.pupil_id}`)}>
                    <Td>
                      <div className="flex items-center gap-3">
                        <Avatar name={p.full_name} />
                        <div className="min-w-0">
                          <p className="flex items-center gap-1.5 font-semibold text-navy">
                            {p.full_name}
                            {p.health_alert && <HeartPulse className="h-4 w-4 text-bad" aria-label="Alerta de saúde" />}
                          </p>
                          <p className="text-xs text-ink-muted">{p.number} · {age(p.birth_date)} anos</p>
                        </div>
                      </div>
                    </Td>
                    <Td>{p.class_name ?? '—'}</Td>
                    <Td>
                      <p className="text-ink">{p.payer_name ?? '—'}</p>
                      <p className="text-xs text-ink-muted">{p.payer_phone}</p>
                    </Td>
                    <Td><Badge tone={p.status === 'confirmed' ? 'ok' : p.status === 'draft' ? 'warn' : 'neutral'}>{pt.enrolmentStatus[p.status]}</Badge></Td>
                    {showMoney && <Td align="right"><BalanceBadge overdue={p.overdue} /></Td>}
                    {showMoney && <Td align="right" className="text-ink">{money(p.outstanding)}</Td>}
                    <Td align="right">
                      {can('payments.create') && entity && p.status !== 'draft' && (
                        <Button size="sm" variant="secondary" onClick={(e) => {
                          e.stopPropagation()
                          payment.open({ id: p.pupil_id, name: p.full_name, number: p.number, className: p.class_name })
                        }}>
                          <Wallet className="h-4 w-4" /> Pagar
                        </Button>
                      )}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>

            {/* Mobile cards */}
            <ul className="divide-y divide-line md:hidden">
              {items.map((p) => (
                <li key={p.enrolment_id}>
                  <Link to={`/alunos/${p.pupil_id}`} className="flex items-center gap-3 px-4 py-3">
                    <Avatar name={p.full_name} />
                    <div className="min-w-0 flex-1">
                      <p className="flex items-center gap-1.5 truncate font-semibold text-navy">
                        {p.full_name}
                        {p.health_alert && <HeartPulse className="h-4 w-4 shrink-0 text-bad" />}
                      </p>
                      <p className="truncate text-xs text-ink-muted">{p.class_name ?? '—'} · {p.payer_phone ?? p.number}</p>
                    </div>
                    {showMoney && <BalanceBadge overdue={p.overdue} />}
                  </Link>
                </li>
              ))}
            </ul>

            <div className="flex items-center justify-between border-t border-line px-4 py-3 text-sm text-ink-muted">
              <span>Página {page} de {pages}</span>
              <div className="flex gap-2">
                <Button variant="secondary" size="sm" disabled={page <= 1} onClick={() => set('page', String(page - 1))} aria-label="Anterior"><ChevronLeft className="h-4 w-4" /></Button>
                <Button variant="secondary" size="sm" disabled={page >= pages} onClick={() => set('page', String(page + 1))} aria-label="Seguinte"><ChevronRight className="h-4 w-4" /></Button>
              </div>
            </div>
          </>
        )}
      </Card>
    </>
  )
}
