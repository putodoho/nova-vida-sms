import * as React from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { GraduationCap } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { pt } from '@/lib/i18n/pt'
import { Button, Field, Input } from '@/components/ui'
import type { Me } from '@/api/types'

export function LoginPage() {
  const qc = useQueryClient()
  const [login, setLogin] = React.useState('')
  const [password, setPassword] = React.useState('')
  const m = useMutation({
    mutationFn: () => api.post<Me>('/auth/login', { login, password }),
    onSuccess: (me) => qc.setQueryData(['me'], me),
  })
  const err = m.error instanceof ApiError ? m.error : null

  return (
    <div className="grid min-h-screen lg:grid-cols-[1.1fr_1fr]">
      <div className="relative hidden overflow-hidden bg-navy lg:block">
        <div className="absolute -right-24 -top-24 h-96 w-96 rounded-full bg-accent/20" />
        <div className="absolute -bottom-32 -left-20 h-[28rem] w-[28rem] rounded-full bg-brand/40" />
        <div className="relative flex h-full flex-col justify-between p-12 text-white">
          <div className="flex items-center gap-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-accent text-navy"><GraduationCap className="h-7 w-7" /></div>
            <div>
              <p className="text-lg font-bold">{pt.app.name}</p>
              <p className="text-sm text-white/60">Xai-Xai</p>
            </div>
          </div>
          <div>
            <p className="max-w-md text-[34px] font-bold leading-tight">Matrículas, mensalidades e recibos num só lugar.</p>
            <p className="mt-4 max-w-md text-white/70">Jardim Infantil e Escola Primária Nova Vida.</p>
          </div>
          <p className="text-sm italic text-accent">Educando para a Eternidade</p>
        </div>
      </div>

      <div className="flex items-center justify-center px-4 py-12">
        <form
          className="w-full max-w-sm space-y-5"
          onSubmit={(e) => {
            e.preventDefault()
            m.mutate()
          }}
        >
          <div className="lg:hidden">
            <div className="mb-6 flex h-12 w-12 items-center justify-center rounded-2xl bg-navy text-accent"><GraduationCap className="h-7 w-7" /></div>
          </div>
          <div>
            <h1 className="text-2xl font-bold text-navy">Entrar</h1>
            <p className="mt-1 text-sm text-ink-muted">Use o seu email ou telefone e a palavra-passe.</p>
          </div>
          {err && !Object.keys(err.fields).length && (
            <div className="rounded-lg border border-bad/20 bg-bad-tint px-3 py-2 text-sm text-bad">{err.message}</div>
          )}
          {m.isError && !err && <div className="rounded-lg bg-bad-tint px-3 py-2 text-sm text-bad">{pt.errors.network}</div>}
          <Field label="Email ou telefone" error={err?.fields.login} htmlFor="login">
            <Input id="login" autoComplete="username" autoFocus value={login} onChange={(e) => setLogin(e.target.value)} invalid={!!err?.fields.login} />
          </Field>
          <Field label="Palavra-passe" error={err?.fields.password} htmlFor="password">
            <Input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} invalid={!!err?.fields.password} />
          </Field>
          <Button type="submit" className="w-full" loading={m.isPending}>Entrar</Button>
          <p className="text-center text-xs text-ink-muted">Esqueceu a palavra-passe? Peça ao administrador para a repor.</p>
        </form>
      </div>
    </div>
  )
}
