<script setup lang="ts">
import { ref, watch } from 'vue'
import { useMediaEvents } from '../composables/useMediaEvents'
import { useNotificationEvents } from '../composables/useNotificationEvents'
import { useSystemEvents } from '../composables/useSystemEvents'
import NotchBar from './NotchBar.vue'
import DropPanel from './DropPanel.vue'

useMediaEvents()
useNotificationEvents()
useSystemEvents()

const expanded = ref(false)
let collapseTimer: ReturnType<typeof setTimeout> | null = null

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
</script>

<template>
  <div class="island-container">
    <NotchBar @toggle="toggle" />
    <Transition name="panel">
      <DropPanel v-if="expanded" @mouseenter="resetTimer" />
    </Transition>
  </div>
</template>

<style scoped>
.island-container {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
}
</style>
