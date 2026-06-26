import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { Notification } from '../../bindings/timo/models'

const DONE_CLEAR_DELAY = 5000
const ATTENTION_CLEAR_DELAY = 8000

export const useNotificationStore = defineStore('notification', () => {
  const state = ref<'running' | 'attention' | 'done' | ''>('')
  const source = ref<'claude' | 'reasonix' | ''>('')
  const message = ref('')
  const tool = ref('')
  const workDir = ref('')
  const topic = ref('')
  const subagent = ref(false)
  const startedAt = ref(0)
  const elapsed = ref(0)
  let clearTimer: ReturnType<typeof setTimeout> | null = null
  let tickTimer: ReturnType<typeof setInterval> | null = null

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

  function handle(notif: Notification & { topic?: string }) {
    if (clearTimer) {
      clearTimeout(clearTimer)
      clearTimer = null
    }

    switch (notif.type) {
      case 'claude-prompt':
      case 'reasonix-prompt':
        // User submitted a new prompt — primary "work started" signal
        source.value = notif.type.startsWith('reasonix') ? 'reasonix' : 'claude'
        if (notif.topic) topic.value = notif.topic
        if (notif.workDir) workDir.value = notif.workDir
        subagent.value = false
        if (state.value !== 'running') {
          startedAt.value = Date.now()
          elapsed.value = 0
        }
        state.value = 'running'
        message.value = ''
        tool.value = ''
        startTicker()
        break

      case 'claude-tool':
      case 'reasonix-tool':
        // PostToolUse: update current tool name (lightweight)
        if (notif.tool) tool.value = notif.tool
        // If we're not in running state, start (covers edge cases)
        if (state.value !== 'running') {
          source.value = notif.type.startsWith('reasonix') ? 'reasonix' : 'claude'
          state.value = 'running'
          startedAt.value = Date.now()
          elapsed.value = 0
          startTicker()
        }
        break

      case 'claude-subagent':
      case 'reasonix-subagent':
        subagent.value = true
        break

      case 'claude-subagent-done':
        subagent.value = false
        break

      case 'claude-start':
      case 'reasonix-start':
        // Legacy: from PreToolUse (if still configured)
        source.value = notif.type.startsWith('reasonix') ? 'reasonix' : 'claude'
        if (state.value !== 'running') {
          startedAt.value = Date.now()
          elapsed.value = 0
        }
        state.value = 'running'
        if (notif.tool) tool.value = notif.tool
        if (notif.workDir) workDir.value = notif.workDir
        startTicker()
        break

      case 'claude-notify':
      case 'reasonix-notify':
        // Permission prompt or attention needed
        source.value = notif.type.startsWith('reasonix') ? 'reasonix' : 'claude'
        state.value = 'attention'
        message.value = notif.message || '需要关注'
        stopTicker()
        clearTimer = setTimeout(() => reset(), ATTENTION_CLEAR_DELAY)
        break

      case 'claude-done':
      case 'reasonix-done':
        // Work completed (Stop/SessionEnd event or process exit)
        state.value = 'done'
        message.value = notif.message || '完成'
        stopTicker()
        clearTimer = setTimeout(() => reset(), DONE_CLEAR_DELAY)
        break

      default:
        if (notif.message) {
          source.value = notif.type.startsWith('reasonix') ? 'reasonix' : 'claude'
          state.value = 'attention'
          message.value = notif.message
          stopTicker()
          clearTimer = setTimeout(() => reset(), ATTENTION_CLEAR_DELAY)
        }
    }
  }

  function reset() {
    state.value = ''
    source.value = ''
    message.value = ''
    tool.value = ''
    workDir.value = ''
    topic.value = ''
    subagent.value = false
    startedAt.value = 0
    elapsed.value = 0
    stopTicker()
  }

  function clear() {
    if (clearTimer) {
      clearTimeout(clearTimer)
      clearTimer = null
    }
    reset()
  }

  return { state, source, message, tool, workDir, topic, subagent, elapsedText, handle, clear }
})
