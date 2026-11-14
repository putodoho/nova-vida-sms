/** @type {import('tailwindcss').Config} */
// Design tokens from docs/ui-prototype-review.md.
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: { sans: ['"Inter Variable"', 'Inter', 'system-ui', 'sans-serif'] },
      colors: {
        navy: { DEFAULT: '#173b67', dark: '#102d50' },
        brand: { DEFAULT: '#2463a6', tint: '#eaf2fb', hover: '#1d5590' },
        accent: { DEFAULT: '#f4c542', tint: '#fff7dc' },
        ink: { DEFAULT: '#1c2a3a', muted: '#6b7888' },
        line: '#e4e9ef',
        page: '#f5f7fa',
        ok: { DEFAULT: '#15845a', tint: '#e7f6ef' },
        bad: { DEFAULT: '#bf3d45', tint: '#fff0f0' },
        warn: { DEFAULT: '#a96d08', tint: '#fff5df' },
        violet: { DEFAULT: '#6d55a8', tint: '#f1edfa' },
      },
      borderRadius: { card: '13px' },
      boxShadow: { card: '0 8px 28px rgba(20,42,68,.08)', soft: '0 1px 2px rgba(20,42,68,.06)' },
    },
  },
  plugins: [],
}
