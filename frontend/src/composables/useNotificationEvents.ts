import { Events } from '@wailsio/runtime'
import { useNotificationStore } from '../stores/notification'
import type { Notification } from '../../bindings/timo/internal/app/models'

export function useNotificationEvents() {
  const store = useNotificationStore()

  const handler = (event: { data: Notification | null }) => {
    if (!event.data) {
      console.warn('[notification] received null data')
      return
    }
    if (import.meta.env.DEV) {
      console.log('[notification]', event.data.type, event.data.message)
    }
    store.handle(event.data)
  }

  const off = Events.On('notification', handler)
  return off
}
