<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watchEffect } from 'vue'
import { NAlert, NButton, NCard, NConfigProvider, NSelect, NSpin, NTag, NTooltip } from 'naive-ui'
import ConfigEditor from './ConfigEditor.vue'
import { naiveLocales, useI18n, type LocaleSetting } from './i18n'
import { useService } from './store'

const service = useService()
const { locale, setting, setLocale, t } = useI18n()
const tab = ref('overview')
const tabs = computed(() => (['overview', 'ports', 'config', 'logs'] as const).map((id) => ({ id, label: t(`app.tabs.${id}`) })))
// Language names stay in their own language so they are recognizable in any UI language.
const localeOptions = computed(() => [
  { label: t('app.system'), value: 'system' },
  { label: '中文', value: 'zh-CN' },
  { label: 'English', value: 'en' },
  { label: '日本語', value: 'ja' }
])
watchEffect(() => { document.documentElement.lang = locale.value })
const editorError = ref('')
const selected = ref('')
const rows = computed(() => service.status?.proxies || [])
const emptyPortsMessage = computed(() => {
  if (service.error) return t('ports.emptyOffline')
  const mode = service.status?.wsl.networkMode
  if (mode && mode !== 'nat' && mode !== 'unknown') return t('ports.emptyMode', { mode })
  return t('ports.emptyNone')
})
const label = (state: string) => t(`state.${state}`)
const decisionTypes: Record<string, 'success' | 'default' | 'warning'> = {
  forwarded: 'success', blocked: 'warning', ignored: 'default', 'udp-disabled': 'default', 'not-predefined': 'default', conflict: 'warning', 'not-listening': 'warning'
}
const discovered = computed(() => service.status?.discovered || [])
const emptyDiscoveredMessage = computed(() => {
  const wsl = service.status?.wsl
  if (!wsl || service.error) return t('overview.emptyOffline')
  if (wsl.error) return t('overview.emptyError')
  if (wsl.state !== 'running') return t('overview.emptyStopped')
  if (wsl.networkMode !== 'nat') return t('overview.emptyMode', { mode: wsl.networkMode })
  return t('overview.emptyNone')
})
const now = ref(Date.now())
const lastScan = computed(() => {
  const at = service.status?.wsl.lastScan
  return at ? new Date(at).toLocaleTimeString(locale.value) : '—'
})
// Scans run every 2s; no fresh result for 10s means the loop is not running.
const scanState = computed<'running' | 'error' | 'stopped'>(() => {
  if (service.error) return 'stopped'
  if (service.status?.wsl.error) return 'error'
  const at = service.status?.wsl.lastScan
  return at && now.value - new Date(at).getTime() <= 10000 ? 'running' : 'stopped'
})
let timer: ReturnType<typeof setInterval>
let clock: ReturnType<typeof setInterval>

async function selectTab(id: string) {
  tab.value = id
  if (id === 'logs') {
    try { await service.loadLogs(); editorError.value = '' } catch (err) { editorError.value = String(err) }
  }
}
onMounted(async () => {
  await service.refresh()
  timer = setInterval(() => { void service.refresh() }, 3000)
  clock = setInterval(() => { now.value = Date.now() }, 1000)
})
onUnmounted(() => { clearInterval(timer); clearInterval(clock) })
</script>

<template>
  <NConfigProvider :locale="naiveLocales[locale].locale" :date-locale="naiveLocales[locale].date" :theme-overrides="{ common: { primaryColor: '#176b54', primaryColorHover: '#218769', borderRadius: '8px' } }">
    <div class="shell">
      <header>
        <div class="brand"><img src="/icon.svg" alt="" /><div><strong>wslpp</strong><span>{{ t('app.subtitle') }}</span></div></div>
        <div class="header-right">
          <span class="scan-status" :title="t('app.lastScan', { time: lastScan })"><NSpin v-if="scanState === 'running'" :size="14" /><span v-else :class="['scan-flag', scanState]">{{ scanState === 'error' ? t('app.scanError') : t('app.scanStopped') }}</span>{{ t('app.scan', { time: lastScan }) }}</span>
          <NTag :type="service.error ? 'warning' : 'success'" :bordered="false">{{ service.error ? t('app.disconnected') : t('app.connected') }}</NTag>
          <NSelect :value="setting" :options="localeOptions" size="small" class="locale" :aria-label="t('app.language')" @update:value="(v: LocaleSetting) => setLocale(v)" />
          <NButton :loading="service.busy" @click="service.refresh">{{ t('app.refresh') }}</NButton>
        </div>
      </header>
      <nav :aria-label="t('app.nav')"><button v-for="item in tabs" :key="item.id" :class="{ active: tab === item.id }" @click="selectTab(item.id)">{{ item.label }}</button></nav>
      <main>
        <NAlert v-if="service.error" type="warning" class="alert" :title="t('app.unreachable')">{{ service.error }}</NAlert>
        <NAlert v-if="scanState === 'error'" type="error" class="alert" :title="t('app.scanError')">{{ service.status?.wsl.error }}</NAlert>
        <NAlert v-if="service.status?.configError" type="error" class="alert" :title="t('app.configNotApplied')">{{ service.status.configError }}</NAlert>
        <NAlert v-if="service.status?.wsl.networkMode === 'mirrored'" type="info" class="alert" :title="t('app.mirroredTitle')">{{ t('app.mirroredBody') }}</NAlert>
        <section v-if="tab === 'overview'">
          <div class="page-title"><div><h1>{{ t('overview.title') }}</h1><p>{{ t('overview.description') }}</p></div><span class="muted">{{ service.status?.version || '—' }}</span></div>
          <div class="summary">
            <NCard :title="t('overview.distro')"><strong class="value">{{ service.status?.wsl.distro || t('overview.waiting') }}</strong><p>{{ label(service.status?.wsl.state || 'unknown') }}</p></NCard>
            <NCard :title="t('overview.network')"><strong class="value">{{ service.status?.wsl.networkMode || '—' }}</strong><p>{{ service.status?.wsl.ip || t('overview.noTarget') }}</p></NCard>
            <NCard :title="t('overview.listening')"><strong class="value">{{ rows.filter(row => row.state === 'active').length }}</strong><p>{{ t('overview.blockedCount', { n: rows.filter(row => row.state === 'blocked').length }) }}</p></NCard>
          </div>
          <NCard>
            <template #header>{{ t('overview.discovered') }} <NTooltip><template #trigger><span class="hint">?</span></template>{{ t('overview.discoveredHint') }}</NTooltip></template>
            <div class="table-wrap"><table><thead><tr><th>{{ t('overview.protocol') }}</th><th>{{ t('overview.wslPort') }}</th><th>{{ t('overview.decision') }}</th><th>{{ t('overview.windowsPort') }}</th></tr></thead><tbody>
              <tr v-for="item in discovered" :key="item.protocol + item.port + item.decision + (item.local || []).join()"><td>{{ item.protocol.toUpperCase() }}</td><td>{{ item.port }}</td><td><NTag :type="decisionTypes[item.decision] || 'default'" size="small" :bordered="false">{{ t(`decision.${item.decision}`) }}</NTag></td><td>{{ item.local?.join(', ') || '—' }}</td></tr>
              <tr v-if="!discovered.length"><td colspan="4" class="empty">{{ emptyDiscoveredMessage }}</td></tr>
            </tbody></table></div>
          </NCard>
        </section>
        <section v-else-if="tab === 'ports'">
          <div class="page-title"><div><h1>{{ t('ports.title') }}</h1><p>{{ t('ports.description') }}</p></div><NButton @click="selectTab('config')">{{ t('ports.editRules') }}</NButton></div>
          <div class="table-wrap"><table><thead><tr><th>{{ t('ports.protocol') }}</th><th>Windows</th><th>{{ t('ports.target') }}</th><th>{{ t('ports.state') }}</th><th>{{ t('ports.sessions') }}</th></tr></thead><tbody>
            <tr v-for="row in rows" :key="row.id" :class="{ selected: selected === row.id }"><td>{{ row.protocol.toUpperCase() }}</td><td><button class="row-select" @click="selected = selected === row.id ? '' : row.id">{{ row.listen }}</button></td><td>{{ row.target }}</td><td><NTag :type="row.state === 'active' ? 'success' : 'warning'" size="small" :bordered="false">{{ label(row.state) }}</NTag></td><td>{{ row.activeConnections }}</td></tr>
            <tr v-if="!rows.length"><td colspan="5" class="empty">{{ emptyPortsMessage }}</td></tr>
          </tbody></table></div>
          <NCard v-for="row in rows.filter(row => row.id === selected)" :key="row.id" :title="t('ports.details')" class="details"><p>{{ row.listen }} → {{ row.target }}</p><p>{{ t('ports.traffic', { in: row.bytesIn, out: row.bytesOut, dropped: row.dropped }) }}</p><p v-if="row.error" class="error">{{ row.error }}</p></NCard>
        </section>
        <section v-else-if="tab === 'logs'">
          <div class="page-title"><div><h1>{{ t('logs.title') }}</h1><p>{{ t('logs.description') }}</p></div><NButton @click="selectTab('logs')">{{ t('logs.refresh') }}</NButton></div>
          <NAlert v-if="editorError" type="error" class="alert">{{ editorError }}</NAlert>
          <NCard><div v-if="!service.logs.length" class="empty">{{ t('logs.empty') }}</div><ol class="logs"><li v-for="(entry, index) in service.logs" :key="index"><time>{{ new Date(entry.time).toLocaleTimeString(locale) }}</time><NTag size="small" :bordered="false">{{ entry.level }}</NTag><span>{{ entry.message }}</span></li></ol></NCard>
        </section>
        <section v-show="tab === 'config'"><ConfigEditor :offline="!!service.error" :discovered="service.status?.discovered || []" @saved="service.refresh" /></section>
      </main>
      <footer>Windows → WSL · TCP / UDP</footer>
    </div>
  </NConfigProvider>
</template>
