import * as React from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { date, money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { Button, Dialog, DialogContent, Field, Textarea } from '@/components/ui'
import { type Errors, handleError } from '@/features/enrolment/shared'

export interface ChargeRef { id: string; kind: 'monthly' | 'one_off' | 'penalty'; description: string; due_date: string; remaining: number; paid: number; pupil_name?: string }

/**
 * Cancel a fee / one-off item, or waive a penalty — always with a reason.
 * Nothing is deleted: the charge stays on the account as "Anulado".
 */
export function ChargeActionDialog({ charge, onClose }: { charge: ChargeRef | null; onClose: () => void }) {
  const qc = useQueryClient()
  const [reason, setReason] = React.useState('')
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)
  React.useEffect(() => { setReason(''); setErrors({}) }, [charge?.id])

  const waive = charge?.kind === 'penalty'
  async function submit() {
    if (!charge) return
    if (reason.trim().length < 5) return setErrors({ reason: 'Indique o motivo.' })
    setSaving(true)
    try {
      await api.post(waive ? `/penalties/${charge.id}/waive` : `/charges/${charge.id}/cancel`, { reason: reason.trim() })
      toast.success(waive ? 'Multa perdoada.' : 'Cobrança anulada.')
      void qc.invalidateQueries()
      onClose()
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={!!charge} onOpenChange={(o) => !o && onClose()}>
      <DialogContent title={waive ? 'Perdoar multa' : 'Anular cobrança'} description={charge?.pupil_name}>
        {charge && (
          <div className="space-y-4 p-5">
            <div className="rounded-lg border border-line p-3">
              <p className="text-sm font-medium text-navy">{charge.description}</p>
              <p className="text-xs text-ink-muted">Vence {date(charge.due_date)} · por pagar {money(charge.remaining)}</p>
            </div>
            <p className="text-sm text-ink">
              {waive
                ? 'A multa fica anulada e não volta a ser cobrada.'
                : 'A cobrança fica anulada (não é apagada) e as multas dela também.'}
              {charge.paid > 0 && ` Os ${money(charge.paid)} já pagos passam para as outras dívidas do aluno, ou ficam como crédito.`}
            </p>
            <Field label="Motivo" error={errors.reason}>
              <Textarea autoFocus value={reason} onChange={(e) => { setReason(e.target.value); setErrors({}) }} placeholder={waive ? 'Ex.: primeiro atraso, família avisou' : 'Ex.: não almoçou em Março'} />
            </Field>
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={onClose}>{pt.actions.cancel}</Button>
              <Button variant="danger" loading={saving} onClick={submit}>{waive ? 'Perdoar multa' : 'Anular cobrança'}</Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
