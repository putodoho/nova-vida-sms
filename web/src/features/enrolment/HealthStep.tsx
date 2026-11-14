import * as React from 'react'
import { api } from '@/lib/api'
import { Field, Input, Textarea } from '@/components/ui'
import type { Health, PupilProfile } from '@/api/types'
import { type Errors, handleError, Section, Segmented, StepFooter } from './shared'

const questions = [
  { key: 'chronic_condition', label: 'Sofre de alguma doença crónica?', placeholder: 'Qual? Ex.: asma' },
  { key: 'allergies', label: 'Tem alguma alergia?', placeholder: 'A quê? Ex.: amendoim, penicilina' },
  { key: 'diet_restrictions', label: 'Tem alguma restrição alimentar?', placeholder: 'Qual? Ex.: não come carne de porco' },
  { key: 'medication', label: 'Toma alguma medicação?', placeholder: 'Qual e quando?' },
] as const
type QKey = (typeof questions)[number]['key']

export const emptyHealth: Health = {
  chronic_condition: '', allergies: '', diet_restrictions: '', medication: '', doctor_name: '', doctor_phone: '', notes: '', emergency_care_consent_at: null,
}

/** Save the health record; the emergency-care consent is kept unless given. */
export function putHealth(pupilId: string, h: Health, consent?: boolean) {
  return api.put<PupilProfile>(`/pupils/${pupilId}/health`, { ...h, emergency_care_consent: consent ?? !!h.emergency_care_consent_at })
}

/** Step 2. Each condition is a Yes/No with a "which?" text, as on the paper form. */
export function HealthStep({ pupilId, health, onBack, onSaved }: {
  pupilId: string; health: Health | null; onBack: () => void; onSaved: () => void
}) {
  const h = health ?? emptyHealth
  const [f, setF] = React.useState<Health>(h)
  const [yes, setYes] = React.useState<Record<QKey, boolean>>(() =>
    Object.fromEntries(questions.map((q) => [q.key, !!h[q.key]])) as Record<QKey, boolean>,
  )
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)
  const set = (k: keyof Health, v: string) => {
    setF((s) => ({ ...s, [k]: v }))
    setErrors((e) => ({ ...e, [k]: '' }))
  }

  async function save() {
    const e: Errors = {}
    for (const q of questions) if (yes[q.key] && !f[q.key].trim()) e[q.key] = 'Indique qual.'
    if (Object.keys(e).length) return setErrors(e)
    const body = { ...f }
    for (const q of questions) if (!yes[q.key]) body[q.key] = ''
    setSaving(true)
    try {
      await putHealth(pupilId, body)
      onSaved()
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div className="space-y-6 p-5">
        <Section title="Saúde" description="Alergias, dieta e medicação aparecem em destaque na ficha do aluno.">
          <div className="divide-y divide-line rounded-lg border border-line">
            {questions.map((q) => (
              <div key={q.key} className="space-y-3 p-4">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <p className="text-sm font-medium text-ink">{q.label}</p>
                  <Segmented
                    value={yes[q.key] ? 'yes' : 'no'}
                    onChange={(v) => setYes((s) => ({ ...s, [q.key]: v === 'yes' }))}
                    options={[{ value: 'no', label: 'Não' }, { value: 'yes', label: 'Sim' }]}
                    className="w-full sm:w-40"
                  />
                </div>
                {yes[q.key] && (
                  <Field label="" error={errors[q.key]}>
                    <Input autoFocus value={f[q.key]} placeholder={q.placeholder} invalid={!!errors[q.key]} onChange={(e) => set(q.key, e.target.value)} />
                  </Field>
                )}
              </div>
            ))}
          </div>
        </Section>
        <Section title="Médico da criança">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Nome" htmlFor="doctor_name"><Input id="doctor_name" value={f.doctor_name} onChange={(e) => set('doctor_name', e.target.value)} /></Field>
            <Field label="Telefone" htmlFor="doctor_phone"><Input id="doctor_phone" inputMode="tel" value={f.doctor_phone} onChange={(e) => set('doctor_phone', e.target.value)} /></Field>
            <Field label="Outras observações" className="md:col-span-2" htmlFor="notes"><Textarea id="notes" value={f.notes} onChange={(e) => set('notes', e.target.value)} /></Field>
          </div>
        </Section>
      </div>
      <StepFooter onBack={onBack} onNext={save} loading={saving} />
    </>
  )
}
