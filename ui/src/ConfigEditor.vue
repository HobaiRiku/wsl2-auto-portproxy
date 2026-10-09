<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NInput, NInputNumber, NRadioButton, NRadioGroup, NSelect, NSwitch } from 'naive-ui'
import { api, type ConfigDocument, type Discovered, type Distro } from './api'
import { fromConfig, toConfig, type ConfigForm, type Protocol } from './configForm'
import { useI18n } from './i18n'

const props = defineProps<{ offline: boolean; discovered: Discovered[] }>()
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()

const mode = ref<'form' | 'json'>('form')
const loaded = ref<ConfigDocument | null>(null)
const form = ref<ConfigForm | null>(null)
const text = ref('')
const distros = ref<Distro[]>([])
const saving = ref(false)
const errors = ref<string[]>([])
const notice = ref('')

const protocols = [{ label: 'TCP', value: 'tcp' }, { label: 'UDP', value: 'udp' }]
const distroOptions = computed(() => {
  const current = distros.value.find((d) => d.default)
  return [
    { label: current ? t('config.distroDefaultCurrent', { name: current.name }) : t('config.distroDefault'), value: '' },
    ...distros.value.map((d) => ({ label: d.version === '2' ? d.name : t('config.distroUnsupported', { name: d.name, version: d.version }), value: d.name, disabled: d.version !== '2' }))
  ]
})
const portOptions = (protocol: Protocol) => props.discovered
  .filter((d) => d.protocol === protocol)
  .map((d) => ({ label: t('config.listeningOption', { port: d.port }), value: String(d.port) }))

function load(doc: ConfigDocument) {
  loaded.value = doc
  form.value = fromConfig(doc.config, t)
  text.value = JSON.stringify(doc.config, null, 2)
}
async function reload() {
  errors.value = []
  notice.value = ''
  try { load(await api.config()) } catch (err) { errors.value = [String(err)] }
}
function switchMode(next: 'form' | 'json') {
  errors.value = []
  if (next === 'json' && form.value) {
    const result = toConfig(form.value, t)
    if (!result.config) { errors.value = result.errors; return }
    text.value = JSON.stringify(result.config, null, 2)
  }
  if (next === 'form') {
    try { form.value = fromConfig(JSON.parse(text.value), t) } catch (err) {
      errors.value = [t('config.toFormFailed', { error: err instanceof Error ? err.message : String(err) })]
      return
    }
  }
  mode.value = next
}
async function save() {
  if (!loaded.value || !form.value) return
  errors.value = []
  notice.value = ''
  let body = text.value
  if (mode.value === 'form') {
    const result = toConfig(form.value, t)
    if (!result.config) { errors.value = result.errors; return }
    body = JSON.stringify(result.config)
  } else {
    try {
      const parsed: unknown = JSON.parse(body)
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(t('config.notObject'))
    } catch (err) { errors.value = [err instanceof Error ? err.message : String(err)]; return }
  }
  saving.value = true
  try {
    load(await api.save(loaded.value.revision, body))
    notice.value = t('config.saved')
    emit('saved')
  } catch (err) { errors.value = [err instanceof Error ? err.message : String(err)] }
  finally { saving.value = false }
}
onMounted(async () => {
  await reload()
  try { distros.value = await api.distros() } catch { distros.value = [] }
})
</script>

<template>
  <div class="page-title">
    <div><h1>{{ t('config.title') }}</h1><p>{{ t('config.description') }}</p></div>
    <NRadioGroup :value="mode" size="small" @update:value="switchMode"><NRadioButton value="form">{{ t('config.form') }}</NRadioButton><NRadioButton value="json">{{ t('config.json') }}</NRadioButton></NRadioGroup>
  </div>
  <NAlert v-if="errors.length" type="error" class="alert"><div v-for="e in errors" :key="e">{{ e }}</div></NAlert>
  <NAlert v-if="notice" type="success" class="alert">{{ notice }}</NAlert>

  <template v-if="mode === 'form' && form">
    <NCard :title="t('config.basic')" class="form-card">
      <div class="field"><label>{{ t('config.distro') }}</label><NSelect v-model:value="form.distro" :options="distroOptions" /></div>
      <div class="field"><label>{{ t('config.listen') }}</label><NInput v-model:value="form.listenAddress" :placeholder="t('config.listenPlaceholder')" /><span class="muted">{{ t('config.listenHint') }}</span></div>
      <div class="field row"><NSwitch v-model:value="form.onlyPredefined" /><div><strong>{{ t('config.onlyPredefined') }}</strong><span class="muted">{{ t('config.onlyPredefinedHint') }}</span></div></div>
      <div class="field row"><NSwitch v-model:value="form.udpEnabled" /><div><strong>{{ t('config.udp') }}</strong><span class="muted">{{ t('config.udpHint') }}</span></div></div>
    </NCard>

    <NCard :title="t('config.mappings')" class="form-card">
      <p class="muted">{{ t('config.mappingsHint') }}</p>
      <div v-for="(m, i) in form.mappings" :key="i" class="line">
        <NSelect v-model:value="m.protocol" :options="protocols" class="proto" />
        <NInputNumber v-model:value="m.local" :min="1" :max="65535" :show-button="false" :placeholder="t('config.windowsPort')" class="port" />
        <span class="arrow">→</span>
        <NInputNumber v-model:value="m.remote" :min="1" :max="65535" :show-button="false" :placeholder="t('config.wslPort')" class="port" />
        <NButton quaternary type="error" @click="form.mappings.splice(i, 1)">{{ t('config.delete') }}</NButton>
      </div>
      <NButton dashed @click="form.mappings.push({ protocol: 'tcp', local: null, remote: null })">{{ t('config.addMapping') }}</NButton>
    </NCard>

    <NCard :title="t('config.ignore')" class="form-card">
      <p class="muted">{{ t('config.ignoreHint') }}</p>
      <div class="field"><label>TCP</label><NSelect v-model:value="form.ignoreTcp" multiple filterable tag :options="portOptions('tcp')" :placeholder="t('config.example', { value: 445 })" /></div>
      <div class="field"><label>UDP</label><NSelect v-model:value="form.ignoreUdp" multiple filterable tag :options="portOptions('udp')" :placeholder="t('config.example', { value: 53 })" /></div>
    </NCard>

    <NCard :title="t('config.allow')" class="form-card">
      <p class="muted">{{ t('config.allowHint') }}</p>
      <div v-for="(rule, i) in form.allow" :key="i" class="line">
        <NSelect v-model:value="rule.protocol" :options="protocols" class="proto" />
        <NInputNumber v-model:value="rule.port" :min="1" :max="65535" :show-button="false" :placeholder="t('config.windowsPort')" class="port" />
        <NSelect v-model:value="rule.sources" multiple filterable tag :show-arrow="false" :options="[]" :placeholder="t('config.sourcesPlaceholder')" class="sources" />
        <NButton quaternary type="error" @click="form.allow.splice(i, 1)">{{ t('config.delete') }}</NButton>
      </div>
      <NButton dashed @click="form.allow.push({ protocol: 'tcp', port: null, sources: [] })">{{ t('config.addRule') }}</NButton>
    </NCard>

    <NCard :title="t('config.advanced')" class="form-card">
      <div class="field"><label>{{ t('config.maxConnections') }}</label><NInputNumber v-model:value="form.maxConnections" :min="1" :max="1024" clearable :placeholder="t('config.defaultValue', { value: 128 })" /></div>
      <div class="field"><label>{{ t('config.udpIdle') }}</label><NInputNumber v-model:value="form.udpIdleSeconds" :min="1" :max="86400" clearable :placeholder="t('config.defaultValue', { value: 60 })" /></div>
    </NCard>
  </template>

  <NCard v-else-if="mode === 'json'">
    <NInput v-model:value="text" type="textarea" :autosize="{ minRows: 18, maxRows: 30 }" :input-props="{ 'aria-label': t('config.jsonLabel') }" />
  </NCard>

  <div class="actions">
    <NButton type="primary" :loading="saving" :disabled="!loaded || offline" @click="save">{{ t('config.save') }}</NButton>
    <NButton :disabled="saving" @click="reload">{{ t('config.discard') }}</NButton>
  </div>
</template>
