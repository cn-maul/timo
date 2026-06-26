import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface Notification {
  type: string
  message: string
  tool?: string
  workDir?: string
}

export const useNotificationStore = defineStore('notification', () => {
  const state = ref<'running' | 'attention' | 'done' | ''>('')
  const message = ref('')
  const tool = ref('')
  const workDir = ref('')
  const startedAt = ref(0)
  const elapsed = ref(0)
  let clearTimer: ReturnType<typeof setTimeout> | null = null
  let tickTimer: ReturnType<typeof setInterval> | null = null
  let idleTimer: ReturnType<typeof setTimeout> | null = null

  const elapsedText = computed(() => {
    const sec = Math.floor(elapsed.value / 1000)
    const m = Math.floor(sec / 60)
    const s = sec % 60
    return `${m}:${s.toString().padStart(2, '0')}`
  })

  function startTicker() {
    stopTicker()
    tickTimer = setInterval(() => {
      if (startedAt.value > 0) {
        elapsed.value = Date.now() - startedAt.value
      }
    }, 1000)
  }

  function stopTicker() {
    if (tickTimer) {
      clearInterval(tickTimer)
      tickTimer = null
    }
  }

  function resetIdleTimer() {
    if (idleTimer) clearTimeout(idleTimer)
    idleTimer = setTimeout(() => {
      // No tool call for 10s → Claude is idle/done
      if (state.value === 'running') {
        state.value = 'done'
        message.value = '任务完成'
        stopTicker()
        clearTimer = setTimeout(() => reset(), 5000)
      }
    }, 10000)
  }

  function handle(notif: Notification) {
    if (clearTimer) {
      clearTimeout(clearTimer)
      clearTimer = null
    }

    switch (notif.type) {
      case 'claude-start':
        // Only reset timer on first start, not on every tool call
        if (state.value !== 'running') {
          startedAt.value = Date.now()
          elapsed.value = 0
        }
        state.value = 'running'
        if (notif.tool) tool.value = notif.tool
        if (notif.workDir) workDir.value = notif.workDir
        startTicker()
        resetIdleTimer()
        break

      case 'claude-notify':
        state.value = 'attention'
        message.value = notif.message || '需要关注'
        stopTicker()
        if (idleTimer) { clearTimeout(idleTimer); idleTimer = null }
        clearTimer = setTimeout(() => reset(), 8000)
        break

      case 'claude-done':
        state.value = 'done'
        message.value = notif.message || '完成'
        stopTicker()
        if (idleTimer) { clearTimeout(idleTimer); idleTimer = null }
        clearTimer = setTimeout(() => reset(), 5000)
        break

      default:
        reset()
    }
  }

  function reset() {
    state.value = ''
    message.value = ''
    tool.value = ''
    workDir.value = ''
    startedAt.value = 0
    elapsed.value = 0
    stopTicker()
    if (idleTimer) { clearTimeout(idleTimer); idleTimer = null }
  }

  function clear() {
    if (clearTimer) {
      clearTimeout(clearTimer)
      clearTimer = null
    }
    reset()
  }

  return { state, message, tool, workDir, elapsedText, handle, clear }
})
