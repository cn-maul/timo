<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { Events } from '@wailsio/runtime'
import Island from './components/Island.vue'
import SettingsPanel from './components/SettingsPanel.vue'
import { useSettingsStore } from './stores/settings'

// Detect if this window is the standalone settings view
const isSettingsView = computed(() => {
  return typeof window !== 'undefined' && window.location.search.includes('settings=1')
})

const settings = useSettingsStore()

onMounted(() => {
  // If this is the main notch window, ensure theme is loaded
  if (!isSettingsView.value) {
    settings.load()
  }
})

// Listen for settings-updated in BOTH windows and apply theme locally.
// Each Wails window has its own DOM, so each needs to set data-theme independently.
Events.On('settings-updated', (event: { data: { theme?: string } }) => {
  if (event.data && event.data.theme) {
    document.documentElement.setAttribute('data-theme', event.data.theme)
  }
})
</script>

<template>
  <SettingsPanel v-if="isSettingsView" standalone />
  <Island v-else />
</template>
