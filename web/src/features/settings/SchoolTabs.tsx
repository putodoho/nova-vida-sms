import * as React from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Copy, Pencil, Plus } from 'lucide-react'
import { api } from '@/lib/api'
import { date, parseDate } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Checkbox, handleError } from '@/lib/forms'
import { Badge, Button, Card, CardHeader, EmptyState, Field, Input, Select, Skeleton, Table, Td, Textarea, Th } from '@/components/ui'
import { FormDialog, useEditor } from '@/features/common/FormDialog'
import type { Account, Entity, List, SchoolYear } from '@/api/types'

export function NeedEntity() {
  return <Card><EmptyState title="Escolha a escola" description="Estas configurações são de cada escola. Escolha a Primária ou o Jardim no topo do ecrã." /></Card>
}

// ---------------------------------------------------------------------------
// School data and receiving accounts
// ---------------------------------------------------------------------------

const vatMethods = [
  { value: 'included_flat', label: 'IVA incluído, 5% sobre o valor (modelo actual da escola: 2.800 → 140)' },
  { value: 'included_standard', label: 'IVA incluído, cálculo legal (2.800 → 133,33)' },
  { value: 'added', label: 'IVA acrescentado ao preço' },
]

interface AccountForm { id?: string; kind: Account['kind']; label: string; holder: string; number: string; nib: string; active: boolean }

export function SchoolTab() {
  const { entity } = useSession()
  const qc = useQueryClient()
  const ent = useQuery({ queryKey: ['entity', entity?.id], queryFn: () => api.get<Entity>(`/entities/${entity!.id}`), enabled: !!entity })
  const accounts = useQuery({ queryKey: ['accounts', entity?.id], queryFn: () => api.get<List<Account>>(`/entities/${entity!.id}/accounts`), enabled: !!entity })
  const [form, setForm] = React.useState<Entity | null>(null)
  const [errors, setErrors] = React.useState<Record<string, string>>({})
  const [saving, setSaving] = React.useState(false)
  const acc = useEditor<AccountForm>()
  React.useEffect(() => { if (ent.data) setForm(ent.data) }, [ent.data])

  if (!entity) return <NeedEntity />
  if (!form) return <Skeleton className="h-64" />
  const set = (k: keyof Entity, v: string) => { setForm((f) => (f ? { ...f, [k]: v } : f)); setErrors({}) }

  async function save() {
    if (!form) return
    setSaving(true)
    try {
      await api.put(`/entities/${entity!.id}`, { legal_name: form.legal_name, short_name: form.short_name, nuit: form.nuit, address: form.address, motto: form.motto, vat_method: form.vat_method })
      toast.success('Dados da escola guardados.')
      void qc.invalidateQueries({ queryKey: ['entity'] })
    } catch (err) {
      handleError(err, setErrors)
    } finally {
      setSaving(false)
    }
  }
  async function saveAccount() {
    const a = acc.item
    if (!a) return
    if (!a.label.trim()) return acc.setErrors({ label: 'Obrigatório.' })
    acc.setSaving(true)
    try {
      const body = { kind: a.kind, label: a.label.trim(), holder: a.holder.trim(), number: a.number.trim(), nib: a.nib.trim(), active: a.active }
      if (a.id) await api.put(`/entities/${entity!.id}/accounts/${a.id}`, body)
      else await api.post(`/entities/${entity!.id}/accounts`, body)
      toast.success('Conta guardada.')
      void qc.invalidateQueries({ queryKey: ['accounts'] })
      acc.close()
    } catch (err) {
      handleError(err, acc.setErrors)
    } finally {
      acc.setSaving(false)
    }
  }

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader title="Dados da escola" description="Aparecem nos recibos, extratos e fichas de matrícula." />
        <div className="grid gap-4 p-5 md:grid-cols-2">
          <Field label="Nome legal" error={errors.legal_name}><Input value={form.legal_name} onChange={(e) => set('legal_name', e.target.value)} /></Field>
          <Field label="Nome curto" error={errors.short_name}><Input value={form.short_name} onChange={(e) => set('short_name', e.target.value)} /></Field>
          <Field label="NUIT" error={errors.nuit}><Input inputMode="numeric" value={form.nuit} onChange={(e) => set('nuit', e.target.value)} /></Field>
          <Field label="Lema"><Input value={form.motto} onChange={(e) => set('motto', e.target.value)} /></Field>
          <Field label="Endereço" className="md:col-span-2"><Textarea rows={2} value={form.address} onChange={(e) => set('address', e.target.value)} /></Field>
          <Field label="Cálculo do IVA" className="md:col-span-2" hint="Vale para as cobranças criadas daqui em diante; as já lançadas mantêm o valor.">
            <Select value={form.vat_method} onChange={(e) => set('vat_method', e.target.value)}>
              {vatMethods.map((m) => <option key={m.value} value={m.value}>{m.label}</option>)}
            </Select>
          </Field>
        </div>
        <div className="flex justify-end border-t border-line px-5 py-3"><Button loading={saving} onClick={save}>{pt.actions.save}</Button></div>
      </Card>

      <Card>
        <CardHeader title="Contas de recebimento" description="Onde o dinheiro entra. As contas activas (excepto numerário) aparecem nos extratos."
          action={<Button size="sm" onClick={() => acc.open({ kind: 'bank', label: '', holder: form.legal_name, number: '', nib: '', active: true })}><Plus className="h-4 w-4" /> Nova conta</Button>} />
        {accounts.data && (
          <Table>
            <thead><tr><Th>Conta</Th><Th>Método</Th><Th className="hidden md:table-cell">Número / NIB</Th><Th>Estado</Th><Th /></tr></thead>
            <tbody>
              {accounts.data.items.map((a) => (
                <tr key={a.id} className={cn(!a.active && 'opacity-60')}>
                  <Td><p className="font-medium text-navy">{a.label}</p><p className="text-xs text-ink-muted">{a.holder}</p></Td>
                  <Td>{pt.methods[a.kind]}</Td>
                  <Td className="hidden md:table-cell"><p>{a.number || '—'}</p>{a.nib && <p className="text-xs text-ink-muted">NIB {a.nib}</p>}</Td>
                  <Td><Badge tone={a.active ? 'ok' : 'neutral'}>{a.active ? 'Activa' : 'Inactiva'}</Badge></Td>
                  <Td align="right"><Button size="sm" variant="ghost" aria-label="Editar conta" onClick={() => acc.open({ ...a })}><Pencil className="h-4 w-4" /></Button></Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <FormDialog open={!!acc.item} title={acc.item?.id ? 'Editar conta' : 'Nova conta'} description={entity.short_name} onClose={acc.close} onSubmit={saveAccount} saving={acc.saving}>
        {acc.item && (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Método">
              <Select value={acc.item.kind} onChange={(e) => acc.patch({ kind: e.target.value as Account['kind'] })}>
                {(['bank', 'mpesa', 'emola', 'cash'] as const).map((k) => <option key={k} value={k}>{pt.methods[k]}</option>)}
              </Select>
            </Field>
            <Field label="Nome da conta" error={acc.errors.label} hint="Ex.: Millennium BIM, M-Pesa 84…"><Input autoFocus value={acc.item.label} onChange={(e) => acc.patch({ label: e.target.value })} /></Field>
            <Field label="Titular" className="sm:col-span-2"><Input value={acc.item.holder} onChange={(e) => acc.patch({ holder: e.target.value })} /></Field>
            <Field label={acc.item.kind === 'bank' ? 'Número da conta' : 'Número'}><Input value={acc.item.number} onChange={(e) => acc.patch({ number: e.target.value })} /></Field>
            {acc.item.kind === 'bank' && <Field label="NIB" error={acc.errors.nib}><Input inputMode="numeric" value={acc.item.nib} onChange={(e) => acc.patch({ nib: e.target.value })} /></Field>}
            <div className="sm:col-span-2"><Checkbox checked={acc.item.active} onChange={(v) => acc.patch({ active: v })} label="Activa" hint="Contas inactivas deixam de aparecer ao registar pagamentos e nos extratos." /></div>
          </div>
        )}
      </FormDialog>
    </div>
  )
}

// ---------------------------------------------------------------------------
// School years
// ---------------------------------------------------------------------------

interface YearForm { id?: string; year: string; starts: string; ends: string; months: number[]; due_day: string; status: SchoolYear['status']; locked?: boolean }

const statusLabel: Record<SchoolYear['status'], string> = { planning: 'Em preparação', open: 'Aberto', closed: 'Fechado' }

export function YearsTab() {
  const { years, entity, setYear } = useSession()
  const qc = useQueryClient()
  const ed = useEditor<YearForm>()
  const [copying, setCopying] = React.useState<SchoolYear | null>(null)
  const [from, setFrom] = React.useState('')
  const [busy, setBusy] = React.useState(false)
  const sorted = [...years].sort((a, b) => b.year - a.year)

  function blank(): YearForm {
    const last = sorted[0]
    const y = (last?.year ?? new Date().getFullYear()) + 1
    return { year: String(y), starts: `01/02/${y}`, ends: `30/11/${y}`, months: last?.billing_months ?? [2, 3, 4, 5, 6, 7, 8, 9, 10, 11], due_day: String(last?.due_day ?? 5), status: 'planning' }
  }
  async function save() {
    const f = ed.item
    if (!f) return
    const e: Record<string, string> = {}
    const starts = parseDate(f.starts), ends = parseDate(f.ends)
    if (!/^\d{4}$/.test(f.year)) e.year = 'Ano inválido.'
    if (!starts) e.starts_on = 'dd/mm/aaaa'
    if (!ends) e.ends_on = 'dd/mm/aaaa'
    if (f.months.length === 0) e.billing_months = 'Escolha os meses cobrados.'
    if (!(Number(f.due_day) >= 1 && Number(f.due_day) <= 28)) e.due_day = 'Entre 1 e 28.'
    if (Object.keys(e).length) return ed.setErrors(e)
    ed.setSaving(true)
    try {
      const body = { year: Number(f.year), starts_on: starts, ends_on: ends, billing_months: [...f.months].sort((a, b) => a - b), due_day: Number(f.due_day), status: f.status }
      if (f.id) await api.put(`/school-years/${f.id}`, body)
      else await api.post('/school-years', body)
      toast.success(f.id ? 'Ano lectivo actualizado.' : `Ano lectivo ${f.year} criado. Crie as turmas e a tabela de preços.`)
      void qc.invalidateQueries({ queryKey: ['school-years'] })
      ed.close()
    } catch (err) {
      handleError(err, ed.setErrors)
    } finally {
      ed.setSaving(false)
    }
  }
  async function copy() {
    if (!copying || !from) return
    setBusy(true)
    try {
      const r = await api.post<{ copied: number; skipped: string[]; inactive: string[] }>('/fee-items/copy-from-year', { from_year: Number(from), to_year: copying.year })
      toast.success(`${r.copied} preços copiados${r.skipped.length ? `; ${r.skipped.length} já existiam` : ''}.`)
      if (r.inactive.length) toast.warning(`Ficaram inactivos (as turmas não existem em ${copying.year}): ${r.inactive.join(', ')}. Reveja na tabela de preços.`, { duration: 10000 })
      void qc.invalidateQueries({ queryKey: ['fee-items'] })
      setCopying(null)
    } catch (err) {
      handleError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader title="Anos lectivos" description="Comuns às duas escolas. Os meses e o dia de vencimento ficam fixos quando já há cobranças."
        action={<Button size="sm" onClick={() => ed.open(blank())}><Plus className="h-4 w-4" /> Novo ano</Button>} />
      <Table>
        <thead><tr><Th>Ano</Th><Th className="hidden md:table-cell">Período</Th><Th>Meses cobrados</Th><Th className="hidden md:table-cell">Vence</Th><Th>Estado</Th><Th /></tr></thead>
        <tbody>
          {sorted.map((y) => (
            <tr key={y.id}>
              <Td className="font-semibold text-navy">{y.year}</Td>
              <Td className="hidden md:table-cell">{date(y.starts_on)} a {date(y.ends_on)}</Td>
              <Td>{y.billing_months.length} ({pt.monthsShort[y.billing_months[0]]}–{pt.monthsShort[y.billing_months[y.billing_months.length - 1]]})</Td>
              <Td className="hidden md:table-cell">dia {y.due_day}</Td>
              <Td><Badge tone={y.status === 'open' ? 'ok' : y.status === 'planning' ? 'info' : 'neutral'}>{statusLabel[y.status]}</Badge></Td>
              <Td align="right">
                <div className="flex justify-end gap-1">
                  {entity && sorted.some((o) => o.year < y.year) && (
                    <Button size="sm" variant="ghost" onClick={() => { setCopying(y); setFrom(String(y.year - 1)) }}><Copy className="h-4 w-4" /> Copiar preços</Button>
                  )}
                  <Button size="sm" variant="ghost" onClick={() => setYear(y.year)}>Usar</Button>
                  <Button size="sm" variant="ghost" aria-label="Editar ano" onClick={() => ed.open({ id: y.id, year: String(y.year), starts: date(y.starts_on), ends: date(y.ends_on), months: y.billing_months, due_day: String(y.due_day), status: y.status })}><Pencil className="h-4 w-4" /></Button>
                </div>
              </Td>
            </tr>
          ))}
        </tbody>
      </Table>

      <FormDialog open={!!ed.item} title={ed.item?.id ? `Ano lectivo ${ed.item.year}` : 'Novo ano lectivo'} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {ed.item && (
          <>
            <div className="grid gap-3 sm:grid-cols-3">
              <Field label="Ano" error={ed.errors.year}><Input inputMode="numeric" value={ed.item.year} disabled={!!ed.item.id} onChange={(e) => ed.patch({ year: e.target.value })} /></Field>
              <Field label="Início" error={ed.errors.starts_on}><Input value={ed.item.starts} onChange={(e) => ed.patch({ starts: e.target.value })} /></Field>
              <Field label="Fim" error={ed.errors.ends_on}><Input value={ed.item.ends} onChange={(e) => ed.patch({ ends: e.target.value })} /></Field>
            </div>
            <Field label="Meses cobrados" error={ed.errors.billing_months}>
              <div className="grid grid-cols-4 gap-1 sm:grid-cols-6">
                {Array.from({ length: 12 }, (_, i) => i + 1).map((m) => {
                  const on = ed.item!.months.includes(m)
                  return (
                    <button key={m} type="button" onClick={() => ed.patch({ months: on ? ed.item!.months.filter((x) => x !== m) : [...ed.item!.months, m] })}
                      className={cn('h-9 rounded-md border text-sm', on ? 'border-brand bg-brand-tint font-semibold text-brand' : 'border-line text-ink-muted hover:bg-page')}>
                      {pt.monthsShort[m]}
                    </button>
                  )
                })}
              </div>
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Dia de vencimento" error={ed.errors.due_day} hint="Mensalidades vencem neste dia de cada mês."><Input inputMode="numeric" value={ed.item.due_day} onChange={(e) => ed.patch({ due_day: e.target.value })} /></Field>
              <Field label="Estado" hint="Só um ano deve estar aberto de cada vez.">
                <Select value={ed.item.status} onChange={(e) => ed.patch({ status: e.target.value as SchoolYear['status'] })}>
                  {(['planning', 'open', 'closed'] as const).map((s) => <option key={s} value={s}>{statusLabel[s]}</option>)}
                </Select>
              </Field>
            </div>
          </>
        )}
      </FormDialog>

      <FormDialog open={!!copying} title={`Copiar tabela de preços para ${copying?.year}`} description={entity?.short_name} onClose={() => setCopying(null)} onSubmit={copy} saving={busy} submitLabel="Copiar">
        <Field label="Copiar de">
          <Select value={from} onChange={(e) => setFrom(e.target.value)}>
            {sorted.filter((o) => copying && o.year < copying.year).map((o) => <option key={o.id} value={o.year}>{o.year}</option>)}
          </Select>
        </Field>
        <p className="text-sm text-ink-muted">Copia os preços desta escola. As turmas são associadas pelo nome, por isso crie primeiro as turmas de {copying?.year}. Preços que já existam não são alterados.</p>
      </FormDialog>
    </Card>
  )
}
