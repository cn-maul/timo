<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from 'vue'
import { useMediaStore, formatTime } from '../stores/media'
import { useNotificationStore } from '../stores/notification'
import { useSystemStore } from '../stores/system'
import WaveformBars from './WaveformBars.vue'

const media = useMediaStore()
const notif = useNotificationStore()
const sys = useSystemStore()

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

const hasAIActivity = computed(() => notif.state !== '')

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

// Shorten workDir for display
const shortDir = computed(() => {
  if (!notif.workDir) return ''
  const parts = notif.workDir.split('/')
  return parts.length > 3 ? '…/' + parts.slice(-2).join('/') : notif.workDir
})

const emit = defineEmits<{
  toggle: []
}>()
</script>

<template>
  <div class="notch-bar" @click="emit('toggle')">
    <!-- Claude status -->
    <template v-if="hasAIActivity">
      <div class="notch-content">
        <div class="notch-left">
          <img :src="notif.source === 'reasonix' ? '/reasonix.png' : '/claude.png'" class="claude-logo" :alt="notif.source === 'reasonix' ? 'Reasonix' : 'Claude'" />
          <div class="claude-info">
            <span class="claude-text" v-if="notif.state === 'running'">
              {{ notif.topic || notif.tool || (notif.source === 'reasonix' ? 'Reasonix 运行中' : 'Claude 运行中') }}
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
