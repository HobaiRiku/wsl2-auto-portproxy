import { translate } from './i18n'

export interface ProxyStatus {
  id: string
  protocol: 'tcp' | 'udp'
  listen: string
  target: string
  state: string
  activeConnections: number
  bytesIn: number
  bytesOut: number
  dropped: number
  error?: string
}
export interface Discovered {
  protocol: 'tcp' | 'udp'
  port: number
  decision: 'forwarded' | 'blocked' | 'ignored' | 'udp-disabled' | 'not-predefined' | 'conflict' | 'not-listening'
  local?: number[]
}
export interface Status {
  version: string
  startedAt: string
  configError?: string
  wsl: { state: string; distro: string; ip: string; networkMode: string; lastScan?: string; error?: string }
  discovered: Discovered[]
  proxies: ProxyStatus[]
}
export interface ConfigDocument { revision: string; config: Record<string, unknown> }
export interface Distro { name: string; default: boolean; version: string }
export interface LogEntry { time: string; level: string; message: string }

export class APIError extends Error {
  constructor(public status: number, message: string) { super(message) }
}
async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`/api${path}`, { ...options, credentials: 'same-origin', cache: 'no-store',
    headers: { 'Content-Type': 'application/json', ...options.headers } })
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string }
    throw new APIError(response.status, body.error || translate('app.requestFailed', { status: response.status }))
  }
  return response.json() as Promise<T>
}
export const api = {
  status: () => request<Status>('/status'),
  config: () => request<ConfigDocument>('/config'),
  // Preserve editor text so the server can reject duplicate JSON fields.
  save: (revision: string, configJSON: string) => request<ConfigDocument>('/config', {
    method: 'PUT', body: `{"revision":${JSON.stringify(revision)},"config":${configJSON}}`
  }),
  logs: () => request<LogEntry[]>('/logs'),
  distros: () => request<Distro[]>('/distros')
}
