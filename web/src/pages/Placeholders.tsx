import { Link } from 'react-router-dom'
import { Card, EmptyState } from '@/components/ui'

export function NotFoundPage() {
  return (
    <Card>
      <EmptyState title="Página não encontrada" action={<Link className="font-semibold text-brand" to="/">Voltar ao início</Link>} />
    </Card>
  )
}
