/** Chart.js draws on a canvas, which cannot read CSS variables, so read the theme colours once here. */
export function themeColor(name: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value || fallback
}
