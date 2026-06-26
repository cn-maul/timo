import { Events } from '@wailsio/runtime'
import { useMediaStore } from '../stores/media'
import type { MediaInfo } from '../types/media'

export function useMediaEvents() {
  const store = useMediaStore()

  const handler = (event: { data: MediaInfo | null }) => {
    if (!event.data) return
    if (import.meta.env.DEV) {
      console.log('[media-update]', event.data.title, 'playing:', event.data.playing)
    }
    store.update(event.data)
  }

  const off = Events.On('media-update', handler)
  return off
}
