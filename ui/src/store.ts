import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, APIError, type Status, type LogEntry } from './api'

export const useService = defineStore('service', () => {
  const status = ref<Status | null>(null)
  const logs = ref<LogEntry[]>([])
  const error = ref('')
  const needsAuth = ref(false)
  const busy = ref(false)
  async function refresh() {
    if (busy.value) return
    busy.value = true
    try {
      const next = await api.status()
      status.value = next
      error.value = ''
      needsAuth.value = false
    } catch (err) {
      error.value = err instanceof Error ? err.message : '服务连接失败'
      needsAuth.value = err instanceof APIError && err.status === 401
    } finally { busy.value = false }
  }
  async function loadLogs() { logs.value = await api.logs() }
  return { status, logs, error, needsAuth, busy, refresh, loadLogs }
})
