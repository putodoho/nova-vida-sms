import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Badge, EmptyState, ErrorState, Field, Select, Skeleton } from '@/components/ui'
import type { ClassItem, Enrolment, FeeItem, List } from '@/api/types'
import { Checkbox, type Errors, handleError, Section, StepFooter } from './shared'

export interface EnrolmentBody {
  class_id: string | null
  type: 'new' | 'returning'
  start_month: number
  monthly_fee_ids: string[]
  one_off_fee_ids: string[]
  consents: Enrolment['consents']
}

export function bodyFrom(e: Enrolment): EnrolmentBody {
  return {
    class_id: e.class_id,
    type: e.type,
    start_month: e.start_month,
    monthly_fee_ids: e.services.filter((s) => !s.mandatory && s.to_month === null).map((s) => s.fee_item_id),
    one_off_fee_ids: e.one_off_fee_ids,
    consents: e.consents,
  }
}

/** Save the draft: create it on the first save, update afterwards. */
export function saveEnrolment(id: string | null, pupilId: string, schoolYearId: string, body: EnrolmentBody) {
  return id
    ? api.put<Enrolment>(`/enrolments/${id}`, body)
    : api.post<Enrolment>('/enrolments', { ...body, pupil_id: pupilId, school_year_id: schoolYearId })
}

export const appliesTo = (f: FeeItem, classId: string | null) => f.active && (f.class_ids.length === 0 || (!!classId && f.class_ids.includes(classId)))

export function useCatalogue() {
  const { year, entity } = useSession()
  const classes = useQuery({
    queryKey: ['classes', entity?.id, year?.year],
    queryFn: () => api.get<List<ClassItem>>('/classes', { year: year?.year }),
    enabled: !!year && !!entity,
  })
  const fees = useQuery({
    queryKey: ['fee-items', entity?.id, year?.year],
    queryFn: () => api.get<List<FeeItem>>('/fee-items', { year: year?.year }),
    enabled: !!year && !!entity,
  })
  return { classes, fees }
}

/** One-off items picked by default for a new draft: mandatory ones, the matching enrolment fee and the class's books. */
export function defaultOneOffs(fees: FeeItem[], classId: string, type: 'new' | 'returning'): string[] {
  return fees
    .filter((f) => f.kind === 'one_off' && appliesTo(f, classId))
    .filter((f) => f.mandatory || f.code === (type === 'new' ? 'enrolment' : 're_enrolment') || (f.code.startsWith('books') && f.class_ids.length > 0))
    .map((f) => f.id)
}

/** First billing month not yet past on the school's date. */
function defaultStartMonth(months: number[], yearNum: number, today: string) {
  const [y, m] = today.split('-').map(Number)
  if (y < yearNum) return months[0]
  if (y > yearNum) return months[months.length - 1]
  return months.find((x) => x >= m) ?? months[months.length - 1]
}

/** Step 5. Class (with seats left), first billing month and optional monthly services. */
export function ClassStep({ pupilId, enrolment, type, onBack, onSaved }: {
  pupilId: string; enrolment: Enrolment | null; type: 'new' | 'returning'; onBack: () => void; onSaved: (e: Enrolment) => void
}) {
  const { year, me } = useSession()
  const { classes, fees } = useCatalogue()
  const months = year?.billing_months ?? []
  const [classId, setClassId] = React.useState<string | null>(enrolment?.class_id ?? null)
  const [start, setStart] = React.useState<number>(() => enrolment?.start_month ?? defaultStartMonth(months, year?.year ?? 0, me.today))
  const [optional, setOptional] = React.useState<string[]>(() => (enrolment ? bodyFrom(enrolment).monthly_fee_ids : []))
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)

  const allFees = fees.data?.items ?? []
  const monthly = allFees.filter((f) => f.kind === 'monthly' && appliesTo(f, classId))
  const mandatory = monthly.filter((f) => f.mandatory)
  const extras = monthly.filter((f) => !f.mandatory)
  const chosen = [...mandatory, ...extras.filter((f) => optional.includes(f.id))]
  const monthlyGross = chosen.reduce((s, f) => s + f.lines.gross, 0)
  const nMonths = months.filter((m) => m >= start).length

  const pickClass = (id: string) => {
    setClassId(id)
    setErrors({})
    // Keep only services that exist for the new class.
    setOptional((o) => o.filter((fid) => allFees.some((f) => f.id === fid && appliesTo(f, id))))
  }

  async function save() {
    if (!classId) return setErrors({ class_id: 'Escolha a turma.' })
    if (!year) return
    const base = enrolment ? bodyFrom(enrolment) : null
    const classChanged = base?.class_id !== classId
    const body: EnrolmentBody = {
      class_id: classId,
      type,
      start_month: start,
      monthly_fee_ids: optional,
      one_off_fee_ids: !base || classChanged || base.type !== type
        ? defaultOneOffs(allFees, classId, type)
        : base.one_off_fee_ids,
      consents: base?.consents ?? { image_use: false, fees_agreement: false, uniform_books: false, emergency_care: false },
    }
    setSaving(true)
    try {
      onSaved(await saveEnrolment(enrolment?.id ?? null, pupilId, year.id, body))
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  if (classes.isError) return <div className="p-5"><ErrorState error={classes.error} onRetry={() => classes.refetch()} /></div>
  if (!classes.data || !fees.data) return <div className="space-y-3 p-5"><Skeleton className="h-24" /><Skeleton className="h-40" /></div>
  if (classes.data.items.length === 0) {
    return <EmptyState title={`Não há turmas para ${year?.year}`} description="Crie as turmas do ano lectivo nas configurações antes de matricular." />
  }

  return (
    <>
      <div className="space-y-8 p-5">
        <Section title={`Turma · ano lectivo ${year?.year}`}>
          {errors.class_id && <p className="rounded-lg bg-bad-tint px-3 py-2 text-sm text-bad">{errors.class_id}</p>}
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {classes.data.items.map((c) => {
              const left = c.capacity - c.enrolled
              // A confirmed pupil keeps their seat; a draft does not hold one.
              const full = left <= 0 && c.id !== classId
              const selected = c.id === classId
              return (
                <button
                  key={c.id}
                  type="button"
                  disabled={full}
                  onClick={() => pickClass(c.id)}
                  className={cn(
                    'rounded-lg border p-4 text-left transition-colors',
                    selected ? 'border-brand bg-brand-tint ring-2 ring-brand/20' : 'border-line bg-white hover:border-brand/50',
                    full && 'cursor-not-allowed opacity-50 hover:border-line',
                  )}
                >
                  <div className="flex items-start justify-between gap-2">
                    <p className="font-semibold text-navy">{c.name}</p>
                    <Badge tone={left <= 0 ? 'bad' : left <= 3 ? 'warn' : 'ok'}>{left <= 0 ? 'Cheia' : `${left} ${left === 1 ? 'vaga' : 'vagas'}`}</Badge>
                  </div>
                  <p className="mt-1 text-xs text-ink-muted">{[c.room, c.teacher_name].filter(Boolean).join(' · ') || '—'}</p>
                  <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-line">
                    <div className={cn('h-full rounded-full', left <= 0 ? 'bg-bad' : left <= 3 ? 'bg-warn' : 'bg-ok')} style={{ width: `${Math.min(100, (c.enrolled / Math.max(1, c.capacity)) * 100)}%` }} />
                  </div>
                  <p className="mt-1 text-xs text-ink-muted">{c.enrolled} de {c.capacity} alunos</p>
                </button>
              )
            })}
          </div>
        </Section>

        {classId && (
          <div className="grid gap-6 lg:grid-cols-[1fr_300px]">
            <Section title="Serviços mensais">
              <Field label="Primeiro mês a cobrar" hint="Meses anteriores não são cobrados." className="max-w-xs">
                <Select value={start} onChange={(e) => setStart(Number(e.target.value))}>
                  {months.map((m) => <option key={m} value={m}>{pt.months[m]}</option>)}
                </Select>
              </Field>
              <div className="divide-y divide-line rounded-lg border border-line">
                {mandatory.length === 0 && <p className="p-4 text-sm text-bad">Esta turma não tem mensalidade na tabela de preços.</p>}
                {mandatory.map((f) => (
                  <div key={f.id} className="flex items-center justify-between gap-3 p-4">
                    <div>
                      <p className="text-sm font-medium text-ink">{f.name}</p>
                      <p className="text-xs text-ink-muted">Obrigatório{f.lines.vat > 0 && ` · inclui IVA ${money(f.lines.vat)}`}</p>
                    </div>
                    <p className="text-sm font-semibold tabular-nums text-navy">{money(f.lines.gross)}</p>
                  </div>
                ))}
                {extras.map((f) => (
                  <div key={f.id} className="flex items-center justify-between gap-3 px-3 py-2">
                    <Checkbox
                      checked={optional.includes(f.id)}
                      onChange={(v) => setOptional((o) => (v ? [...o, f.id] : o.filter((x) => x !== f.id)))}
                      label={f.name}
                      hint={f.lines.vat > 0 ? `inclui IVA ${money(f.lines.vat)}` : 'Opcional'}
                    />
                    <p className="text-sm tabular-nums text-ink">{money(f.lines.gross)}</p>
                  </div>
                ))}
              </div>
            </Section>
            <div className="h-fit rounded-card bg-navy p-5 text-white lg:sticky lg:top-4">
              <p className="text-xs font-semibold uppercase tracking-wide text-white/60">Por mês</p>
              <p className="mt-1 text-[26px] font-bold tabular-nums">{money(monthlyGross)}</p>
              <div className="my-4 h-px bg-white/15" />
              <p className="text-xs font-semibold uppercase tracking-wide text-white/60">No ano ({nMonths} {nMonths === 1 ? 'mês' : 'meses'})</p>
              <p className="mt-1 text-lg font-semibold tabular-nums">{money(monthlyGross * nMonths)}</p>
              <p className="mt-3 text-xs text-white/60">{pt.months[start]} a {pt.months[months[months.length - 1]]}. Taxas únicas no passo seguinte.</p>
            </div>
          </div>
        )}
      </div>
      <StepFooter onBack={onBack} onNext={save} loading={saving} />
    </>
  )
}
