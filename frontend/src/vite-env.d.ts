/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** 开发模式下设为 "1" 时强制使用演示数据（不连接 portico-server）。 */
  readonly VITE_PORTICO_MOCK?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
