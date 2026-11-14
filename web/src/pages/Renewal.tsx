import * as React from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowLeft, ArrowRight, CheckCircle2, RefreshCw, XCircle } from 'lucide-react'
import { api } from '@/lib/api'
import { age } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Badge, Button, Card, CardHeader, EmptyState, ErrorState, Field, PageHeader, Select, Skeleton, Table, Td, Th } from '@/components/ui'
import { Checkbox, handleError } from '@/features/enrolment/shared'
import type { ClassItem, EnrolmentListItem, List, RenewResult } from '@/api/types'

/** All pages of an enrolments query (lists are capped at 200 per page). */
async function allEnrolments(entity: string, query: Record<string, string | number>) {
  const out: EnrolmentListItem[] = []
  for (let page = 1; page < 50; page++) {
    const r = await api.getAs<List<EnrolmentListItem>>(entity, '/enrolments', { ...query, page, page_size: 200 })
    out.push(...r.items)
    if (out.length >= r.total || r.items.length === 0) break
  }
  return out
}

/**
 * Re-enrolment (brief 7.6): last year's pupils of one class → next year's
 * class, services carried over, re-enrolment fee added. The source can be the
 * other school, so the last preschool class can move into the 1st grade.
 */
export function RenewalPage() {
  const { entity, entities, years, year: sessionYear, can } = useSession()
  const qc = useQueryClient()
  const targets = years.filter((y) => y.status !== 'closed' && years.some((p) => p.year === y.year - 1))
  // The years list may still be loading on first render: derive the default.
  const [pickedYear, setTargetYear] = React.useState<number | null>(null)
  const targetYear = pickedYear ?? (targets.find((y) => y.year === sessionYear?.year) ?? targets[targets.length - 1])?.year ?? null
  const target = years.find((y) => y.year === targetYear) ?? null
  const sourceYear = target ? target.year - 1 : null
  const [sourceEntity, setSourceEntity] = React.useState(entity?.id ?? '')
  const [sourceClass, setSourceClass] = React.useState('')
  const [targetClass, setTargetClass] = React.useState('')
  const [selected, setSelected] = React.useState<string[]>([])
  const [confirmNow, setConfirmNow] = React.useState(true)
  const [busy, setBusy] = React.useState(false)
  const [results, setResults] = React.useState<RenewResult[] | null>(null)

  const sourceClasses = useQuery({
    queryKey: ['classes', sourceEntity, sourceYear],
    queryFn: () => api.getAs<List<ClassItem>>(sourceEntity, '/classes', { year: sourceYear! }),
    enabled: !!sourceEntity && !!sourceYear,
  })
  const targetClasses = useQuery({
    queryKey: ['classes', entity?.id, target?.year],
    queryFn: () => api.get<List<ClassItem>>('/classes', { year: target!.year }),
    enabled: !!entity && !!target,
  })
  const pupils = useQuery({
    queryKey: ['enrolments', sourceEntity, sourceYear, 'renewal-source', sourceClass],
    queryFn: () => allEnrolments(sourceEntity, { year: sourceYear!, class_id: sourceClass, status: 'confirmed' }),
    enabled: !!sourceClass && !!sourceYear,
  })
  // Who already has an enrolment (any status) next year in this school.
  const alreadyQ = useQuery({
    queryKey: ['enrolments', entity?.id, target?.year, 'renewal-target'],
    queryFn: () => allEnrolments(entity!.id, { year: target!.year }),
    enabled: !!entity && !!target,
  })
  const alreadyMap = React.useMemo(() => alreadyQ.data && new Map(alreadyQ.data.map((i) => [i.pupil_id, i])), [alreadyQ.data])
  const already = { data: alreadyMap, isLoading: alreadyQ.isLoading }

  // Select everyone not yet re-enrolled when the class list arrives.
  React.useEffect(() => {
    if (pupils.data && alreadyMap) setSelected(pupils.data.filter((p) => !alreadyMap.has(p.pupil_id)).map((p) => p.pupil_id))
  }, [pupils.data, alreadyMap])

  if (!entity || !can('enrolments.write')) {
    return <Card><EmptyState title="Escolha a escola" description="A renovação é feita para uma escola de cada vez. Escolha a Primária ou o Jardim no topo do ecrã." /></Card>
  }

  const tc = targetClasses.data?.items.find((c) => c.id === targetClass)
  const seatsLeft = tc ? tc.capacity - tc.enrolled : 0
  const rows = pupils.data ?? []
  const pending = rows.filter((p) => !already.data?.has(p.pupil_id))
  const overCapacity = confirmNow && !!tc && selected.length > seatsLeft

  async function submit() {
    if (!target || !targetClass || selected.length === 0) return
    setBusy(true)
    try {
      const r = await api.post<{ results: RenewResult[] }>('/enrolments/renew', {
        school_year_id: target.id,
        confirm: confirmNow,
        items: selected.map((id) => ({ pupil_id: id, class_id: targetClass })),
      })
      setResults(r.results)
      const ok = r.results.filter((x) => x.status !== 'error').length
      if (ok) toast.success(`${ok} ${ok === 1 ? 'aluno renovado' : 'alunos renovados'}.`)
      void qc.invalidateQueries({ queryKey: ['enrolments'] })
      void qc.invalidateQueries({ queryKey: ['classes'] })
    } catch (err) {
      handleError(err)
    } finally {
      setBusy(false)
    }
  }

  const header = (
    <>
      <Link to="/matriculas" className="mb-3 inline-flex items-center gap-1 text-sm font-medium text-ink-muted hover:text-navy"><ArrowLeft className="h-4 w-4" /> {pt.nav.enrolments}</Link>
      <PageHeader title="Renovação de matrículas" description={target ? `Passar os alunos de ${sourceYear} para ${target.year} · ${entity.short_name}` : entity.short_name} />
    </>
  )

  if (!target) {
    return (
      <>
        {header}
        <Card><EmptyState icon={<RefreshCw className="h-6 w-6" />} title="Ainda não há ano lectivo seguinte" description="Para renovar, crie o novo ano lectivo, as suas turmas e a tabela de preços nas configurações." /></Card>
      </>
    )
  }

  if (results) {
    const byId = new Map(rows.map((r) => [r.pupil_id, r]))
    return (
      <>
        {header}
        <Card>
          <CardHeader title="Resultado da renovação" description={`${tc?.name} · ${target.year}`} action={<Button variant="secondary" onClick={() => { setResults(null); void pupils.refetch() }}>Renovar outra turma</Button>} />
          <ul className="divide-y divide-line">
            {results.map((r) => (
              <li key={r.pupil_id} className="flex items-start gap-3 px-5 py-3">
                {r.status === 'error' ? <XCircle className="mt-0.5 h-5 w-5 shrink-0 text-bad" /> : <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-ok" />}
                <div className="min-w-0 flex-1">
                  <p className="font-medium text-navy">{byId.get(r.pupil_id)?.full_name ?? r.pupil_id}</p>
                  {r.error && <p className="text-sm text-bad">{r.error}</p>}
                </div>
                {r.status === 'confirmed' && <Badge tone="ok">Matriculado</Badge>}
                {r.status === 'draft' && <Link to={`/matriculas/${r.enrolment_id}`}><Badge tone="warn">Rascunho · abrir</Badge></Link>}
              </li>
            ))}
          </ul>
        </Card>
      </>
    )
  }

  return (
    <>
      {header}
      <div className="grid gap-5 lg:grid-cols-[1fr_340px]">
        <Card>
          <div className="grid gap-3 border-b border-line p-4 sm:grid-cols-3">
            {targets.length > 1 && (
              <Field label="Ano de destino">
                <Select value={targetYear ?? ''} onChange={(e) => { setTargetYear(Number(e.target.value)); setSourceClass(''); setTargetClass('') }}>
                  {targets.map((y) => <option key={y.id} value={y.year}>{y.year}</option>)}
                </Select>
              </Field>
            )}
            {entities.length > 1 && (
              <Field label={`Escola em ${sourceYear}`}>
                <Select value={sourceEntity} onChange={(e) => { setSourceEntity(e.target.value); setSourceClass('') }}>
                  {entities.map((x) => <option key={x.id} value={x.id}>{x.short_name}</option>)}
                </Select>
              </Field>
            )}
            <Field label={`Turma em ${sourceYear}`}>
              <Select value={sourceClass} onChange={(e) => setSourceClass(e.target.value)}>
                <option value="">Escolher…</option>
                {sourceClasses.data?.items.map((c) => <option key={c.id} value={c.id}>{c.name} ({c.enrolled})</option>)}
              </Select>
            </Field>
          </div>

          {!sourceClass && <EmptyState title="Escolha a turma de origem" description={`Mostra os alunos matriculados em ${sourceYear} e quem já foi renovado.`} />}
          {pupils.isError && <div className="p-4"><ErrorState error={pupils.error} onRetry={() => pupils.refetch()} /></div>}
          {sourceClass && (pupils.isLoading || already.isLoading) && <div className="space-y-2 p-4">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10" />)}</div>}
          {pupils.data && already.data && (
            rows.length === 0 ? <EmptyState title="Turma sem alunos matriculados" /> : (
              <Table>
                <thead>
                  <tr>
                    <Th className="w-10">
                      <input
                        type="checkbox"
                        aria-label="Seleccionar todos"
                        className="h-4 w-4 accent-brand"
                        checked={pending.length > 0 && selected.length === pending.length}
                        onChange={(e) => setSelected(e.target.checked ? pending.map((p) => p.pupil_id) : [])}
                      />
                    </Th>
                    <Th>Aluno</Th>
                    <Th>Em {target.year}</Th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((p) => {
                    const next = alreadyMap?.get(p.pupil_id)
                    return (
                      <tr key={p.pupil_id} className={cn(next && 'bg-page/60')}>
                        <Td>
                          <input
                            type="checkbox"
                            aria-label={p.full_name}
                            className="h-4 w-4 accent-brand"
                            disabled={!!next}
                            checked={selected.includes(p.pupil_id)}
                            onChange={(e) => setSelected((s) => (e.target.checked ? [...s, p.pupil_id] : s.filter((x) => x !== p.pupil_id)))}
                          />
                        </Td>
                        <Td>
                          <Link to={`/alunos/${p.pupil_id}`} className="font-medium text-navy hover:underline">{p.full_name}</Link>
                          <p className="text-xs text-ink-muted">{p.number} · {age(p.birth_date)} anos</p>
                        </Td>
                        <Td>
                          {next ? (
                            <Badge tone={next.status === 'confirmed' ? 'ok' : next.status === 'draft' ? 'warn' : 'neutral'}>
                              {pt.enrolmentStatus[next.status]}{next.class_name && ` · ${next.class_name}`}
                            </Badge>
                          ) : <span className="text-sm text-ink-muted">Por renovar</span>}
                        </Td>
                      </tr>
                    )
                  })}
                </tbody>
              </Table>
            )
          )}
        </Card>

        <Card className="h-fit p-5 lg:sticky lg:top-4">
          <p className="text-[15px] font-semibold text-navy">Destino</p>
          <div className="mt-4 space-y-4">
            <Field label={`Turma em ${target.year}`}>
              <Select value={targetClass} onChange={(e) => setTargetClass(e.target.value)}>
                <option value="">Escolher…</option>
                {targetClasses.data?.items.map((c) => (
                  <option key={c.id} value={c.id}>{c.name} · {Math.max(0, c.capacity - c.enrolled)} vagas</option>
                ))}
              </Select>
            </Field>
            <div className="rounded-lg bg-page p-3 text-xs text-ink-muted">
              Os serviços opcionais (ex.: almoço) passam para o novo ano e é cobrada a taxa de renovação, se existir na tabela de preços.
            </div>
            <Checkbox
              checked={confirmNow}
              onChange={setConfirmNow}
              label="Confirmar já as matrículas"
              hint="O encarregado assinou a renovação em papel. Sem isto ficam em rascunho para confirmar uma a uma."
            />
            {overCapacity && <p className="text-sm text-bad">A turma só tem {seatsLeft} {seatsLeft === 1 ? 'vaga' : 'vagas'}; os alunos a mais ficam com erro.</p>}
            <Button className="w-full justify-center" disabled={!targetClass || selected.length === 0} loading={busy} onClick={submit}>
              Renovar {selected.length} {selected.length === 1 ? 'aluno' : 'alunos'} <ArrowRight className="h-4 w-4" />
            </Button>
          </div>
        </Card>
      </div>
    </>
  )
}
