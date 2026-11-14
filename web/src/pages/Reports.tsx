import * as React from 'react'
import { useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AlertTriangle, BarChart3, Download, GraduationCap, Percent, Receipt, Scale, Wallet } from 'lucide-react'
import { api, downloadXlsx } from '@/lib/api'
import { date, money, parseDate } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Button, Card, EmptyState, ErrorState, Input, PageHeader, Skeleton, Table, Td, Th } from '@/components/ui'
import { Segmented } from '@/features/enrolment/shared'
import type { ReportTable } from '@/api/types'

const reports = [
  { key: 'income', label: 'Receitas', description: 'Pagamentos recebidos por mês, método ou conta.', icon: Wallet, period: true },
  { key: 'arrears', label: 'Dívidas', description: 'O que cada aluno deve, com antiguidade.', icon: Receipt },
  { key: 'pupils', label: 'Alunos por turma', description: 'Matriculados, vagas e ocupação.', icon: GraduationCap },
  { key: 'discounts', label: 'Descontos', description: 'Descontos concedidos por tipo e aluno.', icon: Percent },
  { key: 'penalties', label: 'Multas', description: 'Multas lançadas, pagas e perdoadas.', icon: AlertTriangle },
  { key: 'vat', label: 'IVA', description: 'IVA das cobranças por mês, para a contabilidade.', icon: Scale },
] as const
type Key = (typeof reports)[number]['key']

function cell(v: string | number | null, type: ReportTable['columns'][number]['type']) {
  if (v === null || v === '') return '—'
  switch (type) {
    case 'money': return money(Number(v))
    case 'date': return date(String(v))
    case 'percent': return `${(Number(v) * 100).toFixed(0)}%`
    case 'int': return Number(v).toLocaleString('pt-PT')
    default: return String(v)
  }
}

/** Reports (brief 7.12): one generic table per report, the same data as the Excel export. */
export function ReportsPage() {
  const { entity, year, me } = useSession()
  const [params, setParams] = useSearchParams()
  const key = (reports.find((r) => r.key === params.get('r'))?.key ?? 'income') as Key
  const def = reports.find((r) => r.key === key)!
  const set = (patch: Record<string, string>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(patch)) {
      if (v) next.set(k, v)
      else next.delete(k)
    }
    setParams(next, { replace: true })
  }
  const from = params.get('from') ?? `${me.today.slice(0, 4)}-01-01`
  const to = params.get('to') ?? me.today
  const group = params.get('group') ?? 'month'
  const [fromText, setFromText] = React.useState(date(from))
  const [toText, setToText] = React.useState(date(to))

  const query = 'period' in def && def.period ? { from, to, group } : { year: year?.year }
  const data = useQuery({
    queryKey: ['report', key, entity?.id ?? 'all', query],
    queryFn: () => api.get<ReportTable>(`/reports/${key}`, query),
    enabled: !!year,
    placeholderData: keepPreviousData,
  })

  const applyDate = (k: 'from' | 'to', text: string) => {
    const iso = parseDate(text)
    if (iso) set({ [k]: iso })
    else toast.error('Data no formato dd/mm/aaaa.')
  }
  const t = data.data

  return (
    <>
      <PageHeader title={pt.nav.reports} description={entity ? entity.short_name : 'Todas as escolas'} />
      <div className="grid gap-5 xl:grid-cols-[230px_1fr]">
        <nav className="flex gap-2 overflow-x-auto xl:flex-col xl:overflow-visible">
          {reports.map((r) => {
            const Icon = r.icon
            return (
              <button key={r.key} type="button" onClick={() => set({ r: r.key === 'income' ? '' : r.key })}
                className={cn('flex shrink-0 items-start gap-3 rounded-card border p-3 text-left transition-colors xl:w-full',
                  key === r.key ? 'border-brand bg-brand-tint' : 'border-line bg-white hover:border-brand/40')}>
                <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', key === r.key ? 'text-brand' : 'text-ink-muted')} />
                <span>
                  <span className="block text-sm font-semibold text-navy">{r.label}</span>
                  <span className="hidden text-xs text-ink-muted xl:block">{r.description}</span>
                </span>
              </button>
            )
          })}
        </nav>

        <Card className="min-w-0">
          <div className="flex flex-col gap-3 border-b border-line p-4 md:flex-row md:items-center">
            <div className="min-w-0 md:mr-auto">
              <p className="font-semibold text-navy">{t?.title ?? def.label}</p>
              <p className="text-xs text-ink-muted">{'period' in def && def.period ? `${date(from)} a ${date(to)}` : `Ano lectivo ${year?.year ?? ''}`}</p>
            </div>
            {'period' in def && def.period && (
              <div className="flex flex-wrap items-center gap-2">
                <Input aria-label="De" value={fromText} onChange={(e) => setFromText(e.target.value)} onBlur={() => applyDate('from', fromText)} onKeyDown={(e) => e.key === 'Enter' && applyDate('from', fromText)} className="w-32" />
                <span className="text-sm text-ink-muted">a</span>
                <Input aria-label="Até" value={toText} onChange={(e) => setToText(e.target.value)} onBlur={() => applyDate('to', toText)} onKeyDown={(e) => e.key === 'Enter' && applyDate('to', toText)} className="w-32" />
                <Segmented value={group} onChange={(g) => set({ group: g === 'month' ? '' : g })}
                  options={[{ value: 'month', label: 'Mês' }, { value: 'method', label: 'Método' }, { value: 'account', label: 'Conta' }]} />
              </div>
            )}
            <Button variant="secondary" className="shrink-0 whitespace-nowrap" onClick={() => downloadXlsx(`/reports/${key}`, query, `${key}-${('period' in def && def.period) ? `${from}_${to}` : year?.year}.xlsx`).catch((e) => toast.error(e.message))}>
              <Download className="h-4 w-4" /> {pt.actions.export}
            </Button>
          </div>
          {data.isError && <div className="p-4"><ErrorState error={data.error} onRetry={() => data.refetch()} /></div>}
          {!t && !data.isError && <div className="space-y-2 p-4">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-10" />)}</div>}
          {t && t.rows.length === 0 && <EmptyState icon={<BarChart3 className="h-6 w-6" />} title="Sem dados para este período" />}
          {t && t.rows.length > 0 && (
            <div className={cn('overflow-x-auto', data.isFetching && 'opacity-60')}>
              <Table>
                <thead>
                  <tr>{t.columns.map((c) => <Th key={c.key} align={c.type === 'text' || c.type === 'date' ? undefined : 'right'} className="whitespace-nowrap">{c.label}</Th>)}</tr>
                </thead>
                <tbody>
                  {t.rows.map((row, i) => (
                    <tr key={i} className="hover:bg-page/70">
                      {row.map((v, j) => (
                        <Td key={j} align={t.columns[j].type === 'text' || t.columns[j].type === 'date' ? undefined : 'right'} className={cn('whitespace-nowrap', t.columns[j].type !== 'text' && 'tabular-nums')}>
                          {cell(v, t.columns[j].type)}
                        </Td>
                      ))}
                    </tr>
                  ))}
                </tbody>
                {t.totals && (
                  <tfoot>
                    <tr className="bg-page/70 font-semibold text-navy">
                      {t.totals.map((v, j) => (
                        <Td key={j} align={t.columns[j].type === 'text' || t.columns[j].type === 'date' ? undefined : 'right'} className="whitespace-nowrap">
                          {v === null ? '' : cell(v, t.columns[j].type === 'date' ? 'text' : t.columns[j].type)}
                        </Td>
                      ))}
                    </tr>
                  </tfoot>
                )}
              </Table>
            </div>
          )}
        </Card>
      </div>
    </>
  )
}
