import * as React from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AlertTriangle, ArrowLeft, Ban, ClipboardPlus, FileText, LogOut, Pencil, Percent, HeartPulse, Phone, Printer, Receipt, ShieldCheck, Wallet } from 'lucide-react'
import { api, openDocument } from '@/lib/api'
import { age, date, dateTime, money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import {
  Avatar, Badge, Button, Card, CardHeader, ChargeStatus, EmptyState, EntityTag, ErrorState, Skeleton, Table, Tabs, TabsContent, TabsList, Td, Th,
} from '@/components/ui'
import { usePaymentSheet } from '@/features/payments/PaymentSheet'
import { WithdrawDialog } from '@/features/enrolment/WithdrawDialog'
import { ChargeActionDialog, type ChargeRef } from '@/features/charges/ChargeActionDialog'
import { describeValue, DiscountRequestDialog } from '@/features/discounts/DiscountRequestDialog'
import type { AccountCharge, DiscountRequest, Enrolment, List, Payment, PupilAccount, PupilProfile, StatementRef } from '@/api/types'

const orphanLabel: Record<string, string> = { none: 'Não', father: 'Órfão de pai', mother: 'Órfão de mãe', both: 'Órfão de pai e mãe' }

function Info({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs text-ink-muted">{label}</dt>
      <dd className="mt-0.5 text-sm font-medium text-ink">{value || '—'}</dd>
    </div>
  )
}

export function PupilProfilePage() {
  const { id = '' } = useParams()
  const { year, entity, entities, can } = useSession()
  const payment = usePaymentSheet()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [withdrawing, setWithdrawing] = React.useState(false)
  const [discounting, setDiscounting] = React.useState(false)
  const [acting, setActing] = React.useState<ChargeRef | null>(null)

  const profile = useQuery({ queryKey: ['pupil', id], queryFn: () => api.get<PupilProfile>(`/pupils/${id}`) })
  const p = profile.data
  // Enrolment of the selected year in the selected entity (or any accessible one under "Todas").
  const current = p?.enrolments.find((e) => e.year === year?.year && (!entity || e.entity_id === entity.id))
  const enrolment = useQuery({
    queryKey: ['enrolment', current?.id],
    queryFn: () => api.get<Enrolment>(`/enrolments/${current!.id}`),
    enabled: !!current,
  })
  const account = useQuery({
    queryKey: ['account', id, entity?.id ?? 'all', year?.year],
    queryFn: () => api.get<PupilAccount>(`/pupils/${id}/account`, { year: year?.year }),
    enabled: can('charges.read') && !!year,
  })
  const payments = useQuery({
    queryKey: ['payments', 'pupil', id, entity?.id ?? 'all'],
    queryFn: () => api.get<List<Payment>>('/payments', { pupil_id: id, page_size: 100 }),
    enabled: can('charges.read'),
  })
  const discounts = useQuery({
    queryKey: ['discount-requests', 'enrolment', current?.id],
    queryFn: () => api.get<List<DiscountRequest>>('/discount-requests', { enrolment_id: current!.id }),
    enabled: can('charges.read') && current?.status === 'confirmed',
  })
  const statements = useQuery({
    queryKey: ['statements', id, entity?.id ?? 'all'],
    queryFn: () => api.get<List<StatementRef>>('/statements', { pupil_id: id }),
    enabled: can('charges.read'),
  })
  const issue = useMutation({
    mutationFn: () => api.post<StatementRef>('/statements', { pupil_id: id, year: year?.year }),
    onSuccess: (s) => {
      void qc.invalidateQueries({ queryKey: ['statements', id] })
      toast.success(`Extrato ${s.number} emitido`)
      openDocument(`/statements/${s.id}`).catch((e) => toast.error(e.message))
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : pt.errors.generic),
  })

  if (profile.isError) return <ErrorState error={profile.error} onRetry={() => profile.refetch()} />
  if (!p) return <div className="space-y-4"><Skeleton className="h-40 rounded-card" /><Skeleton className="h-96 rounded-card" /></div>

  const summary = account.data?.summary
  const entRef = entities.find((e) => e.id === current?.entity_id)
  const h = p.health

  return (
    <>
      <Link to="/alunos" className="mb-3 inline-flex items-center gap-1 text-sm font-medium text-ink-muted hover:text-navy"><ArrowLeft className="h-4 w-4" /> {pt.nav.pupils}</Link>

      <Card className="p-5">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-center">
          <div className="flex flex-1 items-center gap-4">
            <Avatar name={p.full_name} className="h-16 w-16 text-xl" />
            <div className="min-w-0">
              <h1 className="text-[22px] font-bold leading-tight text-navy">{p.full_name}</h1>
              <div className="mt-1.5 flex flex-wrap items-center gap-2 text-sm text-ink-muted">
                <span>{p.number}</span>
                <span>·</span>
                <span>{age(p.birth_date)} anos</span>
                {current?.class_name && <><span>·</span><span className="font-medium text-ink">{current.class_name}</span></>}
                {entRef && <EntityTag code={entRef.code} name={entRef.short_name} />}
                {current ? <Badge tone={current.status === 'confirmed' ? 'ok' : 'warn'}>{pt.enrolmentStatus[current.status]}</Badge> : <Badge>Sem matrícula em {year?.year}</Badge>}
              </div>
            </div>
          </div>
          {summary && (
            <div className="grid grid-cols-2 gap-3 sm:min-w-[340px]">
              <div className={cn('rounded-lg p-3', summary.overdue > 0 ? 'bg-bad-tint' : 'bg-ok-tint')}>
                <p className="text-xs text-ink-muted">{pt.account.overdue}</p>
                <p className={cn('text-xl font-bold tabular-nums', summary.overdue > 0 ? 'text-bad' : 'text-ok')}>{money(summary.overdue)}</p>
              </div>
              <div className="rounded-lg bg-page p-3">
                <p className="text-xs text-ink-muted">{pt.account.outstanding}</p>
                <p className="text-xl font-bold tabular-nums text-navy">{money(summary.outstanding)}</p>
              </div>
            </div>
          )}
        </div>
        <div className="mt-5 flex flex-wrap gap-2 border-t border-line pt-4">
          {can('payments.create') && entity && current && current.status !== 'draft' && (
            <Button variant="accent" onClick={() => payment.open({ id: p.id, name: p.full_name, number: p.number, className: current.class_name })}>
              <Wallet className="h-4 w-4" /> {pt.actions.recordPayment}
            </Button>
          )}
          {can('documents.generate') && entity && current && current.status !== 'draft' && (
            <Button variant="secondary" loading={issue.isPending} onClick={() => issue.mutate()}><FileText className="h-4 w-4" /> Emitir extrato</Button>
          )}
          {current && (
            <Button variant="secondary" onClick={() => openDocument(`/enrolments/${current.id}/form`).catch((e) => toast.error(e.message))}>
              <Printer className="h-4 w-4" /> Ficha de matrícula
            </Button>
          )}
          {can('enrolments.write') && entity && current?.status === 'draft' && current.entity_id === entity.id && (
            <Button onClick={() => navigate(`/matriculas/${current.id}`)}><ClipboardPlus className="h-4 w-4" /> Continuar matrícula</Button>
          )}
          {can('enrolments.write') && entity && !current && p.status === 'active' && (
            <Button onClick={() => navigate(`/matriculas/nova?pupil=${p.id}`)}><ClipboardPlus className="h-4 w-4" /> Matricular em {year?.year}</Button>
          )}
          {can('discounts.request') && entity && current?.status === 'confirmed' && current.entity_id === entity.id && (
            <Button variant="ghost" onClick={() => setDiscounting(true)}><Percent className="h-4 w-4" /> Pedir desconto</Button>
          )}
          {can('pupils.write') && (
            <Button variant="ghost" onClick={() => navigate(`/alunos/${p.id}/editar`)}><Pencil className="h-4 w-4" /> Editar dados</Button>
          )}
          {can('enrolments.write') && entity && current?.status === 'confirmed' && current.entity_id === entity.id && (
            <Button variant="ghost" className="text-bad hover:bg-bad-tint sm:ml-auto" onClick={() => setWithdrawing(true)}><LogOut className="h-4 w-4" /> Registar desistência</Button>
          )}
        </div>
        {current && discounting && (
          <DiscountRequestDialog open onClose={() => setDiscounting(false)} target={{ enrolmentId: current.id, pupilName: p.full_name, className: current.class_name }} />
        )}
        <ChargeActionDialog charge={acting} onClose={() => setActing(null)} />
        {current && withdrawing && (
          <WithdrawDialog enrolmentId={current.id} pupilId={p.id} pupilName={p.full_name} open={withdrawing} onOpenChange={setWithdrawing} />
        )}
      </Card>

      {p.health_alert && (
        <div className="mt-4 flex items-start gap-3 rounded-card border border-bad/25 bg-bad-tint px-4 py-3 text-sm text-bad">
          <HeartPulse className="mt-0.5 h-5 w-5 shrink-0" />
          <div>
            <p className="font-semibold">Atenção à saúde</p>
            {h ? (
              <p className="text-ink">
                {[h.allergies && `Alergias: ${h.allergies}`, h.medication && `Medicação: ${h.medication}`, h.diet_restrictions && `Dieta: ${h.diet_restrictions}`, h.chronic_condition && `Doença crónica: ${h.chronic_condition}`].filter(Boolean).join(' · ')}
              </p>
            ) : (
              <p className="text-ink">Este aluno tem informação de saúde registada. Peça acesso para a ver.</p>
            )}
          </div>
        </div>
      )}

      <Tabs defaultValue={can('charges.read') ? 'account' : 'summary'} className="mt-5">
        <TabsList
          tabs={[
            { value: 'summary', label: 'Resumo' },
            ...(can('charges.read') ? [{ value: 'account', label: 'Conta corrente' }, { value: 'payments', label: 'Pagamentos' }] : []),
            { value: 'guardians', label: 'Encarregados' },
            ...(h ? [{ value: 'health', label: 'Saúde' }] : []),
            ...(can('charges.read') ? [{ value: 'documents', label: 'Documentos' }] : []),
            { value: 'history', label: 'Histórico' },
          ]}
        />

        <TabsContent value="summary" className="mt-4 grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader title="Dados pessoais" />
            <dl className="grid grid-cols-2 gap-4 p-5">
              <Info label="Sexo" value={p.sex === 'F' ? 'Feminino' : 'Masculino'} />
              <Info label="Data de nascimento" value={date(p.birth_date)} />
              <Info label="Bairro" value={p.neighbourhood} />
              <Info label="Religião" value={p.religion} />
              <Info label="Orfandade" value={orphanLabel[p.orphan]} />
              <Info label="Estado" value={p.status === 'active' ? 'Activo' : 'Saiu da escola'} />
            </dl>
          </Card>
          <Card>
            <CardHeader title={`Serviços ${year?.year ?? ''}`} description={enrolment.data ? `Desde ${pt.months[enrolment.data.start_month]}` : undefined} />
            {!current && <p className="p-5 text-sm text-ink-muted">Sem matrícula neste ano e empresa.</p>}
            {enrolment.data && (
              <div className="p-5">
                <ul className="divide-y divide-line">
                  {enrolment.data.services.filter((s) => s.to_month === null).map((s) => (
                    <li key={s.fee_item_id + s.from_month} className="flex justify-between py-2 text-sm">
                      <span>{s.name}{s.mandatory && <span className="ml-2 text-xs text-ink-muted">obrigatório</span>}</span>
                      <span className="tabular-nums">{money(s.amount)}</span>
                    </li>
                  ))}
                </ul>
                <div className="mt-2 flex justify-between border-t-2 border-navy pt-2 text-sm font-bold text-navy">
                  <span>Total mensal</span>
                  <span className="tabular-nums">{money(enrolment.data.totals.monthly_gross)}</span>
                </div>
              </div>
            )}
          </Card>
        </TabsContent>

        <TabsContent value="account" className="mt-4">
          {discounts.data && discounts.data.items.some((d) => d.status !== 'rejected') && (
            <ul className="mb-4 space-y-2">
              {discounts.data.items.filter((d) => d.status !== 'rejected').map((d) => (
                <li key={d.id} className={cn('flex flex-wrap items-center gap-2 rounded-card border px-4 py-2.5 text-sm',
                  d.status === 'pending' ? 'border-warn/30 bg-warn-tint' : 'border-ok/25 bg-ok-tint')}>
                  <Percent className="h-4 w-4 shrink-0 text-ink-muted" />
                  <span className="font-medium text-navy">{d.discount_type} · {describeValue(d)}</span>
                  <span className="text-ink-muted">desde {pt.months[d.scope.from_month]}</span>
                  <Badge tone={d.status === 'pending' ? 'warn' : 'ok'} className="ml-auto">
                    {pt.discountStatus[d.status]}{d.status === 'approved' && ` · ${money(d.estimated_amount)}`}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
          <AccountTab
            account={account.data}
            loading={account.isLoading}
            canAct={(c) => !!entity && c.entity_id === entity.id && c.status !== 'cancelled' && (c.kind === 'penalty' ? can('penalties.waive') : can('charges.cancel'))}
            onAct={(c) => setActing({ ...c, pupil_name: p.full_name })}
          />
        </TabsContent>

        <TabsContent value="payments" className="mt-4">
          <Card>
            {payments.data && payments.data.items.length === 0 && <EmptyState icon={<Receipt className="h-6 w-6" />} title="Sem pagamentos" />}
            {payments.data && payments.data.items.length > 0 && (
              <Table>
                <thead><tr><Th>Recibo</Th><Th>Data</Th><Th>Método</Th><Th>Referência</Th><Th>Recebido por</Th><Th align="right">Valor</Th><Th /></tr></thead>
                <tbody>
                  {payments.data.items.map((pm) => (
                    <tr key={pm.id} className={cn(pm.cancelled_at && 'opacity-60')}>
                      <Td className="font-semibold text-navy">{pm.receipt_no}{pm.cancelled_at && <Badge tone="bad" className="ml-2">Anulado</Badge>}</Td>
                      <Td>{date(pm.paid_on)}</Td>
                      <Td>{pt.methods[pm.method]}</Td>
                      <Td className="text-ink-muted">{pm.reference || '—'}</Td>
                      <Td className="text-ink-muted">{pm.received_by_name}</Td>
                      <Td align="right" className="font-semibold">{money(pm.amount)}</Td>
                      <Td align="right">
                        <Button size="sm" variant="ghost" onClick={() => openDocument(`/payments/${pm.id}/receipt`).catch((e) => toast.error(e.message))}><FileText className="h-4 w-4" /> Recibo</Button>
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            )}
          </Card>
        </TabsContent>

        <TabsContent value="guardians" className="mt-4 grid gap-4 lg:grid-cols-2">
          {p.guardians.map((g) => (
            <Card key={g.guardian_id} className="p-5">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <Link to={`/encarregados/${g.guardian_id}`} className="font-semibold text-navy hover:underline">{g.full_name}</Link>
                  <p className="text-sm text-ink-muted">{pt.relationship[g.relationship]}{g.neighbourhood && ` · ${g.neighbourhood}`}</p>
                </div>
                {g.is_payer && <Badge tone="info">Responsável pelos pagamentos</Badge>}
              </div>
              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-sm">
                <a href={`tel:${g.phone1}`} className="inline-flex items-center gap-1 text-brand"><Phone className="h-4 w-4" /> {g.phone1}</a>
                {g.phone2 && <a href={`tel:${g.phone2}`} className="inline-flex items-center gap-1 text-brand"><Phone className="h-4 w-4" /> {g.phone2}</a>}
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                {g.receives_notices && <Badge>Recebe comunicações</Badge>}
                {g.may_pick_up && <Badge tone="ok">Pode buscar</Badge>}
              </div>
            </Card>
          ))}
          <Card>
            <CardHeader title="Pessoas autorizadas a buscar" />
            <ul className="divide-y divide-line">
              {p.pickup_persons.length === 0 && <li className="px-5 py-4 text-sm text-ink-muted">Apenas os encarregados.</li>}
              {p.pickup_persons.map((x, i) => (
                <li key={i} className="flex justify-between px-5 py-3 text-sm"><span className="font-medium">{x.full_name}</span><span className="text-ink-muted">{x.phone} {x.id_doc && `· ${x.id_doc}`}</span></li>
              ))}
            </ul>
          </Card>
          <Card>
            <CardHeader title="Contactos de emergência" />
            <ol className="divide-y divide-line">
              {p.emergency_contacts.length === 0 && <li className="px-5 py-4 text-sm text-ink-muted">—</li>}
              {p.emergency_contacts.map((x) => (
                <li key={x.position} className="flex justify-between px-5 py-3 text-sm">
                  <span><span className="mr-2 text-ink-muted">{x.position}.</span><span className="font-medium">{x.full_name}</span> <span className="text-ink-muted">{x.relationship}</span></span>
                  <a href={`tel:${x.phone}`} className="text-brand">{x.phone}</a>
                </li>
              ))}
            </ol>
          </Card>
        </TabsContent>

        {h && (
          <TabsContent value="health" className="mt-4">
            <Card>
              <dl className="grid gap-5 p-5 sm:grid-cols-2">
                <Info label="Doença crónica" value={h.chronic_condition || 'Não'} />
                <Info label="Alergias" value={h.allergies || 'Não'} />
                <Info label="Restrições alimentares" value={h.diet_restrictions || 'Não'} />
                <Info label="Medicação" value={h.medication || 'Não'} />
                <Info label="Médico" value={[h.doctor_name, h.doctor_phone].filter(Boolean).join(' · ')} />
                <Info label="Autorização para assistência de emergência" value={h.emergency_care_consent_at ? `Sim, desde ${dateTime(h.emergency_care_consent_at)}` : 'Não'} />
                {h.notes && <div className="sm:col-span-2"><Info label="Notas" value={h.notes} /></div>}
              </dl>
            </Card>
          </TabsContent>
        )}

        <TabsContent value="documents" className="mt-4">
          <Card>
            <CardHeader title="Extratos emitidos" />
            {statements.data?.items.length === 0 && <p className="px-5 py-4 text-sm text-ink-muted">Ainda não foi emitido nenhum extrato.</p>}
            <ul className="divide-y divide-line">
              {statements.data?.items.map((s) => (
                <li key={s.id} className="flex items-center justify-between gap-3 px-5 py-3">
                  <div>
                    <p className="text-sm font-semibold text-navy">{s.number}</p>
                    <p className="text-xs text-ink-muted">Ano {s.year} · emitido {dateTime(s.issued_at)} por {s.issued_by}</p>
                  </div>
                  <Button size="sm" variant="secondary" onClick={() => openDocument(`/statements/${s.id}`).catch((e) => toast.error(e.message))}><FileText className="h-4 w-4" /> Abrir</Button>
                </li>
              ))}
            </ul>
          </Card>
        </TabsContent>

        <TabsContent value="history" className="mt-4">
          <Card>
            <Table>
              <thead><tr><Th>Ano</Th><Th>Empresa</Th><Th>Turma</Th><Th>Tipo</Th><Th>Estado</Th></tr></thead>
              <tbody>
                {p.enrolments.map((e) => (
                  <tr key={e.id}>
                    <Td className="font-semibold">{e.year}</Td>
                    <Td>{e.entity_name}</Td>
                    <Td>{e.class_name ?? '—'}</Td>
                    <Td>{e.type === 'new' ? 'Novo' : 'Renovação'}</Td>
                    <Td><Badge tone={e.status === 'confirmed' ? 'ok' : e.status === 'draft' ? 'warn' : 'neutral'}>{pt.enrolmentStatus[e.status]}</Badge></Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  )
}

function AccountTab({ account, loading, canAct, onAct }: {
  account?: PupilAccount; loading: boolean; canAct: (c: AccountCharge) => boolean; onAct: (c: AccountCharge) => void
}) {
  if (loading) return <Skeleton className="h-96 rounded-card" />
  if (!account || account.charges.length === 0) {
    return <Card><EmptyState icon={<ShieldCheck className="h-6 w-6" />} title="Sem cobranças neste ano" /></Card>
  }
  const s = account.summary
  const visible = account.charges.filter((c) => c.status !== 'cancelled')
  const cancelled = account.charges.length - visible.length
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {[
          { l: 'Total do ano', v: s.total },
          { l: pt.account.paid, v: s.paid, c: 'text-ok' },
          { l: pt.account.overdue, v: s.overdue, c: s.overdue > 0 ? 'text-bad' : 'text-ok' },
          { l: s.credit > 0 ? pt.account.credit : pt.account.outstanding, v: s.credit > 0 ? s.credit : s.outstanding },
        ].map((k) => (
          <Card key={k.l} className="p-4">
            <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">{k.l}</p>
            <p className={cn('mt-1 text-lg font-bold tabular-nums text-navy', k.c)}>{money(k.v)}</p>
          </Card>
        ))}
      </div>
      <Card>
        <ul className="divide-y divide-line md:hidden">
          {visible.map((c) => (
            <li key={c.id} className={cn('flex items-start justify-between gap-3 px-4 py-3', c.status === 'overdue' && 'bg-bad-tint/40', c.kind === 'penalty' && 'pl-7')}>
              <div className="min-w-0">
                <p className={cn('text-sm', c.kind === 'penalty' ? 'text-ink-muted' : 'text-ink')}>{c.description}</p>
                <p className="mt-0.5 text-xs text-ink-muted">Vence {date(c.due_date)} · {money(c.gross)}{c.discount ? ` · desconto ${money(c.discount)}` : ''}</p>
              </div>
              <div className="shrink-0 text-right">
                {c.remaining > 0 && <p className="text-sm font-semibold tabular-nums text-navy">{money(c.remaining)}</p>}
                <ChargeStatus status={c.status} />
                {canAct(c) && (
                  <button type="button" className="mt-1 block w-full text-right text-xs font-medium text-ink-muted hover:text-bad" onClick={() => onAct(c)}>
                    {c.kind === 'penalty' ? 'Perdoar' : 'Anular'}
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
        <Table className="hidden md:block">
          <thead>
            <tr><Th>Descrição</Th><Th>Vencimento</Th><Th align="right">Valor</Th><Th align="right">Desconto</Th><Th align="right">Pago</Th><Th align="right">Por pagar</Th><Th>Estado</Th><Th /></tr>
          </thead>
          <tbody>
            {visible.map((c) => (
              <tr key={c.id} className={cn(c.status === 'overdue' && 'bg-bad-tint/40')}>
                <Td className={cn(c.kind === 'penalty' && 'pl-8 text-ink-muted')}>
                  {c.kind === 'penalty' && <AlertTriangle className="mr-1.5 inline h-3.5 w-3.5 text-warn" />}
                  {c.description}
                </Td>
                <Td className="whitespace-nowrap">{date(c.due_date)}</Td>
                <Td align="right">{money(c.gross)}</Td>
                <Td align="right" className="text-ink-muted">{c.discount ? money(c.discount) : ''}</Td>
                <Td align="right">{c.paid ? money(c.paid) : ''}</Td>
                <Td align="right" className="font-semibold text-navy">{c.remaining ? money(c.remaining) : ''}</Td>
                <Td><ChargeStatus status={c.status} /></Td>
                <Td align="right">
                  {canAct(c) && (
                    <button type="button" title={c.kind === 'penalty' ? 'Perdoar multa' : 'Anular cobrança'} aria-label={c.kind === 'penalty' ? 'Perdoar multa' : 'Anular cobrança'}
                      className="rounded-md p-1.5 text-ink-muted hover:bg-bad-tint hover:text-bad" onClick={() => onAct(c)}>
                      <Ban className="h-4 w-4" />
                    </button>
                  )}
                </Td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="bg-accent-tint/60 font-bold text-navy">
              <Td>Total</Td><Td />
              <Td align="right">{money(visible.reduce((a, c) => a + c.gross, 0))}</Td>
              <Td align="right">{money(visible.reduce((a, c) => a + c.discount, 0))}</Td>
              <Td align="right">{money(s.paid)}</Td>
              <Td align="right">{money(s.outstanding)}</Td>
              <Td /><Td />
            </tr>
          </tfoot>
        </Table>
        {cancelled > 0 && <p className="border-t border-line px-4 py-2 text-xs text-ink-muted">{cancelled} cobranças anuladas não estão incluídas.</p>}
      </Card>
    </div>
  )
}
