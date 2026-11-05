// token 数量的紧凑格式化：1.2k / 3.4M，不足 1000 原样显示。
export const formatTokenCount = (value?: number | null): string => {
  if (value === undefined || value === null || Number.isNaN(value)) {
    return '0'
  }
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)}k`
  }
  return String(value)
}
