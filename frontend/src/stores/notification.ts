import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { Notification } from '../../bindings/timo/models'

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
function getToolDisplayInfo(tool: string, toolInput: Record<string, unknown> | undefined): { target: string; context: string } {
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
  const toolInput = ref<Record<string, unknown> | undefined>()
  const toolOutput = ref<Record<string, unknown> | undefined>()
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

    const typePrefix = notif.type.startsWith('reasonix') ? 'reasonix' : 'claude'

    switch (notif.type) {
      case 'claude-prompt':
      case 'reasonix-prompt':
        // User submitted a new prompt — primary "work started" signal
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
        break

      case 'claude-pre-tool':
      case 'reasonix-pre-tool':
        // PreToolUse: tool is about to execute (before execution)
        source.value = typePrefix
        tool.value = notif.tool || ''
        toolInput.value = notif.toolInput
        isPreTool.value = true
        durationMs.value = 0
        // If we're not in running state, start
        if (state.value !== 'running') {
          state.value = 'running'
          startedAt.value = Date.now()
          elapsed.value = 0
          startTicker()
        }
        break

      case 'claude-tool':
      case 'reasonix-tool':
        // PostToolUse: tool completed
        source.value = typePrefix
        if (notif.tool) tool.value = notif.tool
        toolInput.value = notif.toolInput
        toolOutput.value = notif.toolOutput
        durationMs.value = notif.durationMs || 0
        isPreTool.value = false
        // Increment tool count for progress
        toolCount.value++
        // Add to history
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
        // If we're not in running state, start (covers edge cases)
        if (state.value !== 'running') {
          state.value = 'running'
          startedAt.value = Date.now()
          elapsed.value = 0
          startTicker()
        }
        break

      case 'claude-subagent':
      case 'reasonix-subagent':
        subagent.value = true
        agentType.value = notif.agentType || ''
        agentDesc.value = notif.agentDesc || ''
        break

      case 'claude-subagent-stop':
      case 'reasonix-subagent-stop':
        // Subagent completed with result
        agentResult.value = notif.agentResult || ''
        // Keep subagent flag true until next main tool
        break

      case 'claude-subagent-done':
        // Legacy event - subagent finished (old behavior)
        subagent.value = false
        agentResult.value = ''
        break

      case 'claude-notify':
      case 'reasonix-notify':
        // Permission prompt or attention needed
        source.value = typePrefix
        state.value = 'attention'
        message.value = notif.message || '需要关注'
        stopTicker()
        clearTimer = setTimeout(() => reset(), ATTENTION_CLEAR_DELAY)
        break

      case 'claude-done':
      case 'reasonix-done':
        // Work completed (Stop/SessionEnd event or process exit)
        state.value = 'done'
        finalMsg.value = notif.finalMsg || notif.message || '完成'
        message.value = finalMsg.value
        stopTicker()
        clearTimer = setTimeout(() => reset(), DONE_CLEAR_DELAY)
        break

      case 'claude-stop':
      case 'reasonix-stop':
        // Stop event from hook - Claude finished responding
        // Use last_assistant_message as summary if available
        state.value = 'done'
        finalMsg.value = notif.finalMsg || notif.agentResult || notif.message || '任务完成'
        message.value = finalMsg.value
        stopTicker()
        clearTimer = setTimeout(() => reset(), DONE_CLEAR_DELAY)
        break

      case 'reasonix-session-start':
        // Session started - reset state, set source
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
        break

      case 'reasonix-llm':
        // PostLLMCall - keep running heartbeat (no UI display of reasoning per plan)
        source.value = 'reasonix'
        if (state.value !== 'running') {
          state.value = 'running'
          startedAt.value = Date.now()
          elapsed.value = 0
          startTicker()
        }
        break

      case 'reasonix-precompact':
        // PreCompact - keep running heartbeat, no special UI
        source.value = 'reasonix'
        if (state.value !== 'running') {
          state.value = 'running'
          startedAt.value = Date.now()
          elapsed.value = 0
          startTicker()
        }
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
    toolTarget,
    toolContext,
    agentTypeName,
    primaryText,
    secondaryText,
    // History
    toolHistory,
    // Methods
    handle,
    clear,
    // Helpers (exported for use in components)
    TOOL_ICONS,
  }
})
