import {
  Delete,
  Endpoint,
  GetInjectionStatus,
  Inject,
  List,
  ListTools,
  RemoveInjection,
  Save,
  SetEnabled,
  Statuses,
  TestConnection,
} from '../../bindings/portico/internal/interfaces/wails/mcpgatewayfacade'
import {
  McpServer,
  ServerType,
  type TestResult,
} from '../../bindings/portico/internal/mcpgateway/domain/models'
import type { ToolInfo, UpstreamStatus } from '../../bindings/portico/internal/mcpgateway/application/models'

export type McpServerRecord = McpServer
export type { TestResult, ToolInfo, UpstreamStatus }
export { McpServer, ServerType }

export const fetchMcpServers = (): Promise<McpServer[]> => List()
export const saveMcpServer = (server: McpServer): Promise<McpServer> => Save(server)
export const deleteMcpServer = (id: number): Promise<void> => Delete(id)
export const setMcpServerEnabled = (id: number, enabled: boolean): Promise<void> =>
  SetEnabled(id, enabled)
export const testMcpConnection = (server: McpServer): Promise<TestResult> => TestConnection(server)
export const fetchMcpStatuses = (): Promise<UpstreamStatus[]> => Statuses()
export const fetchMcpEndpoint = (): Promise<string> => Endpoint()
export const fetchInjectionStatus = (platform: string): Promise<boolean> =>
  GetInjectionStatus(platform)
export const injectMcp = (platform: string): Promise<void> => Inject(platform)
export const removeMcpInjection = (platform: string): Promise<void> => RemoveInjection(platform)

export const fetchMcpTools = (): Promise<ToolInfo[]> => ListTools()
