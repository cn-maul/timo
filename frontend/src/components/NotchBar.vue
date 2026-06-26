<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMediaStore } from '../stores/media'
import { useNotificationStore } from '../stores/notification'
import { useSystemStore } from '../stores/system'
import { Play, Pause } from '../../bindings/timo/mediaservice'
import WaveformBars from './WaveformBars.vue'

const media = useMediaStore()
const notif = useNotificationStore()
const sys = useSystemStore()

const titleText = computed(() => {
  if (!media.hasMedia) return ''
  return media.artist ? `${media.title} — ${media.artist}` : media.title
})

const needsScrolling = computed(() => titleText.value.length > 20)

const progressPercent = computed(() => {
  if (media.durationMs <= 0) return 0
  return Math.min(100, (media.positionMs / media.durationMs) * 100)
})

const remainingText = computed(() => {
  if (media.durationMs <= 0) return ''
  const remain = Math.max(0, media.durationMs - media.positionMs)
  const sec = Math.floor(remain / 1000)
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `-${m}:${s.toString().padStart(2, '0')}`
})

const hasClaudeActivity = computed(() => notif.state !== '')

// Current time for idle display
const now = ref(new Date())
setInterval(() => { now.value = new Date() }, 1000)
const timeText = computed(() => {
  const h = now.value.getHours()
  const m = now.value.getMinutes()
  return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}`
})

const cpuText = computed(() => {
  const v = sys.cpu
  return v < 10 ? `${v.toFixed(1)}%` : `${Math.round(v)}%`
})

const memText = computed(() => {
  return `${sys.memUsedGB.toFixed(1)}G`
})

// Shorten workDir for display
const shortDir = computed(() => {
  if (!notif.workDir) return ''
  const parts = notif.workDir.split('/')
  return parts.length > 3 ? '…/' + parts.slice(-2).join('/') : notif.workDir
})

const emit = defineEmits<{
  toggle: []
}>()

async function togglePlay(e: MouseEvent) {
  e.stopPropagation()
  try {
    media.playing ? await Pause() : await Play()
  } catch (err) {
    console.error('togglePlay failed:', err)
  }
}
</script>

<template>
  <div class="notch-bar" @click="emit('toggle')">
    <!-- Claude status -->
    <template v-if="hasClaudeActivity">
      <div class="notch-content">
        <div class="notch-left">
          <img src="/claude.png" class="claude-logo" alt="Claude" />
          <div class="claude-info">
            <span class="claude-text" v-if="notif.state === 'running'">
              {{ notif.tool || 'Claude 运行中' }}
            </span>
            <span class="claude-text" v-else-if="notif.state === 'attention'">
              {{ notif.message || '需要关注' }}
            </span>
            <span class="claude-text" v-else>
              {{ notif.message || '完成' }}
            </span>
            <span class="claude-dir" v-if="shortDir">{{ shortDir }}</span>
        </div>
        </div>
        <div class="notch-right">
          <span class="claude-timer" v-if="notif.state === 'running'">{{ notif.elapsedText }}</span>
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
        <div class="notch-progress-fill claude-progress" />
      </div>
    </template>

    <!-- Media -->
    <template v-else-if="media.hasMedia">
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
          <span class="notch-play-icon" @click="togglePlay">
            {{ media.playing ? '❚❚' : '▶' }}
          </span>
          <WaveformBars :playing="media.playing" />
        </div>
      </div>
      <div class="notch-progress">
        <div class="notch-progress-fill" :style="{ width: progressPercent + '%' }" />
      </div>
    </template>

    <!-- Idle: CPU + Mem + Clock -->
    <template v-else>
      <div class="notch-content">
        <div class="notch-left">
          <span class="sys-stat">
            <span class="sys-label">CPU</span>
            <span class="sys-value">{{ cpuText }}</span>
          </span>
          <span class="sys-stat">
            <span class="sys-label">MEM</span>
            <span class="sys-value">{{ memText }}</span>
          </span>
        </div>
        <div class="notch-right">
          <span class="idle-clock">{{ timeText }}</span>
        </div>
      </div>
    </template>
  </div>
</template>
