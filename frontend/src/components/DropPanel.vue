<script setup lang="ts">
import { useMediaStore, formatTime } from '../stores/media'
import { Next, Previous } from '../../bindings/timo/mediaservice'

const store = useMediaStore()

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
          :style="{ width: store.progressPercent + '%' }"
        />
      </div>
      <div class="progress-time">
        <span>{{ formatTime(store.positionMs) }}</span>
        <span>{{ formatTime(store.durationMs) }}</span>
      </div>
    </div>

    <div class="controls">
      <button class="control-btn" aria-label="Previous" @click="doPrev">⏮</button>
      <button class="control-btn play-pause" aria-label="Play/Pause" @click="store.togglePlay">
        {{ store.playing ? '❚❚' : '▶' }}
      </button>
      <button class="control-btn" aria-label="Next" @click="doNext">⏭</button>
    </div>
  </div>
</template>
