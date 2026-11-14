import * as React from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { Command } from 'cmdk'
import { useQuery } from '@tanstack/react-query'
import {
  BarChart3, BookOpen, Building2, ChevronDown, ClipboardList, FileText, GraduationCap, LayoutDashboard, LogOut, Menu,
  Percent, Plus, Receipt, Search, Settings, ShieldCheck, Users, Wallet, X,
} from 'lucide-react'
import { useSession } from '@/lib/session'
import { pt } from '@/lib/i18n/pt'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Avatar, Button, EntityTag } from '@/components/ui'
import { usePaymentSheet } from '@/features/payments/PaymentSheet'
import type { Permission, PupilSearchHit, List } from '@/api/types'

interface NavItem {
  to: string
  label: string
  icon: React.ElementType
  perm?: Permission
  soon?: boolean
}

const nav: NavItem[] = [
  { to: '/', label: pt.nav.dashboard, icon: LayoutDashboard, perm: 'reports.read' },
  { to: '/alunos', label: pt.nav.pupils, icon: Users, perm: 'pupils.read' },
  { to: '/matriculas', label: pt.nav.enrolments, icon: ClipboardList, perm: 'enrolments.write', soon: true },
  { to: '/pagamentos', label: pt.nav.payments, icon: Wallet, perm: 'charges.read', soon: true },
  { to: '/cobrancas', label: pt.nav.charges, icon: Receipt, perm: 'charges.read', soon: true },
  { to: '/descontos', label: pt.nav.discounts, icon: Percent, perm: 'charges.read', soon: true },
  { to: '/documentos', label: pt.nav.documents, icon: FileText, perm: 'documents.generate', soon: true },
  { to: '/turmas', label: pt.nav.classes, icon: BookOpen, perm: 'classes.read', soon: true },
  { to: '/relatorios', label: pt.nav.reports, icon: BarChart3, perm: 'reports.read', soon: true },
  { to: '/funcionarios', label: pt.nav.staff, icon: ShieldCheck, perm: 'users.manage', soon: true },
  { to: '/configuracoes', label: pt.nav.settings, icon: Settings, perm: 'settings.manage', soon: true },
]

function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { can } = useSession()
  return (
    <div className="flex h-full flex-col bg-navy text-white">
      <div className="flex items-center gap-3 px-5 py-5">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-accent text-navy">
          <GraduationCap className="h-6 w-6" />
        </div>
        <div className="leading-tight">
          <p className="text-[15px] font-bold">{pt.app.name}</p>
          <p className="text-xs text-white/60">{pt.app.subtitle}</p>
        </div>
      </div>
      <nav className="flex-1 space-y-0.5 overflow-y-auto px-3 pb-4">
        {nav.filter((n) => !n.perm || can(n.perm)).map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            end={n.to === '/'}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-lg px-3 py-2.5 text-[14px] font-medium text-white/75 transition-colors hover:bg-white/10 hover:text-white',
                isActive && 'bg-white/15 text-white shadow-[inset_3px_0_0_#f4c542]',
              )
            }
          >
            <n.icon className="h-[18px] w-[18px] shrink-0" />
            <span className="flex-1 truncate">{n.label}</span>
            {n.soon && <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-white/50">7b</span>}
          </NavLink>
        ))}
      </nav>
    </div>
  )
}

function EntitySwitcher() {
  const { entity, entities, setEntity } = useSession()
  if (entities.length === 0) return null
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button className="flex h-9 items-center gap-2 rounded-lg border border-line bg-white px-2.5 text-sm hover:bg-page" aria-label="Empresa">
          <Building2 className="h-4 w-4 text-ink-muted" />
          {entity ? <EntityTag code={entity.code} name={entity.short_name} /> : <span className="font-semibold text-navy">Todas</span>}
          <ChevronDown className="h-4 w-4 text-ink-muted" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className="z-50 min-w-[260px] rounded-lg border border-line bg-white p-1 shadow-card">
          {entities.map((e) => (
            <DropdownMenu.Item key={e.id} onSelect={() => setEntity(e.id)} className="flex cursor-pointer flex-col rounded-md px-3 py-2 outline-none data-[highlighted]:bg-brand-tint">
              <EntityTag code={e.code} name={e.short_name} />
              <span className="mt-1 text-xs text-ink-muted">{e.legal_name}</span>
            </DropdownMenu.Item>
          ))}
          {entities.length > 1 && (
            <DropdownMenu.Item onSelect={() => setEntity('all')} className="cursor-pointer rounded-md px-3 py-2 text-sm font-semibold text-navy outline-none data-[highlighted]:bg-brand-tint">
              Todas (só consulta)
            </DropdownMenu.Item>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function YearSwitcher() {
  const { year, years, setYear } = useSession()
  if (!year) return null
  return (
    <select
      value={year.year}
      onChange={(e) => setYear(Number(e.target.value))}
      className="h-9 rounded-lg border border-line bg-white px-2 text-sm font-semibold text-navy"
      aria-label="Ano lectivo"
    >
      {years.map((y) => (
        <option key={y.id} value={y.year}>{y.year}</option>
      ))}
    </select>
  )
}

function UserMenu() {
  const { me, logout } = useSession()
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button className="flex items-center gap-2 rounded-lg p-1 hover:bg-page" aria-label="Utilizador">
          <Avatar name={me.user.display_name} className="h-8 w-8" />
          <span className="hidden text-sm font-medium text-ink lg:inline">{me.user.display_name}</span>
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={6} className="z-50 min-w-[220px] rounded-lg border border-line bg-white p-1 shadow-card">
          <div className="px-3 py-2">
            <p className="text-sm font-semibold text-navy">{me.user.display_name}</p>
            <p className="text-xs text-ink-muted">{me.user.login}</p>
          </div>
          <DropdownMenu.Separator className="my-1 h-px bg-line" />
          <DropdownMenu.Item onSelect={() => void logout()} className="flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm outline-none data-[highlighted]:bg-brand-tint">
            <LogOut className="h-4 w-4" /> {pt.actions.logout}
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

/** Ctrl+K search for pupils by name, number or guardian phone. */
function SearchPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [q, setQ] = React.useState('')
  const navigate = useNavigate()
  const { data, isFetching } = useQuery({
    queryKey: ['pupil-search', q],
    queryFn: () => api.get<List<PupilSearchHit>>('/pupils/search', { q }),
    enabled: q.trim().length >= 2,
  })
  React.useEffect(() => {
    if (!open) setQ('')
  }, [open])
  return (
    <Command.Dialog
      open={open}
      onOpenChange={onOpenChange}
      label="Pesquisar alunos"
      shouldFilter={false}
      overlayClassName="fixed inset-0 z-40 bg-navy-dark/40"
      contentClassName="fixed left-1/2 top-[12vh] z-50 w-[calc(100%-32px)] max-w-xl -translate-x-1/2 overflow-hidden rounded-card border border-line bg-white shadow-card"
    >
      <div className="flex items-center gap-2 border-b border-line px-4">
        <Search className="h-4 w-4 text-ink-muted" />
        <Command.Input value={q} onValueChange={setQ} placeholder="Nome, número do aluno ou telefone do encarregado…" className="h-12 flex-1 bg-transparent text-sm outline-none" />
      </div>
      <Command.List className="max-h-[50vh] overflow-y-auto p-2">
        {q.trim().length < 2 && <p className="px-3 py-6 text-center text-sm text-ink-muted">Escreva pelo menos 2 letras.</p>}
        {q.trim().length >= 2 && !isFetching && <Command.Empty className="px-3 py-6 text-center text-sm text-ink-muted">Nenhum aluno encontrado.</Command.Empty>}
        {data?.items.map((p) => (
          <Command.Item
            key={p.id}
            value={p.id}
            onSelect={() => {
              onOpenChange(false)
              navigate(`/alunos/${p.id}`)
            }}
            className="flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 data-[selected=true]:bg-brand-tint"
          >
            <Avatar name={p.full_name} />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold text-navy">{p.full_name}</p>
              <p className="truncate text-xs text-ink-muted">
                {p.number}
                {p.last_class && ` · ${p.last_class} (${p.last_year})`}
                {p.payer_phone && ` · ${p.payer_phone}`}
              </p>
            </div>
          </Command.Item>
        ))}
      </Command.List>
    </Command.Dialog>
  )
}

export function AppShell() {
  const { can, entity } = useSession()
  const [drawer, setDrawer] = React.useState(false)
  const [search, setSearch] = React.useState(false)
  const payment = usePaymentSheet()

  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setSearch((s) => !s)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className="min-h-screen bg-page lg:pl-[248px]">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[248px] lg:block">
        <Sidebar />
      </aside>
      {drawer && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-navy-dark/50" onClick={() => setDrawer(false)} />
          <div className="absolute inset-y-0 left-0 w-[264px]">
            <Sidebar onNavigate={() => setDrawer(false)} />
            <button className="absolute right-3 top-5 text-white/70" onClick={() => setDrawer(false)} aria-label="Fechar menu">
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>
      )}

      <header className="sticky top-0 z-20 border-b border-line bg-white/95 backdrop-blur">
        <div className="flex h-16 items-center gap-2 px-4 sm:gap-3 lg:px-8">
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setDrawer(true)} aria-label="Menu">
            <Menu className="h-5 w-5" />
          </Button>
          <EntitySwitcher />
          <YearSwitcher />
          <button
            onClick={() => setSearch(true)}
            className="ml-1 hidden h-9 flex-1 items-center gap-2 rounded-lg border border-line bg-page px-3 text-sm text-ink-muted hover:border-brand/40 md:flex md:max-w-sm"
          >
            <Search className="h-4 w-4" />
            <span className="flex-1 text-left">Pesquisar aluno…</span>
            <kbd className="rounded border border-line bg-white px-1.5 text-[11px]">Ctrl K</kbd>
          </button>
          <Button variant="ghost" size="icon" className="md:hidden" onClick={() => setSearch(true)} aria-label="Pesquisar">
            <Search className="h-5 w-5" />
          </Button>
          <div className="flex-1" />
          {can('payments.create') && entity && (
            <Button variant="accent" onClick={() => payment.open()} className="hidden sm:inline-flex">
              <Plus className="h-4 w-4" /> {pt.actions.recordPayment}
            </Button>
          )}
          <UserMenu />
        </div>
      </header>

      <main className="mx-auto max-w-[1400px] px-4 py-6 lg:px-8">
        <Outlet />
      </main>

      {can('payments.create') && entity && (
        <button
          onClick={() => payment.open()}
          className="fixed bottom-5 right-5 z-20 flex h-14 w-14 items-center justify-center rounded-full bg-accent text-navy shadow-card sm:hidden"
          aria-label={pt.actions.recordPayment}
        >
          <Plus className="h-6 w-6" />
        </button>
      )}
      <SearchPalette open={search} onOpenChange={setSearch} />
    </div>
  )
}
