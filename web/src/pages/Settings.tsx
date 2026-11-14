import { useSearchParams } from 'react-router-dom'
import { pt } from '@/lib/i18n/pt'
import { useSession } from '@/lib/session'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui'
import { SchoolTab, YearsTab } from '@/features/settings/SchoolTabs'
import { DiscountTypesTab, FeesTab, PenaltiesTab } from '@/features/settings/CatalogueTabs'

const tabs = [
  { key: 'escola', label: 'Escola e contas', Comp: SchoolTab },
  { key: 'anos', label: 'Anos lectivos', Comp: YearsTab },
  { key: 'precos', label: 'Tabela de preços', Comp: FeesTab },
  { key: 'multas', label: 'Multas', Comp: PenaltiesTab },
  { key: 'descontos', label: 'Tipos de desconto', Comp: DiscountTypesTab },
] as const

/** Settings (brief 7.15): school data and accounts, school years, fee catalogue, penalty rules, discount types. */
export function SettingsPage() {
  const { entity, year } = useSession()
  const [params, setParams] = useSearchParams()
  const tab = tabs.find((t) => t.key === params.get('tab')) ?? tabs[0]
  const Comp = tab.Comp
  return (
    <>
      <PageHeader title={pt.nav.settings} description={`${entity ? entity.short_name : 'Todas as escolas'} · ano lectivo ${year?.year ?? ''}`} />
      <div className="mb-5 flex gap-1 overflow-x-auto border-b border-line">
        {tabs.map((t) => (
          <button key={t.key} onClick={() => setParams(t.key === 'escola' ? {} : { tab: t.key }, { replace: true })}
            className={cn('whitespace-nowrap border-b-2 px-3 py-3 text-sm font-medium', tab.key === t.key ? 'border-brand text-brand' : 'border-transparent text-ink-muted hover:text-navy')}>
            {t.label}
          </button>
        ))}
      </div>
      <Comp />
    </>
  )
}
