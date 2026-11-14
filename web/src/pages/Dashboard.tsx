import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { AlertTriangle, ArrowRight, Percent, Phone } from 'lucide-react'
import { api } from '@/lib/api'
import { date, money, moneyShort, percent } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { Badge, Card, CardHeader, EmptyState, ErrorState, PageHeader, Skeleton, Stat } from '@/components/ui'
import type { Dashboard } from '@/api/types'

const methodColors: Record<string, string> = { cash: '#2463a6', mpesa: '#bf3d45', emola: '#6d55a8', bank: '#15845a' }

export function DashboardPage() {
  const { year, entity } = useSession()
  const q = useQuery({
    queryKey: ['dashboard', entity?.id ?? 'all', year?.year],
    queryFn: () => api.get<Dashboard>('/reports/dashboard', { year: year?.year }),
    enabled: !!year,
  })
  const d = q.data
  const monthName = d ? pt.months[Number(d.today.slice(5, 7))] : ''

  return (
    <>
      <PageHeader
        title={pt.nav.dashboard}
        description={d ? `${entity ? entity.short_name : 'Todas as empresas'} · ${monthName} ${d.today.slice(0, 4)} · ano lectivo ${d.year}` : undefined}
      />
      {q.isError && <ErrorState error={q.error} onRetry={() => q.refetch()} />}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {!d ? (
          Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-[104px] rounded-card" />)
        ) : (
          <>
            <Stat label={`Previsto em ${monthName}`} value={moneyShort(d.expected_month)} hint="Mensalidades e taxas a vencer no mês" />
            <Stat label={`Recebido em ${monthName}`} value={moneyShort(d.received_month)} hint={`Cobrança do ano: ${percent(d.collection_rate)}`} tone="ok" />
            <Stat label="Em dívida" value={moneyShort(d.overdue)} hint={`${d.pupils_overdue} ${d.pupils_overdue === 1 ? 'aluno' : 'alunos'} em atraso`} tone={d.overdue > 0 ? 'bad' : undefined} />
            <Stat label="Alunos matriculados" value={d.active_pupils} hint={d.pending_discounts ? `${d.pending_discounts} ${d.pending_discounts === 1 ? 'desconto' : 'descontos'} por aprovar` : 'Sem aprovações pendentes'} />
          </>
        )}
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <CardHeader title="Previsto vs recebido" description="Por mês do calendário de cobrança" />
          <div className="h-[280px] p-4 pr-6">
            {d ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={d.monthly.map((m) => ({ name: pt.monthsShort[m.month], Previsto: m.expected / 100, Recebido: m.received / 100 }))} barGap={2}>
                  <CartesianGrid vertical={false} stroke="#e4e9ef" />
                  <XAxis dataKey="name" tickLine={false} axisLine={false} fontSize={12} stroke="#6b7888" />
                  <YAxis tickLine={false} axisLine={false} fontSize={11} stroke="#6b7888" width={56}
                    tickFormatter={(v: number) => (v >= 1000 ? `${Math.round(v / 1000)} mil` : String(v))} />
                  <Tooltip formatter={(v: number) => money(Math.round(v * 100))} cursor={{ fill: '#eaf2fb' }} contentStyle={{ borderRadius: 10, borderColor: '#e4e9ef' }} />
                  <Legend iconType="circle" wrapperStyle={{ fontSize: 12 }} />
                  <Bar dataKey="Previsto" fill="#c9d6e6" radius={[4, 4, 0, 0]} isAnimationActive={false} />
                  <Bar dataKey="Recebido" fill="#2463a6" radius={[4, 4, 0, 0]} isAnimationActive={false} />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <Skeleton className="h-full w-full" />
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title={`Recebido em ${monthName}`} description="Por método de pagamento" />
          <div className="space-y-3 p-5">
            {d && Object.keys(d.by_method_month).length === 0 && <p className="text-sm text-ink-muted">Ainda sem pagamentos este mês.</p>}
            {d &&
              ['cash', 'mpesa', 'emola', 'bank'].filter((m) => d.by_method_month[m]).map((m) => {
                const v = d.by_method_month[m]
                const share = d.received_month ? v / d.received_month : 0
                return (
                  <div key={m}>
                    <div className="flex justify-between text-sm">
                      <span className="font-medium text-ink">{pt.methods[m]}</span>
                      <span className="tabular-nums text-navy">{money(v)}</span>
                    </div>
                    <div className="mt-1.5 h-2 rounded-full bg-page">
                      <div className="h-2 rounded-full" style={{ width: `${Math.max(share * 100, 2)}%`, background: methodColors[m] }} />
                    </div>
                  </div>
                )
              })}
            {d && d.pending_discounts > 0 && (
              <div className="mt-2 flex items-center gap-2 rounded-lg bg-accent-tint px-3 py-2 text-sm">
                <Percent className="h-4 w-4 text-[#8a6500]" />
                <span className="flex-1">{d.pending_discounts} {d.pending_discounts === 1 ? 'pedido de desconto' : 'pedidos de desconto'} por aprovar</span>
              </div>
            )}
          </div>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <CardHeader title="Contas com maior atraso" action={<Link to="/alunos?debt=overdue" className="flex items-center gap-1 text-sm font-semibold text-brand">Ver todas <ArrowRight className="h-4 w-4" /></Link>} />
          {d && d.top_overdue.length === 0 ? (
            <EmptyState icon={<AlertTriangle className="h-5 w-5" />} title="Nenhum aluno em atraso" />
          ) : (
            <ul className="divide-y divide-line">
              {d?.top_overdue.map((a) => (
                <li key={a.pupil_id}>
                  <Link to={`/alunos/${a.pupil_id}`} className="flex items-center gap-3 px-5 py-3 hover:bg-page">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-semibold text-navy">{a.name}</p>
                      <p className="flex items-center gap-2 text-xs text-ink-muted">
                        {a.class_name ?? '—'}
                        {a.phone && <span className="inline-flex items-center gap-1"><Phone className="h-3 w-3" />{a.phone}</span>}
                      </p>
                    </div>
                    <div className="text-right">
                      <p className="text-sm font-bold tabular-nums text-bad">{money(a.overdue)}</p>
                      <p className="text-xs text-ink-muted">{a.days} dias</p>
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
        <Card>
          <CardHeader title="Pagamentos recentes" />
          {d && d.recent_payments.length === 0 && <p className="px-5 py-4 text-sm text-ink-muted">Sem pagamentos.</p>}
          <ul className="divide-y divide-line">
            {d?.recent_payments.map((p) => (
              <li key={p.id} className="flex items-center gap-3 px-5 py-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-ink">{p.pupil_name}</p>
                  <p className="text-xs text-ink-muted">{p.receipt_no} · {date(p.paid_on)}</p>
                </div>
                <div className="text-right">
                  <p className="text-sm font-semibold tabular-nums text-navy">{money(p.amount)}</p>
                  <Badge tone="info" className="mt-0.5">{pt.methods[p.method]}</Badge>
                </div>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </>
  )
}
