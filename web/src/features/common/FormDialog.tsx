import * as React from 'react'
import { pt } from '@/lib/i18n/pt'
import { Button, Dialog, DialogContent } from '@/components/ui'

/** A dialog with a form body and Cancel / Save buttons; Enter submits. */
export function FormDialog({ open, title, description, onClose, onSubmit, saving, submitLabel = pt.actions.save, danger, children }: {
  open: boolean; title: string; description?: string; onClose: () => void; onSubmit: () => void; saving?: boolean
  submitLabel?: string; danger?: boolean; children: React.ReactNode
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent title={title} description={description}>
        <form className="space-y-4 p-5" onSubmit={(e) => { e.preventDefault(); onSubmit() }}>
          {children}
          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="secondary" onClick={onClose}>{pt.actions.cancel}</Button>
            <Button type="submit" variant={danger ? 'danger' : 'primary'} loading={saving}>{submitLabel}</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Small state helper for edit dialogs: the item being edited (or a blank one), errors and saving flag. */
export function useEditor<T>() {
  const [item, setItem] = React.useState<T | null>(null)
  const [errors, setErrors] = React.useState<Record<string, string>>({})
  const [saving, setSaving] = React.useState(false)
  const open = (v: T) => { setItem(v); setErrors({}) }
  const close = () => setItem(null)
  const patch = (p: Partial<T>) => {
    setItem((s) => (s ? { ...s, ...p } : s))
    setErrors({})
  }
  return { item, open, close, patch, errors, setErrors, saving, setSaving }
}
