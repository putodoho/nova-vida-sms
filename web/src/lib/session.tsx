// Session: current user, permissions, active entity and school year.
import * as React from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, setEntityHeader, setUnauthorizedHandler } from './api'
import { store } from './utils'
import type { EntityRef, List, Me, Permission, SchoolYear } from '@/api/types'

interface Session {
  me: Me
  can: (p: Permission) => boolean
  /** Selected entity, or null when "Todas" is selected (read-only aggregate views). */
  entity: EntityRef | null
  entities: EntityRef[]
  setEntity: (id: string | 'all') => void
  year: SchoolYear | null
  years: SchoolYear[]
  setYear: (year: number) => void
  logout: () => Promise<void>
}

const SessionContext = React.createContext<Session | null>(null)

export function useSession(): Session {
  const s = React.useContext(SessionContext)
  if (!s) throw new Error('useSession outside SessionProvider')
  return s
}

const ENTITY_KEY = 'nv.entity'
const YEAR_KEY = 'nv.year'

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: () => api.get<Me>('/auth/me'),
    retry: (count, err) => !(err instanceof ApiError && err.status === 401) && count < 2,
    staleTime: 5 * 60_000,
  })
}

export function SessionProvider({ me, children }: { me: Me; children: React.ReactNode }) {
  const qc = useQueryClient()
  const [entityId, setEntityId] = React.useState<string>(() => {
    const saved = store.get(ENTITY_KEY)
    if (saved === 'all' && me.entities.length > 1) return 'all'
    if (saved && me.entities.some((e) => e.id === saved)) return saved
    return me.entities[0]?.id ?? 'all'
  })
  const [yearNum, setYearNum] = React.useState<number | null>(() => Number(store.get(YEAR_KEY)) || null)

  // The header must be set before any scoped query runs.
  setEntityHeader(entityId)

  const years = useQuery({ queryKey: ['school-years'], queryFn: () => api.get<List<SchoolYear>>('/school-years') })
  const yearList = years.data?.items ?? []
  const year =
    yearList.find((y) => y.year === yearNum) ?? yearList.find((y) => y.status === 'open') ?? yearList[0] ?? null

  React.useEffect(() => {
    setUnauthorizedHandler(() => qc.setQueryData(['me'], null))
  }, [qc])

  const value: Session = {
    me,
    can: (p) => me.user.permissions.includes(p),
    entity: me.entities.find((e) => e.id === entityId) ?? null,
    entities: me.entities,
    setEntity: (id) => {
      store.set(ENTITY_KEY, id)
      setEntityHeader(id)
      setEntityId(id)
      // Everything scoped must reload for the new entity.
      void qc.resetQueries({ predicate: (q) => !['me', 'school-years'].includes(String(q.queryKey[0])) })
    },
    year,
    years: yearList,
    setYear: (y) => {
      store.set(YEAR_KEY, String(y))
      setYearNum(y)
    },
    logout: async () => {
      try {
        await api.post('/auth/logout')
      } finally {
        qc.clear()
        qc.setQueryData(['me'], null)
      }
    },
  }
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}
