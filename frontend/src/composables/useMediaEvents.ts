import { Events } from '@wailsio/runtime'
import { useMediaStore } from '../stores/media'
import type { MediaInfo } from '../types/media'

export function useMediaEvents() {
  const store = useMediaStore()

  Events.On('media-update', (event: { data: MediaInfo | null }) => {
    if (event.data) {
      console.log('[media-update]', event.data.title, 'playing:', event.data.playing)
      store.update(event.data)
    } else {
      store.clear()
    }
  })
}
