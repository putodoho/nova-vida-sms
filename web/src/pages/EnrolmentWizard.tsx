import * as React from 'react'
import { Link, Navigate, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { CheckCircle2, LogOut, Plus, Printer, UserRound, Wallet } from 'lucide-react'
import { api, openDocument } from '@/lib/api'
import { money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { Button, Card, EmptyState, ErrorState, PageHeader, Skeleton } from '@/components/ui'
import { usePaymentSheet } from '@/features/payments/PaymentSheet'
import { PupilStep, pupilForm } from '@/features/enrolment/PupilStep'
import { HealthStep } from '@/features/enrolment/HealthStep'
import { GuardiansStep } from '@/features/enrolment/GuardiansStep'
import { PickupStep } from '@/features/enrolment/PickupStep'
import { ClassStep } from '@/features/enrolment/ClassStep'
import { FeesStep } from '@/features/enrolment/FeesStep'
import { ReviewStep } from '@/features/enrolment/ReviewStep'
import { handleError, type StepKey, Stepper, steps } from '@/features/enrolment/shared'
import type { Enrolment, PupilProfile } from '@/api/types'

type Mode = 'new' | 'draft' | 'edit'

/**
 * Enrolment wizard (brief 7.5). Every step saves on "Seguinte", so a draft can
 * be left and resumed: the pupil exists after step 1, the enrolment draft after
 * step 5. The URL carries the pupil / enrolment and the step, so a reload
 * resumes where the receptionist was. In "edit" mode only the pupil steps
 * (1–4) are shown, to correct a pupil's data from the profile.
 */
export function EnrolmentWizardPage({ mode }: { mode: Mode }) {
  const { id: routeId } = useParams()
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const payment = usePaymentSheet()
  const { entity, year, can } = useSession()

  const enrolmentId = mode === 'draft' ? routeId ?? null : null
  const enrolment = useQuery({
    queryKey: ['enrolment', enrolmentId],
    queryFn: () => api.get<Enrolment>(`/enrolments/${enrolmentId}`),
    enabled: !!enrolmentId,
  })
  const pupilId = mode === 'edit' ? routeId ?? null : mode === 'draft' ? enrolment.data?.pupil_id ?? null : params.get('pupil')
  const profile = useQuery({
    queryKey: ['pupil', pupilId],
    queryFn: () => api.get<PupilProfile>(`/pupils/${pupilId}`),
    enabled: !!pupilId,
  })

  const list = steps.filter((s) => (s.key !== 'health' || can('health.read')) && (mode !== 'edit' || ['pupil', 'health', 'guardians', 'pickup'].includes(s.key)))
  // A draft opened from the list resumes at the review (or the class, if none was chosen yet).
  const resumeAt: StepKey = mode === 'draft' && enrolment.data ? (enrolment.data.class_id ? 'review' : 'class') : 'pupil'
  const step = (list.find((s) => s.key === params.get('step'))?.key ?? resumeAt) as StepKey
  const [type, setType] = React.useState<'new' | 'returning' | null>(null)
  const [done, setDone] = React.useState<Enrolment | null>(null)
  const e = enrolment.data
  const p = profile.data

  const go = (k: StepKey, extra?: Record<string, string>) => {
    const next = new URLSearchParams(params)
    next.set('step', k)
    for (const [key, v] of Object.entries(extra ?? {})) next.set(key, v)
    setParams(next, { replace: true })
    window.scrollTo({ top: 0 })
  }
  const nextOf = (k: StepKey) => list[list.findIndex((s) => s.key === k) + 1]?.key
  const prevOf = (k: StepKey) => list[list.findIndex((s) => s.key === k) - 1]?.key
  const reachable = (k: StepKey) => {
    if (k === 'pupil') return true
    if (['health', 'guardians', 'pickup', 'class'].includes(k)) return !!pupilId
    return !!e
  }
  const refreshProfile = () => qc.invalidateQueries({ queryKey: ['pupil', pupilId] })

  // A pupil who already has an enrolment this year in this school continues it.
  const existing = p?.enrolments.find((x) => x.year === year?.year && x.entity_id === entity?.id)
  React.useEffect(() => {
    if (mode !== 'new' || !existing || done) return
    if (existing.status === 'draft') {
      toast.info('Este aluno já tem uma matrícula em rascunho; continue-a.')
      navigate(`/matriculas/${existing.id}`, { replace: true })
    } else if (existing.status === 'confirmed') {
      toast.error(`${p?.full_name} já está matriculado em ${year?.year}.`)
      navigate(`/alunos/${p?.id}`, { replace: true })
    }
  }, [mode, existing, done, navigate, p, year])

  async function pickExisting(id: string) {
    try {
      const prof = await qc.fetchQuery({ queryKey: ['pupil', id], queryFn: () => api.get<PupilProfile>(`/pupils/${id}`) })
      setType(prof.enrolments.some((x) => x.status === 'confirmed') ? 'returning' : 'new')
      go('pupil', { pupil: id })
    } catch (err) {
      handleError(err)
    }
  }

  if (!entity) {
    return (
      <Card>
        <EmptyState title="Escolha a escola" description="As matrículas são feitas numa escola de cada vez. Escolha a Primária ou o Jardim no topo do ecrã." />
      </Card>
    )
  }
  if (enrolment.isError) return <ErrorState error={enrolment.error} onRetry={() => enrolment.refetch()} />
  if (profile.isError) return <ErrorState error={profile.error} onRetry={() => profile.refetch()} />
  if (e && e.status !== 'draft' && !done) return <Navigate to={`/alunos/${e.pupil_id}`} replace />
  const loading = (!!enrolmentId && !e) || (!!pupilId && !p)

  const title = mode === 'edit' ? 'Editar dados do aluno' : mode === 'draft' ? 'Continuar matrícula' : pt.actions.newEnrolment
  const subtitle = p ? `${p.full_name} · ${p.number} · ${entity.short_name} ${year?.year}` : `${entity.short_name} · ano lectivo ${year?.year}`
  const effectiveType = type ?? e?.type ?? (p ? pupilForm(p).type : 'new')

  if (done && p) {
    return (
      <Card className="mx-auto max-w-xl">
        <div className="flex flex-col items-center px-6 py-10 text-center">
          <div className="rounded-full bg-ok-tint p-3 text-ok"><CheckCircle2 className="h-8 w-8" /></div>
          <h1 className="mt-4 text-xl font-bold text-navy">Matrícula confirmada</h1>
          <p className="mt-1 text-sm text-ink-muted">{p.full_name} ({p.number}) · {done.class_name} · {done.year}</p>
          <p className="mt-3 text-sm text-ink">{done.totals.months} mensalidades de <strong>{money(done.totals.monthly_gross)}</strong>{done.totals.one_off_gross > 0 && <> e taxas de <strong>{money(done.totals.one_off_gross)}</strong></>} foram lançadas na conta.</p>
          <div className="mt-6 grid w-full gap-2 sm:grid-cols-2">
            {can('payments.create') && (
              <Button variant="accent" className="justify-center" onClick={() => payment.open({ id: p.id, name: p.full_name, number: p.number, className: done.class_name })}>
                <Wallet className="h-4 w-4" /> {pt.actions.recordPayment}
              </Button>
            )}
            <Button variant="secondary" className="justify-center" onClick={() => openDocument(`/enrolments/${done.id}/form`).catch((x) => toast.error(x.message))}>
              <Printer className="h-4 w-4" /> Imprimir ficha
            </Button>
            <Button variant="secondary" className="justify-center" onClick={() => navigate(`/alunos/${p.id}`)}><UserRound className="h-4 w-4" /> Ver ficha do aluno</Button>
            <Button variant="secondary" className="justify-center" onClick={() => { setDone(null); setType(null); navigate('/matriculas/nova') }}><Plus className="h-4 w-4" /> Nova matrícula</Button>
          </div>
        </div>
      </Card>
    )
  }

  return (
    <>
      <PageHeader
        title={title}
        description={subtitle}
        actions={
          <Button variant="ghost" onClick={() => navigate(mode === 'edit' ? `/alunos/${routeId}` : '/matriculas')}>
            <LogOut className="h-4 w-4" /> {mode === 'edit' ? 'Sair' : 'Guardar e sair'}
          </Button>
        }
      />
      <Card>
        <div className="border-b border-line px-5 py-4">
          <Stepper list={list} current={step} reachable={reachable} onGo={(k) => go(k)} />
        </div>
        {loading ? (
          <div className="space-y-3 p-5"><Skeleton className="h-10" /><Skeleton className="h-10" /><Skeleton className="h-40" /></div>
        ) : (
          <WizardStep
            key={`${step}-${pupilId ?? 'new'}-${e?.id ?? ''}`}
            step={step}
            mode={mode}
            pupilId={pupilId}
            profile={p ?? null}
            enrolment={e ?? null}
            type={effectiveType}
            onPickExisting={mode === 'new' ? pickExisting : undefined}
            onBack={(k) => { const prev = prevOf(k); if (prev) go(prev) }}
            onPupilSaved={(id, t) => {
              setType(t)
              void refreshProfile()
              const next = nextOf('pupil')!
              if (!pupilId) {
                void qc.invalidateQueries({ queryKey: ['pupil', id] })
                go(next, { pupil: id })
              } else go(next)
            }}
            onStepSaved={(k) => {
              void refreshProfile()
              const next = nextOf(k)
              if (next) go(next)
              else {
                toast.success('Dados do aluno guardados.')
                navigate(`/alunos/${routeId}`)
              }
            }}
            onEnrolmentSaved={(k, saved) => {
              qc.setQueryData(['enrolment', saved.id], saved)
              void qc.invalidateQueries({ queryKey: ['enrolments'] })
              const next = nextOf(k)!
              if (mode === 'new') navigate(`/matriculas/${saved.id}?step=${next}`, { replace: true })
              else go(next)
            }}
            onConfirmed={(saved, pay) => {
              // Show the success screen before the cached draft turns "confirmed"
              // (a confirmed enrolment opened here otherwise redirects to the profile).
              setDone(saved)
              qc.setQueryData(['enrolment', saved.id], saved)
              toast.success('Matrícula confirmada.')
              if (pay && p) payment.open({ id: p.id, name: p.full_name, number: p.number, className: saved.class_name })
            }}
            onEdit={(k) => go(k)}
          />
        )}
      </Card>
      {mode !== 'edit' && <p className="mt-3 text-center text-xs text-ink-muted">Cada passo fica guardado ao avançar. Os rascunhos aparecem em <Link to="/matriculas" className="font-medium text-brand">Matrículas</Link>.</p>}
    </>
  )
}

function WizardStep(props: {
  step: StepKey
  mode: Mode
  pupilId: string | null
  profile: PupilProfile | null
  enrolment: Enrolment | null
  type: 'new' | 'returning'
  onPickExisting?: (id: string) => void
  onBack: (k: StepKey) => void
  onPupilSaved: (id: string, type: 'new' | 'returning') => void
  onStepSaved: (k: StepKey) => void
  onEnrolmentSaved: (k: StepKey, e: Enrolment) => void
  onConfirmed: (e: Enrolment, pay: boolean) => void
  onEdit: (k: StepKey) => void
}) {
  const { step, mode, pupilId, profile, enrolment, type } = props
  const navigate = useNavigate()
  const lastPupilStep = mode === 'edit' && step === 'pickup' ? 'Guardar' : undefined

  if (step === 'pupil') {
    return (
      <PupilStep
        pupilId={pupilId}
        initial={pupilForm(profile, type)}
        showType={mode !== 'edit' && !enrolment}
        onPickExisting={props.onPickExisting}
        onCancel={mode === 'new' && !pupilId ? () => navigate('/matriculas') : undefined}
        onSaved={(p, t) => props.onPupilSaved(p.id, t)}
      />
    )
  }
  if (!pupilId || !profile) return null
  switch (step) {
    case 'health':
      return <HealthStep pupilId={pupilId} health={profile.health} onBack={() => props.onBack('health')} onSaved={() => props.onStepSaved('health')} />
    case 'guardians':
      return <GuardiansStep pupilId={pupilId} links={profile.guardians} onBack={() => props.onBack('guardians')} onSaved={() => props.onStepSaved('guardians')} />
    case 'pickup':
      return <PickupStep pupilId={pupilId} profile={profile} nextLabel={lastPupilStep} onBack={() => props.onBack('pickup')} onSaved={() => props.onStepSaved('pickup')} />
    case 'class':
      return <ClassStep pupilId={pupilId} enrolment={enrolment} type={type} onBack={() => props.onBack('class')} onSaved={(e) => props.onEnrolmentSaved('class', e)} />
    case 'fees':
      return enrolment && <FeesStep pupilId={pupilId} enrolment={enrolment} health={profile.health} onBack={() => props.onBack('fees')} onSaved={(e) => props.onEnrolmentSaved('fees', e)} />
    case 'review':
      return enrolment && <ReviewStep enrolment={enrolment} profile={profile} onEdit={props.onEdit} onConfirmed={props.onConfirmed} />
    default:
      return null
  }
}
