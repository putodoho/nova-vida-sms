import * as React from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { KeyRound, Pencil, Plus, ShieldCheck, Trash2 } from 'lucide-react'
import { api } from '@/lib/api'
import { date } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { Checkbox, handleError } from '@/lib/forms'
import { Avatar, Badge, Button, Card, CardHeader, Field, Input, PageHeader, Select, Skeleton, Table, Td, Th } from '@/components/ui'
import { FormDialog, useEditor } from '@/features/common/FormDialog'
import type { AppUser, List, PermissionInfo, Role, Staff } from '@/api/types'

const tabs = [
  { key: 'utilizadores', label: 'Utilizadores' },
  { key: 'funcoes', label: 'Funções e permissões' },
  { key: 'funcionarios', label: 'Funcionários' },
] as const

/** Staff, users and roles (brief 7.14). */
export function StaffPage() {
  const [params, setParams] = useSearchParams()
  const tab = tabs.find((t) => t.key === params.get('tab'))?.key ?? 'utilizadores'
  return (
    <>
      <PageHeader title={pt.nav.staff} description="Quem entra no sistema, o que pode fazer e em que escola." />
      <div className="mb-5 flex gap-1 overflow-x-auto border-b border-line">
        {tabs.map((t) => (
          <button key={t.key} onClick={() => setParams(t.key === 'utilizadores' ? {} : { tab: t.key }, { replace: true })}
            className={cn('whitespace-nowrap border-b-2 px-3 py-3 text-sm font-medium', tab === t.key ? 'border-brand text-brand' : 'border-transparent text-ink-muted hover:text-navy')}>
            {t.label}
          </button>
        ))}
      </div>
      {tab === 'utilizadores' && <UsersTab />}
      {tab === 'funcoes' && <RolesTab />}
      {tab === 'funcionarios' && <StaffTab />}
    </>
  )
}

const useRoles = () => useQuery({ queryKey: ['roles'], queryFn: () => api.get<List<Role>>('/roles') })
const useStaff = () => useQuery({ queryKey: ['staff'], queryFn: () => api.get<List<Staff>>('/staff') })

// ---------------------------------------------------------------------------

interface UserForm { id?: string; display_name: string; login: string; password: string; staff_id: string; role_ids: string[]; entity_ids: string[]; active: boolean }

function UsersTab() {
  const { entities, me: session } = useSession()
  const me = session.user
  const qc = useQueryClient()
  const ed = useEditor<UserForm>()
  const [resetting, setResetting] = React.useState<AppUser | null>(null)
  const [password, setPassword] = React.useState('')
  const [resetErr, setResetErr] = React.useState('')
  const users = useQuery({ queryKey: ['users'], queryFn: () => api.get<List<AppUser>>('/users') })
  const roles = useRoles()
  const staff = useStaff()
  const roleName = (id: string) => roles.data?.items.find((r) => r.id === id)?.name ?? '?'
  const entName = (id: string) => entities.find((e) => e.id === id)?.short_name ?? '?'

  async function save() {
    const f = ed.item
    if (!f) return
    const e: Record<string, string> = {}
    if (!f.display_name.trim()) e.display_name = 'Obrigatório.'
    if (!f.login.trim()) e.login = 'Obrigatório.'
    if (!f.id && f.password.length < 8) e.password = 'Mínimo de 8 caracteres.'
    if (f.role_ids.length === 0) e.role_ids = 'Escolha pelo menos uma função.'
    if (f.entity_ids.length === 0) e.entity_ids = 'Escolha pelo menos uma escola.'
    if (Object.keys(e).length) return ed.setErrors(e)
    ed.setSaving(true)
    try {
      const body = { display_name: f.display_name.trim(), login: f.login.trim().toLowerCase(), staff_id: f.staff_id || null, role_ids: f.role_ids, entity_ids: f.entity_ids, active: f.active }
      if (f.id) await api.put(`/users/${f.id}`, body)
      else await api.post('/users', { ...body, password: f.password })
      toast.success(f.id ? 'Utilizador actualizado.' : `Utilizador criado. Entra com ${body.login}.`)
      void qc.invalidateQueries({ queryKey: ['users'] })
      void qc.invalidateQueries({ queryKey: ['roles'] })
      ed.close()
    } catch (err) {
      handleError(err, ed.setErrors)
    } finally {
      ed.setSaving(false)
    }
  }
  async function reset() {
    if (!resetting) return
    if (password.length < 8) return setResetErr('Mínimo de 8 caracteres.')
    try {
      await api.post(`/users/${resetting.id}/reset-password`, { password })
      toast.success('Palavra-passe alterada. As sessões abertas desse utilizador foram terminadas.')
      setResetting(null)
    } catch (err) {
      handleError(err, (x) => setResetErr(Object.values(x).join(' ')))
    }
  }

  const f = ed.item
  return (
    <Card>
      <CardHeader title="Utilizadores" description="Desactivar um utilizador termina as sessões dele; o histórico fica."
        action={<Button size="sm" onClick={() => ed.open({ display_name: '', login: '', password: '', staff_id: '', role_ids: [], entity_ids: entities.length === 1 ? [entities[0].id] : [], active: true })}><Plus className="h-4 w-4" /> Novo utilizador</Button>} />
      {!users.data ? <div className="p-4"><Skeleton className="h-32" /></div> : (
        <Table>
          <thead><tr><Th>Utilizador</Th><Th>Funções</Th><Th className="hidden md:table-cell">Escolas</Th><Th className="hidden lg:table-cell">Último acesso</Th><Th /></tr></thead>
          <tbody>
            {users.data.items.map((u) => (
              <tr key={u.id} className={cn(!u.active && 'opacity-50')}>
                <Td>
                  <div className="flex items-center gap-3">
                    <Avatar name={u.display_name} />
                    <div className="min-w-0"><p className="truncate font-medium text-navy">{u.display_name}{u.id === me.id && <span className="text-ink-muted"> (eu)</span>}</p><p className="truncate text-xs text-ink-muted">{u.login}{!u.active && ' · inactivo'}</p></div>
                  </div>
                </Td>
                <Td><div className="flex flex-wrap gap-1">{u.role_ids.map((r) => <Badge key={r} tone="info">{roleName(r)}</Badge>)}</div></Td>
                <Td className="hidden text-ink-muted md:table-cell">{u.entity_ids.map(entName).join(', ')}</Td>
                <Td className="hidden text-ink-muted lg:table-cell">{u.last_login_at ? date(u.last_login_at) : 'Nunca'}</Td>
                <Td align="right">
                  <div className="flex justify-end">
                    <Button size="sm" variant="ghost" aria-label="Alterar palavra-passe" title="Alterar palavra-passe" onClick={() => { setResetting(u); setPassword(''); setResetErr('') }}><KeyRound className="h-4 w-4" /></Button>
                    <Button size="sm" variant="ghost" aria-label="Editar" onClick={() => ed.open({ id: u.id, display_name: u.display_name, login: u.login, password: '', staff_id: u.staff_id ?? '', role_ids: u.role_ids, entity_ids: u.entity_ids, active: u.active })}><Pencil className="h-4 w-4" /></Button>
                  </div>
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}

      <FormDialog open={!!f} title={f?.id ? 'Editar utilizador' : 'Novo utilizador'} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {f && (
          <>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Nome a mostrar" error={ed.errors.display_name}><Input autoFocus value={f.display_name} onChange={(e) => ed.patch({ display_name: e.target.value })} /></Field>
              <Field label="Login (email)" error={ed.errors.login}><Input value={f.login} autoComplete="off" onChange={(e) => ed.patch({ login: e.target.value })} /></Field>
              {!f.id && <Field label="Palavra-passe inicial" error={ed.errors.password} hint="A pessoa pode mudá-la depois."><Input type="text" autoComplete="off" value={f.password} onChange={(e) => ed.patch({ password: e.target.value })} /></Field>}
              <Field label="Funcionário" hint="Opcional: liga o acesso à ficha do funcionário.">
                <Select value={f.staff_id} onChange={(e) => ed.patch({ staff_id: e.target.value })}>
                  <option value="">—</option>
                  {staff.data?.items.map((s) => <option key={s.id} value={s.id}>{s.full_name}</option>)}
                </Select>
              </Field>
            </div>
            <Field label="Funções" error={ed.errors.role_ids}>
              <div className="grid gap-x-4 rounded-lg border border-line p-2 sm:grid-cols-2">
                {roles.data?.items.map((r) => (
                  <Checkbox key={r.id} checked={f.role_ids.includes(r.id)} label={r.name} hint={r.description || undefined}
                    onChange={(v) => ed.patch({ role_ids: v ? [...f.role_ids, r.id] : f.role_ids.filter((x) => x !== r.id) })} />
                ))}
              </div>
            </Field>
            <Field label="Escolas" error={ed.errors.entity_ids}>
              <div className="flex flex-wrap gap-x-4">
                {entities.map((e) => (
                  <Checkbox key={e.id} checked={f.entity_ids.includes(e.id)} label={e.short_name}
                    onChange={(v) => ed.patch({ entity_ids: v ? [...f.entity_ids, e.id] : f.entity_ids.filter((x) => x !== e.id) })} />
                ))}
              </div>
            </Field>
            {f.id && <Checkbox checked={f.active} onChange={(v) => ed.patch({ active: v })} label="Activo" hint="Desmarcar impede a entrada e termina as sessões abertas." />}
            {ed.errors.active && <p className="text-sm text-bad">{ed.errors.active}</p>}
          </>
        )}
      </FormDialog>
      <FormDialog open={!!resetting} title="Alterar palavra-passe" description={resetting?.display_name} onClose={() => setResetting(null)} onSubmit={reset}>
        <Field label="Nova palavra-passe" error={resetErr}><Input autoFocus type="text" autoComplete="off" value={password} onChange={(e) => { setPassword(e.target.value); setResetErr('') }} /></Field>
        <p className="text-sm text-ink-muted">Comunique-a à pessoa; ela pode mudá-la depois de entrar.</p>
      </FormDialog>
    </Card>
  )
}

// ---------------------------------------------------------------------------

interface RoleForm { id?: string; code: string; name: string; description: string; permissions: string[]; system?: boolean; admin?: boolean }

function RolesTab() {
  const qc = useQueryClient()
  const ed = useEditor<RoleForm>()
  const [removing, setRemoving] = React.useState<Role | null>(null)
  const roles = useRoles()
  const perms = useQuery({ queryKey: ['permissions'], queryFn: () => api.get<PermissionInfo[]>('/permissions') })
  const modules = [...new Set((perms.data ?? []).map((p) => p.module))]

  async function save() {
    const f = ed.item
    if (!f) return
    if (!f.name.trim()) return ed.setErrors({ name: 'Obrigatório.' })
    ed.setSaving(true)
    try {
      const body = { code: f.code || f.name.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, ''), name: f.name.trim(), description: f.description.trim(), permissions: f.permissions }
      if (f.id) await api.put(`/roles/${f.id}`, body)
      else await api.post('/roles', body)
      toast.success('Função guardada. Vale a partir do próximo pedido de cada utilizador.')
      void qc.invalidateQueries({ queryKey: ['roles'] })
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
      await api.del(`/roles/${removing.id}`)
      toast.success('Função apagada.')
      void qc.invalidateQueries({ queryKey: ['roles'] })
      setRemoving(null)
    } catch (err) {
      handleError(err)
    }
  }

  const f = ed.item
  return (
    <Card>
      <CardHeader title="Funções" description="Cada utilizador pode ter várias; as permissões somam-se."
        action={<Button size="sm" onClick={() => ed.open({ code: '', name: '', description: '', permissions: ['pupils.read'] })}><Plus className="h-4 w-4" /> Nova função</Button>} />
      {!roles.data || !perms.data ? <div className="p-4"><Skeleton className="h-32" /></div> : (
        <ul className="divide-y divide-line">
          {roles.data.items.map((r) => (
            <li key={r.id} className="flex items-start gap-3 px-5 py-4">
              <ShieldCheck className="mt-0.5 h-5 w-5 shrink-0 text-brand" />
              <div className="min-w-0 flex-1">
                <p className="font-semibold text-navy">{r.name}{r.system && <Badge className="ml-2">Sistema</Badge>}</p>
                <p className="text-sm text-ink-muted">{r.description || '—'}</p>
                <p className="mt-1 text-xs text-ink-muted">{r.permissions.length} de {perms.data.length} permissões · {r.users} {r.users === 1 ? 'utilizador' : 'utilizadores'}</p>
              </div>
              <div className="flex">
                <Button size="sm" variant="secondary" onClick={() => ed.open({ id: r.id, code: r.code, name: r.name, description: r.description, permissions: r.permissions, system: r.system, admin: r.code === 'admin' })}><Pencil className="h-4 w-4" /> Editar</Button>
                {!r.system && r.users === 0 && <Button size="sm" variant="ghost" aria-label="Apagar função" onClick={() => setRemoving(r)}><Trash2 className="h-4 w-4" /></Button>}
              </div>
            </li>
          ))}
        </ul>
      )}
      <FormDialog open={!!f} title={f?.id ? `Função · ${f.name}` : 'Nova função'} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {f && (
          <>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Nome" error={ed.errors.name}><Input autoFocus value={f.name} onChange={(e) => ed.patch({ name: e.target.value })} /></Field>
              <Field label="Descrição"><Input value={f.description} onChange={(e) => ed.patch({ description: e.target.value })} /></Field>
            </div>
            {f.admin && <p className="rounded-lg bg-page px-3 py-2 text-sm text-ink-muted">O administrador tem sempre todas as permissões.</p>}
            <div className="max-h-[50vh] space-y-3 overflow-y-auto pr-1">
              {modules.map((m) => (
                <div key={m}>
                  <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-ink-muted">{m}</p>
                  {perms.data!.filter((p) => p.module === m).map((p) => (
                    <Checkbox key={p.code} checked={f.admin || f.permissions.includes(p.code)} disabled={f.admin} label={p.label}
                      onChange={(v) => ed.patch({ permissions: v ? [...f.permissions, p.code] : f.permissions.filter((x) => x !== p.code) })} />
                  ))}
                </div>
              ))}
            </div>
            {ed.errors.permissions && <p className="text-sm text-bad">{ed.errors.permissions}</p>}
          </>
        )}
      </FormDialog>
      <FormDialog open={!!removing} title="Apagar função?" description={removing?.name} onClose={() => setRemoving(null)} onSubmit={remove} submitLabel="Apagar" danger>
        <p className="text-sm text-ink">Nenhum utilizador tem esta função.</p>
      </FormDialog>
    </Card>
  )
}

// ---------------------------------------------------------------------------

interface StaffForm { id?: string; full_name: string; function: string; phone: string; email: string; active: boolean }

function StaffTab() {
  const qc = useQueryClient()
  const ed = useEditor<StaffForm>()
  const staff = useStaff()

  async function save() {
    const f = ed.item
    if (!f) return
    if (!f.full_name.trim()) return ed.setErrors({ full_name: 'Obrigatório.' })
    ed.setSaving(true)
    try {
      const body = { full_name: f.full_name.trim(), function: f.function.trim(), phone: f.phone.trim(), email: f.email.trim(), active: f.active }
      if (f.id) await api.put(`/staff/${f.id}`, body)
      else await api.post('/staff', body)
      toast.success('Funcionário guardado.')
      void qc.invalidateQueries({ queryKey: ['staff'] })
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
      <CardHeader title="Funcionários" description="Professores e pessoal. Um funcionário só entra no sistema se tiver utilizador."
        action={<Button size="sm" onClick={() => ed.open({ full_name: '', function: '', phone: '', email: '', active: true })}><Plus className="h-4 w-4" /> Novo funcionário</Button>} />
      {!staff.data ? <div className="p-4"><Skeleton className="h-32" /></div> : (
        <Table>
          <thead><tr><Th>Nome</Th><Th>Função</Th><Th className="hidden md:table-cell">Contacto</Th><Th /></tr></thead>
          <tbody>
            {staff.data.items.map((s) => (
              <tr key={s.id} className={cn(!s.active && 'opacity-50')}>
                <Td className="font-medium text-navy">{s.full_name}{!s.active && <span className="text-xs font-normal text-ink-muted"> · inactivo</span>}</Td>
                <Td>{s.function || '—'}</Td>
                <Td className="hidden text-ink-muted md:table-cell">{[s.phone, s.email].filter(Boolean).join(' · ') || '—'}</Td>
                <Td align="right"><Button size="sm" variant="ghost" aria-label="Editar" onClick={() => ed.open({ ...s })}><Pencil className="h-4 w-4" /></Button></Td>
              </tr>
            ))}
            {staff.data.items.length === 0 && <tr><Td className="text-ink-muted">Ainda não há funcionários.</Td><Td /><Td className="hidden md:table-cell" /><Td /></tr>}
          </tbody>
        </Table>
      )}
      <FormDialog open={!!f} title={f?.id ? 'Editar funcionário' : 'Novo funcionário'} onClose={ed.close} onSubmit={save} saving={ed.saving}>
        {f && (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Nome completo" error={ed.errors.full_name} className="sm:col-span-2"><Input autoFocus value={f.full_name} onChange={(e) => ed.patch({ full_name: e.target.value })} /></Field>
            <Field label="Função"><Input value={f.function} placeholder="Ex.: Professora, Recepcionista" onChange={(e) => ed.patch({ function: e.target.value })} /></Field>
            <Field label="Telefone"><Input inputMode="tel" value={f.phone} onChange={(e) => ed.patch({ phone: e.target.value })} /></Field>
            <Field label="Email" className="sm:col-span-2"><Input type="email" value={f.email} onChange={(e) => ed.patch({ email: e.target.value })} /></Field>
            {f.id && <div className="sm:col-span-2"><Checkbox checked={f.active} onChange={(v) => ed.patch({ active: v })} label="Activo" /></div>}
          </div>
        )}
      </FormDialog>
    </Card>
  )
}
