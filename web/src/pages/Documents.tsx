import * as React from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ClipboardList, FileText, Files, Receipt, Search } from 'lucide-react'
import { api, openDocument } from '@/lib/api'
import { date } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { Avatar, Button, Card, CardHeader, EmptyState, ErrorState, Field, Input, PageHeader, Select, Skeleton, Table, Td, Th } from '@/components/ui'
import { handleError } from '@/features/enrolment/shared'
import type { ClassItem, EnrolmentListItem, List, StatementRef } from '@/api/types'

/** Confirmed pupils of the selected year, searched by name or number. */
function EnrolmentSearch({ onPick, placeholder }: { onPick: (e: EnrolmentListItem) => void; placeholder: string }) {
  const { year } = useSession()
  const [q, setQ] = React.useState('')
  const [term, setTerm] = React.useState('')
  React.useEffect(() => {
    const t = setTimeout(() => setTerm(q.trim()), 250)
    return () => clearTimeout(t)
  }, [q])
  const found = useQuery({
    queryKey: ['enrolments', 'doc-search', year?.year, term],
    queryFn: () => api.get<List<EnrolmentListItem>>('/enrolments', { year: year?.year, status: 'confirmed', q: term, page_size: 6 }),
    enabled: term.length >= 2,
  })
  return (
    <div>
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted" />
        <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={placeholder} className="pl-9" />
      </div>
      {found.data && term.length >= 2 && (
        <ul className="mt-1 divide-y divide-line rounded-lg border border-line">
          {found.data.items.map((x) => (
            <li key={x.enrolment_id}>
              <button type="button" onClick={() => { onPick(x); setQ(''); setTerm('') }} className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-page">
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
  )
}

/** Documents (brief 7.13): annual statements (one pupil or a whole class), enrolment forms, and the issued-statement register. */
export function DocumentsPage() {
  const { entity, year, can } = useSession()
  const qc = useQueryClient()
  const [classId, setClassId] = React.useState('')
  const [busy, setBusy] = React.useState<string | null>(null)
  const canIssue = can('documents.generate') && !!entity

  const classes = useQuery({
    queryKey: ['classes', entity?.id ?? 'all', year?.year],
    queryFn: () => api.get<List<ClassItem>>('/classes', { year: year?.year }),
    enabled: !!year && !!entity,
  })
  const issued = useQuery({
    queryKey: ['statements', 'all', entity?.id ?? 'all'],
    queryFn: () => api.get<List<StatementRef>>('/statements'),
  })

  async function classStatements() {
    if (!classId) return toast.error('Escolha a turma.')
    setBusy('class')
    try {
      await openDocument('/statements/class', 'pdf', 'POST', { class_id: classId })
      void qc.invalidateQueries({ queryKey: ['statements'] })
    } catch (err) {
      handleError(err)
    } finally {
      setBusy(null)
    }
  }
  async function pupilStatement(e: EnrolmentListItem) {
    setBusy('pupil')
    try {
      const s = await api.post<StatementRef>('/statements', { pupil_id: e.pupil_id, year: year?.year })
      toast.success(`Extrato ${s.number} emitido.`)
      void qc.invalidateQueries({ queryKey: ['statements'] })
      await openDocument(`/statements/${s.id}`)
    } catch (err) {
      handleError(err)
    } finally {
      setBusy(null)
    }
  }
  const cls = classes.data?.items.find((c) => c.id === classId)

  return (
    <>
      <PageHeader title={pt.nav.documents} description={`${entity ? entity.short_name : 'Todas as escolas'} · ano lectivo ${year?.year ?? ''}`} />

      {canIssue ? (
        <div className="mb-5 grid gap-4 lg:grid-cols-3">
          <Card className="p-5">
            <div className="flex items-center gap-2"><Files className="h-4 w-4 text-brand" /><p className="font-semibold text-navy">Extratos de uma turma</p></div>
            <p className="mt-1 text-xs text-ink-muted">Um extrato numerado por aluno, todos num só PDF para imprimir.</p>
            <div className="mt-4 space-y-3">
              <Field label="Turma">
                <Select value={classId} onChange={(e) => setClassId(e.target.value)}>
                  <option value="">Escolher…</option>
                  {classes.data?.items.map((c) => <option key={c.id} value={c.id}>{c.name} ({c.enrolled})</option>)}
                </Select>
              </Field>
              <Button className="w-full justify-center" disabled={!classId || cls?.enrolled === 0} loading={busy === 'class'} onClick={classStatements}>
                <FileText className="h-4 w-4" /> Emitir {cls ? `${cls.enrolled} extratos` : 'extratos'}
              </Button>
            </div>
          </Card>
          <Card className="p-5">
            <div className="flex items-center gap-2"><FileText className="h-4 w-4 text-brand" /><p className="font-semibold text-navy">Extrato de um aluno</p></div>
            <p className="mt-1 text-xs text-ink-muted">Fica guardado com número e pode ser reaberto tal como foi emitido.</p>
            <div className="mt-4"><EnrolmentSearch placeholder="Procurar aluno…" onPick={pupilStatement} /></div>
            {busy === 'pupil' && <p className="mt-2 text-xs text-ink-muted">A emitir…</p>}
          </Card>
          <Card className="p-5">
            <div className="flex items-center gap-2"><ClipboardList className="h-4 w-4 text-brand" /><p className="font-semibold text-navy">Ficha de matrícula</p></div>
            <p className="mt-1 text-xs text-ink-muted">Para o encarregado assinar. Os recibos estão em <Link to="/pagamentos" className="font-medium text-brand">Pagamentos</Link>.</p>
            <div className="mt-4">
              <EnrolmentSearch placeholder="Procurar aluno…" onPick={(e) => openDocument(`/enrolments/${e.enrolment_id}/form`).catch((x) => toast.error(x.message))} />
            </div>
          </Card>
        </div>
      ) : (
        !entity && <p className="mb-4 rounded-lg bg-page px-4 py-3 text-sm text-ink-muted">Para emitir documentos, escolha a Primária ou o Jardim no topo do ecrã.</p>
      )}

      <Card>
        <CardHeader title="Extratos emitidos" description="Os 200 mais recentes." />
        {issued.isError && <div className="p-4"><ErrorState error={issued.error} onRetry={() => issued.refetch()} /></div>}
        {issued.isLoading && <div className="space-y-2 p-4">{Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-10" />)}</div>}
        {issued.data && issued.data.items.length === 0 && <EmptyState icon={<Receipt className="h-6 w-6" />} title="Ainda não foi emitido nenhum extrato" />}
        {issued.data && issued.data.items.length > 0 && (
          <Table>
            <thead><tr><Th>Número</Th><Th>Aluno</Th><Th className="hidden md:table-cell">Ano</Th><Th className="hidden md:table-cell">Emitido</Th><Th /></tr></thead>
            <tbody>
              {issued.data.items.map((s) => (
                <tr key={s.id} className="hover:bg-page/70">
                  <Td className="whitespace-nowrap font-medium text-navy">{s.number}</Td>
                  <Td><Link to={`/alunos/${s.pupil_id}`} className="hover:underline">{s.pupil_name}</Link></Td>
                  <Td className="hidden md:table-cell">{s.year}</Td>
                  <Td className="hidden md:table-cell"><p>{date(s.issued_at)}</p><p className="text-xs text-ink-muted">{s.issued_by}</p></Td>
                  <Td align="right"><Button size="sm" variant="secondary" onClick={() => openDocument(`/statements/${s.id}`).catch((e) => toast.error(e.message))}><FileText className="h-4 w-4" /> Abrir</Button></Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </>
  )
}
