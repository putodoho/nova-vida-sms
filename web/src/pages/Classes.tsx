import * as React from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowLeft, BookOpen, FileText, HeartPulse, Pencil, Phone, Plus, Trash2, Users } from 'lucide-react'
import { api, downloadXlsx, openDocument } from '@/lib/api'
import { age, money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { handleError } from '@/lib/forms'
import { Avatar, Badge, Button, Card, EmptyState, ErrorState, Field, Input, PageHeader, Select, Skeleton, Table, Td, Th } from '@/components/ui'
import { FormDialog, useEditor } from '@/features/common/FormDialog'
import type { ClassItem, EnrolmentListItem, Level, List, Staff } from '@/api/types'

interface ClassForm { id?: string; name: string; level_id: string; grade: string; capacity: string; room: string; teacher_staff_id: string }

function useLevels() {
  const { entity } = useSession()
  return useQuery({ queryKey: ['levels', entity?.id ?? 'all'], queryFn: () => api.get<List<Level>>('/levels') })
}

/** Classes (brief 7.8): occupancy per class; settings managers create, edit and remove classes. */
export function ClassesPage() {
  const { entity, year, can } = useSession()
  const qc = useQueryClient()
  const ed = useEditor<ClassForm>()
  const [removing, setRemoving] = React.useState<ClassItem | null>(null)
  const classes = useQuery({
    queryKey: ['classes', entity?.id ?? 'all', year?.year],
    queryFn: () => api.get<List<ClassItem>>('/classes', { year: year?.year }),
    enabled: !!year,
  })
  const levels = useLevels()
  const staff = useQuery({ queryKey: ['staff'], queryFn: () => api.get<List<Staff>>('/staff'), enabled: can('settings.manage') })
  const manage = can('settings.manage') && !!entity
  const items = classes.data?.items ?? []
  const total = items.reduce((s, c) => s + c.enrolled, 0)
  const capacity = items.reduce((s, c) => s + c.capacity, 0)

  async function save() {
    const f = ed.item
    if (!f || !year) return
    const e: Record<string, string> = {}
    if (!f.name.trim()) e.name = 'Obrigatório.'
    if (!f.level_id) e.level_id = 'Escolha o nível.'
    if (!(Number(f.capacity) > 0)) e.capacity = 'Indique a capacidade.'
    if (Object.keys(e).length) return ed.setErrors(e)
    const body = {
      school_year_id: year.id, name: f.name.trim(), level_id: f.level_id, grade: f.grade ? Number(f.grade) : null,
      capacity: Number(f.capacity), room: f.room.trim(), teacher_staff_id: f.teacher_staff_id || null,
    }
    ed.setSaving(true)
    try {
      if (f.id) await api.put(`/classes/${f.id}`, body)
      else await api.post('/classes', body)
      toast.success(f.id ? 'Turma actualizada.' : 'Turma criada.')
      void qc.invalidateQueries({ queryKey: ['classes'] })
      ed.close()
    } catch (err) {
      handleError(err, ed.setErrors)
    } finally {
      ed.setSaving(false)
    }
  }
  async function remove() {
    if (!removing) return
    try {
      await api.del(`/classes/${removing.id}`)
      toast.success('Turma apagada.')
      void qc.invalidateQueries({ queryKey: ['classes'] })
      setRemoving(null)
    } catch (err) {
      handleError(err)
    }
  }

  const blank = (): ClassForm => ({ name: '', level_id: levels.data?.items.length === 1 ? levels.data.items[0].id : '', grade: '', capacity: '20', room: '', teacher_staff_id: '' })

  return (
    <>
      <PageHeader
        title={pt.nav.classes}
        description={`Ano lectivo ${year?.year ?? ''}${entity ? ` · ${entity.short_name}` : ''} · ${total} de ${capacity} lugares ocupados`}
        actions={manage && <Button onClick={() => ed.open(blank())}><Plus className="h-4 w-4" /> Nova turma</Button>}
      />
      {classes.isError && <ErrorState error={classes.error} onRetry={() => classes.refetch()} />}
      {classes.isLoading && <div className="grid gap-3 md:grid-cols-3">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-32" />)}</div>}
      {classes.data && items.length === 0 && (
        <Card><EmptyState icon={<BookOpen className="h-6 w-6" />} title={`Ainda não há turmas em ${year?.year}`} description={manage ? 'Crie as turmas do ano para poder matricular.' : undefined}
          action={manage && <Button onClick={() => ed.open(blank())}><Plus className="h-4 w-4" /> Nova turma</Button>} /></Card>
      )}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {items.map((c) => {
          const left = c.capacity - c.enrolled
          return (
            <Card key={c.id} className="p-4">
              <div className="flex items-start gap-2">
                <Link to={`/turmas/${c.id}`} className="min-w-0 flex-1">
                  <p className="text-lg font-bold text-navy hover:underline">{c.name}</p>
                  <p className="truncate text-xs text-ink-muted">{[levels.data?.items.find((l) => l.id === c.level_id)?.name, c.room, c.teacher_name].filter(Boolean).join(' · ') || '—'}</p>
                </Link>
                {manage && (
                  <div className="flex">
                    <button type="button" aria-label="Editar turma" className="rounded-md p-1.5 text-ink-muted hover:bg-page hover:text-navy"
                      onClick={() => ed.open({ id: c.id, name: c.name, level_id: c.level_id, grade: c.grade?.toString() ?? '', capacity: String(c.capacity), room: c.room, teacher_staff_id: c.teacher_staff_id ?? '' })}>
                      <Pencil className="h-4 w-4" />
                    </button>
                    {c.enrolled === 0 && (
                      <button type="button" aria-label="Apagar turma" className="rounded-md p-1.5 text-ink-muted hover:bg-bad-tint hover:text-bad" onClick={() => setRemoving(c)}><Trash2 className="h-4 w-4" /></button>
                    )}
                  </div>
                )}
              </div>
              <div className="mt-3 flex items-end justify-between">
                <p className="text-sm"><span className="text-2xl font-bold tabular-nums text-navy">{c.enrolled}</span><span className="text-ink-muted"> / {c.capacity} alunos</span></p>
                <Badge tone={left <= 0 ? 'bad' : left <= 3 ? 'warn' : 'ok'}>{left <= 0 ? 'Cheia' : `${left} ${left === 1 ? 'vaga' : 'vagas'}`}</Badge>
              </div>
              <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-line">
                <div className={cn('h-full rounded-full', left <= 0 ? 'bg-bad' : left <= 3 ? 'bg-warn' : 'bg-ok')} style={{ width: `${Math.min(100, (c.enrolled / Math.max(1, c.capacity)) * 100)}%` }} />
              </div>
            </Card>
          )
        })}
      </div>

      <FormDialog open={!!ed.item} title={ed.item?.id ? 'Editar turma' : 'Nova turma'} description={`Ano lectivo ${year?.year}`} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {ed.item && (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Nome" error={ed.errors.name}><Input autoFocus value={ed.item.name} placeholder="Ex.: 1.ª A" onChange={(e) => ed.patch({ name: e.target.value })} /></Field>
            <Field label="Nível" error={ed.errors.level_id}>
              <Select value={ed.item.level_id} onChange={(e) => ed.patch({ level_id: e.target.value })}>
                <option value="">Escolher…</option>
                {levels.data?.items.filter((l) => l.entity_id === entity?.id).map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
              </Select>
            </Field>
            <Field label="Classe (número)" hint="Ex.: 1 para a 1.ª classe; vazio no Jardim" error={ed.errors.grade}><Input inputMode="numeric" value={ed.item.grade} onChange={(e) => ed.patch({ grade: e.target.value })} /></Field>
            <Field label="Capacidade" error={ed.errors.capacity}><Input inputMode="numeric" value={ed.item.capacity} onChange={(e) => ed.patch({ capacity: e.target.value })} /></Field>
            <Field label="Sala"><Input value={ed.item.room} onChange={(e) => ed.patch({ room: e.target.value })} /></Field>
            <Field label="Professor(a)">
              <Select value={ed.item.teacher_staff_id} onChange={(e) => ed.patch({ teacher_staff_id: e.target.value })}>
                <option value="">—</option>
                {staff.data?.items.filter((s) => s.active).map((s) => <option key={s.id} value={s.id}>{s.full_name}</option>)}
              </Select>
            </Field>
          </div>
        )}
      </FormDialog>
      <FormDialog open={!!removing} title="Apagar turma?" description={removing?.name} onClose={() => setRemoving(null)} onSubmit={remove} submitLabel="Apagar" danger>
        <p className="text-sm text-ink">Só turmas sem matrículas podem ser apagadas.</p>
      </FormDialog>
    </>
  )
}

/** One class: its pupils with health alerts and payer contacts, and printing. */
export function ClassDetailPage() {
  const { id = '' } = useParams()
  const { entity, year, can } = useSession()
  const classes = useQuery({
    queryKey: ['classes', entity?.id ?? 'all', year?.year],
    queryFn: () => api.get<List<ClassItem>>('/classes', { year: year?.year }),
    enabled: !!year,
  })
  const pupils = useQuery({
    queryKey: ['enrolments', 'class', id],
    queryFn: () => api.get<List<EnrolmentListItem>>('/enrolments', { year: year?.year, class_id: id, status: 'confirmed', page_size: 200 }),
    enabled: !!year,
  })
  const c = classes.data?.items.find((x) => x.id === id)
  if (classes.data && !c) return <Card><EmptyState title="Turma não encontrada" description="Pode ser de outro ano lectivo ou de outra escola." /></Card>
  const items = pupils.data?.items ?? []

  return (
    <>
      <Link to="/turmas" className="mb-3 inline-flex items-center gap-1 text-sm font-medium text-ink-muted hover:text-navy"><ArrowLeft className="h-4 w-4" /> {pt.nav.classes}</Link>
      <PageHeader
        title={c ? `Turma ${c.name}` : 'Turma'}
        description={c ? `${c.enrolled} de ${c.capacity} alunos${c.teacher_name ? ` · ${c.teacher_name}` : ''}${c.room ? ` · ${c.room}` : ''}` : undefined}
        actions={c && (
          <>
            {can('charges.read') && (
              <Button variant="secondary" onClick={() => downloadXlsx('/charges/arrears', { year: year?.year, class_id: c.id }, `dividas-${c.name}.xlsx`).catch((e) => toast.error(e.message))}>
                Dívidas (Excel)
              </Button>
            )}
            {can('documents.generate') && entity && (
              <Button variant="secondary" onClick={() => openDocument('/statements/class', 'pdf', 'POST', { class_id: c.id }).catch((e) => toast.error(e.message))}>
                <FileText className="h-4 w-4" /> Extratos da turma
              </Button>
            )}
          </>
        )}
      />
      <Card>
        {pupils.isError && <div className="p-4"><ErrorState error={pupils.error} onRetry={() => pupils.refetch()} /></div>}
        {pupils.isLoading && <div className="space-y-2 p-4">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-11" />)}</div>}
        {pupils.data && items.length === 0 && <EmptyState icon={<Users className="h-6 w-6" />} title="Ainda sem alunos matriculados" />}
        {items.length > 0 && (
          <Table>
            <thead><tr><Th className="w-10">#</Th><Th>Aluno</Th><Th className="hidden md:table-cell">Encarregado</Th>{can('charges.read') && <Th align="right">Em dívida</Th>}</tr></thead>
            <tbody>
              {items.map((p, i) => (
                <tr key={p.enrolment_id} className="hover:bg-page/70">
                  <Td className="text-ink-muted">{i + 1}</Td>
                  <Td>
                    <Link to={`/alunos/${p.pupil_id}`} className="flex items-center gap-3">
                      <Avatar name={p.full_name} />
                      <div className="min-w-0">
                        <p className="flex items-center gap-1.5 truncate font-semibold text-navy">{p.full_name}{p.health_alert && <HeartPulse className="h-4 w-4 shrink-0 text-bad" aria-label="Alerta de saúde" />}</p>
                        <p className="text-xs text-ink-muted">{p.number} · {age(p.birth_date)} anos{p.type === 'new' && ' · novo'}</p>
                      </div>
                    </Link>
                  </Td>
                  <Td className="hidden md:table-cell">
                    <p>{p.payer_name ?? '—'}</p>
                    {p.payer_phone && <a href={`tel:${p.payer_phone.replace(/\s/g, '')}`} className="inline-flex items-center gap-1 text-xs text-ink-muted hover:text-brand"><Phone className="h-3 w-3" /> {p.payer_phone}</a>}
                  </Td>
                  {can('charges.read') && <Td align="right" className={cn('whitespace-nowrap', (p.overdue ?? 0) > 0 ? 'font-semibold text-bad' : 'text-ink-muted')}>{money(p.overdue ?? 0)}</Td>}
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </>
  )
}
