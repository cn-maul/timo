<script setup lang="ts">
import { computed } from 'vue'
import { useMediaStore } from '../stores/media'
import { Play, Pause, Next, Previous } from '../../bindings/timo/mediaservice'

const store = useMediaStore()

function formatTime(ms: number): string {
  if (ms <= 0) return '0:00'
  const totalSec = Math.floor(ms / 1000)
  const min = Math.floor(totalSec / 60)
  const sec = totalSec % 60
  return `${min}:${sec.toString().padStart(2, '0')}`
}

const progressPercent = computed(() => {
  if (store.durationMs <= 0) return 0
  return Math.min(100, (store.positionMs / store.durationMs) * 100)
})

async function togglePlay() {
  try {
    if (store.playing) {
      await Pause()
    } else {
      await Play()
    }
  } catch (e) {
    console.error('togglePlay failed:', e)
  }
}

async function doNext() {
  try { await Next() } catch (e) { console.error('Next failed:', e) }
}

async function doPrev() {
  try { await Previous() } catch (e) { console.error('Previous failed:', e) }
}
</script>

<template>
  <div class="drop-panel">
    <div class="panel-top">
      <img
        v-if="store.coverUrl"
        :src="store.coverUrl"
        class="panel-cover"
        alt="Album cover"
      />
      <div v-else class="panel-cover" />
      <div class="panel-info">
        <div class="panel-title">{{ store.title || 'Unknown' }}</div>
        <div class="panel-artist">{{ store.artist || 'Unknown Artist' }}</div>
      </div>
    </div>

    <div class="progress-container">
      <div class="progress-bar-bg">
        <div
          class="progress-bar-fill"
          :style="{ width: progressPercent + '%' }"
        />
      </div>
      <div class="progress-time">
        <span>{{ formatTime(store.positionMs) }}</span>
        <span>{{ formatTime(store.durationMs) }}</span>
      </div>
    </div>

    <div class="controls">
      <button class="control-btn" @click="doPrev">⏮</button>
      <button class="control-btn play-pause" @click="togglePlay">
        {{ store.playing ? '❚❚' : '▶' }}
      </button>
      <button class="control-btn" @click="doNext">⏭</button>
    </div>
  </div>
</template>
