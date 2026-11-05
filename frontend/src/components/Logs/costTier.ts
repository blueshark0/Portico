import type { LogCostTier, RequestLog } from '../../services/logs'

export type CostTier = LogCostTier

// 费用档位（0.1/0.5 阈值）只在后端计算一份，随日志的 cost_tier 字段下发；
// 前端仅把档位映射为着色 class，不再复写阈值。
export const getCostValueClass = (log: Pick<RequestLog, 'cost_tier'>): string => {
  switch (log.cost_tier) {
    case 'low':
      return 'token-cost-low'
    case 'medium':
      return 'token-cost-medium'
    case 'high':
      return 'token-cost-high'
    default:
      return ''
  }
}
