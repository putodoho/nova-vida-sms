import * as React from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Check, Percent, Plus, X } from 'lucide-react'
import { api } from '@/lib/api'
import { date, money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Badge, Button, Card, Dialog, DialogContent, EmptyState, ErrorState, Field, PageHeader, Skeleton, Textarea } from '@/components/ui'
import { describeValue, DiscountRequestDialog } from '@/features/discounts/DiscountRequestDialog'
import { type Errors, handleError } from '@/features/enrolment/shared'
import type { DiscountRequest, List } from '@/api/types'

const tabs = [
  { value: 'pending', label: 'Pendentes' },
  { value: 'approved', label: 'Aprovados' },
  { value: 'rejected', label: 'Recusados' },
] as const

/** Discounts (brief 7.11): the approval queue and its history. */
export function DiscountsPage() {
  const { entity, can } = useSession()
  const [params, setParams] = useSearchParams()
  const status = (params.get('status') ?? 'pending') as (typeof tabs)[number]['value']
  const [creating, setCreating] = React.useState(false)
  const [deciding, setDeciding] = React.useState<{ req: DiscountRequest; approve: boolean } | null>(null)

  const list = useQuery({
    queryKey: ['discount-requests', entity?.id ?? 'all'],
    queryFn: () => api.get<List<DiscountRequest>>('/discount-requests'),
  })
  const all = list.data?.items ?? []
  const items = all.filter((r) => r.status === status)
  const count = (s: string) => all.filter((r) => r.status === s).length
  const canDecide = can('discounts.approve') && !!entity

  return (
    <>
      <PageHeader
        title={pt.nav.discounts}
        description={entity ? entity.short_name : 'Todas as escolas'}
        actions={can('discounts.request') && entity && <Button onClick={() => setCreating(true)}><Plus className="h-4 w-4" /> Pedir desconto</Button>}
      />
      <Card>
        <div className="flex gap-1 overflow-x-auto border-b border-line px-3">
          {tabs.map((t) => (
            <button key={t.value} onClick={() => setParams(t.value === 'pending' ? {} : { status: t.value }, { replace: true })}
              className={cn('whitespace-nowrap border-b-2 px-3 py-3 text-sm font-medium', status === t.value ? 'border-brand text-brand' : 'border-transparent text-ink-muted hover:text-navy')}>
              {t.label}
              {list.data && <span className={cn('ml-1.5 rounded-full px-1.5 text-xs', t.value === 'pending' && count('pending') > 0 ? 'bg-warn text-white' : 'bg-page')}>{count(t.value)}</span>}
            </button>
          ))}
        </div>
        {list.isError && <div className="p-4"><ErrorState error={list.error} onRetry={() => list.refetch()} /></div>}
        {list.isLoading && <div className="space-y-3 p-4">{Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-24" />)}</div>}
        {list.data && items.length === 0 && (
          <EmptyState
            icon={<Percent className="h-6 w-6" />}
            title={status === 'pending' ? 'Nenhum pedido à espera' : status === 'approved' ? 'Ainda não há descontos aprovados' : 'Nenhum pedido recusado'}
            description={status === 'pending' ? 'Os pedidos feitos pela secretaria aparecem aqui para a direcção decidir.' : undefined}
          />
        )}
        <ul className="divide-y divide-line">
          {items.map((r) => (
            <li key={r.id} className="flex flex-col gap-3 p-4 md:flex-row md:items-start">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Link to={`/alunos/${r.pupil_id}`} className="font-semibold text-navy hover:underline">{r.pupil_name}</Link>
                  <span className="text-sm text-ink-muted">{r.class_name ?? '—'}</span>
                  <Badge tone="violet">{r.discount_type}</Badge>
                </div>
                <p className="mt-1 text-sm text-ink">
                  <strong>{describeValue(r)}</strong> a partir de {pt.months[r.scope.from_month]}
                  {r.status !== 'rejected' && (
                    <span className="text-ink-muted"> · {r.status === 'pending' ? 'aprovar daria ' : 'concedido '}<strong className="text-ok">{money(r.estimated_amount)}</strong></span>
                  )}
                </p>
                <p className="mt-1 text-sm text-ink-muted">“{r.reason}”</p>
                <p className="mt-1 text-xs text-ink-muted">Pedido por {r.requested_by} em {date(r.created_at)}</p>
                {r.decided_by && (
                  <p className={cn('mt-1 text-xs', r.status === 'approved' ? 'text-ok' : 'text-bad')}>
                    {pt.discountStatus[r.status]} por {r.decided_by} em {date(r.decided_at)}{r.decision_note && ` — ${r.decision_note}`}
                  </p>
                )}
              </div>
              {r.status === 'pending' && canDecide && r.entity_id === entity?.id && (
                <div className="flex shrink-0 gap-2">
                  <Button variant="secondary" size="sm" onClick={() => setDeciding({ req: r, approve: false })}><X className="h-4 w-4" /> Recusar</Button>
                  <Button size="sm" onClick={() => setDeciding({ req: r, approve: true })}><Check className="h-4 w-4" /> Aprovar</Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      </Card>

      {creating && <DiscountRequestDialog open onClose={() => setCreating(false)} />}
      <DecisionDialog state={deciding} onClose={() => setDeciding(null)} />
    </>
  )
}

function DecisionDialog({ state, onClose }: { state: { req: DiscountRequest; approve: boolean } | null; onClose: () => void }) {
  const qc = useQueryClient()
  const [note, setNote] = React.useState('')
  const [errors, setErrors] = React.useState<Errors>({})
  const [saving, setSaving] = React.useState(false)
  React.useEffect(() => { setNote(''); setErrors({}) }, [state])
  const r = state?.req

  async function submit() {
    if (!state || !r) return
    if (!state.approve && note.trim().length < 3) return setErrors({ note: 'Indique o motivo da recusa.' })
    setSaving(true)
    try {
      await api.post(`/discount-requests/${r.id}/${state.approve ? 'approve' : 'reject'}`, { note: note.trim() })
      toast.success(state.approve ? 'Desconto aprovado e aplicado.' : 'Pedido recusado.')
      void qc.invalidateQueries()
      onClose()
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={!!state} onOpenChange={(o) => !o && onClose()}>
      <DialogContent title={state?.approve ? 'Aprovar desconto' : 'Recusar pedido'} description={r?.pupil_name}>
        {r && state && (
          <div className="space-y-4 p-5">
            <div className="rounded-lg border border-line p-3 text-sm">
              <p className="font-medium text-navy">{r.discount_type} · {describeValue(r)}</p>
              <p className="text-ink-muted">A partir de {pt.months[r.scope.from_month]} · pedido por {r.requested_by}</p>
              <p className="mt-1 text-ink-muted">“{r.reason}”</p>
            </div>
            {state.approve && (
              <p className="text-sm text-ink">É aplicado já às mensalidades por pagar: <strong className="text-ok">{money(r.estimated_amount)}</strong> no total.</p>
            )}
            <Field label={state.approve ? 'Nota (opcional)' : 'Motivo da recusa'} error={errors.note}>
              <Textarea autoFocus value={note} onChange={(e) => { setNote(e.target.value); setErrors({}) }} />
            </Field>
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={onClose}>{pt.actions.cancel}</Button>
              <Button variant={state.approve ? 'primary' : 'danger'} loading={saving} onClick={submit}>{state.approve ? 'Aprovar' : 'Recusar pedido'}</Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
