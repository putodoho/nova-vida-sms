import * as React from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Search, X } from 'lucide-react'
import { api } from '@/lib/api'
import { money, moneyInput, parseMoney, percentBP } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { Avatar, Button, Dialog, DialogContent, Field, Input, Select, Textarea } from '@/components/ui'
import { type Errors, handleError, Segmented } from '@/features/enrolment/shared'
import type { DiscountRequest, DiscountType, EnrolmentListItem, List } from '@/api/types'

export function describeValue(r: Pick<DiscountRequest, 'mode' | 'value'>) {
  return r.mode === 'percent' ? `${percentBP(r.value)} por mês` : `${money(r.value)} por mês`
}

export interface DiscountTarget { enrolmentId: string; pupilName: string; className: string | null }

/** First billing month not yet past (discounts never touch earlier months anyway). */
function defaultMonth(months: number[], year: number, today: string) {
  const [y, m] = today.split('-').map(Number)
  if (y !== year) return months[0]
  return months.find((x) => x >= m) ?? months[months.length - 1]
}

/**
 * Ask for a discount on a confirmed enrolment. The type sets the default
 * mode, value and which monthly items it applies to; types that need no
 * approval apply at once.
 */
export function DiscountRequestDialog({ open, onClose, target }: { open: boolean; onClose: () => void; target?: DiscountTarget }) {
  const { year, me } = useSession()
  const qc = useQueryClient()
  const [picked, setPicked] = React.useState<DiscountTarget | null>(target ?? null)
  const [q, setQ] = React.useState('')
  const [term, setTerm] = React.useState('')
  const [typeId, setTypeId] = React.useState('')
  const [mode, setMode] = React.useState<'percent' | 'fixed'>('percent')
  const [value, setValue] = React.useState('')
  const months = year?.billing_months ?? []
  const [fromMonth, setFromMonth] = React.useState(() => defaultMonth(months, year?.year ?? 0, me.today))
  const [reason, setReason] = React.useState('')
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)

  React.useEffect(() => {
    const t = setTimeout(() => setTerm(q.trim()), 250)
    return () => clearTimeout(t)
  }, [q])

  const types = useQuery({
    queryKey: ['discount-types'],
    queryFn: () => api.get<List<DiscountType>>('/discount-types'),
    enabled: open,
    select: (d) => d.items.filter((t) => t.active),
  })
  const found = useQuery({
    queryKey: ['enrolments', 'discount-search', year?.year, term],
    queryFn: () => api.get<List<EnrolmentListItem>>('/enrolments', { year: year?.year, status: 'confirmed', q: term, page_size: 8 }),
    enabled: open && !picked && term.length >= 2,
  })

  const type = types.data?.find((t) => t.id === typeId)
  const pickType = (id: string) => {
    setTypeId(id)
    const t = types.data?.find((x) => x.id === id)
    if (t) {
      setMode(t.mode)
      // A type without a default value (e.g. social case) is decided case by case.
      setValue(t.value === 0 ? '' : t.mode === 'percent' ? String(t.value / 100).replace('.', ',') : moneyInput(t.value))
    }
    setErrors({})
  }

  function parsedValue(): number | null {
    if (mode === 'percent') {
      const n = Number(value.replace(',', '.'))
      return Number.isFinite(n) && n > 0 && n <= 100 ? Math.round(n * 100) : null
    }
    const c = parseMoney(value)
    return c && c > 0 ? c : null
  }

  async function submit() {
    const e: Errors = {}
    const v = parsedValue()
    if (!picked) e.pupil = 'Escolha o aluno.'
    if (!typeId) e.discount_type_id = 'Escolha o tipo de desconto.'
    if (v === null) e.value = mode === 'percent' ? 'Percentagem entre 0 e 100.' : 'Indique o valor.'
    if (reason.trim().length < 5) e.reason = 'Indique o motivo.'
    if (Object.keys(e).length) return setErrors(e)
    setSaving(true)
    try {
      const r = await api.post<DiscountRequest>('/discount-requests', {
        enrolment_id: picked!.enrolmentId, discount_type_id: typeId, mode, value: v, from_month: fromMonth, reason: reason.trim(),
      })
      toast.success(r.status === 'approved' ? `Desconto aplicado: ${money(r.estimated_amount)}.` : 'Pedido enviado para aprovação.')
      void qc.invalidateQueries({ queryKey: ['discount-requests'] })
      if (r.status === 'approved') void qc.invalidateQueries()
      onClose()
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent title="Pedir desconto" description={`Ano lectivo ${year?.year ?? ''}`}>
        <div className="space-y-4 p-5">
          <Field label="Aluno" error={errors.pupil}>
            {picked ? (
              <div className="flex items-center gap-3 rounded-lg border border-line px-3 py-2">
                <Avatar name={picked.pupilName} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-semibold text-navy">{picked.pupilName}</p>
                  <p className="text-xs text-ink-muted">{picked.className ?? '—'}</p>
                </div>
                {!target && (
                  <button type="button" className="rounded-md p-1 text-ink-muted hover:bg-page" aria-label="Trocar aluno" onClick={() => setPicked(null)}><X className="h-4 w-4" /></button>
                )}
              </div>
            ) : (
              <div>
                <div className="relative">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted" />
                  <Input autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder="Nome ou número do aluno" className="pl-9" />
                </div>
                {found.data && (
                  <ul className="mt-1 max-h-56 divide-y divide-line overflow-y-auto rounded-lg border border-line">
                    {found.data.items.map((x) => (
                      <li key={x.enrolment_id}>
                        <button type="button" className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-page"
                          onClick={() => { setPicked({ enrolmentId: x.enrolment_id, pupilName: x.full_name, className: x.class_name }); setErrors({}) }}>
                          <Avatar name={x.full_name} />
                          <span className="min-w-0">
                            <span className="block truncate text-sm font-medium text-navy">{x.full_name}</span>
                            <span className="block text-xs text-ink-muted">{x.number} · {x.class_name ?? '—'}</span>
                          </span>
                        </button>
                      </li>
                    ))}
                    {found.data.items.length === 0 && <li className="px-3 py-2 text-sm text-ink-muted">Nenhum aluno matriculado com esse nome.</li>}
                  </ul>
                )}
              </div>
            )}
          </Field>

          <Field label="Tipo de desconto" error={errors.discount_type_id}>
            <Select value={typeId} onChange={(e) => pickType(e.target.value)} invalid={!!errors.discount_type_id}>
              <option value="">Escolher…</option>
              {types.data?.map((t) => (
                <option key={t.id} value={t.id}>{t.name}{t.value > 0 && ` · ${t.mode === 'percent' ? percentBP(t.value) : money(t.value)}`}</option>
              ))}
            </Select>
          </Field>
          {type && (
            <p className="-mt-2 text-xs text-ink-muted">
              {type.requires_approval ? 'Precisa de aprovação da direcção.' : 'Aplica-se logo, sem aprovação.'} Só sobre as mensalidades por pagar a partir do mês escolhido.
            </p>
          )}

          <div className="grid gap-3 sm:grid-cols-[auto_1fr_1fr]">
            <Field label="Tipo">
              <Segmented value={mode} onChange={(m) => { setMode(m); setValue('') }} options={[{ value: 'percent', label: '%' }, { value: 'fixed', label: 'MT' }]} />
            </Field>
            <Field label={mode === 'percent' ? 'Percentagem por mês' : 'Valor por mês (MT)'} error={errors.value}>
              <Input inputMode="decimal" value={value} invalid={!!errors.value} onChange={(e) => { setValue(e.target.value); setErrors({}) }} placeholder={mode === 'percent' ? '10' : '500,00'} />
            </Field>
            <Field label="A partir de">
              <Select value={fromMonth} onChange={(e) => setFromMonth(Number(e.target.value))}>
                {months.map((m) => <option key={m} value={m}>{pt.months[m]}</option>)}
              </Select>
            </Field>
          </div>

          <Field label="Motivo" error={errors.reason}>
            <Textarea value={reason} onChange={(e) => { setReason(e.target.value); setErrors({}) }} placeholder="Ex.: pai desempregado desde Março; dois irmãos na escola" />
          </Field>

          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={onClose}>{pt.actions.cancel}</Button>
            <Button loading={saving} onClick={submit}>{type && !type.requires_approval ? 'Aplicar desconto' : 'Enviar pedido'}</Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
