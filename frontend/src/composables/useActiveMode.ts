import { computed, type ComputedRef } from 'vue'

export type ActiveMode = 'ai' | 'media' | 'idle' | 'none'

/**
 * useActiveMode determines which display mode is currently active based on
 * the user-configured priority order and the current state of each mode.
 *
 * Accepts getter functions (or arrow functions that close over reactive state)
 * so it works with Pinia stores, refs, and computed properties alike.
 *
 * @param getDisplayPriority - Getter for the ordered priority list
 * @param getNotifState - Getter for current notification state
 * @param getHasMedia - Getter for media-playing flag
 * @param getIdleDisplay - Getter for idle display mode
 * @returns The currently active mode
 */
export function useActiveMode(
  getDisplayPriority: () => string[],
  getNotifState: () => string,
  getHasMedia: () => boolean,
  getIdleDisplay: () => string,
): ComputedRef<ActiveMode> {
  return computed(() => {
    for (const mode of getDisplayPriority()) {
      switch (mode) {
        case 'ai':
          if (getNotifState() !== '') return 'ai'
          break
        case 'media':
          if (getHasMedia()) return 'media'
          break
      }
    }
    // No activity — fallback to idle or none
    return getIdleDisplay() === 'none' ? 'none' : 'idle'
  })
}
