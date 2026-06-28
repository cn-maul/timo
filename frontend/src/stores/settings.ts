import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'

export interface TimoSettings {
  displayPriority: string[]
  idleDisplay: 'all' | 'cpu' | 'mem' | 'none'
  theme: 'dark' | 'light'
}

function defaultSettings(): TimoSettings {
  return {
    displayPriority: ['ai', 'media'],
    idleDisplay: 'all',
    theme: 'dark',
  }
}

export const useSettingsStore = defineStore('settings', () => {
  const displayPriority = ref<string[]>(['ai', 'media'])
  const idleDisplay = ref<'all' | 'cpu' | 'mem' | 'none'>('all')
  const theme = ref<'dark' | 'light'>('dark')
  const loaded = ref(false)

  // Request settings from Go backend
  function load() {
    Events.Emit('get-settings')
  }

  // Save settings to Go backend
  function save() {
    const s: TimoSettings = {
      displayPriority: displayPriority.value,
      idleDisplay: idleDisplay.value,
      theme: theme.value,
    }
    Events.Emit('save-settings', s)
  }

  // Apply theme by setting data-theme on <html>
  function applyTheme(name: string) {
    document.documentElement.setAttribute('data-theme', name)
  }

  // Listen for settings-loaded response from backend
  Events.On('settings-loaded', (event: { data: TimoSettings }) => {
    if (!event.data) return
    displayPriority.value = event.data.displayPriority || defaultSettings().displayPriority
    idleDisplay.value = (event.data.idleDisplay as any) || defaultSettings().idleDisplay
    theme.value = (event.data.theme as any) || defaultSettings().theme
    loaded.value = true
    applyTheme(theme.value)
  })

  // Listen for settings-updated broadcast (from tray or other sources)
  Events.On('settings-updated', (event: { data: TimoSettings }) => {
    if (!event.data) return
    displayPriority.value = event.data.displayPriority || defaultSettings().displayPriority
    idleDisplay.value = (event.data.idleDisplay as any) || defaultSettings().idleDisplay
    theme.value = (event.data.theme as any) || defaultSettings().theme
    applyTheme(theme.value)
  })

  // Auto-apply theme whenever it changes
  watch(theme, (val) => {
    applyTheme(val)
  })

  return {
    displayPriority,
    idleDisplay,
    theme,
    loaded,
    load,
    save,
    applyTheme,
  }
})
