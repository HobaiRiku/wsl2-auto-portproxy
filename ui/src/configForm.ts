// Converts between the config JSON served by /api/config and an editable
// form model. The server still validates everything on save; checks here
// only give earlier, field-level messages.

import type { Translate } from './i18n'

export type Protocol = 'tcp' | 'udp'
export interface Mapping { protocol: Protocol; local: number | null; remote: number | null }
export interface AllowRule { protocol: Protocol; port: number | null; sources: string[] }
export interface ConfigForm {
  schemaVersion?: number
  distro: string
  listenAddress: string
  onlyPredefined: boolean
  udpEnabled: boolean
  mappings: Mapping[]
  ignoreTcp: string[]
  ignoreUdp: string[]
  allow: AllowRule[]
  maxConnections: number | null
  udpIdleSeconds: number | null
}

type JSONObject = Record<string, unknown>
const isObject = (v: unknown): v is JSONObject => !!v && typeof v === 'object' && !Array.isArray(v)
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : [])
const both = (v: unknown) => (isObject(v) ? v : {})

export function fromConfig(config: unknown, t: Translate): ConfigForm {
  if (!isObject(config)) throw new Error(t('config.notObject'))
  const predefined = both(config.predefined)
  const ignore = both(config.ignore)
  const allowlist = both(config.allowlist)
  const mappings: Mapping[] = []
  for (const protocol of ['tcp', 'udp'] as const) {
    for (const item of list(predefined[protocol])) {
      const [local, remote] = String(item).split(':').map(Number)
      mappings.push({ protocol, local: local ?? null, remote: remote ?? null })
    }
  }
  const allow: AllowRule[] = []
  for (const protocol of ['tcp', 'udp'] as const) {
    for (const [port, sources] of Object.entries(both(allowlist[protocol]))) {
      allow.push({ protocol, port: Number(port), sources: list(sources).map(String) })
    }
  }
  return {
    schemaVersion: typeof config.schemaVersion === 'number' ? config.schemaVersion : undefined,
    distro: typeof config.distro === 'string' ? config.distro : '',
    listenAddress: typeof config.listenAddress === 'string' ? config.listenAddress : '',
    onlyPredefined: config.onlyPredefined === true,
    udpEnabled: config.udpEnabled === true,
    mappings,
    ignoreTcp: list(ignore.tcp).map(String),
    ignoreUdp: list(ignore.udp).map(String),
    allow,
    maxConnections: typeof config.maxConnections === 'number' && config.maxConnections > 0 ? config.maxConnections : null,
    udpIdleSeconds: typeof config.udpIdleSeconds === 'number' && config.udpIdleSeconds > 0 ? config.udpIdleSeconds : null
  }
}

const validPort = (p: unknown): p is number => typeof p === 'number' && Number.isInteger(p) && p >= 1 && p <= 65535
const ipv4 = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/
const validSource = (s: string) => {
  const [ip, bits, extra] = s.trim().split('/')
  if (extra !== undefined || !ip) return false
  const v4 = ipv4.test(ip)
  if (!v4 && !(ip.includes(':') && /^[0-9a-fA-F:.]+$/.test(ip))) return false
  if (bits === undefined) return true
  const n = Number(bits)
  return /^\d+$/.test(bits) && n <= (v4 ? 32 : 128)
}

// Returns the config object, or the list of problems that block saving.
export function toConfig(form: ConfigForm, t: Translate): { config?: JSONObject; errors: string[] } {
  const errors: string[] = []
  const predefined: Record<Protocol, string[]> = { tcp: [], udp: [] }
  const seen = new Set<string>()
  form.mappings.forEach((m, i) => {
    if (!validPort(m.local) || !validPort(m.remote)) {
      errors.push(t('validation.mappingPort', { n: i + 1 }))
      return
    }
    const key = `${m.protocol}/${m.local}`
    if (seen.has(key)) errors.push(t('validation.mappingDuplicate', { protocol: m.protocol.toUpperCase(), port: m.local }))
    seen.add(key)
    predefined[m.protocol].push(`${m.local}:${m.remote}`)
  })
  const ports = (values: string[], protocol: string) => values.map((v) => {
    const n = Number(v)
    if (!validPort(n)) errors.push(t('validation.ignorePort', { protocol, value: v }))
    return n
  })
  const ignore = { tcp: ports(form.ignoreTcp, 'TCP'), udp: ports(form.ignoreUdp, 'UDP') }
  const allowlist: Record<Protocol, Record<string, string[]>> = { tcp: {}, udp: {} }
  form.allow.forEach((rule, i) => {
    if (!validPort(rule.port)) {
      errors.push(t('validation.rulePort', { n: i + 1 }))
      return
    }
    if (allowlist[rule.protocol][rule.port]) errors.push(t('validation.ruleDuplicate', { protocol: rule.protocol.toUpperCase(), port: rule.port }))
    for (const s of rule.sources) if (!validSource(s)) errors.push(t('validation.ruleSource', { port: rule.port, value: s }))
    allowlist[rule.protocol][rule.port] = rule.sources.map((s) => s.trim())
  })
  const listen = form.listenAddress.trim()
  if (listen && (listen.includes('/') || !validSource(listen))) errors.push(t('validation.listen'))
  if (form.maxConnections !== null && (form.maxConnections < 1 || form.maxConnections > 1024)) errors.push(t('validation.maxConnections'))
  if (form.udpIdleSeconds !== null && (form.udpIdleSeconds < 1 || form.udpIdleSeconds > 86400)) errors.push(t('validation.udpIdle'))
  if (errors.length) return { errors }
  const config: JSONObject = {}
  if (form.schemaVersion !== undefined) config.schemaVersion = form.schemaVersion
  if (form.distro.trim()) config.distro = form.distro.trim()
  config.onlyPredefined = form.onlyPredefined
  config.predefined = predefined
  config.ignore = ignore
  config.allowlist = allowlist
  if (listen) config.listenAddress = listen
  config.udpEnabled = form.udpEnabled
  if (form.maxConnections !== null) config.maxConnections = form.maxConnections
  if (form.udpIdleSeconds !== null) config.udpIdleSeconds = form.udpIdleSeconds
  return { config, errors }
}
