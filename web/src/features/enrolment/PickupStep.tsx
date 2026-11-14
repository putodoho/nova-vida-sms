import * as React from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { api } from '@/lib/api'
import { Button, Field, Input } from '@/components/ui'
import type { PupilProfile } from '@/api/types'
import { type Errors, handleError, Section, StepFooter } from './shared'

type Pickup = PupilProfile['pickup_persons'][number]
type Contact = Omit<PupilProfile['emergency_contacts'][number], 'position'>

const MAX_CONTACTS = 3

/** Step 4. Other people allowed to pick the child up, and emergency contacts in order. */
export function PickupStep({ pupilId, profile, onBack, onSaved, nextLabel }: {
  pupilId: string; profile: PupilProfile; onBack: () => void; onSaved: () => void; nextLabel?: string
}) {
  const [pickup, setPickup] = React.useState<Pickup[]>(profile.pickup_persons)
  const [contacts, setContacts] = React.useState<Contact[]>(() =>
    profile.emergency_contacts.length
      ? profile.emergency_contacts.map(({ full_name, relationship, phone }) => ({ full_name, relationship, phone }))
      : [{ full_name: '', relationship: '', phone: '' }],
  )
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)

  async function save() {
    const e: Errors = {}
    const p = pickup.filter((x) => x.full_name.trim() || x.phone.trim() || x.id_doc.trim())
    const c = contacts.filter((x) => x.full_name.trim() || x.phone.trim())
    p.forEach((x, i) => { if (!x.full_name.trim()) e[`p${i}.full_name`] = 'Nome obrigatório.' })
    c.forEach((x, i) => {
      if (!x.full_name.trim()) e[`c${i}.full_name`] = 'Nome obrigatório.'
      if (!x.phone.trim()) e[`c${i}.phone`] = 'Telefone obrigatório.'
    })
    if (Object.keys(e).length) return setErrors(e)
    setSaving(true)
    try {
      await api.put(`/pupils/${pupilId}/pickup-persons`, p)
      await api.put(`/pupils/${pupilId}/emergency-contacts`, c.map((x, i) => ({ ...x, position: i + 1 })))
      onSaved()
    } catch (err) {
      handleError(err)
    } finally {
      setSaving(false)
    }
  }

  const payerPickup = profile.guardians.filter((g) => g.may_pick_up).map((g) => g.full_name)

  return (
    <>
      <div className="space-y-8 p-5">
        <Section
          title="Outras pessoas autorizadas a buscar a criança"
          description={payerPickup.length ? `Já autorizados: ${payerPickup.join(', ')}.` : 'Além dos encarregados marcados como autorizados.'}
        >
          {pickup.map((x, i) => (
            <div key={i} className="grid items-end gap-3 md:grid-cols-[1fr_180px_180px_auto]">
              <Field label="Nome" error={errors[`p${i}.full_name`]}>
                <Input value={x.full_name} onChange={(e) => setPickup((ps) => ps.map((y, j) => (j === i ? { ...y, full_name: e.target.value } : y)))} />
              </Field>
              <Field label="Telefone">
                <Input inputMode="tel" value={x.phone} onChange={(e) => setPickup((ps) => ps.map((y, j) => (j === i ? { ...y, phone: e.target.value } : y)))} />
              </Field>
              <Field label="BI / documento">
                <Input value={x.id_doc} onChange={(e) => setPickup((ps) => ps.map((y, j) => (j === i ? { ...y, id_doc: e.target.value } : y)))} />
              </Field>
              <Button variant="ghost" size="icon" aria-label="Remover" onClick={() => setPickup((ps) => ps.filter((_, j) => j !== i))}><Trash2 className="h-4 w-4" /></Button>
            </div>
          ))}
          {pickup.length < 10 && (
            <Button variant="secondary" size="sm" onClick={() => setPickup((ps) => [...ps, { full_name: '', phone: '', id_doc: '' }])}><Plus className="h-4 w-4" /> Adicionar pessoa</Button>
          )}
        </Section>

        <Section title="Contactos de emergência" description={`Até ${MAX_CONTACTS}, pela ordem em que devem ser chamados.`}>
          {contacts.map((x, i) => (
            <div key={i} className="grid items-end gap-3 md:grid-cols-[32px_1fr_180px_180px_auto]">
              <span className="hidden h-10 items-center justify-center rounded-full bg-brand-tint text-sm font-bold text-brand md:flex">{i + 1}</span>
              <Field label="Nome" error={errors[`c${i}.full_name`]}>
                <Input value={x.full_name} onChange={(e) => setContacts((cs) => cs.map((y, j) => (j === i ? { ...y, full_name: e.target.value } : y)))} />
              </Field>
              <Field label="Parentesco">
                <Input value={x.relationship} placeholder="Ex.: tia" onChange={(e) => setContacts((cs) => cs.map((y, j) => (j === i ? { ...y, relationship: e.target.value } : y)))} />
              </Field>
              <Field label="Telefone" error={errors[`c${i}.phone`]}>
                <Input inputMode="tel" value={x.phone} onChange={(e) => setContacts((cs) => cs.map((y, j) => (j === i ? { ...y, phone: e.target.value } : y)))} />
              </Field>
              <Button variant="ghost" size="icon" aria-label="Remover" onClick={() => setContacts((cs) => cs.filter((_, j) => j !== i))}><Trash2 className="h-4 w-4" /></Button>
            </div>
          ))}
          {contacts.length < MAX_CONTACTS && (
            <Button variant="secondary" size="sm" onClick={() => setContacts((cs) => [...cs, { full_name: '', relationship: '', phone: '' }])}><Plus className="h-4 w-4" /> Adicionar contacto</Button>
          )}
        </Section>
      </div>
      <StepFooter onBack={onBack} onNext={save} loading={saving} nextLabel={nextLabel} />
    </>
  )
}
