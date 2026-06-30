<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useMediaEvents } from '../composables/useMediaEvents'
import { useNotificationEvents } from '../composables/useNotificationEvents'
import { useSystemEvents } from '../composables/useSystemEvents'
import { useSettingsStore } from '../stores/settings'
import NotchBar from './NotchBar.vue'
import DropPanel from './DropPanel.vue'

const cleanupMedia = useMediaEvents()
const cleanupNotification = useNotificationEvents()
const cleanupSystem = useSystemEvents()
const settings = useSettingsStore()

const expanded = ref(false)
let collapseTimer: ReturnType<typeof setTimeout> | null = null

// Load settings from backend on mount
onMounted(() => {
  settings.load()
})

function toggle() {
  expanded.value = !expanded.value
  if (expanded.value) startCollapseTimer()
}

function startCollapseTimer() {
  clearCollapseTimer()
  collapseTimer = setTimeout(() => { expanded.value = false }, 5000)
}

function clearCollapseTimer() {
  if (collapseTimer) { clearTimeout(collapseTimer); collapseTimer = null }
}

function resetTimer() {
  if (expanded.value) startCollapseTimer()
}

watch(expanded, (val) => { if (!val) clearCollapseTimer() })

onUnmounted(() => {
  clearCollapseTimer()
  cleanupMedia()
  cleanupNotification()
  cleanupSystem()
})
</script>

<template>
  <div
    class="island-container"
    role="region"
    aria-label="Timo 状态面板"
    :aria-expanded="expanded"
  >
    <NotchBar :expanded="expanded" @toggle="toggle" />
    <Transition name="panel">
      <DropPanel v-if="expanded" @mouseenter="resetTimer" />
    </Transition>
    <div aria-live="polite" aria-atomic="true" class="sr-only">
      {{ expanded ? '面板已展开' : '面板已收起' }}
    </div>
  </div>
</template>

<style scoped>
.island-container {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}

.island-container:focus-visible {
  outline: 2px solid var(--timo-green, #22c55e);
  outline-offset: 2px;
  border-radius: 8px;
}
</style>
