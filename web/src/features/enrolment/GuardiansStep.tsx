import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import { Plus, Search, Trash2, UserPlus, Users } from 'lucide-react'
import { api } from '@/lib/api'
import { pt } from '@/lib/i18n/pt'
import { cn } from '@/lib/utils'
import { Badge, Button, Card, Field, Input, Select } from '@/components/ui'
import type { Guardian, GuardianLink, List } from '@/api/types'
import { Checkbox, type Errors, handleError, Section, StepFooter } from './shared'

interface Row {
  key: string
  guardian_id: string | null // null: new guardian
  original: Guardian | null
  full_name: string
  phone1: string
  phone2: string
  email: string
  neighbourhood: string
  relationship: 'mother' | 'father' | 'guardian' | 'other'
  is_payer: boolean
  receives_notices: boolean
  may_pick_up: boolean
  lives_with: boolean
}

let seq = 0
const newKey = () => `g${++seq}`

function fromLink(l: GuardianLink): Row {
  return {
    key: newKey(), guardian_id: l.guardian_id,
    original: { id: l.guardian_id, full_name: l.full_name, phone1: l.phone1, phone2: l.phone2, email: l.email, neighbourhood: l.neighbourhood, id_doc: '' },
    full_name: l.full_name, phone1: l.phone1, phone2: l.phone2, email: l.email, neighbourhood: l.neighbourhood,
    relationship: l.relationship as Row['relationship'], is_payer: l.is_payer, receives_notices: l.receives_notices, may_pick_up: l.may_pick_up, lives_with: l.lives_with,
  }
}

function blank(relationship: Row['relationship'], payer: boolean): Row {
  return {
    key: newKey(), guardian_id: null, original: null, full_name: '', phone1: '', phone2: '', email: '', neighbourhood: '',
    relationship, is_payer: payer, receives_notices: true, may_pick_up: true, lives_with: true,
  }
}

const changed = (r: Row) =>
  !!r.original && (['full_name', 'phone1', 'phone2', 'email', 'neighbourhood'] as const).some((k) => r[k].trim() !== (r.original![k] ?? ''))

/**
 * Step 3. Guardians are global: search first so siblings share the same
 * guardian (one family, one payer, family payments), then add new ones.
 */
export function GuardiansStep({ pupilId, links, onBack, onSaved, nextLabel }: {
  pupilId: string; links: GuardianLink[]; onBack: () => void; onSaved: () => void; nextLabel?: string
}) {
  const [rows, setRows] = React.useState<Row[]>(() => (links.length ? links.map(fromLink) : [blank('mother', true)]))
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)
  const [searching, setSearching] = React.useState(false)

  const update = (key: string, patch: Partial<Row>) => {
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : patch.is_payer ? { ...r, is_payer: false } : r)))
    setErrors({})
  }
  const remove = (key: string) => setRows((rs) => {
    const left = rs.filter((r) => r.key !== key)
    if (left.length && !left.some((r) => r.is_payer)) left[0] = { ...left[0], is_payer: true }
    return left
  })
  const addExisting = (g: Guardian) => {
    setRows((rs) => [...rs.filter((r) => r.full_name || r.guardian_id), {
      ...blank('guardian', !rs.some((r) => r.is_payer && (r.full_name || r.guardian_id))),
      guardian_id: g.id, original: g, full_name: g.full_name, phone1: g.phone1, phone2: g.phone2, email: g.email, neighbourhood: g.neighbourhood,
    }])
    setSearching(false)
  }

  async function save() {
    const e: Errors = {}
    if (rows.length === 0) e.items = 'Indique pelo menos um encarregado.'
    rows.forEach((r) => {
      if (r.full_name.trim().length < 3) e[`${r.key}.full_name`] = 'Nome obrigatório.'
      if (!r.phone1.trim()) e[`${r.key}.phone1`] = 'Telefone obrigatório.'
      if (r.email && !r.email.includes('@')) e[`${r.key}.email`] = 'Email inválido.'
    })
    if (rows.length && rows.filter((r) => r.is_payer).length !== 1) e.items = 'Escolha quem é responsável pelos pagamentos.'
    if (Object.keys(e).length) return setErrors(e)

    setSaving(true)
    try {
      // Contact changes on an existing guardian update that guardian (all their children).
      for (const r of rows.filter(changed)) {
        await api.put(`/guardians/${r.guardian_id}`, {
          full_name: r.full_name.trim(), phone1: r.phone1.trim(), phone2: r.phone2.trim(), email: r.email.trim(), neighbourhood: r.neighbourhood.trim(), id_doc: r.original?.id_doc ?? '',
        })
      }
      await api.put(`/pupils/${pupilId}/guardians`, rows.map((r) => ({
        guardian_id: r.guardian_id ?? undefined,
        guardian: r.guardian_id ? undefined : {
          full_name: r.full_name.trim(), phone1: r.phone1.trim(), phone2: r.phone2.trim(), email: r.email.trim(), neighbourhood: r.neighbourhood.trim(), id_doc: '',
        },
        relationship: r.relationship, is_payer: r.is_payer, receives_notices: r.receives_notices, may_pick_up: r.may_pick_up, lives_with: r.lives_with,
      })))
      onSaved()
    } catch (err) {
      handleError(err, (f) => setErrors({ items: Object.values(f).join(' ') }))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div className="space-y-6 p-5">
        <Section title="Encarregados de educação" description="Procure primeiro: se a família já tem outro filho na escola, use o mesmo encarregado.">
          {errors.items && <p className="rounded-lg bg-bad-tint px-3 py-2 text-sm text-bad">{errors.items}</p>}

          <div className="space-y-4">
            {rows.map((r, i) => (
              <Card key={r.key} className={cn('p-4 shadow-none', r.is_payer && 'border-brand/40 ring-1 ring-brand/20')}>
                <div className="mb-3 flex flex-wrap items-center gap-2">
                  <p className="font-semibold text-navy">Encarregado {i + 1}</p>
                  {r.guardian_id && <Badge tone="info">Já registado</Badge>}
                  {r.is_payer && <Badge tone="accent">Responsável pelos pagamentos</Badge>}
                  <button type="button" className="ml-auto rounded-md p-1.5 text-ink-muted hover:bg-bad-tint hover:text-bad" onClick={() => remove(r.key)} aria-label="Remover encarregado">
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
                <div className="grid gap-3 md:grid-cols-6">
                  <Field label="Parentesco" className="md:col-span-2">
                    <Select value={r.relationship} onChange={(e) => update(r.key, { relationship: e.target.value as Row['relationship'] })}>
                      {(['mother', 'father', 'guardian', 'other'] as const).map((k) => <option key={k} value={k}>{pt.relationship[k]}</option>)}
                    </Select>
                  </Field>
                  <Field label="Nome completo" error={errors[`${r.key}.full_name`]} className="md:col-span-4">
                    <Input value={r.full_name} invalid={!!errors[`${r.key}.full_name`]} onChange={(e) => update(r.key, { full_name: e.target.value })} />
                  </Field>
                  <Field label="Telefone" error={errors[`${r.key}.phone1`]} className="md:col-span-2">
                    <Input inputMode="tel" placeholder="84 123 4567" value={r.phone1} invalid={!!errors[`${r.key}.phone1`]} onChange={(e) => update(r.key, { phone1: e.target.value })} />
                  </Field>
                  <Field label="Telefone alternativo" className="md:col-span-2">
                    <Input inputMode="tel" value={r.phone2} onChange={(e) => update(r.key, { phone2: e.target.value })} />
                  </Field>
                  <Field label="Bairro" className="md:col-span-2">
                    <Input value={r.neighbourhood} onChange={(e) => update(r.key, { neighbourhood: e.target.value })} />
                  </Field>
                  <Field label="Email (opcional)" error={errors[`${r.key}.email`]} className="md:col-span-3">
                    <Input type="email" value={r.email} onChange={(e) => update(r.key, { email: e.target.value })} />
                  </Field>
                </div>
                {r.guardian_id && changed(r) && (
                  <p className="mt-2 text-xs text-warn">Os novos contactos ficam também nas fichas dos irmãos.</p>
                )}
                <div className="mt-3 grid gap-x-4 sm:grid-cols-2">
                  <label className="flex cursor-pointer items-center gap-3 px-1 py-1.5 text-sm">
                    <input type="radio" name="payer" className="h-4 w-4 accent-brand" checked={r.is_payer} onChange={() => update(r.key, { is_payer: true })} />
                    Responsável pelos pagamentos
                  </label>
                  <Checkbox checked={r.receives_notices} onChange={(v) => update(r.key, { receives_notices: v })} label="Recebe avisos e comunicados" />
                  <Checkbox checked={r.may_pick_up} onChange={(v) => update(r.key, { may_pick_up: v })} label="Pode ir buscar a criança" />
                  <Checkbox checked={r.lives_with} onChange={(v) => update(r.key, { lives_with: v })} label="A criança vive com este encarregado" />
                </div>
              </Card>
            ))}
          </div>

          {rows.length < 4 && (
            searching ? (
              <GuardianSearch onPick={addExisting} onClose={() => setSearching(false)} exclude={rows.map((r) => r.guardian_id).filter(Boolean) as string[]} />
            ) : (
              <div className="flex flex-wrap gap-2">
                <Button variant="secondary" onClick={() => setSearching(true)}><Search className="h-4 w-4" /> Procurar encarregado existente</Button>
                <Button variant="secondary" onClick={() => setRows((rs) => [...rs, blank(rs.some((r) => r.relationship === 'mother') ? 'father' : 'mother', rs.length === 0)])}>
                  <UserPlus className="h-4 w-4" /> Novo encarregado
                </Button>
              </div>
            )
          )}
        </Section>
      </div>
      <StepFooter onBack={onBack} onNext={save} loading={saving} nextLabel={nextLabel} />
    </>
  )
}

function GuardianSearch({ onPick, onClose, exclude }: { onPick: (g: Guardian) => void; onClose: () => void; exclude: string[] }) {
  const [q, setQ] = React.useState('')
  const [term, setTerm] = React.useState('')
  React.useEffect(() => {
    const t = setTimeout(() => setTerm(q.trim()), 250)
    return () => clearTimeout(t)
  }, [q])
  const res = useQuery({
    queryKey: ['guardian-search', term],
    queryFn: () => api.get<List<Guardian>>('/guardians', { q: term }),
    enabled: term.length >= 2,
  })
  const items = (res.data?.items ?? []).filter((g) => !exclude.includes(g.id))
  return (
    <Card className="p-4 shadow-none">
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted" />
          <Input autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder="Nome ou telefone do encarregado" className="pl-9" />
        </div>
        <Button variant="ghost" onClick={onClose}>{pt.actions.cancel}</Button>
      </div>
      {term.length >= 2 && (
        <ul className="mt-3 divide-y divide-line">
          {items.map((g) => (
            <li key={g.id}>
              <button type="button" onClick={() => onPick(g)} className="flex w-full items-center gap-3 rounded-md px-2 py-2.5 text-left hover:bg-page">
                <Users className="h-4 w-4 shrink-0 text-ink-muted" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium text-navy">{g.full_name}</span>
                  <span className="block truncate text-xs text-ink-muted">{[g.phone1, g.phone2, g.neighbourhood].filter(Boolean).join(' · ')}</span>
                </span>
                <Plus className="h-4 w-4 text-brand" />
              </button>
            </li>
          ))}
          {res.data && items.length === 0 && <li className="px-2 py-3 text-sm text-ink-muted">Nenhum encarregado encontrado. Use “Novo encarregado”.</li>}
        </ul>
      )}
    </Card>
  )
}
