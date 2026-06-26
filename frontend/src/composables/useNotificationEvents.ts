import { Events } from '@wailsio/runtime'
import { useNotificationStore } from '../stores/notification'
import type { Notification } from '../stores/notification'

export function useNotificationEvents() {
  const store = useNotificationStore()

  Events.On('notification', (event: { data: Notification | null }) => {
    if (event.data) {
      console.log('[notification]', event.data.type, event.data.message)
      store.handle(event.data)
    }
  })
}
