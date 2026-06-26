import { Events } from '@wailsio/runtime'
import { useSystemStore } from '../stores/system'
import type { SystemStats } from '../stores/system'

export function useSystemEvents() {
  const store = useSystemStore()

  const handler = (event: { data: SystemStats | null }) => {
    if (!event.data) {
      console.warn('[sys-stats] received null data')
      return
    }
    store.update(event.data)
  }

  Events.On('sys-stats', handler)
  return () => Events.Off('sys-stats')
}
