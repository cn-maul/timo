import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { Events } from '@wailsio/runtime'
import type { Notification } from '../../bindings/timo/internal/app/models'

const DONE_CLEAR_DELAY = 5000
const ATTENTION_CLEAR_DELAY = 8000

// Tool icon mapping for visual enhancement
const TOOL_ICONS: Record<string, string> = {
  Read: '📄',
  Edit: '✏️',
  Write: '📝',
  Bash: '⚙️',
  Agent: '🤖',
  WebFetch: '🌐',
  WebSearch: '🔍',
  AskUserQuestion: '❓',
  Glob: '📁',
  Grep: '🔎',
  NotebookEdit: '📓',
}

// Agent type display names
const AGENT_TYPE_NAMES: Record<string, string> = {
  Explore: '探索分析',
  Plan: '规划设计',
  'general-purpose': '通用任务',
  'code-reviewer': '代码审查',
  'security-reviewer': '安全审查',
}

// Phase detection based on tool name
type Phase = 'idle' | 'thinking' | 'reading' | 'editing' | 'executing' | 'subagent' | 'attention' | 'done'

function detectPhase(tool: string): Phase {
  if (tool === 'Read' || tool === 'Glob' || tool === 'Grep') return 'reading'
  if (tool === 'Edit' || tool === 'Write' || tool === 'NotebookEdit') return 'editing'
  if (tool === 'Bash') return 'executing'
  if (tool === 'Agent') return 'subagent'
  if (tool === 'WebFetch' || tool === 'WebSearch') return 'reading'
  return 'thinking'
}

// Shorten file path for display
function shortenPath(path: string): string {
  if (!path) return ''
  const parts = path.split('/')
  const filename = parts[parts.length - 1]
  if (parts.length > 3) {
    return '…/' + parts.slice(-2).join('/')
  }
  return path
}

// Summarize bash command
function summarizeCommand(cmd: string): string {
  if (!cmd) return ''
  // Remove common prefixes like VAR=value
  let cleaned = cmd.replace(/^[\w_]+=\S+\s+/, '')
  // Take first command
  const firstCmd = cleaned.split('&&')[0].split(';')[0].split('|')[0].trim()
  if (firstCmd.length > 30) {
    return firstCmd.slice(0, 30) + '…'
  }
  return firstCmd
}

// Extract display info from tool input
function getToolDisplayInfo(tool: string, toolInput: Record<string, any> | undefined | null): { target: string; context: string } {
  if (!toolInput) return { target: '', context: '' }

  switch (tool) {
    case 'Read':
    case 'Edit':
    case 'Write':
    case 'NotebookEdit':
      return {
        target: shortenPath((toolInput.file_path as string) || ''),
        context: tool === 'Edit' ? ((toolInput.new_string as string) || '').slice(0, 40) : '',
      }
    case 'Bash':
      return {
        target: summarizeCommand((toolInput.command as string) || ''),
        context: (toolInput.description as string) || '',
      }
    case 'Agent':
      return {
        target: (toolInput.subagent_type as string) || 'Agent',
        context: (toolInput.description as string) || ((toolInput.prompt as string) || '').slice(0, 50),
      }
    case 'WebFetch':
      return {
        target: '🌐',
        context: ((toolInput.url as string) || '').slice(0, 40),
      }
    case 'WebSearch':
      return {
        target: '🔍',
        context: ((toolInput.query as string) || '').slice(0, 40),
      }
    default:
      return { target: tool, context: '' }
  }
}

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

  // Extended fields
  const toolInput = ref<Record<string, any> | undefined | null>()
  const toolOutput = ref<Record<string, any> | undefined | null>()
  const durationMs = ref(0)
  const agentType = ref('')
  const agentDesc = ref('')
  const agentResult = ref('')
  const finalMsg = ref('')
  const toolCount = ref(0)
  const effortLevel = ref('')
  const isPreTool = ref(false)

  let clearTimer: ReturnType<typeof setTimeout> | null = null
  let tickTimer: ReturnType<typeof setInterval> | null = null

  const elapsedText = computed(() => {
    const sec = Math.floor(elapsed.value / 1000)
    const m = Math.floor(sec / 60)
    const s = sec % 60
    return `${m}:${s.toString().padStart(2, '0')}`
  })

  // Computed display values
  const toolIcon = computed(() => TOOL_ICONS[tool.value] || '🔧')

  /** True when we're in 'attention' state with a question message (approval needed) */
  const isApproval = computed(() => state.value === 'attention' && message.value !== '')

  const phase = computed(() => {
    if (state.value === 'attention') return 'attention'
    if (state.value === 'done') return 'done'
    if (state.value === '') return 'idle'
    if (subagent.value) return 'subagent'
    return detectPhase(tool.value)
  })

  const toolTarget = computed(() => {
    if (!toolInput.value) return ''
    const info = getToolDisplayInfo(tool.value, toolInput.value)
    return info.target
  })

  const toolContext = computed(() => {
    if (!toolInput.value) return ''
    const info = getToolDisplayInfo(tool.value, toolInput.value)
    return info.context
  })

  const agentTypeName = computed(() => {
    return AGENT_TYPE_NAMES[agentType.value] || agentType.value || '子代理'
  })

  const primaryText = computed(() => {
    if (state.value !== 'running') return ''

    // Subagent display
    if (subagent.value && agentType.value) {
      return `${agentTypeName.value}`
    }

    // Tool with target
    if (toolTarget.value) {
      return toolTarget.value
    }

    // Fallback to topic or generic
    return topic.value || (source.value === 'reasonix' ? 'Reasonix 运行中' : 'Claude 运行中')
  })

  const secondaryText = computed(() => {
    if (state.value !== 'running') return ''

    // Subagent description
    if (subagent.value && agentDesc.value) {
      return agentDesc.value
    }

    // Tool context
    if (toolContext.value) {
      return toolContext.value
    }

    return ''
  })

  // Tool history for progress tracking
  const toolHistory = ref<Array<{ tool: string; target: string; duration: number }>>([])

  // Tool summary for stop event
  const toolSummary = computed(() => {
    if (toolHistory.value.length === 0) return ''

    // Count tools by type
    const counts: Record<string, number> = {}
    const fileTools = new Set<string>()

    for (const h of toolHistory.value) {
      counts[h.tool] = (counts[h.tool] || 0) + 1
      // Track file operations
      if ((h.tool === 'Edit' || h.tool === 'Write' || h.tool === 'Read') && h.target) {
        fileTools.add(h.target)
      }
    }

    // Build summary string
    const parts: string[] = []
    const totalTools = toolHistory.value.length
    parts.push(`${totalTools} 个工具`)

    // Highlight file operations
    if (fileTools.size > 0) {
      parts.push(`${fileTools.size} 个文件`)
    }

    // Mention specific tool types if significant
    if (counts['Bash'] && counts['Bash'] > 2) {
      parts.push(`${counts['Bash']} 条命令`)
    }
    if (counts['Agent'] && counts['Agent'] > 0) {
      parts.push(`${counts['Agent']} 个子代理`)
    }

    return parts.join('、')
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

    const type = notif.type
    const typePrefix = type.startsWith('reasonix') ? 'reasonix' : 'claude'

    switch (type) {
      // ── Prompt start (primary "work started" signal) ──
      case 'claude-prompt':
      case 'reasonix-prompt':
        handlePromptStart(notif, typePrefix)
        break

      // ── Tool lifecycle ──
      case 'claude-pre-tool':
      case 'reasonix-pre-tool':
        handlePreTool(notif, typePrefix)
        break
      case 'claude-tool':
      case 'reasonix-tool':
        handlePostTool(notif, typePrefix)
        break

      // ── Subagent lifecycle ──
      case 'claude-subagent':
      case 'reasonix-subagent':
        handleSubagentStart(notif)
        break
      case 'claude-subagent-stop':
      case 'reasonix-subagent-stop':
        handleSubagentStop(notif)
        break
      case 'claude-subagent-done': // legacy
        subagent.value = false
        agentResult.value = ''
        break

      // ── Attention / notification ──
      case 'claude-notify':
      case 'reasonix-notify':
        handleNotify(notif, typePrefix)
        break

      // ── Completion ──
      case 'claude-done':
      case 'reasonix-done':
        handleDone(notif)
        break
      case 'claude-stop':
      case 'reasonix-stop':
        handleStop(notif)
        break

      // ── Reasonix-specific ──
      case 'reasonix-session-start':
        handleSessionStart(notif)
        break
      case 'reasonix-llm':
      case 'reasonix-precompact':
        handleHeartbeat(typePrefix)
        break

      default:
        console.warn(`[notification] unhandled type: ${notif.type}`)
        if (notif.message) {
          source.value = typePrefix
          state.value = 'attention'
          message.value = notif.message
          stopTicker()
          clearTimer = setTimeout(() => reset(), ATTENTION_CLEAR_DELAY)
        }
    }
  }

  /** Ensure we're in running state, starting the elapsed ticker if needed. */
  function ensureRunning(typePrefix: 'claude' | 'reasonix') {
    source.value = typePrefix
    if (state.value !== 'running') {
      state.value = 'running'
      startedAt.value = Date.now()
      elapsed.value = 0
      startTicker()
    }
  }

  /** Reset tracking for a new prompt session. */
  function resetSession(notif: Notification & { topic?: string }, typePrefix: 'claude' | 'reasonix') {
    source.value = typePrefix
    if (notif.topic) topic.value = notif.topic
    if (notif.workDir) workDir.value = notif.workDir
    subagent.value = false
    agentType.value = ''
    agentDesc.value = ''
    toolHistory.value = []
    toolCount.value = 0
    if (state.value !== 'running') {
      startedAt.value = Date.now()
      elapsed.value = 0
    }
    state.value = 'running'
    message.value = ''
    tool.value = ''
    toolInput.value = undefined
    startTicker()
  }

  function handlePromptStart(notif: Notification & { topic?: string }, typePrefix: 'claude' | 'reasonix') {
    resetSession(notif, typePrefix)
  }

  function handlePreTool(notif: Notification, typePrefix: 'claude' | 'reasonix') {
    ensureRunning(typePrefix)
    tool.value = notif.tool || ''
    toolInput.value = notif.toolInput
    isPreTool.value = true
    durationMs.value = 0
  }

  function handlePostTool(notif: Notification, typePrefix: 'claude' | 'reasonix') {
    ensureRunning(typePrefix)
    if (notif.tool) tool.value = notif.tool
    toolInput.value = notif.toolInput
    toolOutput.value = notif.toolOutput
    durationMs.value = notif.durationMs || 0
    isPreTool.value = false
    toolCount.value++

    if (notif.tool) {
      const info = getToolDisplayInfo(notif.tool, notif.toolInput)
      toolHistory.value.push({
        tool: notif.tool,
        target: info.target,
        duration: notif.durationMs || 0,
      })
      // Keep last 20 items
      if (toolHistory.value.length > 20) {
        toolHistory.value.shift()
      }
    }
  }

  function handleSubagentStart(notif: Notification) {
    subagent.value = true
    agentType.value = notif.agentType || ''
    agentDesc.value = notif.agentDesc || ''
  }

  function handleSubagentStop(notif: Notification) {
    agentResult.value = notif.agentResult || ''
    // Keep subagent flag true until next main tool
  }

  function handleNotify(notif: Notification, typePrefix: 'claude' | 'reasonix') {
    source.value = typePrefix
    state.value = 'attention'
    message.value = notif.message || '需要关注'
    stopTicker()
    clearTimer = setTimeout(() => reset(), ATTENTION_CLEAR_DELAY)
  }

  function handleDone(notif: Notification) {
    state.value = 'done'
    finalMsg.value = notif.finalMsg || notif.message || '完成'
    message.value = finalMsg.value
    stopTicker()
    clearTimer = setTimeout(() => reset(), DONE_CLEAR_DELAY)
  }

  function handleStop(notif: Notification) {
    state.value = 'done'
    const baseMsg = notif.finalMsg || notif.agentResult || notif.message || '任务完成'
    const summary = toolSummary.value
    if (summary) {
      finalMsg.value = `${baseMsg} · ${summary}`
      message.value = finalMsg.value
    } else {
      finalMsg.value = baseMsg
      message.value = baseMsg
    }
    stopTicker()
    clearTimer = setTimeout(() => reset(), DONE_CLEAR_DELAY)
  }

  function handleSessionStart(notif: Notification) {
    source.value = 'reasonix'
    if (notif.workDir) workDir.value = notif.workDir
    subagent.value = false
    agentType.value = ''
    agentDesc.value = ''
    toolHistory.value = []
    toolCount.value = 0
    state.value = 'running'
    message.value = ''
    tool.value = ''
    toolInput.value = undefined
    startedAt.value = Date.now()
    elapsed.value = 0
    startTicker()
  }

  function handleHeartbeat(typePrefix: 'claude' | 'reasonix') {
    source.value = typePrefix
    if (state.value !== 'running') {
      state.value = 'running'
      startedAt.value = Date.now()
      elapsed.value = 0
      startTicker()
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
    toolInput.value = undefined
    toolOutput.value = undefined
    durationMs.value = 0
    agentType.value = ''
    agentDesc.value = ''
    agentResult.value = ''
    finalMsg.value = ''
    toolCount.value = 0
    effortLevel.value = ''
    isPreTool.value = false
    toolHistory.value = []
    stopTicker()
  }

  function clear() {
    if (clearTimer) {
      clearTimeout(clearTimer)
      clearTimer = null
    }
    reset()
  }

  /** Approve the current approval prompt (✅) */
  function approve() {
    Events.Emit('approve-response', 'approve')
    reset()
  }

  /** Reject the current approval prompt (❌) */
  function reject() {
    Events.Emit('approve-response', 'reject')
    reset()
  }

  return {
    state,
    source,
    message,
    tool,
    workDir,
    topic,
    subagent,
    elapsedText,
    // Extended fields
    toolInput,
    toolOutput,
    durationMs,
    agentType,
    agentDesc,
    agentResult,
    finalMsg,
    toolCount,
    effortLevel,
    isPreTool,
    // Computed display values
    toolIcon,
    phase,
    isApproval,
    toolTarget,
    toolContext,
    agentTypeName,
    primaryText,
    secondaryText,
    toolSummary,
    // History
    toolHistory,
    // Methods
    handle,
    clear,
    approve,
    reject,
    // Helpers (exported for use in components)
    TOOL_ICONS,
  }
})
