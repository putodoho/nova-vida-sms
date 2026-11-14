import * as React from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AlertTriangle, CheckCircle2, Pencil, Printer, Wallet } from 'lucide-react'
import { api, openDocument } from '@/lib/api'
import { age, date, money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { Badge, Button, Table, Td, Th } from '@/components/ui'
import type { Enrolment, PupilProfile } from '@/api/types'
import { handleError, type StepKey } from './shared'

function Block({ title, step, onEdit, children }: { title: string; step: StepKey; onEdit: (s: StepKey) => void; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-line p-4">
      <div className="mb-2 flex items-center justify-between">
        <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">{title}</p>
        <button type="button" onClick={() => onEdit(step)} className="inline-flex items-center gap-1 text-xs font-semibold text-brand hover:underline"><Pencil className="h-3 w-3" /> Alterar</button>
      </div>
      {children}
    </div>
  )
}

/** Step 7. Everything on one page, the charges confirming will create, and the confirm actions. */
export function ReviewStep({ enrolment, profile, onEdit, onConfirmed }: {
  enrolment: Enrolment; profile: PupilProfile; onEdit: (s: StepKey) => void; onConfirmed: (e: Enrolment, pay: boolean) => void
}) {
  const { me, can } = useSession()
  const qc = useQueryClient()
  const [busy, setBusy] = React.useState<'confirm' | 'pay' | null>(null)
  const e = enrolment
  const payer = profile.guardians.find((g) => g.is_payer)
  const dueNow = e.preview.filter((c) => c.due_date <= me.today).reduce((s, c) => s + c.lines.gross, 0)
  const h = profile.health

  const problems = [
    !e.class_id && 'Falta escolher a turma.',
    !payer && 'Falta indicar o encarregado responsável pelos pagamentos.',
    !e.consents.fees_agreement && 'O encarregado ainda não aceitou o pagamento das mensalidades.',
  ].filter(Boolean) as string[]

  async function confirm(pay: boolean) {
    setBusy(pay ? 'pay' : 'confirm')
    try {
      const out = await api.post<Enrolment>(`/enrolments/${e.id}/confirm`)
      void qc.invalidateQueries({ queryKey: ['enrolments'] })
      void qc.invalidateQueries({ queryKey: ['classes'] })
      void qc.invalidateQueries({ queryKey: ['pupil', e.pupil_id] })
      void qc.invalidateQueries({ queryKey: ['account', e.pupil_id] })
      onConfirmed(out, pay)
    } catch (err) {
      handleError(err)
    } finally {
      setBusy(null)
    }
  }

  return (
    <>
      <div className="space-y-6 p-5">
        {problems.length > 0 && (
          <div className="flex gap-3 rounded-lg border border-warn/30 bg-warn-tint p-4 text-sm">
            <AlertTriangle className="h-5 w-5 shrink-0 text-warn" />
            <ul className="space-y-0.5 text-ink">{problems.map((p) => <li key={p}>{p}</li>)}</ul>
          </div>
        )}

        <div className="grid gap-3 md:grid-cols-2">
          <Block title="Aluno" step="pupil" onEdit={onEdit}>
            <p className="font-semibold text-navy">{profile.full_name}</p>
            <p className="text-sm text-ink-muted">{profile.number} · {date(profile.birth_date)} ({age(profile.birth_date)} anos) · {profile.sex === 'F' ? 'Feminino' : 'Masculino'}</p>
            <p className="mt-1 text-sm text-ink-muted">{e.type === 'new' ? 'Aluno novo' : 'Renovação'}{profile.neighbourhood && ` · ${profile.neighbourhood}`}</p>
          </Block>
          <Block title="Encarregados" step="guardians" onEdit={onEdit}>
            <ul className="space-y-1 text-sm">
              {profile.guardians.map((g) => (
                <li key={g.guardian_id} className="flex flex-wrap items-center gap-x-2">
                  <span className="font-medium text-ink">{g.full_name}</span>
                  <span className="text-ink-muted">{pt.relationship[g.relationship]} · {g.phone1}</span>
                  {g.is_payer && <Badge tone="accent">Paga</Badge>}
                </li>
              ))}
            </ul>
          </Block>
          {h && (
            <Block title="Saúde" step="health" onEdit={onEdit}>
              {profile.health_alert ? (
                <p className="text-sm text-bad">
                  {[h.allergies && `Alergias: ${h.allergies}`, h.medication && `Medicação: ${h.medication}`, h.diet_restrictions && `Dieta: ${h.diet_restrictions}`, h.chronic_condition && `Doença crónica: ${h.chronic_condition}`].filter(Boolean).join(' · ')}
                </p>
              ) : <p className="text-sm text-ink-muted">Sem alergias, dieta, medicação ou doença crónica.</p>}
            </Block>
          )}
          <Block title="Recolha e emergência" step="pickup" onEdit={onEdit}>
            <p className="text-sm text-ink">
              {profile.emergency_contacts.length ? profile.emergency_contacts.map((c) => `${c.full_name} (${c.phone})`).join(', ') : <span className="text-ink-muted">Sem contactos de emergência.</span>}
            </p>
            {profile.pickup_persons.length > 0 && <p className="mt-1 text-sm text-ink-muted">Também podem buscar: {profile.pickup_persons.map((p) => p.full_name).join(', ')}</p>}
          </Block>
          <Block title="Turma e serviços" step="class" onEdit={onEdit}>
            <p className="font-semibold text-navy">{e.class_name ?? '—'} · desde {pt.months[e.start_month]}</p>
            <p className="text-sm text-ink-muted">{e.services.map((s) => s.name).join(' + ')}</p>
            <p className="mt-1 text-sm text-ink"><strong className="tabular-nums">{money(e.totals.monthly_gross)}</strong> por mês · {e.totals.months} {e.totals.months === 1 ? 'mês' : 'meses'} · <span className="tabular-nums">{money(e.totals.annual_gross)}</span> no ano</p>
          </Block>
          <Block title="Consentimentos" step="fees" onEdit={onEdit}>
            <ul className="space-y-0.5 text-sm">
              {([
                ['fees_agreement', 'Pagamento das mensalidades'],
                ['uniform_books', 'Uniforme e livros obrigatórios'],
                ['emergency_care', 'Tratamento médico de urgência'],
                ['image_use', 'Fotografias nas redes sociais'],
              ] as const).map(([k, label]) => (
                <li key={k} className={e.consents[k] ? 'text-ink' : 'text-ink-muted line-through'}>{label}</li>
              ))}
            </ul>
          </Block>
        </div>

        {can('charges.read') && (
          <div>
            <p className="mb-2 text-sm font-semibold text-navy">Cobranças que serão criadas</p>
            <div className="rounded-lg border border-line">
              <Table>
                <thead><tr><Th>Descrição</Th><Th>Vencimento</Th><Th align="right">IVA</Th><Th align="right">Valor</Th></tr></thead>
                <tbody>
                  {e.preview.map((c, i) => (
                    <tr key={i} className={c.due_date <= me.today ? 'bg-accent-tint/40' : undefined}>
                      <Td>{c.description}</Td>
                      <Td>{date(c.due_date)}{c.due_date <= me.today && <Badge tone="accent" className="ml-2">Já</Badge>}</Td>
                      <Td align="right" className="text-ink-muted">{money(c.lines.vat)}</Td>
                      <Td align="right">{money(c.lines.gross)}</Td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr>
                    <Td className="font-semibold text-navy">Total do ano</Td><Td /><Td />
                    <Td align="right" className="font-semibold text-navy">{money(e.totals.annual_gross + e.totals.one_off_gross)}</Td>
                  </tr>
                </tfoot>
              </Table>
            </div>
          </div>
        )}
      </div>

      <div className="flex flex-col gap-3 border-t border-line bg-page/50 px-5 py-4 lg:flex-row lg:items-center">
        <div className="lg:mr-auto">
          <p className="text-xs text-ink-muted">A pagar já (taxas e meses vencidos)</p>
          <p className="text-[22px] font-bold tabular-nums text-navy">{money(dueNow)}</p>
        </div>
        <Button variant="secondary" className="justify-center" onClick={() => openDocument(`/enrolments/${e.id}/form`).catch((x) => toast.error(x.message))}>
          <Printer className="h-4 w-4" /> Imprimir ficha
        </Button>
        <Button variant="secondary" className="justify-center" disabled={problems.length > 0 || !!busy} loading={busy === 'confirm'} onClick={() => confirm(false)}>
          <CheckCircle2 className="h-4 w-4" /> Confirmar matrícula
        </Button>
        {can('payments.create') && (
          <Button variant="accent" className="justify-center" disabled={problems.length > 0 || !!busy} loading={busy === 'pay'} onClick={() => confirm(true)}>
            <Wallet className="h-4 w-4" /> Confirmar e registar pagamento
          </Button>
        )}
      </div>
    </>
  )
}
