import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { MediaInfo } from '../types/media'
import { Play, Pause } from '../../bindings/timo/mediaservice'

export function formatTime(ms: number): string {
  if (ms <= 0) return '0:00'
  const totalSec = Math.floor(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  if (h > 0) {
    return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
  }
  return `${m}:${s.toString().padStart(2, '0')}`
}

export const useMediaStore = defineStore('media', () => {
  const title = ref('')
  const artist = ref('')
  const album = ref('')
  const coverUrl = ref('')
  const durationMs = ref(0)
  const positionMs = ref(0)
  const playing = ref(false)
  const hasMedia = ref(false)

  const progressPercent = computed(() => {
    if (durationMs.value <= 0) return 0
    return Math.min(100, (positionMs.value / durationMs.value) * 100)
  })

  async function togglePlay() {
    try {
      playing.value ? await Pause() : await Play()
    } catch (err) {
      console.error('togglePlay failed:', err)
    }
  }

  function update(info: MediaInfo) {
    title.value = info.title
    artist.value = info.artist
    album.value = info.album
    coverUrl.value = info.coverUrl
    durationMs.value = info.durationMs
    positionMs.value = info.positionMs
    playing.value = info.playing
    hasMedia.value = info.playing && !!(info.title || info.artist)
  }

  function clear() {
    title.value = ''
    artist.value = ''
    album.value = ''
    coverUrl.value = ''
    durationMs.value = 0
    positionMs.value = 0
    playing.value = false
    hasMedia.value = false
  }

  return {
    title, artist, album, coverUrl,
    durationMs, positionMs, playing, hasMedia,
    progressPercent,
    togglePlay, update, clear,
  }
})
