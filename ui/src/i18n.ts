import { computed, ref } from 'vue'
import { dateEnUS, dateJaJP, dateZhCN, enUS, jaJP, zhCN } from 'naive-ui'

export type Locale = 'zh-CN' | 'en' | 'ja'
// 'system' follows the browser/OS language and is the default.
export type LocaleSetting = Locale | 'system'

const zh = {
  app: {
    subtitle: 'WSL 端口代理',
    nav: '主导航',
    tabs: { overview: '概览', ports: '端口', config: '配置', logs: '诊断' },
    connected: '服务已连接',
    disconnected: '未连接',
    refresh: '刷新',
    language: '语言',
    system: '跟随系统',
    scan: '扫描 {time}',
    lastScan: '上次扫描 {time}',
    scanError: '扫描异常',
    scanStopped: '扫描停止',
    unreachable: '暂时无法连接服务',
    configNotApplied: '配置未应用',
    mirroredTitle: '已识别 mirrored 网络',
    mirroredBody: '此版本暂停代理；请使用 WSL 原生网络连接。',
    requestFailed: '请求失败 ({status})',
    connectFailed: '服务连接失败'
  },
  state: { running: '运行中', stopped: '已停止', unknown: '未知', active: '已监听', blocked: '端口受阻', stale: '等待确认', error: '异常', unsupported: '暂不代理' },
  decision: {
    forwarded: '已转发',
    blocked: '未转发：Windows 端口被占用，自动重试中',
    ignored: '已忽略（ignore）',
    'udp-disabled': '未转发：UDP 未开启（udpEnabled）',
    'not-predefined': '未转发：不在映射中（onlyPredefined）',
    conflict: '未转发：同号 Windows 端口已被映射占用',
    'not-listening': '已配置映射，但 WSL 中没有监听'
  },
  overview: {
    title: '转发概览',
    description: '查看 WSL 状态和 Windows 端口的实际运行情况。',
    distro: 'WSL 发行版',
    waiting: '等待发现',
    network: '网络',
    noTarget: '暂无目标地址',
    listening: '已监听端口',
    blockedCount: '{n} 个端口受阻',
    discovered: 'WSL 中发现的端口',
    discoveredHint: 'WSL 里监听 0.0.0.0 / eth0 的端口，以及配置规则对每个端口的处理结果。“已转发”只表示 Windows 端口已监听，不代表后端服务正常。',
    protocol: '协议',
    wslPort: 'WSL 端口',
    decision: '处理结果',
    windowsPort: 'Windows 端口',
    emptyOffline: '连接服务后显示。',
    emptyError: '扫描出错，见上方错误信息。',
    emptyStopped: 'WSL 未运行，启动后自动扫描。',
    emptyMode: '当前网络模式为 {mode}，不扫描转发端口。',
    emptyNone: 'WSL 中暂无对外监听的端口。只监听 127.0.0.1 的服务无法从 Windows 转发，需改为监听 0.0.0.0。'
  },
  ports: {
    title: '端口转发',
    description: 'Windows 监听地址 → WSL 服务地址',
    editRules: '编辑规则',
    protocol: '协议',
    target: 'WSL 目标',
    state: '状态',
    sessions: '连接 / 会话',
    details: '端口详情',
    traffic: '转入 {in} B · 转出 {out} B · 丢弃 {dropped}',
    emptyOffline: '连接服务后显示端口状态。',
    emptyMode: '当前网络模式为 {mode}，此版本暂停代理。',
    emptyNone: '暂无转发端口。等待 WSL 发行版中的服务被发现。'
  },
  logs: {
    title: '运行诊断',
    description: '查看扫描、配置和端口启动的最近事件。',
    refresh: '刷新日志',
    empty: '暂无日志。'
  },
  config: {
    title: '代理配置',
    description: '保存时校验；无效内容不会生效，服务继续使用上一份有效配置。',
    form: '表单',
    json: 'JSON',
    save: '保存配置',
    discard: '放弃修改',
    saved: '已保存并生效。端口的实际状态见概览和端口页面。',
    rejected: '配置文件有错误，没有生效：{error}。下面是文件原文，修正后保存即可。',
    jsonLabel: 'JSON 配置',
    notObject: '配置必须是 JSON 对象',
    toFormFailed: 'JSON 无法转换为表单：{error}',
    basic: '基本',
    distro: 'WSL 发行版',
    distroDefault: '跟随 WSL 默认发行版',
    distroDefaultCurrent: '跟随 WSL 默认发行版（当前 {name}）',
    distroUnsupported: '{name}（WSL{version}，不支持）',
    listen: 'Windows 监听地址',
    listenPlaceholder: '0.0.0.0（所有网卡）',
    listenHint: '只在某个网卡上开放时填写该网卡的 IP。',
    onlyPredefined: '只转发映射中的端口',
    onlyPredefinedHint: '关闭时，WSL 中发现的端口会自动以同号端口转发。',
    udp: '转发 UDP',
    udpHint: '默认只转发 TCP。',
    mappings: '端口映射',
    mappingsHint: 'Windows 端口 → WSL 端口。例如把 Windows 2222 转到 WSL 的 22（SSH）。同号端口无需填写，会自动转发。',
    windowsPort: 'Windows 端口',
    wslPort: 'WSL 端口',
    delete: '删除',
    addMapping: '+ 添加映射',
    ignore: '忽略端口',
    ignoreHint: '按 WSL 端口忽略，即使在映射中也不转发。可从正在监听的端口中选择，或输入后回车。',
    listeningOption: '{port}（WSL 正在监听）',
    allow: '访问限制',
    allowHint: '按 Windows 端口限制来源地址。未添加规则的端口不限制；规则来源为空时只允许本机访问。本机（loopback）始终允许。',
    sourcesPlaceholder: '允许的 IP 或网段，如 192.168.1.0/24，回车添加',
    addRule: '+ 添加规则',
    advanced: '高级',
    maxConnections: '每个端口的连接上限',
    defaultValue: '默认 {value}',
    example: '例如 {value}',
    udpIdle: 'UDP 空闲回收（秒）'
  },
  validation: {
    mappingPort: '第 {n} 条端口映射：端口需为 1–65535',
    mappingDuplicate: '端口映射：{protocol} Windows 端口 {port} 重复',
    ignorePort: '忽略 {protocol} 端口：“{value}” 不是有效端口',
    rulePort: '第 {n} 条访问规则：端口需为 1–65535',
    ruleDuplicate: '访问规则：{protocol} 端口 {port} 重复',
    ruleSource: '访问规则 {port}：“{value}” 不是 IP 或 CIDR',
    listen: '监听地址需为 IP 地址',
    maxConnections: '连接上限需为 1–1024',
    udpIdle: 'UDP 空闲回收需为 1–86400 秒'
  }
}
type Messages = typeof zh

const en: Messages = {
  app: {
    subtitle: 'WSL port proxy',
    nav: 'Main navigation',
    tabs: { overview: 'Overview', ports: 'Ports', config: 'Config', logs: 'Diagnostics' },
    connected: 'Connected',
    disconnected: 'Disconnected',
    refresh: 'Refresh',
    language: 'Language',
    system: 'System',
    scan: 'Scanned {time}',
    lastScan: 'Last scan {time}',
    scanError: 'Scan error',
    scanStopped: 'Scan stopped',
    unreachable: 'Cannot reach the service',
    configNotApplied: 'Config not applied',
    mirroredTitle: 'Mirrored networking detected',
    mirroredBody: 'Proxying is paused in this version; use WSL native networking instead.',
    requestFailed: 'Request failed ({status})',
    connectFailed: 'Cannot connect to the service'
  },
  state: { running: 'Running', stopped: 'Stopped', unknown: 'Unknown', active: 'Listening', blocked: 'Port blocked', stale: 'Unconfirmed', error: 'Error', unsupported: 'Not proxied' },
  decision: {
    forwarded: 'Forwarded',
    blocked: 'Not forwarded: Windows port in use, retrying',
    ignored: 'Ignored (ignore)',
    'udp-disabled': 'Not forwarded: UDP disabled (udpEnabled)',
    'not-predefined': 'Not forwarded: not mapped (onlyPredefined)',
    conflict: 'Not forwarded: same Windows port used by a mapping',
    'not-listening': 'Mapped, but nothing listens in WSL'
  },
  overview: {
    title: 'Overview',
    description: 'WSL state and what the Windows ports are actually doing.',
    distro: 'WSL distribution',
    waiting: 'Discovering',
    network: 'Network',
    noTarget: 'No target address',
    listening: 'Listening ports',
    blockedCount: '{n} blocked',
    discovered: 'Ports found in WSL',
    discoveredHint: 'Ports listening on 0.0.0.0 / eth0 in WSL, and what the config rules did with each. “Forwarded” only means the Windows port is listening, not that the backend is healthy.',
    protocol: 'Protocol',
    wslPort: 'WSL port',
    decision: 'Result',
    windowsPort: 'Windows port',
    emptyOffline: 'Shown once the service is connected.',
    emptyError: 'Scan failed; see the error above.',
    emptyStopped: 'WSL is not running; scanning resumes when it starts.',
    emptyMode: 'Network mode is {mode}; ports are not scanned for forwarding.',
    emptyNone: 'Nothing in WSL is listening externally. Services bound to 127.0.0.1 cannot be forwarded from Windows; bind them to 0.0.0.0.'
  },
  ports: {
    title: 'Port forwarding',
    description: 'Windows listen address → WSL service address',
    editRules: 'Edit rules',
    protocol: 'Protocol',
    target: 'WSL target',
    state: 'State',
    sessions: 'Connections / sessions',
    details: 'Port details',
    traffic: 'In {in} B · Out {out} B · Dropped {dropped}',
    emptyOffline: 'Port state is shown once the service is connected.',
    emptyMode: 'Network mode is {mode}; proxying is paused in this version.',
    emptyNone: 'No forwarded ports yet. Waiting for services in the WSL distribution.'
  },
  logs: {
    title: 'Diagnostics',
    description: 'Recent scan, config and port events.',
    refresh: 'Refresh logs',
    empty: 'No logs yet.'
  },
  config: {
    title: 'Proxy config',
    description: 'Validated on save; invalid changes never take effect and the last valid config stays in use.',
    form: 'Form',
    json: 'JSON',
    save: 'Save',
    discard: 'Discard changes',
    saved: 'Saved and applied. See Overview and Ports for the actual port state.',
    rejected: 'The config file has an error and was not applied: {error}. Its text is shown below; fix it and save.',
    jsonLabel: 'JSON config',
    notObject: 'Config must be a JSON object',
    toFormFailed: 'JSON cannot be shown as a form: {error}',
    basic: 'General',
    distro: 'WSL distribution',
    distroDefault: 'Follow the WSL default distribution',
    distroDefaultCurrent: 'Follow the WSL default distribution (now {name})',
    distroUnsupported: '{name} (WSL{version}, unsupported)',
    listen: 'Windows listen address',
    listenPlaceholder: '0.0.0.0 (all interfaces)',
    listenHint: 'Set an interface IP to expose ports on that interface only.',
    onlyPredefined: 'Only forward mapped ports',
    onlyPredefinedHint: 'When off, ports found in WSL are forwarded on the same port number automatically.',
    udp: 'Forward UDP',
    udpHint: 'Only TCP is forwarded by default.',
    mappings: 'Port mappings',
    mappingsHint: 'Windows port → WSL port, e.g. Windows 2222 to WSL 22 (SSH). Same-number ports are forwarded automatically.',
    windowsPort: 'Windows port',
    wslPort: 'WSL port',
    delete: 'Delete',
    addMapping: '+ Add mapping',
    ignore: 'Ignored ports',
    ignoreHint: 'WSL ports that are never forwarded, even when mapped. Pick a listening port or type one and press Enter.',
    listeningOption: '{port} (listening in WSL)',
    allow: 'Access restrictions',
    allowHint: 'Limit source addresses per Windows port. Ports without a rule are unrestricted; a rule with no sources allows this PC only. Loopback is always allowed.',
    sourcesPlaceholder: 'Allowed IP or CIDR, e.g. 192.168.1.0/24, press Enter',
    addRule: '+ Add rule',
    advanced: 'Advanced',
    maxConnections: 'Connection limit per port',
    defaultValue: 'Default {value}',
    example: 'e.g. {value}',
    udpIdle: 'UDP idle timeout (seconds)'
  },
  validation: {
    mappingPort: 'Mapping {n}: ports must be 1–65535',
    mappingDuplicate: 'Mappings: {protocol} Windows port {port} is used twice',
    ignorePort: 'Ignored {protocol} ports: “{value}” is not a valid port',
    rulePort: 'Rule {n}: port must be 1–65535',
    ruleDuplicate: 'Access rules: {protocol} port {port} is used twice',
    ruleSource: 'Rule {port}: “{value}” is not an IP or CIDR',
    listen: 'Listen address must be an IP address',
    maxConnections: 'Connection limit must be 1–1024',
    udpIdle: 'UDP idle timeout must be 1–86400 seconds'
  }
}

const ja: Messages = {
  app: {
    subtitle: 'WSL ポートプロキシ',
    nav: 'メインナビゲーション',
    tabs: { overview: '概要', ports: 'ポート', config: '設定', logs: '診断' },
    connected: '接続済み',
    disconnected: '未接続',
    refresh: '更新',
    language: '言語',
    system: 'システム設定',
    scan: 'スキャン {time}',
    lastScan: '最終スキャン {time}',
    scanError: 'スキャン異常',
    scanStopped: 'スキャン停止',
    unreachable: 'サービスに接続できません',
    configNotApplied: '設定が適用されていません',
    mirroredTitle: 'mirrored ネットワークを検出しました',
    mirroredBody: 'このバージョンではプロキシを停止しています。WSL のネイティブネットワークを利用してください。',
    requestFailed: 'リクエストに失敗しました ({status})',
    connectFailed: 'サービスに接続できません'
  },
  state: { running: '実行中', stopped: '停止', unknown: '不明', active: '待ち受け中', blocked: 'ポート使用中', stale: '確認待ち', error: 'エラー', unsupported: '対象外' },
  decision: {
    forwarded: '転送中',
    blocked: '未転送：Windows ポートが使用中のため再試行中',
    ignored: '除外（ignore）',
    'udp-disabled': '未転送：UDP が無効（udpEnabled）',
    'not-predefined': '未転送：マッピング対象外（onlyPredefined）',
    conflict: '未転送：同じ番号の Windows ポートをマッピングが使用中',
    'not-listening': 'マッピング済みですが、WSL で待ち受けていません'
  },
  overview: {
    title: '転送の概要',
    description: 'WSL の状態と Windows ポートの実際の動作を確認します。',
    distro: 'WSL ディストリビューション',
    waiting: '検出中',
    network: 'ネットワーク',
    noTarget: '転送先アドレスなし',
    listening: '待ち受け中のポート',
    blockedCount: '{n} 件が使用中',
    discovered: 'WSL で検出したポート',
    discoveredHint: 'WSL で 0.0.0.0 / eth0 を待ち受けているポートと、設定ルールによる各ポートの処理結果です。「転送中」は Windows ポートが待ち受け中であることのみを示し、バックエンドが正常であることは保証しません。',
    protocol: 'プロトコル',
    wslPort: 'WSL ポート',
    decision: '処理結果',
    windowsPort: 'Windows ポート',
    emptyOffline: 'サービスに接続すると表示されます。',
    emptyError: 'スキャンに失敗しました。上のエラーを確認してください。',
    emptyStopped: 'WSL は停止中です。起動すると自動でスキャンします。',
    emptyMode: 'ネットワークモードが {mode} のため、転送用のスキャンは行いません。',
    emptyNone: 'WSL で外部向けに待ち受けているポートはありません。127.0.0.1 のみで待ち受けるサービスは Windows から転送できないため、0.0.0.0 で待ち受けてください。'
  },
  ports: {
    title: 'ポート転送',
    description: 'Windows 待ち受けアドレス → WSL サービスアドレス',
    editRules: 'ルールを編集',
    protocol: 'プロトコル',
    target: 'WSL 転送先',
    state: '状態',
    sessions: '接続 / セッション',
    details: 'ポートの詳細',
    traffic: '受信 {in} B · 送信 {out} B · 破棄 {dropped}',
    emptyOffline: 'サービスに接続するとポートの状態が表示されます。',
    emptyMode: 'ネットワークモードが {mode} のため、このバージョンではプロキシを停止しています。',
    emptyNone: '転送中のポートはまだありません。WSL ディストリビューションのサービスを検出中です。'
  },
  logs: {
    title: '診断',
    description: 'スキャン、設定、ポート起動の最近のイベントです。',
    refresh: 'ログを更新',
    empty: 'ログはまだありません。'
  },
  config: {
    title: 'プロキシ設定',
    description: '保存時に検証します。無効な内容は反映されず、直前の有効な設定が使われ続けます。',
    form: 'フォーム',
    json: 'JSON',
    save: '設定を保存',
    discard: '変更を破棄',
    saved: '保存して反映しました。実際のポート状態は概要とポートの画面で確認できます。',
    rejected: '設定ファイルにエラーがあり、反映されていません：{error}。以下はファイルの原文です。修正して保存してください。',
    jsonLabel: 'JSON 設定',
    notObject: '設定は JSON オブジェクトである必要があります',
    toFormFailed: 'JSON をフォームに変換できません：{error}',
    basic: '基本',
    distro: 'WSL ディストリビューション',
    distroDefault: 'WSL の既定ディストリビューションに従う',
    distroDefaultCurrent: 'WSL の既定ディストリビューションに従う（現在 {name}）',
    distroUnsupported: '{name}（WSL{version}、非対応）',
    listen: 'Windows 待ち受けアドレス',
    listenPlaceholder: '0.0.0.0（すべてのインターフェイス）',
    listenHint: '特定のネットワークアダプターでのみ公開する場合、その IP を入力します。',
    onlyPredefined: 'マッピングしたポートのみ転送',
    onlyPredefinedHint: 'オフの場合、WSL で検出したポートを同じ番号で自動的に転送します。',
    udp: 'UDP を転送',
    udpHint: '既定では TCP のみ転送します。',
    mappings: 'ポートマッピング',
    mappingsHint: 'Windows ポート → WSL ポート。例：Windows 2222 を WSL の 22（SSH）へ。同じ番号のポートは自動で転送されるため入力不要です。',
    windowsPort: 'Windows ポート',
    wslPort: 'WSL ポート',
    delete: '削除',
    addMapping: '+ マッピングを追加',
    ignore: '除外するポート',
    ignoreHint: 'WSL のポート番号で除外し、マッピングにあっても転送しません。待ち受け中のポートから選ぶか、入力して Enter を押します。',
    listeningOption: '{port}（WSL で待ち受け中）',
    allow: 'アクセス制限',
    allowHint: 'Windows ポートごとに送信元アドレスを制限します。ルールのないポートは制限しません。送信元が空のルールはこの PC からのみ許可します。ループバックは常に許可されます。',
    sourcesPlaceholder: '許可する IP またはネットワーク（例 192.168.1.0/24）、Enter で追加',
    addRule: '+ ルールを追加',
    advanced: '詳細',
    maxConnections: 'ポートごとの接続上限',
    defaultValue: '既定 {value}',
    example: '例：{value}',
    udpIdle: 'UDP アイドル解放（秒）'
  },
  validation: {
    mappingPort: 'マッピング {n}：ポートは 1–65535 で指定してください',
    mappingDuplicate: 'マッピング：{protocol} の Windows ポート {port} が重複しています',
    ignorePort: '除外する {protocol} ポート：「{value}」は有効なポートではありません',
    rulePort: 'ルール {n}：ポートは 1–65535 で指定してください',
    ruleDuplicate: 'アクセス制限：{protocol} のポート {port} が重複しています',
    ruleSource: 'ルール {port}：「{value}」は IP または CIDR ではありません',
    listen: '待ち受けアドレスは IP アドレスで指定してください',
    maxConnections: '接続上限は 1–1024 で指定してください',
    udpIdle: 'UDP アイドル解放は 1–86400 秒で指定してください'
  }
}

const messages: Record<Locale, Messages> = { 'zh-CN': zh, en, ja }

export const naiveLocales = {
  'zh-CN': { locale: zhCN, date: dateZhCN },
  en: { locale: enUS, date: dateEnUS },
  ja: { locale: jaJP, date: dateJaJP }
} as const

const storageKey = 'wslpp.locale'

export function detectLocale(languages: readonly string[]): Locale {
  for (const language of languages) {
    const l = language.toLowerCase()
    if (l.startsWith('zh')) return 'zh-CN'
    if (l.startsWith('ja')) return 'ja'
    if (l.startsWith('en')) return 'en'
  }
  return 'en'
}
function systemLocale(): Locale {
  if (typeof navigator === 'undefined') return 'en'
  return detectLocale(navigator.languages?.length ? navigator.languages : [navigator.language])
}
function readSetting(): LocaleSetting {
  try {
    const value = localStorage.getItem(storageKey)
    if (value === 'zh-CN' || value === 'en' || value === 'ja') return value
  } catch { /* storage unavailable: follow the system */ }
  return 'system'
}

const setting = ref<LocaleSetting>(readSetting())
const locale = computed<Locale>(() => (setting.value === 'system' ? systemLocale() : setting.value))

export function setLocale(next: LocaleSetting) {
  setting.value = next
  try {
    if (next === 'system') localStorage.removeItem(storageKey)
    else localStorage.setItem(storageKey, next)
  } catch { /* the choice still applies for this page */ }
}

function lookup(code: Locale, key: string): string {
  let value: unknown = messages[code]
  for (const part of key.split('.')) {
    if (typeof value !== 'object' || value === null || !Object.prototype.hasOwnProperty.call(value, part)) return key
    value = (value as Record<string, unknown>)[part]
  }
  return typeof value === 'string' ? value : key
}

export type Translate = (key: string, params?: Record<string, string | number>) => string
export function translate(key: string, params?: Record<string, string | number>): string {
  let value = lookup(locale.value, key)
  for (const [name, replacement] of Object.entries(params || {})) value = value.replaceAll(`{${name}}`, String(replacement))
  return value
}

export function useI18n() {
  return { locale, setting, setLocale, t: translate }
}
