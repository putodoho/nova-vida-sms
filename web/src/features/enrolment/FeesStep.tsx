import * as React from 'react'
import { date, money } from '@/lib/format'
import { useSession } from '@/lib/session'
import { ErrorState, Skeleton } from '@/components/ui'
import type { Enrolment, FeeItem, Health } from '@/api/types'
import { appliesTo, bodyFrom, saveEnrolment, useCatalogue } from './ClassStep'
import { putHealth } from './HealthStep'
import { Checkbox, type Errors, handleError, Section, StepFooter } from './shared'

// A fixed date already past is charged on the enrolment day.
function dueLabel(f: FeeItem, today: string) {
  if (f.due_rule?.date && f.due_rule.date > today) return `Vence a ${date(f.due_rule.date)}`
  return 'Vence na matrícula'
}

/** Step 6. One-off fees to charge and the consents of the enrolment form. */
export function FeesStep({ pupilId, enrolment, health, onBack, onSaved }: {
  pupilId: string; enrolment: Enrolment; health: Health | null; onBack: () => void; onSaved: (e: Enrolment) => void
}) {
  const { year, can, me } = useSession()
  const { fees } = useCatalogue()
  const base = bodyFrom(enrolment)
  const [oneOffs, setOneOffs] = React.useState<string[]>(base.one_off_fee_ids)
  const [consents, setConsents] = React.useState(base.consents)
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)

  const items = (fees.data?.items ?? []).filter((f) => f.kind === 'one_off' && appliesTo(f, enrolment.class_id))
  const total = items.filter((f) => oneOffs.includes(f.id)).reduce((s, f) => s + f.lines.gross, 0)
  const setConsent = (k: keyof typeof consents, v: boolean) => {
    setConsents((c) => ({ ...c, [k]: v }))
    setErrors({})
  }

  async function save() {
    if (!consents.fees_agreement) return setErrors({ 'consents.fees_agreement': 'Sem este acordo a matrícula não pode ser confirmada.' })
    if (!year) return
    setSaving(true)
    try {
      const e = await saveEnrolment(enrolment.id, pupilId, year.id, { ...base, one_off_fee_ids: oneOffs, consents })
      // The emergency-care consent also lives on the health record (with its date).
      if (health && can('health.read') && consents.emergency_care !== !!health.emergency_care_consent_at) {
        await putHealth(pupilId, health, consents.emergency_care)
      }
      onSaved(e)
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  if (fees.isError) return <div className="p-5"><ErrorState error={fees.error} onRetry={() => fees.refetch()} /></div>
  if (!fees.data) return <div className="p-5"><Skeleton className="h-40" /></div>

  return (
    <>
      <div className="space-y-8 p-5">
        <Section title="Taxas únicas" description="Escolha o que cobrar a este aluno. O uniforme pode ficar para depois.">
          {items.length === 0 ? (
            <p className="rounded-lg bg-page px-4 py-3 text-sm text-ink-muted">Não há taxas únicas na tabela de preços para esta turma.</p>
          ) : (
            <div className="divide-y divide-line rounded-lg border border-line">
              {items.map((f) => (
                <div key={f.id} className="flex items-center justify-between gap-3 px-3 py-2">
                  <Checkbox
                    checked={oneOffs.includes(f.id)}
                    disabled={f.mandatory}
                    onChange={(v) => setOneOffs((o) => (v ? [...o, f.id] : o.filter((x) => x !== f.id)))}
                    label={f.name}
                    hint={`${dueLabel(f, me.today)}${f.mandatory ? ' · obrigatório' : ''}`}
                  />
                  <p className="text-sm tabular-nums text-ink">{money(f.lines.gross)}</p>
                </div>
              ))}
              <div className="flex items-center justify-between bg-page/60 px-4 py-3">
                <p className="text-sm font-semibold text-navy">Total das taxas</p>
                <p className="text-sm font-bold tabular-nums text-navy">{money(total)}</p>
              </div>
            </div>
          )}
        </Section>

        <Section title="Consentimentos" description="Como na ficha de matrícula em papel; ficam impressos na ficha.">
          <div className="space-y-1 rounded-lg border border-line p-3">
            <Checkbox
              checked={consents.fees_agreement}
              onChange={(v) => setConsent('fees_agreement', v)}
              label={<><strong>Aceita pagar as {year?.billing_months.length} mensalidades</strong> do ano lectivo até ao dia {year?.due_day} de cada mês</>}
              hint="Obrigatório para confirmar a matrícula."
            />
            {errors['consents.fees_agreement'] && <p className="pl-8 text-xs text-bad">{errors['consents.fees_agreement']}</p>}
            <Checkbox checked={consents.uniform_books} onChange={(v) => setConsent('uniform_books', v)} label="Tem conhecimento de que o uniforme e os livros são obrigatórios" />
            <Checkbox checked={consents.emergency_care} onChange={(v) => setConsent('emergency_care', v)} label="Autoriza tratamento médico de urgência, se não for possível contactar a família" />
            <Checkbox checked={consents.image_use} onChange={(v) => setConsent('image_use', v)} label="Autoriza o uso de fotografias da criança nas redes sociais da escola" />
          </div>
        </Section>
      </div>
      <StepFooter onBack={onBack} onNext={save} loading={saving} />
    </>
  )
}
