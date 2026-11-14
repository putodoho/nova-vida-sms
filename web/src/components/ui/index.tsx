// Small UI kit in the shadcn/ui style (Tailwind + Radix), using the
// prototype's tokens. Import from '@/components/ui'.
import * as React from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as TabsPrimitive from '@radix-ui/react-tabs'
import { Loader2, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { money } from '@/lib/format'
import { pt } from '@/lib/i18n/pt'

// ---------------------------------------------------------------------------
// Button
// ---------------------------------------------------------------------------

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'accent'
type ButtonSize = 'sm' | 'md' | 'icon'

const buttonVariants: Record<ButtonVariant, string> = {
  primary: 'bg-brand text-white hover:bg-brand-hover shadow-soft',
  secondary: 'bg-white text-ink border border-line hover:bg-page',
  ghost: 'text-ink hover:bg-brand-tint',
  danger: 'bg-bad text-white hover:opacity-90',
  accent: 'bg-accent text-navy hover:brightness-95 shadow-soft',
}
const buttonSizes: Record<ButtonSize, string> = {
  sm: 'h-8 px-3 text-[13px]',
  md: 'h-10 px-4 text-sm',
  icon: 'h-9 w-9 justify-center',
}

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  loading?: boolean
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', loading, children, disabled, ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'inline-flex items-center gap-2 rounded-lg font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40 disabled:pointer-events-none disabled:opacity-50',
        buttonVariants[variant],
        buttonSizes[size],
        className,
      )}
      disabled={disabled || loading}
      {...props}
    >
      {loading && <Loader2 className="h-4 w-4 animate-spin" />}
      {children}
    </button>
  ),
)
Button.displayName = 'Button'

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(
  ({ className, invalid, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'h-10 w-full rounded-lg border border-line bg-white px-3 text-sm text-ink placeholder:text-ink-muted/70 focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20 disabled:bg-page',
        invalid && 'border-bad focus:border-bad focus:ring-bad/20',
        className,
      )}
      aria-invalid={invalid || undefined}
      {...props}
    />
  ),
)
Input.displayName = 'Input'

export const Select = React.forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean }>(
  ({ className, invalid, children, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        'h-10 w-full rounded-lg border border-line bg-white px-3 text-sm text-ink focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20',
        invalid && 'border-bad',
        className,
      )}
      {...props}
    >
      {children}
    </select>
  ),
)
Select.displayName = 'Select'

export const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn('min-h-[72px] w-full rounded-lg border border-line bg-white px-3 py-2 text-sm focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/20', className)}
      {...props}
    />
  ),
)
Textarea.displayName = 'Textarea'

export function Field({ label, error, hint, children, className, htmlFor }: {
  label: string; error?: string; hint?: string; children: React.ReactNode; className?: string; htmlFor?: string
}) {
  return (
    <div className={cn('space-y-1.5', className)}>
      <label htmlFor={htmlFor} className="block text-[13px] font-medium text-ink">{label}</label>
      {children}
      {error ? <p className="text-xs text-bad">{error}</p> : hint ? <p className="text-xs text-ink-muted">{hint}</p> : null}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Surfaces
// ---------------------------------------------------------------------------

export function Card({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('rounded-card border border-line bg-white shadow-card', className)} {...props} />
}

export function CardHeader({ title, description, action, className }: { title: React.ReactNode; description?: React.ReactNode; action?: React.ReactNode; className?: string }) {
  return (
    <div className={cn('flex items-start justify-between gap-3 border-b border-line px-5 py-4', className)}>
      <div>
        <h3 className="text-[15px] font-semibold text-navy">{title}</h3>
        {description && <p className="mt-0.5 text-[13px] text-ink-muted">{description}</p>}
      </div>
      {action}
    </div>
  )
}

export function PageHeader({ title, description, actions }: { title: string; description?: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 className="text-[22px] font-bold tracking-tight text-navy">{title}</h1>
        {description && <p className="mt-1 text-sm text-ink-muted">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Badges and status
// ---------------------------------------------------------------------------

type Tone = 'ok' | 'bad' | 'warn' | 'info' | 'neutral' | 'accent' | 'violet'
const tones: Record<Tone, string> = {
  ok: 'bg-ok-tint text-ok',
  bad: 'bg-bad-tint text-bad',
  warn: 'bg-warn-tint text-warn',
  info: 'bg-brand-tint text-brand',
  neutral: 'bg-page text-ink-muted',
  accent: 'bg-accent-tint text-[#8a6500]',
  violet: 'bg-violet-tint text-violet',
}

export function Badge({ tone = 'neutral', className, children }: { tone?: Tone; className?: string; children: React.ReactNode }) {
  return <span className={cn('inline-flex items-center gap-1 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-semibold', tones[tone], className)}>{children}</span>
}

const chargeTone: Record<string, Tone> = { paid: 'ok', partial: 'warn', overdue: 'bad', upcoming: 'warn', cancelled: 'neutral' }

export function ChargeStatus({ status }: { status: string }) {
  return <Badge tone={chargeTone[status] ?? 'neutral'}>{pt.chargeStatus[status] ?? status}</Badge>
}

/** Account badge: Em dia, or Em dívida with the amount. */
export function BalanceBadge({ overdue }: { overdue?: number }) {
  if (overdue === undefined) return null
  return overdue > 0 ? <Badge tone="bad">{money(overdue)}</Badge> : <Badge tone="ok">{pt.account.upToDate}</Badge>
}

export function EntityTag({ code, name }: { code: string; name: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-semibold', code === 'preschool' ? 'bg-accent-tint text-[#8a6500]' : 'bg-brand-tint text-brand')}>
      <span className={cn('h-1.5 w-1.5 rounded-full', code === 'preschool' ? 'bg-accent' : 'bg-brand')} />
      {name}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Feedback
// ---------------------------------------------------------------------------

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-line/70', className)} />
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn('h-5 w-5 animate-spin text-brand', className)} />
}

export function EmptyState({ icon, title, description, action }: { icon?: React.ReactNode; title: string; description?: string; action?: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-12 text-center">
      {icon && <div className="mb-3 rounded-full bg-brand-tint p-3 text-brand">{icon}</div>}
      <p className="font-semibold text-navy">{title}</p>
      {description && <p className="mt-1 max-w-sm text-sm text-ink-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const msg = error instanceof Error ? error.message : pt.errors.generic
  return (
    <div className="rounded-lg border border-bad/20 bg-bad-tint px-4 py-3 text-sm text-bad">
      {msg}
      {onRetry && (
        <button className="ml-3 font-semibold underline" onClick={onRetry}>Tentar novamente</button>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Dialog and Sheet (Radix)
// ---------------------------------------------------------------------------

export const Dialog = DialogPrimitive.Root
export const DialogTrigger = DialogPrimitive.Trigger
export const DialogClose = DialogPrimitive.Close

export function DialogContent({ title, description, children, className, side }: {
  title: string; description?: string; children: React.ReactNode; className?: string; side?: 'right'
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-40 bg-navy-dark/40 backdrop-blur-[1px]" />
      <DialogPrimitive.Content
        className={cn(
          'fixed z-50 flex flex-col bg-white shadow-card focus:outline-none',
          side === 'right'
            ? 'inset-y-0 right-0 w-full max-w-[560px] border-l border-line'
            : 'left-1/2 top-1/2 max-h-[90vh] w-[calc(100%-32px)] max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-card',
          className,
        )}
      >
        <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
          <div>
            <DialogPrimitive.Title className="text-[17px] font-semibold text-navy">{title}</DialogPrimitive.Title>
            {description ? (
              <DialogPrimitive.Description className="mt-0.5 text-[13px] text-ink-muted">{description}</DialogPrimitive.Description>
            ) : (
              <DialogPrimitive.Description className="sr-only">{title}</DialogPrimitive.Description>
            )}
          </div>
          <DialogPrimitive.Close className="rounded-md p-1 text-ink-muted hover:bg-page" aria-label={pt.actions.close}>
            <X className="h-5 w-5" />
          </DialogPrimitive.Close>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}

// ---------------------------------------------------------------------------
// Tabs
// ---------------------------------------------------------------------------

export const Tabs = TabsPrimitive.Root
export const TabsContent = TabsPrimitive.Content

export function TabsList({ tabs }: { tabs: { value: string; label: React.ReactNode }[] }) {
  return (
    <TabsPrimitive.List className="-mx-1 flex gap-1 overflow-x-auto border-b border-line px-1">
      {tabs.map((t) => (
        <TabsPrimitive.Trigger
          key={t.value}
          value={t.value}
          className="whitespace-nowrap border-b-2 border-transparent px-3 py-2.5 text-sm font-medium text-ink-muted hover:text-navy data-[state=active]:border-brand data-[state=active]:text-brand"
        >
          {t.label}
        </TabsPrimitive.Trigger>
      ))}
    </TabsPrimitive.List>
  )
}

// ---------------------------------------------------------------------------
// Table
// ---------------------------------------------------------------------------

export function Table({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn('overflow-x-auto', className)}>
      <table className="w-full border-collapse text-sm">{children}</table>
    </div>
  )
}
export function Th({ children, className, align }: { children?: React.ReactNode; className?: string; align?: 'right' }) {
  return <th className={cn('whitespace-nowrap border-b border-line bg-page/60 px-4 py-2.5 text-left text-xs font-semibold uppercase tracking-wide text-ink-muted', align === 'right' && 'text-right', className)}>{children}</th>
}
export function Td({ children, className, align, colSpan }: { children?: React.ReactNode; className?: string; align?: 'right'; colSpan?: number }) {
  return <td colSpan={colSpan} className={cn('border-b border-line px-4 py-3 align-middle', align === 'right' && 'text-right tabular-nums', className)}>{children}</td>
}

// ---------------------------------------------------------------------------
// Stat tile
// ---------------------------------------------------------------------------

export function Stat({ label, value, hint, tone, icon }: { label: string; value: React.ReactNode; hint?: React.ReactNode; tone?: 'bad' | 'ok'; icon?: React.ReactNode }) {
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">{label}</p>
        {icon && <span className="text-ink-muted">{icon}</span>}
      </div>
      <p className={cn('mt-2 text-[22px] font-bold tabular-nums tracking-tight text-navy', tone === 'bad' && 'text-bad', tone === 'ok' && 'text-ok')}>{value}</p>
      {hint && <p className="mt-1 text-xs text-ink-muted">{hint}</p>}
    </Card>
  )
}

export function Avatar({ name, className }: { name: string; className?: string }) {
  const parts = name.trim().split(/\s+/)
  const ini = ((parts[0]?.[0] ?? '') + (parts.length > 1 ? parts[parts.length - 1][0] : '')).toUpperCase()
  return <div className={cn('flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-brand-tint text-[13px] font-bold text-brand', className)}>{ini}</div>
}
