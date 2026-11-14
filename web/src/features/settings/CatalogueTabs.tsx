import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { api } from '@/lib/api'
import { date, money, moneyInput, parseDate, parseMoney, percentBP } from '@/lib/format'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Checkbox, handleError, Segmented } from '@/lib/forms'
import { Badge, Button, Card, CardHeader, EmptyState, Field, Input, Select, Skeleton, Table, Td, Th } from '@/components/ui'
import { FormDialog, useEditor } from '@/features/common/FormDialog'
import { NeedEntity } from './SchoolTabs'
import type { ClassItem, DiscountType, FeeItem, Level, List, PenaltyRule } from '@/api/types'

const pctText = (bp: number) => String(bp / 100).replace('.', ',')
const parsePct = (s: string) => {
  const n = Number(s.replace(',', '.'))
  return Number.isFinite(n) && n > 0 && n <= 100 ? Math.round(n * 100) : null
}
const slug = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '').slice(0, 40)

// ---------------------------------------------------------------------------
// Fee catalogue
// ---------------------------------------------------------------------------

interface FeeForm {
  id?: string; code: string; name: string; kind: 'monthly' | 'one_off'; amount: string; vat: 'none' | 'included' | 'added'; rate: string
  due: 'enrolment' | 'date'; dueDate: string; mandatory: boolean; active: boolean; classIds: string[]
}

export function FeesTab() {
  const { entity, year } = useSession()
  const qc = useQueryClient()
  const ed = useEditor<FeeForm>()
  const fees = useQuery({ queryKey: ['fee-items', entity?.id, year?.year], queryFn: () => api.get<List<FeeItem>>('/fee-items', { year: year?.year }), enabled: !!entity && !!year })
  const classes = useQuery({ queryKey: ['classes', entity?.id, year?.year], queryFn: () => api.get<List<ClassItem>>('/classes', { year: year?.year }), enabled: !!entity && !!year })
  if (!entity) return <NeedEntity />
  const cls = classes.data?.items ?? []
  const scope = (f: FeeItem) => f.class_ids.length === 0 ? 'Todas as turmas' : f.class_ids.map((id) => cls.find((c) => c.id === id)?.name ?? '?').join(', ')

  const blank = (kind: FeeForm['kind']): FeeForm => ({
    code: '', name: '', kind, amount: '', vat: kind === 'monthly' && entity.code === 'primary' ? 'included' : 'none', rate: '5',
    due: 'enrolment', dueDate: '', mandatory: kind === 'monthly', active: true, classIds: [],
  })
  const edit = (f: FeeItem): FeeForm => ({
    id: f.id, code: f.code, name: f.name, kind: f.kind, amount: moneyInput(f.amount), vat: f.vat_mode, rate: f.vat_rate_bp ? pctText(f.vat_rate_bp) : '5',
    due: f.due_rule?.date ? 'date' : 'enrolment', dueDate: f.due_rule?.date ? date(f.due_rule.date) : '', mandatory: f.mandatory, active: f.active, classIds: f.class_ids,
  })

  async function save() {
    const f = ed.item
    if (!f || !year) return
    const e: Record<string, string> = {}
    const amount = parseMoney(f.amount)
    const rate = f.vat === 'none' ? 0 : parsePct(f.rate)
    const code = f.code || slug(f.name)
    if (!f.name.trim()) e.name = 'Obrigatório.'
    if (amount === null || amount < 0) e.amount = 'Valor inválido.'
    if (rate === null) e.vat_rate_bp = 'Taxa inválida.'
    if (f.kind === 'one_off' && f.due === 'date' && !parseDate(f.dueDate)) e.due_rule = 'Data dd/mm/aaaa.'
    if (Object.keys(e).length) return ed.setErrors(e)
    const body = {
      school_year_id: year.id, code, name: f.name.trim(), kind: f.kind, amount, vat_mode: f.vat, vat_rate_bp: rate,
      due_rule: f.kind === 'monthly' ? {} : f.due === 'date' ? { date: parseDate(f.dueDate) } : { on_enrolment: true },
      mandatory: f.mandatory, active: f.active, class_ids: f.classIds,
    }
    ed.setSaving(true)
    try {
      if (f.id) await api.put(`/fee-items/${f.id}`, body)
      else await api.post('/fee-items', body)
      toast.success('Preço guardado. As cobranças já lançadas mantêm o valor antigo.')
      void qc.invalidateQueries({ queryKey: ['fee-items'] })
      ed.close()
    } catch (err) {
      handleError(err, ed.setErrors)
    } finally {
      ed.setSaving(false)
    }
  }

  const section = (kind: FeeForm['kind'], title: string, description: string) => {
    const items = (fees.data?.items ?? []).filter((f) => f.kind === kind)
    return (
      <Card>
        <CardHeader title={title} description={description} action={<Button size="sm" onClick={() => ed.open(blank(kind))}><Plus className="h-4 w-4" /> Novo</Button>} />
        {!fees.data ? <div className="p-4"><Skeleton className="h-24" /></div> : items.length === 0 ? (
          <EmptyState title="Nada configurado" description={kind === 'monthly' ? `Sem mensalidade não é possível matricular em ${year?.year}. Pode copiar a tabela do ano anterior em Anos lectivos.` : undefined} />
        ) : (
          <Table>
            <thead><tr><Th>Item</Th><Th className="hidden lg:table-cell">Turmas</Th>{kind === 'one_off' && <Th className="hidden md:table-cell">Vence</Th>}<Th align="right">IVA</Th><Th align="right">Valor</Th><Th /></tr></thead>
            <tbody>
              {items.map((f) => (
                <tr key={f.id} className={cn(!f.active && 'opacity-50')}>
                  <Td>
                    <p className="font-medium text-navy">{f.name}</p>
                    <p className="text-xs text-ink-muted">{f.code}{f.mandatory ? ' · obrigatório' : ' · opcional'}{!f.active && ' · inactivo'}</p>
                  </Td>
                  <Td className="hidden max-w-xs truncate text-ink-muted lg:table-cell">{scope(f)}</Td>
                  {kind === 'one_off' && <Td className="hidden md:table-cell">{f.due_rule?.date ? date(f.due_rule.date) : 'Na matrícula'}</Td>}
                  <Td align="right" className="whitespace-nowrap text-ink-muted">{f.vat_mode === 'none' ? '—' : `${percentBP(f.vat_rate_bp)} ${f.vat_mode === 'included' ? 'incl.' : 'acresc.'}`}</Td>
                  <Td align="right" className="whitespace-nowrap font-semibold">{money(f.lines.gross)}</Td>
                  <Td align="right"><Button size="sm" variant="ghost" aria-label="Editar" onClick={() => ed.open(edit(f))}><Pencil className="h-4 w-4" /></Button></Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    )
  }

  const f = ed.item
  return (
    <div className="space-y-5">
      {section('monthly', `Serviços mensais · ${year?.year}`, 'Mensalidades e serviços opcionais (almoço, extraclasse). Cobrados nos meses do ano lectivo.')}
      {section('one_off', `Taxas únicas · ${year?.year}`, 'Matrícula, renovação, uniforme, livros. Cobradas uma vez.')}
      <FormDialog open={!!f} title={f?.id ? 'Editar preço' : f?.kind === 'monthly' ? 'Novo serviço mensal' : 'Nova taxa única'} description={`${entity.short_name} · ${year?.year}`} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {f && (
          <>
            <div className="grid gap-3 sm:grid-cols-[1fr_160px]">
              <Field label="Nome" error={ed.errors.name}><Input autoFocus value={f.name} onChange={(e) => ed.patch({ name: e.target.value })} placeholder={f.kind === 'monthly' ? 'Ex.: Mensalidade 1.ª–3.ª classe' : 'Ex.: Uniforme completo'} /></Field>
              <Field label="Valor (MT)" error={ed.errors.amount}><Input inputMode="decimal" value={f.amount} onChange={(e) => ed.patch({ amount: e.target.value })} onBlur={() => { const v = parseMoney(f.amount); if (v !== null) ed.patch({ amount: moneyInput(v) }) }} /></Field>
            </div>
            <Field label="Código" hint={f.id ? 'Não muda depois de criado (usado na renovação e nos descontos).' : 'Gerado a partir do nome; usado para ligar anos e descontos.'} error={ed.errors.code}>
              <Input value={f.code || slug(f.name)} disabled={!!f.id} onChange={(e) => ed.patch({ code: e.target.value })} />
            </Field>
            <div className="grid gap-3 sm:grid-cols-[1fr_120px]">
              <Field label="IVA">
                <Select value={f.vat} onChange={(e) => ed.patch({ vat: e.target.value as FeeForm['vat'] })}>
                  <option value="none">Sem IVA</option>
                  <option value="included">IVA incluído no valor</option>
                  <option value="added">IVA acrescentado ao valor</option>
                </Select>
              </Field>
              {f.vat !== 'none' && <Field label="Taxa (%)" error={ed.errors.vat_rate_bp}><Input inputMode="decimal" value={f.rate} onChange={(e) => ed.patch({ rate: e.target.value })} /></Field>}
            </div>
            {f.kind === 'one_off' && (
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Vencimento">
                  <Segmented value={f.due} onChange={(v) => ed.patch({ due: v })} options={[{ value: 'enrolment', label: 'Na matrícula' }, { value: 'date', label: 'Data fixa' }]} className="w-full" />
                </Field>
                {f.due === 'date' && <Field label="Data" error={ed.errors.due_rule} hint="Se já passou, cobra-se na matrícula."><Input value={f.dueDate} placeholder="dd/mm/aaaa" onChange={(e) => ed.patch({ dueDate: e.target.value })} /></Field>}
              </div>
            )}
            <Field label="Turmas" hint="Nenhuma marcada = todas as turmas.">
              <div className="flex flex-wrap gap-1.5">
                {cls.map((c) => {
                  const on = f.classIds.includes(c.id)
                  return (
                    <button key={c.id} type="button" onClick={() => ed.patch({ classIds: on ? f.classIds.filter((x) => x !== c.id) : [...f.classIds, c.id] })}
                      className={cn('h-8 rounded-full border px-3 text-sm', on ? 'border-brand bg-brand-tint font-semibold text-brand' : 'border-line text-ink-muted hover:bg-page')}>
                      {c.name}
                    </button>
                  )
                })}
                {cls.length === 0 && <span className="text-sm text-ink-muted">Ainda não há turmas neste ano.</span>}
              </div>
            </Field>
            <div className="grid gap-x-4 sm:grid-cols-2">
              <Checkbox checked={f.mandatory} onChange={(v) => ed.patch({ mandatory: v })} label="Obrigatório" hint={f.kind === 'monthly' ? 'Incluído em todas as matrículas destas turmas.' : 'Não pode ser desmarcado na matrícula.'} />
              <Checkbox checked={f.active} onChange={(v) => ed.patch({ active: v })} label="Activo" hint="Inactivo deixa de aparecer em novas matrículas." />
            </div>
          </>
        )}
      </FormDialog>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Penalty rules per level
// ---------------------------------------------------------------------------

export function describeRule(r: PenaltyRule) {
  if (r.type === 'none') return 'Sem multa'
  if (r.type === 'percent') return `${percentBP(r.rate_bp ?? 0)} do valor em atraso${r.grace_days ? `, após ${r.grace_days} dias` : ''}`
  const tiers = (r.tiers ?? []).map((t) => `${money(t.amount)} após ${t.after_day} dias`).join(', ')
  return `${tiers} (${r.mode === 'cumulative' ? 'acumulam' : 'substituem'})`
}

interface RuleForm { level: Level; type: PenaltyRule['type']; rate: string; grace: string; mode: 'replace' | 'cumulative'; tiers: { day: string; amount: string }[] }

export function PenaltiesTab() {
  const { entity } = useSession()
  const qc = useQueryClient()
  const ed = useEditor<RuleForm>()
  const levels = useQuery({ queryKey: ['levels', entity?.id ?? 'all'], queryFn: () => api.get<List<Level>>('/levels'), enabled: !!entity })
  if (!entity) return <NeedEntity />

  const open = (l: Level) => ed.open({
    level: l, type: l.penalty_rule.type, rate: l.penalty_rule.rate_bp ? pctText(l.penalty_rule.rate_bp) : '5', grace: String(l.penalty_rule.grace_days ?? 0),
    mode: l.penalty_rule.mode ?? 'replace', tiers: (l.penalty_rule.tiers ?? [{ after_day: 5, amount: 10000 }]).map((t) => ({ day: String(t.after_day), amount: moneyInput(t.amount) })),
  })
  async function save() {
    const f = ed.item
    if (!f) return
    const e: Record<string, string> = {}
    let rule: PenaltyRule = { type: 'none', grace_days: 0 }
    if (f.type === 'percent') {
      const bp = parsePct(f.rate)
      if (bp === null) e.rate = 'Percentagem inválida.'
      rule = { type: 'percent', rate_bp: bp ?? 0, grace_days: Number(f.grace) || 0 }
    } else if (f.type === 'fixed_tiers') {
      const tiers = f.tiers.map((t) => ({ after_day: Number(t.day), amount: parseMoney(t.amount) ?? 0 }))
      if (tiers.some((t) => !(t.after_day > 0) || t.amount <= 0)) e.tiers = 'Cada escalão precisa de dias e valor.'
      rule = { type: 'fixed_tiers', grace_days: 0, mode: f.mode, tiers: tiers.sort((a, b) => a.after_day - b.after_day) }
    }
    if (Object.keys(e).length) return ed.setErrors(e)
    ed.setSaving(true)
    try {
      await api.put(`/levels/${f.level.id}`, { name: f.level.name, penalty_rule: rule })
      toast.success('Regra de multa guardada. Aplica-se às próximas multas calculadas.')
      void qc.invalidateQueries({ queryKey: ['levels'] })
      ed.close()
    } catch (err) {
      handleError(err, (x) => ed.setErrors({ tiers: Object.values(x).join(' ') }))
    } finally {
      ed.setSaving(false)
    }
  }

  const items = (levels.data?.items ?? []).filter((l) => l.entity_id === entity.id)
  const f = ed.item
  return (
    <Card>
      <CardHeader title="Multas por atraso" description="Por nível de ensino. Só sobre mensalidades; uma multa já lançada ou perdoada nunca é repetida." />
      {!levels.data ? <div className="p-4"><Skeleton className="h-24" /></div> : (
        <ul className="divide-y divide-line">
          {items.map((l) => (
            <li key={l.id} className="flex items-center gap-3 px-5 py-4">
              <div className="min-w-0 flex-1">
                <p className="font-semibold text-navy">{l.name}</p>
                <p className="text-sm text-ink-muted">{describeRule(l.penalty_rule)}</p>
              </div>
              <Button size="sm" variant="secondary" onClick={() => open(l)}><Pencil className="h-4 w-4" /> Alterar</Button>
            </li>
          ))}
        </ul>
      )}
      <FormDialog open={!!f} title={`Multa · ${f?.level.name ?? ''}`} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {f && (
          <>
            <Segmented value={f.type} onChange={(v) => ed.patch({ type: v })} className="w-full"
              options={[{ value: 'none', label: 'Sem multa' }, { value: 'percent', label: 'Percentagem' }, { value: 'fixed_tiers', label: 'Valores fixos' }]} />
            {f.type === 'percent' && (
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Percentagem do valor em atraso" error={ed.errors.rate}><Input inputMode="decimal" value={f.rate} onChange={(e) => ed.patch({ rate: e.target.value })} /></Field>
                <Field label="Dias de tolerância" hint="Depois do dia de vencimento."><Input inputMode="numeric" value={f.grace} onChange={(e) => ed.patch({ grace: e.target.value })} /></Field>
              </div>
            )}
            {f.type === 'fixed_tiers' && (
              <>
                <Field label="Quando passa para o escalão seguinte">
                  <Segmented value={f.mode} onChange={(v) => ed.patch({ mode: v })} className="w-full"
                    options={[{ value: 'replace', label: 'Substitui (fica o maior)' }, { value: 'cumulative', label: 'Acumula (soma)' }]} />
                </Field>
                <div className="space-y-2">
                  {f.tiers.map((t, i) => (
                    <div key={i} className="grid grid-cols-[1fr_1fr_auto] items-end gap-2">
                      <Field label={i === 0 ? 'Dias após o vencimento' : ''}><Input inputMode="numeric" value={t.day} onChange={(e) => ed.patch({ tiers: f.tiers.map((x, j) => (j === i ? { ...x, day: e.target.value } : x)) })} /></Field>
                      <Field label={i === 0 ? 'Multa (MT)' : ''}><Input inputMode="decimal" value={t.amount} onChange={(e) => ed.patch({ tiers: f.tiers.map((x, j) => (j === i ? { ...x, amount: e.target.value } : x)) })} /></Field>
                      <Button type="button" variant="ghost" size="icon" aria-label="Remover escalão" disabled={f.tiers.length === 1} onClick={() => ed.patch({ tiers: f.tiers.filter((_, j) => j !== i) })}><Trash2 className="h-4 w-4" /></Button>
                    </div>
                  ))}
                  {f.tiers.length < 5 && <Button type="button" size="sm" variant="secondary" onClick={() => ed.patch({ tiers: [...f.tiers, { day: '', amount: '' }] })}><Plus className="h-4 w-4" /> Escalão</Button>}
                </div>
              </>
            )}
            {ed.errors.tiers && <p className="text-sm text-bad">{ed.errors.tiers}</p>}
          </>
        )}
      </FormDialog>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Discount types
// ---------------------------------------------------------------------------

interface TypeForm { id?: string; code: string; name: string; mode: 'percent' | 'fixed'; value: string; codes: string[]; approval: boolean; active: boolean }

export function DiscountTypesTab() {
  const { entity, year } = useSession()
  const qc = useQueryClient()
  const ed = useEditor<TypeForm>()
  const types = useQuery({ queryKey: ['discount-types', entity?.id], queryFn: () => api.get<List<DiscountType>>('/discount-types'), enabled: !!entity })
  const fees = useQuery({ queryKey: ['fee-items', entity?.id, year?.year], queryFn: () => api.get<List<FeeItem>>('/fee-items', { year: year?.year }), enabled: !!entity && !!year })
  if (!entity) return <NeedEntity />
  const monthly = (fees.data?.items ?? []).filter((f) => f.kind === 'monthly')
  const codeName = (c: string) => monthly.find((f) => f.code === c)?.name ?? c

  async function save() {
    const f = ed.item
    if (!f) return
    const e: Record<string, string> = {}
    let value = 0
    if (f.value.trim()) {
      const v = f.mode === 'percent' ? parsePct(f.value) : parseMoney(f.value)
      if (v === null) e.value = 'Valor inválido.'
      else value = v
    }
    if (!f.name.trim()) e.name = 'Obrigatório.'
    if (f.codes.length === 0) e.fee_item_codes = 'Escolha a que mensalidades se aplica.'
    if (Object.keys(e).length) return ed.setErrors(e)
    ed.setSaving(true)
    try {
      const body = { code: f.code || slug(f.name), name: f.name.trim(), mode: f.mode, value, fee_item_codes: f.codes, requires_approval: f.approval, active: f.active }
      if (f.id) await api.put(`/discount-types/${f.id}`, body)
      else await api.post('/discount-types', body)
      toast.success('Tipo de desconto guardado.')
      void qc.invalidateQueries({ queryKey: ['discount-types'] })
      ed.close()
    } catch (err) {
      handleError(err, ed.setErrors)
    } finally {
      ed.setSaving(false)
    }
  }

  const f = ed.item
  return (
    <Card>
      <CardHeader title="Tipos de desconto" description="O valor é a sugestão ao pedir; cada pedido pode ajustar."
        action={<Button size="sm" onClick={() => ed.open({ code: '', name: '', mode: 'percent', value: '', codes: monthly.filter((m) => m.mandatory).map((m) => m.code), approval: true, active: true })}><Plus className="h-4 w-4" /> Novo tipo</Button>} />
      {!types.data ? <div className="p-4"><Skeleton className="h-24" /></div> : (
        <Table>
          <thead><tr><Th>Tipo</Th><Th>Valor</Th><Th className="hidden md:table-cell">Sobre</Th><Th>Aprovação</Th><Th /></tr></thead>
          <tbody>
            {types.data.items.map((t) => (
              <tr key={t.id} className={cn(!t.active && 'opacity-50')}>
                <Td><p className="font-medium text-navy">{t.name}</p><p className="text-xs text-ink-muted">{t.code}{!t.active && ' · inactivo'}</p></Td>
                <Td className="whitespace-nowrap">{t.value === 0 ? <span className="text-ink-muted">caso a caso</span> : t.mode === 'percent' ? percentBP(t.value) : money(t.value)}</Td>
                <Td className="hidden max-w-xs truncate text-ink-muted md:table-cell">{t.fee_item_codes.map(codeName).join(', ')}</Td>
                <Td>{t.requires_approval ? <Badge tone="warn">Direcção</Badge> : <Badge tone="ok">Automática</Badge>}</Td>
                <Td align="right"><Button size="sm" variant="ghost" aria-label="Editar" onClick={() => ed.open({ id: t.id, code: t.code, name: t.name, mode: t.mode, value: t.value ? (t.mode === 'percent' ? pctText(t.value) : moneyInput(t.value)) : '', codes: t.fee_item_codes, approval: t.requires_approval, active: t.active })}><Pencil className="h-4 w-4" /></Button></Td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
      <FormDialog open={!!f} title={f?.id ? 'Editar tipo de desconto' : 'Novo tipo de desconto'} description={entity.short_name} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {f && (
          <>
            <Field label="Nome" error={ed.errors.name}><Input autoFocus value={f.name} onChange={(e) => ed.patch({ name: e.target.value })} placeholder="Ex.: Filho de funcionário" /></Field>
            <div className="grid gap-3 sm:grid-cols-[auto_1fr]">
              <Field label="Tipo"><Segmented value={f.mode} onChange={(v) => ed.patch({ mode: v, value: '' })} options={[{ value: 'percent', label: '%' }, { value: 'fixed', label: 'MT' }]} /></Field>
              <Field label={f.mode === 'percent' ? 'Percentagem por mês' : 'Valor por mês (MT)'} hint="Vazio = decidido caso a caso." error={ed.errors.value}><Input inputMode="decimal" value={f.value} onChange={(e) => ed.patch({ value: e.target.value })} /></Field>
            </div>
            <Field label="Aplica-se a" error={ed.errors.fee_item_codes}>
              <div className="space-y-0.5 rounded-lg border border-line p-2">
                {monthly.map((m) => (
                  <Checkbox key={m.code} checked={f.codes.includes(m.code)} label={m.name}
                    onChange={(v) => ed.patch({ codes: v ? [...f.codes, m.code] : f.codes.filter((c) => c !== m.code) })} />
                ))}
                {monthly.length === 0 && <p className="px-1 py-1 text-sm text-ink-muted">Sem serviços mensais em {year?.year}.</p>}
              </div>
            </Field>
            <div className="grid gap-x-4 sm:grid-cols-2">
              <Checkbox checked={f.approval} onChange={(v) => ed.patch({ approval: v })} label="Precisa de aprovação" hint="Sem aprovação, aplica-se logo ao pedir." />
              <Checkbox checked={f.active} onChange={(v) => ed.patch({ active: v })} label="Activo" />
            </div>
          </>
        )}
      </FormDialog>
    </Card>
  )
}
