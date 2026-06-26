import { Events } from '@wailsio/runtime'
import { useSystemStore } from '../stores/system'
import type { SystemStats } from '../stores/system'

export function useSystemEvents() {
  const store = useSystemStore()

  Events.On('sys-stats', (event: { data: SystemStats | null }) => {
    if (event.data) {
      store.update(event.data)
    }
  })
}
