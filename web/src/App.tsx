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
import { EnrolmentsPage } from '@/pages/Enrolments'
import { EnrolmentWizardPage } from '@/pages/EnrolmentWizard'
import { RenewalPage } from '@/pages/Renewal'
import { ChargesPage } from '@/pages/Charges'
import { DiscountsPage } from '@/pages/Discounts'
import { PaymentsPage } from '@/pages/Payments'
import { DocumentsPage } from '@/pages/Documents'
import { ReportsPage } from '@/pages/Reports'
import { GuardianProfilePage } from '@/pages/GuardianProfile'

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
            <Route path="alunos/:id/editar" element={<EnrolmentWizardPage key="edit" mode="edit" />} />
            <Route path="matriculas" element={<EnrolmentsPage />} />
            <Route path="matriculas/nova" element={<EnrolmentWizardPage key="enrol" mode="new" />} />
            <Route path="matriculas/renovacao" element={<RenewalPage />} />
            <Route path="matriculas/:id" element={<EnrolmentWizardPage key="enrol" mode="draft" />} />
            <Route path="cobrancas" element={<ChargesPage />} />
            <Route path="descontos" element={<DiscountsPage />} />
            <Route path="pagamentos" element={<PaymentsPage />} />
            <Route path="documentos" element={<DocumentsPage />} />
            <Route path="relatorios" element={<ReportsPage />} />
            <Route path="encarregados/:id" element={<GuardianProfilePage />} />
            {['turmas', 'funcionarios', 'configuracoes'].map((p) => (
              <Route key={p} path={p} element={<ComingSoonPage />} />
            ))}
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </PaymentSheetProvider>
    </SessionProvider>
  )
}
