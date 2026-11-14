import { Link } from 'react-router-dom'
import { Construction } from 'lucide-react'
import { Card, EmptyState } from '@/components/ui'

export function ComingSoonPage() {
  return (
    <Card>
      <EmptyState icon={<Construction className="h-6 w-6" />} title="Disponível na próxima versão" description="Este ecrã faz parte da segunda parte da aplicação web (7b)." />
    </Card>
  )
}

export function NotFoundPage() {
  return (
    <Card>
      <EmptyState title="Página não encontrada" action={<Link className="font-semibold text-brand" to="/">Voltar ao início</Link>} />
    </Card>
  )
}
