// 生产环境的 binding 传输层：把 Wails 生成的 `$Call.ByName(...)` 调用转发到
// portico-server 的管理 API（POST /api/rpc/{Facade}.{Method}，body 为参数数组）。
// bindings 由 cmd/bindgen 以 `-names` 生成，方法名形如
// "portico/internal/interfaces/wails.RoutingFacade.GetProfile"，这里截取末两段。
import { objectNames, setTransport } from '@wailsio/runtime'
import { authHeaders, markAuthRequired } from '../services/auth'

type CallArgs = {
  'call-id'?: string
  methodName?: string
  methodID?: number
  args?: unknown[]
}

export const rpcMethodName = (fullName: string): string => {
  const lastSegment = fullName.slice(fullName.lastIndexOf('/') + 1)
  const parts = lastSegment.split('.')
  if (parts.length < 2) throw new Error(`invalid binding method name: ${fullName}`)
  return parts.slice(-2).join('.')
}

const readError = async (res: Response): Promise<string> => {
  const text = await res.text()
  try {
    const body = JSON.parse(text)
    if (body && typeof body.error === 'string' && body.error.trim()) return body.error
  } catch {
    // 非 JSON 响应（如反向代理的错误页），退回状态行。
  }
  return text.trim() || `HTTP ${res.status} ${res.statusText}`
}

const callRPC = async (call: CallArgs): Promise<unknown> => {
  if (!call.methodName) {
    throw new Error('bindings must be generated with `-names` (run `wails3 task common:generate:bindings`)')
  }
  const res = await fetch(`/api/rpc/${encodeURIComponent(rpcMethodName(call.methodName))}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify(call.args ?? []),
    cache: 'no-store',
  })
  if (res.status === 401) {
    markAuthRequired()
    throw new Error(await readError(res))
  }
  if (!res.ok) {
    throw new Error(await readError(res))
  }
  const text = await res.text()
  return text ? JSON.parse(text) : null
}

export const installHttpTransport = () => {
  setTransport({
    call: async (objectID: number, _method: number, _windowName: string, args: unknown) => {
      if (objectID !== objectNames.Call) {
        // 取消调用及窗口/系统类运行时 API 在浏览器中没有意义，与 mock 一致返回 null。
        return null
      }
      return callRPC((args ?? {}) as CallArgs)
    },
  })
}
