import * as React from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { date, parseDate } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { Button, Dialog, DialogContent, Field, Input, Textarea } from '@/components/ui'
import { type Errors, handleError } from './shared'

/**
 * End a confirmed enrolment. Unpaid fees due after the leaving date are
 * cancelled; debt already due stays on the account.
 */
export function WithdrawDialog({ enrolmentId, pupilId, pupilName, open, onOpenChange }: {
  enrolmentId: string; pupilId: string; pupilName: string; open: boolean; onOpenChange: (o: boolean) => void
}) {
  const { me } = useSession()
  const qc = useQueryClient()
  const [day, setDay] = React.useState(date(me.today))
  const [reason, setReason] = React.useState('')
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)

  async function submit() {
    const iso = parseDate(day)
    const e: Errors = {}
    if (!iso) e.date = 'Data inválida (dd/mm/aaaa).'
    if (!reason.trim()) e.reason = 'Indique o motivo.'
    if (Object.keys(e).length) return setErrors(e)
    setSaving(true)
    try {
      await api.post(`/enrolments/${enrolmentId}/withdraw`, { date: iso, reason: reason.trim() })
      toast.success('Desistência registada.')
      for (const k of [['pupil', pupilId], ['enrolment', enrolmentId], ['account', pupilId], ['enrolments'], ['classes']]) {
        void qc.invalidateQueries({ queryKey: k })
      }
      onOpenChange(false)
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title="Registar desistência" description={pupilName}>
        <div className="space-y-4 p-5">
          <p className="rounded-lg bg-warn-tint px-3 py-2 text-sm text-ink">
            As mensalidades e taxas por pagar com vencimento depois desta data são anuladas. A dívida já vencida mantém-se na conta.
          </p>
          <Field label="Último dia na escola" error={errors.date} hint="dd/mm/aaaa">
            <Input inputMode="numeric" value={day} invalid={!!errors.date} onChange={(e) => setDay(e.target.value)} />
          </Field>
          <Field label="Motivo" error={errors.reason}>
            <Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Ex.: mudança de residência para Maputo" />
          </Field>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => onOpenChange(false)}>{pt.actions.cancel}</Button>
            <Button variant="danger" loading={saving} onClick={submit}>Registar desistência</Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
