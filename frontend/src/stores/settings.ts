import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'

export interface TimoSettings {
  displayPriority: string[]
  idleDisplay: 'all' | 'cpu' | 'mem' | 'net' | 'none'
  theme: 'dark' | 'light' | 'frosted'
  showToolContext: boolean
  showToolProgress: boolean
  showSubagentDetails: boolean
}

function defaultSettings(): TimoSettings {
  return {
    displayPriority: ['ai', 'media'],
    idleDisplay: 'all',
    theme: 'dark',
    showToolContext: true,
    showToolProgress: true,
    showSubagentDetails: true,
  }
}

export const useSettingsStore = defineStore('settings', () => {
  const displayPriority = ref<string[]>(['ai', 'media'])
  const idleDisplay = ref<'all' | 'cpu' | 'mem' | 'net' | 'none'>('all')
  const theme = ref<'dark' | 'light' | 'frosted'>('dark')
  const loaded = ref(false)

  // New display options
  const showToolContext = ref(true)
  const showToolProgress = ref(true)
  const showSubagentDetails = ref(true)

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
      showToolContext: showToolContext.value,
      showToolProgress: showToolProgress.value,
      showSubagentDetails: showSubagentDetails.value,
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
    // New fields with defaults
    showToolContext.value = event.data.showToolContext !== undefined ? event.data.showToolContext : true
    showToolProgress.value = event.data.showToolProgress !== undefined ? event.data.showToolProgress : true
    showSubagentDetails.value = event.data.showSubagentDetails !== undefined ? event.data.showSubagentDetails : true
    loaded.value = true
    applyTheme(theme.value)
  })

  // Listen for settings-updated broadcast (from tray or other sources)
  Events.On('settings-updated', (event: { data: TimoSettings }) => {
    if (!event.data) return
    displayPriority.value = event.data.displayPriority || defaultSettings().displayPriority
    idleDisplay.value = (event.data.idleDisplay as any) || defaultSettings().idleDisplay
    theme.value = (event.data.theme as any) || defaultSettings().theme
    showToolContext.value = event.data.showToolContext !== undefined ? event.data.showToolContext : true
    showToolProgress.value = event.data.showToolProgress !== undefined ? event.data.showToolProgress : true
    showSubagentDetails.value = event.data.showSubagentDetails !== undefined ? event.data.showSubagentDetails : true
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
    showToolContext,
    showToolProgress,
    showSubagentDetails,
    load,
    save,
    applyTheme,
  }
})
