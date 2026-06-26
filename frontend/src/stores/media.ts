import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { MediaInfo } from '../types/media'

export const useMediaStore = defineStore('media', () => {
  const title = ref('')
  const artist = ref('')
  const album = ref('')
  const coverUrl = ref('')
  const durationMs = ref(0)
  const positionMs = ref(0)
  const playing = ref(false)
  const hasMedia = ref(false)

  function update(info: MediaInfo) {
    title.value = info.title
    artist.value = info.artist
    album.value = info.album
    coverUrl.value = info.coverUrl
    durationMs.value = info.durationMs
    positionMs.value = info.positionMs
    playing.value = info.playing
    hasMedia.value = !!info.title
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

  return { title, artist, album, coverUrl, durationMs, positionMs, playing, hasMedia, update, clear }
})
