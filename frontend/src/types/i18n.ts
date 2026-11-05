// i18n 辅助类型的唯一定义处：各 composable 统一从这里导入，
// 不再各自复制 TranslateFn/LocaleRef。
export type TranslateFn = (key: string, named?: Record<string, unknown>) => string
export type LocaleRef = { value: string }
