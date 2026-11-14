import * as React from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowRight, ClipboardList, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { api } from '@/lib/api'
import { age } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import {
  Avatar, Badge, Button, Card, Dialog, DialogContent, EmptyState, ErrorState, PageHeader, Skeleton, Stat, Table, Td, Th,
} from '@/components/ui'
import type { EnrolmentListItem, List } from '@/api/types'

const tabs = [
  { value: 'draft', label: 'Rascunhos' },
  { value: 'confirmed', label: 'Matriculados' },
  { value: 'withdrawn', label: 'Desistências' },
] as const

function useCount(status: string) {
  const { entity, year } = useSession()
  return useQuery({
    queryKey: ['enrolments', entity?.id ?? 'all', year?.year, 'count', status],
    queryFn: () => api.get<List<EnrolmentListItem>>('/enrolments', { year: year?.year, status, page_size: 1 }),
    enabled: !!year,
    select: (d) => d.total,
  })
}

export function EnrolmentsPage() {
  const { entity, year, can } = useSession()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const status = (params.get('status') ?? 'draft') as (typeof tabs)[number]['value']
  const [toDelete, setToDelete] = React.useState<EnrolmentListItem | null>(null)

  const counts = { draft: useCount('draft'), confirmed: useCount('confirmed'), withdrawn: useCount('withdrawn') }
  const list = useQuery({
    queryKey: ['enrolments', entity?.id ?? 'all', year?.year, 'tab', status],
    queryFn: () => api.get<List<EnrolmentListItem>>('/enrolments', { year: year?.year, status, page_size: 200 }),
    enabled: !!year,
    placeholderData: keepPreviousData,
  })
  const del = useMutation({
    mutationFn: (id: string) => api.del(`/enrolments/${id}`),
    onSuccess: () => {
      toast.success('Rascunho apagado. O registo do aluno mantém-se.')
      setToDelete(null)
      void qc.invalidateQueries({ queryKey: ['enrolments'] })
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : pt.errors.generic),
  })

  const items = list.data?.items ?? []
  const writable = can('enrolments.write') && !!entity

  return (
    <>
      <PageHeader
        title={pt.nav.enrolments}
        description={`Ano lectivo ${year?.year ?? ''}${entity ? ` · ${entity.short_name}` : ''}`}
        actions={writable && (
          <>
            <Button variant="secondary" onClick={() => navigate('/matriculas/renovacao')}><RefreshCw className="h-4 w-4" /> Renovação</Button>
            <Button onClick={() => navigate('/matriculas/nova')}><Plus className="h-4 w-4" /> {pt.actions.newEnrolment}</Button>
          </>
        )}
      />

      <div className="mb-5 grid grid-cols-3 gap-3">
        <Stat label="Matriculados" value={counts.confirmed.data ?? '—'} />
        <Stat label="Rascunhos" value={counts.draft.data ?? '—'} />
        <Stat label="Desistências" value={counts.withdrawn.data ?? '—'} />
      </div>

      <Card>
        <div className="flex gap-1 overflow-x-auto border-b border-line px-3">
          {tabs.map((t) => (
            <button
              key={t.value}
              onClick={() => setParams({ status: t.value }, { replace: true })}
              className={cn(
                'whitespace-nowrap border-b-2 px-3 py-3 text-sm font-medium',
                status === t.value ? 'border-brand text-brand' : 'border-transparent text-ink-muted hover:text-navy',
              )}
            >
              {t.label}
              {counts[t.value].data !== undefined && <span className="ml-1.5 rounded-full bg-page px-1.5 text-xs">{counts[t.value].data}</span>}
            </button>
          ))}
        </div>

        {list.isError && <div className="p-4"><ErrorState error={list.error} onRetry={() => list.refetch()} /></div>}
        {list.isLoading && <div className="space-y-2 p-4">{Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-12" />)}</div>}
        {list.data && items.length === 0 && (
          <EmptyState
            icon={<ClipboardList className="h-6 w-6" />}
            title={status === 'draft' ? 'Sem matrículas por concluir' : status === 'confirmed' ? 'Ainda não há matrículas confirmadas' : 'Sem desistências'}
            description={status === 'draft' ? 'Uma matrícula interrompida fica aqui até ser confirmada ou apagada.' : undefined}
            action={status !== 'withdrawn' && writable && <Button onClick={() => navigate('/matriculas/nova')}><Plus className="h-4 w-4" /> {pt.actions.newEnrolment}</Button>}
          />
        )}

        {items.length > 0 && (
          <Table>
            <thead>
              <tr>
                <Th>Aluno</Th>
                <Th className="hidden md:table-cell">Turma</Th>
                <Th className="hidden md:table-cell">Tipo</Th>
                <Th className="hidden lg:table-cell">Encarregado</Th>
                <Th />
              </tr>
            </thead>
            <tbody>
              {items.map((r) => (
                <tr key={r.enrolment_id} className="hover:bg-page/70">
                  <Td>
                    <Link to={r.status === 'draft' && writable ? `/matriculas/${r.enrolment_id}` : `/alunos/${r.pupil_id}`} className="flex items-center gap-3">
                      <Avatar name={r.full_name} />
                      <div className="min-w-0">
                        <p className="truncate font-semibold text-navy">{r.full_name}</p>
                        <p className="truncate text-xs text-ink-muted">{r.number} · {age(r.birth_date)} anos<span className="md:hidden"> · {r.class_name ?? 'sem turma'}</span></p>
                      </div>
                    </Link>
                  </Td>
                  <Td className="hidden md:table-cell">{r.class_name ?? <span className="text-ink-muted">Por escolher</span>}</Td>
                  <Td className="hidden md:table-cell"><Badge tone={r.type === 'new' ? 'info' : 'violet'}>{r.type === 'new' ? 'Novo' : 'Renovação'}</Badge></Td>
                  <Td className="hidden lg:table-cell">
                    <p>{r.payer_name ?? <span className="text-ink-muted">—</span>}</p>
                    <p className="text-xs text-ink-muted">{r.payer_phone}</p>
                  </Td>
                  <Td align="right">
                    {r.status === 'draft' && writable ? (
                      <div className="flex justify-end gap-1">
                        <Button size="sm" variant="ghost" aria-label="Apagar rascunho" onClick={() => setToDelete(r)}><Trash2 className="h-4 w-4" /></Button>
                        <Button size="sm" variant="secondary" onClick={() => navigate(`/matriculas/${r.enrolment_id}`)}>Continuar <ArrowRight className="h-4 w-4" /></Button>
                      </div>
                    ) : (
                      <Button size="sm" variant="ghost" onClick={() => navigate(`/alunos/${r.pupil_id}`)}>Ver <ArrowRight className="h-4 w-4" /></Button>
                    )}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent title="Apagar rascunho?" description={toDelete?.full_name}>
          <div className="space-y-4 p-5">
            <p className="text-sm text-ink">A matrícula em rascunho é apagada. O registo do aluno, os encarregados e a saúde ficam guardados para uma próxima matrícula.</p>
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setToDelete(null)}>{pt.actions.cancel}</Button>
              <Button variant="danger" loading={del.isPending} onClick={() => toDelete && del.mutate(toDelete.enrolment_id)}>Apagar rascunho</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
