import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type Status, type LogEntry } from './api'
import { translate } from './i18n'

export const useService = defineStore('service', () => {
  const status = ref<Status | null>(null)
  const logs = ref<LogEntry[]>([])
  const error = ref('')
  const busy = ref(false)
  async function refresh() {
    if (busy.value) return
    busy.value = true
    try {
      const next = await api.status()
      status.value = next
      error.value = ''
    } catch (err) {
      error.value = err instanceof Error ? err.message : translate('app.connectFailed')
    } finally { busy.value = false }
  }
  async function loadLogs() { logs.value = await api.logs() }
  return { status, logs, error, busy, refresh, loadLogs }
})
