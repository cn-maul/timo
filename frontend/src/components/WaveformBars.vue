<script setup lang="ts">
defineProps<{
  playing: boolean
}>()
</script>

<template>
  <div class="waveform">
    <div
      v-for="i in 3"
      :key="i"
      class="waveform-bar"
      :class="{ playing }"
    />
  </div>
</template>
