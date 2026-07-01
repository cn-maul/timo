<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from 'vue'
import { useMediaStore, formatTime } from '../stores/media'
import { useNotificationStore } from '../stores/notification'
import { useSystemStore } from '../stores/system'
import { useSettingsStore } from '../stores/settings'
import { useActiveMode } from '../composables/useActiveMode'
import { Next } from '../../bindings/timo/internal/app/mediaservice'
import WaveformBars from './WaveformBars.vue'

const media = useMediaStore()
const notif = useNotificationStore()
const sys = useSystemStore()
const settings = useSettingsStore()

async function doNext() {
  try { await Next() } catch (e) { console.error('Next failed:', e) }
}

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
  // Some players (Deepin Music) don't report position.
  // Show total duration instead of misleading remaining time.
  if (media.positionMs <= 0 && media.playing) return formatTime(media.durationMs)
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
  if (settings.netUnit === 'kb') return `${Math.round(v)}K`
  if (settings.netUnit === 'mb') return `${(v / 1024).toFixed(1)}M`
  // auto
  if (v < 1024) return `${Math.round(v)}K`
  return `${(v / 1024).toFixed(1)}M`
})

const netUpText = computed(() => {
  const v = sys.netUpKBps
  if (settings.netUnit === 'kb') return `${Math.round(v)}K`
  if (settings.netUnit === 'mb') return `${(v / 1024).toFixed(1)}M`
  // auto
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

const props = defineProps<{
  expanded?: boolean
}>()
</script>

<template>
  <div
    class="notch-bar"
    tabindex="0"
    role="button"
    aria-label="展开控制面板"
    :aria-expanded="expanded"
    @click="emit('toggle')"
    @keydown.enter="emit('toggle')"
    @keydown.space.prevent="emit('toggle')"
  >
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
            <!-- Attention state: show approval question + buttons -->
            <span class="claude-text" v-if="notif.state === 'attention'">
              ❓ {{ notif.message || '需要关注' }}
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
          <!-- Approval buttons: show instead of traffic light when attention -->
          <template v-if="notif.state === 'attention'">
            <button class="approve-btn approve-yes" aria-label="同意" @click.stop="notif.approve()">✓</button>
            <button class="approve-btn approve-no" aria-label="拒绝" @click.stop="notif.reject()">✗</button>
          </template>
          <span
            v-else
            class="traffic-light"
            :class="{
              'light-green': notif.state === 'running',
              'light-yellow': notif.state === 'done',
            }"
          />
        </div>
      </div>
      <div
        class="notch-progress"
        role="progressbar"
        aria-label="AI 工作进度"
        :aria-valuenow="progressPercent"
        aria-valuemin="0"
        aria-valuemax="100"
      >
        <div class="notch-progress-fill claude-progress" :style="{ width: progressPercent + '%' }" />
      </div>
    </template>
    <template v-else-if="activeMode === 'media'">
      <div class="notch-content">
        <div class="notch-left">
          <img src="/minimax-color.png" class="notch-media-logo" alt="" />
          <img v-if="media.safeCoverUrl" :src="media.safeCoverUrl" class="notch-cover" alt="" />
          <div class="notch-title-wrap">
            <span class="notch-title" :class="{ scrolling: needsScrolling }">
              {{ titleText }}
              <template v-if="needsScrolling">&nbsp;&nbsp;&nbsp;{{ titleText }}</template>
            </span>
          </div>
        </div>
        <div class="notch-right">
          <span class="media-remaining" v-if="remainingText">{{ remainingText }}</span>
          <span
            class="notch-play-icon"
            tabindex="0"
            role="button"
            aria-label="切换播放/暂停"
            @click.stop="media.togglePlay"
            @keydown.enter="media.togglePlay"
            @keydown.space.prevent="media.togglePlay"
          >
            <span class="icon-fix" :class="{ 'icon-play': !media.playing }">{{ media.playing ? '❚❚' : '▶' }}</span>
          </span>
          <span
            class="notch-play-icon"
            tabindex="0"
            role="button"
            aria-label="下一首"
            @click.stop="doNext"
            @keydown.enter="doNext"
            @keydown.space.prevent="doNext"
          >
            <span class="icon-fix">⏭</span>
          </span>
          <WaveformBars :playing="media.playing" />
        </div>
      </div>
      <div
        class="notch-progress"
        role="progressbar"
        :aria-valuenow="media.progressPercent"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-label="`媒体播放进度 ${Math.round(media.progressPercent)}%`"
      >
        <div class="notch-progress-fill" :style="{ width: media.progressPercent + '%' }" />
      </div>
    </template>

    <!-- Idle mode: conditional CPU / Mem / Net / Disk / Clock based on idleDisplay setting -->
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
        </div>
        <div class="notch-right">
          <span class="idle-clock">{{ timeText }}</span>
        </div>
      </div>
    </template>

    <!-- none mode: show nothing (v-else covers activeMode === 'none') -->
  </div>
  <div aria-live="polite" aria-atomic="true" class="sr-only">
    {{ notif.state === 'running' ? `AI 运行中，已调用 ${notif.toolCount} 个工具` : notif.state === 'attention' ? 'AI 需要关注' : notif.state === 'done' ? 'AI 任务完成' : '' }}
  </div>
</template>

<style scoped>
/* ── NotchBar - Spotify-inspired Pill Geometry ── */
.notch-bar:focus-visible {
  outline: 2px solid var(--timo-green);
  outline-offset: 2px;
  border-radius: 8px;
}

.notch-play-icon:focus-visible {
  outline: 2px solid var(--timo-green);
  outline-offset: 2px;
  border-radius: 4px;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}

/* ── Approval Buttons - Spotify Circle Geometry (50% radius) ── */
.approve-btn {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  cursor: pointer;
  font-size: 16px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: transform 0.1s, background 0.15s, box-shadow 0.15s;
}

.approve-btn:active {
  transform: scale(0.9);
}

.approve-btn:focus-visible {
  outline: 2px solid var(--timo-green);
  outline-offset: 2px;
}

.approve-yes {
  background: var(--timo-approve-yes-bg);
  color: var(--timo-green);
}

.approve-yes:hover {
  background: var(--timo-approve-yes-hover);
  box-shadow: 0 0 0 2px var(--timo-green);
}

.approve-no {
  background: var(--timo-approve-no-bg);
  color: var(--timo-red);
}

.approve-no:hover {
  background: var(--timo-approve-no-hover);
  box-shadow: 0 0 0 2px var(--timo-red);
}

/* ── Tool Count Badge - Spotify Pill Style ── */
.tool-count {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 10px;
  border-radius: 9999px;
}

/* ── Subagent Badge - Spotify Pill Style ── */
.subagent-badge {
  border-radius: 9999px;
  font-weight: 600;
}

/* ── Traffic Light - Spotify Circle ── */
.traffic-light {
  width: 20px;
  height: 20px;
  border-radius: 50%;
}

/* ── AI Status Dot - Spotify Circle with Glow ── */
.ai-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.ai-status-dot.running {
  background: var(--timo-green);
  box-shadow: 0 0 0 2px rgba(30, 215, 96, 0.3);
}

.ai-status-dot.attention {
  background: var(--timo-yellow);
  box-shadow: 0 0 0 2px rgba(255, 164, 43, 0.3);
}

.ai-status-dot.done {
  background: var(--timo-red);
}
</style>
