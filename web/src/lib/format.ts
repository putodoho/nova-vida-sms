// Display formats for Mozambique: 5.035,00 MT and dd/mm/yyyy.
// Money travels as integer centavos and is never converted to a float for maths.

export function money(centavos: number | null | undefined): string {
  if (centavos === null || centavos === undefined) return '—'
  const neg = centavos < 0
  const abs = Math.abs(Math.trunc(centavos))
  const units = Math.floor(abs / 100)
  const cents = abs % 100
  const grouped = units.toString().replace(/\B(?=(\d{3})+(?!\d))/g, '.')
  return `${neg ? '-' : ''}${grouped},${cents.toString().padStart(2, '0')} MT`
}

/** Compact money for charts and KPI tiles: 53.000 MT, 1,2 M MT. */
export function moneyShort(centavos: number): string {
  const mt = Math.round(centavos / 100)
  if (Math.abs(mt) >= 1_000_000) return `${(mt / 1_000_000).toFixed(1).replace('.', ',')} M MT`
  return `${mt.toString().replace(/\B(?=(\d{3})+(?!\d))/g, '.')} MT`
}

/**
 * Parse what a person typed ("5035", "5.035,50", "5035.5", "5 035") into
 * centavos. Returns null when it is not a valid amount.
 */
export function parseMoney(input: string): number | null {
  let s = input.replace(/\s|MT/gi, '')
  if (!s) return null
  const lastComma = s.lastIndexOf(',')
  const lastDot = s.lastIndexOf('.')
  // The right-most separator followed by 1–2 digits is the decimal mark.
  const decIdx = Math.max(lastComma, lastDot)
  let intPart = s
  let decPart = ''
  if (decIdx >= 0 && s.length - decIdx - 1 <= 2 && s.length - decIdx - 1 > 0) {
    intPart = s.slice(0, decIdx)
    decPart = s.slice(decIdx + 1)
  }
  intPart = intPart.replace(/[.,]/g, '')
  if (!/^\d+$/.test(intPart || '0') || !/^\d*$/.test(decPart)) return null
  s = intPart || '0'
  const cents = Number(s) * 100 + Number(decPart.padEnd(2, '0') || '0')
  return Number.isSafeInteger(cents) ? cents : null
}

/** Value to show in a money input for a centavos amount: 503500 → "5.035,00". */
export function moneyInput(centavos: number): string {
  return money(centavos).replace(' MT', '')
}

export function date(iso: string | null | undefined): string {
  if (!iso) return '—'
  const [y, m, d] = iso.slice(0, 10).split('-')
  return `${d}/${m}/${y}`
}

export function dateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const t = new Date(iso)
  const p = (n: number) => n.toString().padStart(2, '0')
  return `${p(t.getDate())}/${p(t.getMonth() + 1)}/${t.getFullYear()} ${p(t.getHours())}:${p(t.getMinutes())}`
}

/** dd/mm/yyyy → yyyy-mm-dd, or null. */
export function parseDate(input: string): string | null {
  const m = input.trim().match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})$/)
  if (!m) return null
  const [, d, mo, y] = m
  const iso = `${y}-${mo.padStart(2, '0')}-${d.padStart(2, '0')}`
  const t = new Date(iso + 'T00:00:00Z')
  return Number.isNaN(t.getTime()) || t.getUTCDate() !== Number(d) ? null : iso
}

export function todayISO(): string {
  const t = new Date()
  const p = (n: number) => n.toString().padStart(2, '0')
  return `${t.getFullYear()}-${p(t.getMonth() + 1)}-${p(t.getDate())}`
}

export function age(birthISO: string): number {
  const b = new Date(birthISO + 'T00:00:00')
  const now = new Date()
  let a = now.getFullYear() - b.getFullYear()
  if (now.getMonth() < b.getMonth() || (now.getMonth() === b.getMonth() && now.getDate() < b.getDate())) a--
  return a
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/)
  return ((parts[0]?.[0] ?? '') + (parts.length > 1 ? parts[parts.length - 1][0] : '')).toUpperCase()
}

export function percent(x: number): string {
  return `${(x * 100).toFixed(1).replace('.', ',')}%`
}

/** Basis points (10000 = 100%) as "10%" or "12,5%". */
export function percentBP(bp: number): string {
  const n = bp / 100
  return `${Number.isInteger(n) ? n : n.toFixed(2).replace(/0+$/, '').replace('.', ',')}%`
}
