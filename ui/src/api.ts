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
export interface Status {
  version: string
  startedAt: string
  configError?: string
  wsl: { state: string; distro: string; ip: string; networkMode: string; lastScan?: string; error?: string }
  proxies: ProxyStatus[]
}
export interface ConfigDocument { revision: string; config: Record<string, unknown> }
export interface LogEntry { time: string; level: string; message: string }

export class APIError extends Error {
  constructor(public status: number, message: string) { super(message) }
}
async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`/api${path}`, { ...options, credentials: 'same-origin', cache: 'no-store',
    headers: { 'Content-Type': 'application/json', ...options.headers } })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new APIError(response.status, body.error || `请求失败 (${response.status})`)
  }
  return response.json() as Promise<T>
}
export const api = {
  connect: (code: string) => request('/connect', { method: 'POST', body: JSON.stringify({ code }) }),
  status: () => request<Status>('/status'),
  config: () => request<ConfigDocument>('/config'),
  // Preserve editor text so the server can reject duplicate JSON fields.
  save: (revision: string, configJSON: string) => request<ConfigDocument>('/config', {
    method: 'PUT', body: `{"revision":${JSON.stringify(revision)},"config":${configJSON}}`
  }),
  logs: () => request<LogEntry[]>('/logs')
}
