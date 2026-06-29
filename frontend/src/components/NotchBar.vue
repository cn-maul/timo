<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from 'vue'
import { useMediaStore, formatTime } from '../stores/media'
import { useNotificationStore } from '../stores/notification'
import { useSystemStore } from '../stores/system'
import { useSettingsStore } from '../stores/settings'
import { useActiveMode } from '../composables/useActiveMode'
import WaveformBars from './WaveformBars.vue'

const media = useMediaStore()
const notif = useNotificationStore()
const sys = useSystemStore()
const settings = useSettingsStore()

// Priority-aware active mode
const activeMode = useActiveMode(
  () => settings.displayPriority,
  () => notif.state,
  () => media.hasMedia,
  () => settings.idleDisplay,
)

const titleText = computed(() => {
  if (!media.hasMedia) return ''
  return media.artist ? `${media.title} — ${media.artist}` : media.title
})

const needsScrolling = computed(() => titleText.value.length > 20)

const remainingText = computed(() => {
  if (media.durationMs <= 0) return ''
  const remain = Math.max(0, media.durationMs - media.positionMs)
  return `-${formatTime(remain)}`
})

// Current time for idle display
const now = ref(new Date())
const timer = setInterval(() => { now.value = new Date() }, 1000)
onBeforeUnmount(() => clearInterval(timer))
const timeText = computed(() => {
  const h = now.value.getHours()
  const m = now.value.getMinutes()
  return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}`
})

const cpuText = computed(() => {
  const v = sys.cpuPercent
  return v < 10 ? `${v.toFixed(1)}%` : `${Math.round(v)}%`
})

const memText = computed(() => {
  return `${sys.memUsedGB.toFixed(1)}G`
})

const netDownText = computed(() => {
  const v = sys.netDownKBps
  if (v < 1024) return `${Math.round(v)}K`
  return `${(v / 1024).toFixed(1)}M`
})

const netUpText = computed(() => {
  const v = sys.netUpKBps
  if (v < 1024) return `${Math.round(v)}K`
  return `${(v / 1024).toFixed(1)}M`
})

const diskReadText = computed(() => {
  const v = sys.diskReadKBps
  if (v < 1024) return `${Math.round(v)}K`
  return `${(v / 1024).toFixed(1)}M`
})

const diskWriteText = computed(() => {
  const v = sys.diskWriteKBps
  if (v < 1024) return `${Math.round(v)}K`
  return `${(v / 1024).toFixed(1)}M`
})

const gpuText = computed(() => {
  if (sys.gpuPercent === 0 && sys.gpuTemp === 0) return ''
  if (sys.gpuTemp > 0) return `${Math.round(sys.gpuPercent)}% ${Math.round(sys.gpuTemp)}°C`
  return `${Math.round(sys.gpuPercent)}%`
})

// Shorten workDir for display
const shortDir = computed(() => {
  if (!notif.workDir) return ''
  const parts = notif.workDir.split('/')
  return parts.length > 3 ? '…/' + parts.slice(-2).join('/') : notif.workDir
})

// AI mode display texts using new computed values from store
const aiIcon = computed(() => {
  if (notif.subagent && notif.agentType && settings.showSubagentDetails) return '🤖'
  return notif.toolIcon
})

const aiPrimaryText = computed(() => {
  if (!settings.showToolContext) {
    // Fallback to simpler display
    return notif.topic || notif.tool || (notif.source === 'reasonix' ? 'Reasonix 运行中' : 'Claude 运行中')
  }
  return notif.primaryText
})

const aiSecondaryText = computed(() => {
  if (!settings.showToolContext) return ''
  return notif.secondaryText
})

const showToolLine = computed(() =>
  notif.state === 'running' && settings.showToolContext && notif.primaryText && notif.secondaryText
)

// Progress percentage based on tool count (estimate ~20 tools per task)
const progressPercent = computed(() => {
  if (notif.state !== 'running') return 0
  if (!settings.showToolProgress) return 50 // Fixed progress when disabled
  if (notif.toolCount === 0) return 5 // Show minimal progress when starting
  return Math.min(95, 5 + (notif.toolCount * 4.5)) // Cap at 95% until done
})

// Subagent badge text
const subagentBadgeText = computed(() => {
  if (!notif.subagent) return ''
  if (!settings.showSubagentDetails) return '⚡'
  if (notif.agentType) return notif.agentTypeName
  return '⚡'
})

// Show tool count indicator
const showToolCount = computed(() => settings.showToolProgress && notif.state === 'running' && notif.toolCount > 0)

const emit = defineEmits<{
  toggle: []
}>()
</script>

<template>
  <div class="notch-bar" @click="emit('toggle')">
    <!-- AI mode -->
    <template v-if="activeMode === 'ai'">
      <div class="notch-content">
        <div class="notch-left">
          <img :src="notif.source === 'reasonix' ? '/reasonix.png' : '/claude.png'" class="claude-logo" :alt="notif.source === 'reasonix' ? 'Reasonix' : 'Claude'" />
          <div class="claude-info">
            <!-- Primary text with icon -->
            <span class="claude-text" v-if="notif.state === 'running'">
              <span class="tool-icon">{{ aiIcon }}</span>
              {{ aiPrimaryText }}
              <span v-if="subagentBadgeText" class="subagent-badge">{{ subagentBadgeText }}</span>
            </span>
            <!-- Secondary text (context/target) -->
            <span class="claude-text claude-tool" v-if="showToolLine">
              {{ aiSecondaryText }}
            </span>
            <!-- Attention state -->
            <span class="claude-text" v-else-if="notif.state === 'attention'">
              ⚠️ {{ notif.message || '需要关注' }}
            </span>
            <!-- Done state -->
            <span class="claude-text" v-else-if="notif.state === 'done'">
              ✓ {{ notif.finalMsg || notif.message || '完成' }}
            </span>
            <!-- Idle/fallback -->
            <span class="claude-text" v-else>
              {{ notif.message || '就绪' }}
            </span>
            <span class="claude-dir" v-if="shortDir && notif.state === 'running'">{{ shortDir }}</span>
          </div>
        </div>
        <div class="notch-right">
          <span class="claude-timer" v-if="notif.state === 'running'">{{ notif.elapsedText }}</span>
          <span class="tool-count" v-if="showToolCount">{{ notif.toolCount }}</span>
          <span
            class="traffic-light"
            :class="{
              'light-green': notif.state === 'running',
              'light-yellow': notif.state === 'attention',
              'light-red': notif.state === 'done',
            }"
          />
        </div>
      </div>
      <div class="notch-progress" v-if="notif.state === 'running'">
        <div class="notch-progress-fill claude-progress" :style="{ width: progressPercent + '%' }" />
      </div>
    </template>

    <!-- Media mode -->
    <template v-else-if="activeMode === 'media'">
      <div class="notch-content">
        <div class="notch-left">
          <img v-if="media.coverUrl" :src="media.coverUrl" class="notch-cover" alt="" />
          <div class="notch-title-wrap">
            <span class="notch-title" :class="{ scrolling: needsScrolling }">
              {{ titleText }}
              <template v-if="needsScrolling">&nbsp;&nbsp;&nbsp;{{ titleText }}</template>
            </span>
          </div>
        </div>
        <div class="notch-right">
          <span class="media-remaining" v-if="remainingText">{{ remainingText }}</span>
          <span class="notch-play-icon" @click.stop="media.togglePlay">
            {{ media.playing ? '❚❚' : '▶' }}
          </span>
          <WaveformBars :playing="media.playing" />
        </div>
      </div>
      <div class="notch-progress">
        <div class="notch-progress-fill" :style="{ width: media.progressPercent + '%' }" />
      </div>
    </template>

    <!-- Idle mode: conditional CPU / Mem / Net / Disk / GPU / Clock based on idleDisplay setting -->
    <template v-else-if="activeMode === 'idle'">
      <div class="notch-content">
        <div class="notch-left">
          <span class="sys-stat" v-if="settings.idleDisplay === 'all' || settings.idleDisplay === 'cpu'">
            <span class="sys-label">CPU</span>
            <span class="sys-value">{{ cpuText }}</span>
          </span>
          <span class="sys-stat" v-if="settings.idleDisplay === 'all' || settings.idleDisplay === 'mem'">
            <span class="sys-label">MEM</span>
            <span class="sys-value">{{ memText }}</span>
          </span>
          <span class="sys-stat" v-if="settings.idleDisplay === 'all' || settings.idleDisplay === 'net'">
            <span class="sys-label">↓</span>
            <span class="sys-value">{{ netDownText }}</span>
            <span class="sys-label">↑</span>
            <span class="sys-value">{{ netUpText }}</span>
          </span>
          <span class="sys-stat" v-if="settings.idleDisplay === 'all' && (sys.diskReadKBps > 100 || sys.diskWriteKBps > 100)">
            <span class="sys-label">DISK</span>
            <span class="sys-value">{{ diskReadText }}/{{ diskWriteText }}</span>
          </span>
          <span class="sys-stat" v-if="settings.idleDisplay === 'all' && gpuText">
            <span class="sys-label">GPU</span>
            <span class="sys-value">{{ gpuText }}</span>
          </span>
        </div>
        <div class="notch-right">
          <span class="idle-clock">{{ timeText }}</span>
        </div>
      </div>
    </template>

    <!-- none mode: show nothing (v-else covers activeMode === 'none') -->
  </div>
</template>
