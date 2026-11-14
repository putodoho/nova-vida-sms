import * as React from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { SessionProvider, useMe, useSession } from '@/lib/session'
import { Spinner } from '@/components/ui'
import { AppShell } from '@/layout/AppShell'
import { PaymentSheetProvider } from '@/features/payments/PaymentSheet'
import { LoginPage } from '@/pages/Login'
import { PupilsPage } from '@/pages/Pupils'
import { PupilProfilePage } from '@/pages/PupilProfile'
import { ComingSoonPage, NotFoundPage } from '@/pages/Placeholders'

// The dashboard pulls in the charting library; load it only when needed.
const DashboardPage = React.lazy(() => import('@/pages/Dashboard').then((m) => ({ default: m.DashboardPage })))

function Home() {
  const { can } = useSession()
  // The receptionist has no dashboard: start on the pupils list.
  return can('reports.read') ? (
    <React.Suspense fallback={<div className="flex justify-center py-20"><Spinner className="h-8 w-8" /></div>}>
      <DashboardPage />
    </React.Suspense>
  ) : (
    <Navigate to="/alunos" replace />
  )
}

export function App() {
  const me = useMe()
  if (me.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner className="h-8 w-8" />
      </div>
    )
  }
  if (!me.data) return <LoginPage />
  return (
    <SessionProvider me={me.data}>
      <PaymentSheetProvider>
        <Routes>
          <Route element={<AppShell />}>
            <Route index element={<Home />} />
            <Route path="alunos" element={<PupilsPage />} />
            <Route path="alunos/:id" element={<PupilProfilePage />} />
            {['matriculas', 'pagamentos', 'cobrancas', 'descontos', 'documentos', 'turmas', 'relatorios', 'funcionarios', 'configuracoes'].map((p) => (
              <Route key={p} path={p} element={<ComingSoonPage />} />
            ))}
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </PaymentSheetProvider>
    </SessionProvider>
  )
}
