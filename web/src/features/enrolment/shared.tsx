// Building blocks shared by the enrolment wizard steps.
import * as React from 'react'
import { toast } from 'sonner'
import { ArrowLeft, ArrowRight, Check } from 'lucide-react'
import { ApiError } from '@/lib/api'
import { pt } from '@/lib/i18n/pt'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui'

export type Errors = Record<string, string>

/** Field errors from an API validation error; other errors become a toast. */
export function handleError(e: unknown, setErrors?: (e: Errors) => void) {
  if (e instanceof ApiError && Object.keys(e.fields).length > 0 && setErrors) {
    setErrors(e.fields)
    toast.error('Verifique os campos assinalados.')
    return
  }
  toast.error(e instanceof Error ? e.message : pt.errors.generic)
}

export function StepFooter({ onBack, onNext, nextLabel = 'Seguinte', loading, extra }: {
  onBack?: () => void; onNext: () => void; nextLabel?: string; loading?: boolean; extra?: React.ReactNode
}) {
  return (
    <div className="flex flex-col-reverse gap-2 border-t border-line px-5 py-4 sm:flex-row sm:items-center">
      {onBack ? (
        <Button variant="ghost" onClick={onBack} disabled={loading}><ArrowLeft className="h-4 w-4" /> Voltar</Button>
      ) : <span />}
      <div className="flex flex-col-reverse gap-2 sm:ml-auto sm:flex-row">
        {extra}
        <Button onClick={onNext} loading={loading} className="justify-center">
          {nextLabel} {!loading && <ArrowRight className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  )
}

export function Checkbox({ checked, onChange, label, hint, disabled }: {
  checked: boolean; onChange: (v: boolean) => void; label: React.ReactNode; hint?: React.ReactNode; disabled?: boolean
}) {
  return (
    <label className={cn('flex cursor-pointer items-start gap-3 rounded-lg px-1 py-1.5', disabled && 'cursor-default opacity-60')}>
      <input
        type="checkbox"
        className="mt-0.5 h-4 w-4 shrink-0 rounded border-line accent-brand"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="text-sm">
        <span className="text-ink">{label}</span>
        {hint && <span className="block text-xs text-ink-muted">{hint}</span>}
      </span>
    </label>
  )
}

/** Two- to four-option segmented control. */
export function Segmented<T extends string>({ value, onChange, options, className }: {
  value: T; onChange: (v: T) => void; options: { value: T; label: string }[]; className?: string
}) {
  return (
    <div className={cn('inline-flex rounded-lg border border-line bg-page p-0.5', className)} role="radiogroup">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            'h-9 flex-1 whitespace-nowrap rounded-md px-3 text-sm font-medium transition-colors',
            value === o.value ? 'bg-white text-brand shadow-soft' : 'text-ink-muted hover:text-ink',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

export function Section({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <section className="space-y-4">
      <div>
        <h3 className="text-[15px] font-semibold text-navy">{title}</h3>
        {description && <p className="mt-0.5 text-[13px] text-ink-muted">{description}</p>}
      </div>
      {children}
    </section>
  )
}

export const steps = [
  { key: 'pupil', label: 'Aluno' },
  { key: 'health', label: 'Saúde' },
  { key: 'guardians', label: 'Encarregados' },
  { key: 'pickup', label: 'Recolha e emergência' },
  { key: 'class', label: 'Turma e serviços' },
  { key: 'fees', label: 'Taxas e consentimentos' },
  { key: 'review', label: 'Rever e confirmar' },
] as const
export type StepKey = (typeof steps)[number]['key']

export function Stepper({ list, current, reachable, onGo }: {
  list: { key: StepKey; label: string }[]; current: StepKey; reachable: (k: StepKey) => boolean; onGo: (k: StepKey) => void
}) {
  const idx = list.findIndex((s) => s.key === current)
  return (
    <>
      {/* Mobile: compact progress */}
      <div className="md:hidden">
        <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">Passo {idx + 1} de {list.length}</p>
        <p className="mt-0.5 font-semibold text-navy">{list[idx]?.label}</p>
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-line">
          <div className="h-full rounded-full bg-brand transition-all" style={{ width: `${((idx + 1) / list.length) * 100}%` }} />
        </div>
      </div>
      {/* Desktop: full stepper */}
      <ol className="hidden gap-1 md:flex">
        {list.map((s, i) => {
          const done = i < idx
          const active = i === idx
          const can = !active && reachable(s.key)
          return (
            <li key={s.key} className="flex-1">
              <button
                type="button"
                disabled={!can}
                onClick={() => onGo(s.key)}
                className={cn(
                  'flex w-full flex-col gap-2 rounded-lg px-2 py-2 text-left transition-colors',
                  can && 'hover:bg-brand-tint/60',
                )}
              >
                <span className={cn('h-1 rounded-full', done || active ? 'bg-brand' : 'bg-line')} />
                <span className="flex items-center gap-1.5 text-xs font-semibold">
                  <span className={cn(
                    'flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px]',
                    active ? 'bg-brand text-white' : done ? 'bg-brand-tint text-brand' : 'bg-page text-ink-muted',
                  )}>
                    {done ? <Check className="h-3 w-3" /> : i + 1}
                  </span>
                  <span className={cn('leading-tight', active ? 'text-navy' : 'text-ink-muted')}>{s.label}</span>
                </span>
              </button>
            </li>
          )
        })}
      </ol>
    </>
  )
}
