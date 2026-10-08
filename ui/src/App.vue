<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { NAlert, NButton, NCard, NConfigProvider, NInput, NTag } from 'naive-ui'
import { api, type ConfigDocument } from './api'
import { useService } from './store'

const service = useService()
const tab = ref('overview')
const tabs = [{ id: 'overview', label: '概览' }, { id: 'ports', label: '端口' }, { id: 'config', label: '配置' }, { id: 'logs', label: '诊断' }]
const configText = ref('')
const loaded = ref<ConfigDocument | null>(null)
const saving = ref(false)
const editorError = ref('')
const notice = ref('')
const selected = ref('')
const rows = computed(() => service.status?.proxies || [])
const stateLabels: Record<string, string> = { running: '运行中', stopped: '已停止', unknown: '未知', active: '已监听', blocked: '端口受阻', stale: '等待确认', error: '异常', unsupported: '暂不代理' }
const label = (state: string) => stateLabels[state] || state
let timer: ReturnType<typeof setInterval>

async function selectTab(id: string) {
  tab.value = id
  notice.value = ''
  if (id === 'config' && !loaded.value) await loadConfig()
  if (id === 'logs') {
    try { await service.loadLogs(); editorError.value = '' } catch (err) { editorError.value = String(err) }
  }
}
async function loadConfig() {
  try {
    loaded.value = await api.config()
    configText.value = JSON.stringify(loaded.value.config, null, 2)
    editorError.value = ''
  } catch (err) { editorError.value = String(err) }
}
async function saveConfig() {
  if (!loaded.value) return
  saving.value = true
  editorError.value = ''
  notice.value = ''
  try {
    const config = JSON.parse(configText.value)
    if (!config || Array.isArray(config) || typeof config !== 'object') throw new Error('配置必须是 JSON 对象')
    loaded.value = await api.save({ revision: loaded.value.revision, config })
    configText.value = JSON.stringify(loaded.value.config, null, 2)
    notice.value = '配置已保存。端口应用结果请查看端口页面。'
    await service.refresh()
  } catch (err) { editorError.value = err instanceof Error ? err.message : String(err) }
  finally { saving.value = false }
}
onMounted(async () => {
  const code = new URLSearchParams(location.hash.slice(1)).get('connect')
  if (code) {
    history.replaceState(null, '', location.pathname + location.search)
    try { await api.connect(code) } catch (err) { service.error = String(err) }
  }
  await service.refresh()
  timer = setInterval(() => { void service.refresh() }, 3000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <NConfigProvider :theme-overrides="{ common: { primaryColor: '#176b54', primaryColorHover: '#218769', borderRadius: '8px' } }">
    <div class="shell">
      <header>
        <div class="brand"><img src="/icon.svg" alt="" /><div><strong>wslpp</strong><span>WSL 端口代理</span></div></div>
        <div class="header-right"><NTag :type="service.error ? 'warning' : 'success'" :bordered="false">{{ service.error ? '未连接' : '服务已连接' }}</NTag><NButton :loading="service.busy" @click="service.refresh">刷新</NButton></div>
      </header>
      <nav aria-label="主导航"><button v-for="item in tabs" :key="item.id" :class="{ active: tab === item.id }" @click="selectTab(item.id)">{{ item.label }}</button></nav>
      <main>
        <NAlert v-if="service.error" type="warning" class="alert" :title="service.needsAuth ? '需要连接授权' : '暂时无法连接服务'">
          {{ service.needsAuth ? '请在本机运行 wslpp ui，以授权链接打开此界面。' : service.error }}
        </NAlert>
        <NAlert v-if="service.status?.configError" type="error" class="alert" title="配置未应用">{{ service.status.configError }}</NAlert>
        <section v-if="tab === 'overview'">
          <div class="page-title"><div><h1>转发概览</h1><p>查看 WSL 状态和 Windows 端口的实际运行情况。</p></div><span class="muted">{{ service.status?.version || '—' }}</span></div>
          <div class="summary">
            <NCard title="WSL 发行版"><strong class="value">{{ service.status?.wsl.distro || '等待发现' }}</strong><p>{{ label(service.status?.wsl.state || 'unknown') }}</p></NCard>
            <NCard title="网络"><strong class="value">{{ service.status?.wsl.networkMode || '—' }}</strong><p>{{ service.status?.wsl.ip || '暂无目标地址' }}</p></NCard>
            <NCard title="已监听端口"><strong class="value">{{ rows.filter(row => row.state === 'active').length }}</strong><p>{{ rows.filter(row => row.state === 'blocked').length }} 个端口受阻</p></NCard>
          </div>
          <NCard title="最近发现"><p>{{ service.status?.wsl.lastScan ? new Date(service.status.wsl.lastScan).toLocaleString() : '尚未完成扫描' }}</p><p v-if="service.status?.wsl.error" class="error">{{ service.status.wsl.error }}</p><p class="muted">WSL 停止时等待，不会因轮询启动发行版。已监听表示代理端口可接收流量。</p></NCard>
        </section>
        <section v-else-if="tab === 'ports'">
          <div class="page-title"><div><h1>端口转发</h1><p>Windows 监听地址 → WSL 服务地址</p></div><NButton @click="selectTab('config')">编辑规则</NButton></div>
          <div class="table-wrap"><table><thead><tr><th>协议</th><th>Windows</th><th>WSL 目标</th><th>状态</th><th>连接 / 会话</th></tr></thead><tbody>
            <tr v-for="row in rows" :key="row.id" :class="{ selected: selected === row.id }"><td>{{ row.protocol.toUpperCase() }}</td><td><button class="row-select" @click="selected = selected === row.id ? '' : row.id">{{ row.listen }}</button></td><td>{{ row.target }}</td><td><NTag :type="row.state === 'active' ? 'success' : 'warning'" size="small" :bordered="false">{{ label(row.state) }}</NTag></td><td>{{ row.activeConnections }}</td></tr>
            <tr v-if="!rows.length"><td colspan="5" class="empty">{{ service.error ? '连接服务后显示端口状态。' : '暂无转发端口。WSL 服务启动后将自动发现。' }}</td></tr>
          </tbody></table></div>
          <NCard v-for="row in rows.filter(row => row.id === selected)" :key="row.id" title="端口详情" class="details"><p>{{ row.listen }} → {{ row.target }}</p><p>转入 {{ row.bytesIn }} B · 转出 {{ row.bytesOut }} B · 丢弃 {{ row.dropped }}</p><p v-if="row.error" class="error">{{ row.error }}</p></NCard>
        </section>
        <section v-else-if="tab === 'config'">
          <div class="page-title"><div><h1>代理配置</h1><p>沿用 JSON 配置；保存时校验，失效编辑保留最后有效配置。</p></div></div>
          <NAlert v-if="editorError" type="error" class="alert">{{ editorError }}</NAlert><NAlert v-if="notice" type="success" class="alert">{{ notice }}</NAlert>
          <NCard><NInput v-model:value="configText" type="textarea" :autosize="{ minRows: 18, maxRows: 30 }" aria-label="JSON 配置" :disabled="!loaded" placeholder="连接服务后加载配置" /><p class="muted">修改映射、忽略端口或收紧 allowlist 会影响相应连接。</p><div class="actions"><NButton type="primary" :loading="saving" :disabled="!loaded || !!service.error" @click="saveConfig">保存配置</NButton><NButton :disabled="saving" @click="loadConfig">重新加载</NButton></div></NCard>
        </section>
        <section v-else>
          <div class="page-title"><div><h1>运行诊断</h1><p>查看扫描、配置和端口启动的最近事件。</p></div><NButton @click="selectTab('logs')">刷新日志</NButton></div>
          <NAlert v-if="editorError" type="error" class="alert">{{ editorError }}</NAlert>
          <NCard><div v-if="!service.logs.length" class="empty">暂无日志。</div><ol class="logs"><li v-for="(entry, index) in service.logs" :key="index"><time>{{ new Date(entry.time).toLocaleTimeString() }}</time><NTag size="small" :bordered="false">{{ entry.level }}</NTag><span>{{ entry.message }}</span></li></ol></NCard>
        </section>
      </main>
      <footer>Windows → WSL · TCP / UDP</footer>
    </div>
  </NConfigProvider>
</template>
