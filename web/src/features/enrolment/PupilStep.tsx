import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import { UserCheck } from 'lucide-react'
import { api } from '@/lib/api'
import { age, date, parseDate } from '@/lib/format'
import { useSession } from '@/lib/session'
import { Badge, Field, Input, Select, Textarea } from '@/components/ui'
import type { List, Pupil, PupilProfile, PupilSearchHit } from '@/api/types'
import { type Errors, handleError, Section, Segmented, StepFooter } from './shared'

export interface PupilForm {
  full_name: string
  sex: 'F' | 'M' | ''
  birth: string // dd/mm/yyyy as typed
  neighbourhood: string
  religion: string
  orphan: 'none' | 'father' | 'mother' | 'both'
  home_discipline_notes: string
  type: 'new' | 'returning'
}

export function pupilForm(p?: PupilProfile | null, type?: 'new' | 'returning'): PupilForm {
  return {
    full_name: p?.full_name ?? '',
    sex: p?.sex ?? '',
    birth: p ? date(p.birth_date) : '',
    neighbourhood: p?.neighbourhood ?? '',
    religion: p?.religion ?? '',
    orphan: p?.orphan ?? 'none',
    home_discipline_notes: p?.home_discipline_notes ?? '',
    type: type ?? (p && p.enrolments.some((e) => e.status === 'confirmed') ? 'returning' : 'new'),
  }
}

/**
 * Step 1. Creates the pupil (so the number exists and the draft can be resumed)
 * or updates an existing one. While typing a new name, possible matches are
 * shown so a returning pupil or a sibling record is not duplicated.
 */
export function PupilStep({ pupilId, initial, showType, onPickExisting, onSaved, onCancel }: {
  pupilId: string | null
  initial: PupilForm
  showType: boolean
  onPickExisting?: (id: string) => void
  onSaved: (pupil: Pupil, type: 'new' | 'returning') => void
  onCancel?: () => void
}) {
  const { year, entity } = useSession()
  const [f, setF] = React.useState(initial)
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)
  const set = <K extends keyof PupilForm>(k: K, v: PupilForm[K]) => {
    setF((s) => ({ ...s, [k]: v }))
    setErrors((e) => ({ ...e, [k === 'birth' ? 'birth_date' : k]: '' }))
  }

  // Duplicate check while creating (a disabled query still returns cached data, hence the flag below).
  const checkDup = !pupilId && !!onPickExisting
  const [term, setTerm] = React.useState('')
  React.useEffect(() => {
    const t = setTimeout(() => setTerm(f.full_name.trim()), 350)
    return () => clearTimeout(t)
  }, [f.full_name])
  const matches = useQuery({
    queryKey: ['pupil-search', term],
    queryFn: () => api.get<List<PupilSearchHit>>('/pupils/search', { q: term }),
    enabled: checkDup && term.length >= 4,
  })

  const birthISO = parseDate(f.birth)

  async function save() {
    const e: Errors = {}
    if (f.full_name.trim().split(/\s+/).length < 2) e.full_name = 'Indique o nome completo.'
    if (!f.sex) e.sex = 'Obrigatório.'
    if (!birthISO) e.birth_date = 'Data inválida (dd/mm/aaaa).'
    else if (birthISO > new Date().toISOString().slice(0, 10)) e.birth_date = 'A data não pode ser futura.'
    if (Object.keys(e).length) return setErrors(e)

    const body = {
      full_name: f.full_name.trim().replace(/\s+/g, ' '),
      sex: f.sex,
      birth_date: birthISO,
      neighbourhood: f.neighbourhood,
      religion: f.religion,
      orphan: f.orphan,
      home_discipline_notes: f.home_discipline_notes,
    }
    setSaving(true)
    try {
      const p = pupilId
        ? await api.put<Pupil>(`/pupils/${pupilId}`, { ...body, status: 'active' })
        : await api.post<Pupil>('/pupils', { ...body, year: year?.year })
      onSaved(p, f.type)
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  const hits = checkDup && term.length >= 4 ? (matches.data?.items ?? []) : []

  return (
    <>
      <div className="space-y-6 p-5">
        <Section title="Dados do aluno">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Nome completo" error={errors.full_name} className="md:col-span-2" htmlFor="full_name">
              <Input id="full_name" autoFocus={!pupilId} value={f.full_name} invalid={!!errors.full_name} onChange={(e) => set('full_name', e.target.value)} autoComplete="off" />
            </Field>

            {hits.length > 0 && (
              <div className="rounded-lg border border-warn/30 bg-warn-tint p-3 md:col-span-2">
                <p className="text-sm font-semibold text-warn">Já existe um aluno com nome parecido</p>
                <p className="text-xs text-ink-muted">Se for o mesmo aluno, use o registo existente para não duplicar.</p>
                <ul className="mt-2 divide-y divide-warn/20">
                  {hits.slice(0, 4).map((h) => (
                    <li key={h.id} className="flex items-center gap-3 py-2">
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium text-navy">{h.full_name}</p>
                        <p className="truncate text-xs text-ink-muted">
                          {h.number} · {date(h.birth_date)}
                          {h.last_year && ` · ${h.last_class ?? '—'} (${h.last_year})`}
                          {h.payer_name && ` · ${h.payer_name}`}
                        </p>
                      </div>
                      <button type="button" className="inline-flex items-center gap-1 text-sm font-semibold text-brand hover:underline" onClick={() => onPickExisting?.(h.id)}>
                        <UserCheck className="h-4 w-4" /> Usar
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            <Field label="Sexo" error={errors.sex}>
              <Segmented value={f.sex} onChange={(v) => set('sex', v)} options={[{ value: 'F', label: 'Feminino' }, { value: 'M', label: 'Masculino' }]} className="w-full" />
            </Field>
            <Field label="Data de nascimento" error={errors.birth_date} hint={birthISO ? `${age(birthISO)} anos` : 'dd/mm/aaaa'} htmlFor="birth">
              <Input id="birth" inputMode="numeric" placeholder="dd/mm/aaaa" value={f.birth} invalid={!!errors.birth_date} onChange={(e) => set('birth', e.target.value)} />
            </Field>
            <Field label="Bairro" htmlFor="neighbourhood">
              <Input id="neighbourhood" value={f.neighbourhood} onChange={(e) => set('neighbourhood', e.target.value)} />
            </Field>
            <Field label="Religião (opcional)" htmlFor="religion">
              <Input id="religion" value={f.religion} onChange={(e) => set('religion', e.target.value)} />
            </Field>
            <Field label="Orfandade" htmlFor="orphan">
              <Select id="orphan" value={f.orphan} onChange={(e) => set('orphan', e.target.value as PupilForm['orphan'])}>
                <option value="none">Não</option>
                <option value="father">Órfão de pai</option>
                <option value="mother">Órfão de mãe</option>
                <option value="both">Órfão de pai e mãe</option>
              </Select>
            </Field>
            {showType && (
              <Field label="Tipo de matrícula">
                <Segmented value={f.type} onChange={(v) => set('type', v)} options={[{ value: 'new', label: 'Aluno novo' }, { value: 'returning', label: 'Já estudou cá' }]} className="w-full" />
              </Field>
            )}
          </div>
        </Section>

        {entity?.code === 'preschool' && (
          <Section title="Em casa" description="A ficha do Jardim pergunta como a criança é corrigida em casa.">
            <Textarea value={f.home_discipline_notes} onChange={(e) => set('home_discipline_notes', e.target.value)} placeholder="Ex.: conversa, castigo sem televisão…" />
          </Section>
        )}

        {pupilId && <p className="text-xs text-ink-muted"><Badge>Registo existente</Badge> As alterações ficam no cadastro do aluno.</p>}
      </div>
      <StepFooter onBack={onCancel} onNext={save} loading={saving} />
    </>
  )
}
