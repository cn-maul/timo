import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface SystemStats {
  cpuPercent: number
  memPercent: number
  memUsedGB: number
  memTotalGB: number
}

export const useSystemStore = defineStore('system', () => {
  const cpuPercent = ref(0)
  const memPercent = ref(0)
  const memUsedGB = ref(0)
  const memTotalGB = ref(0)

  function update(stats: SystemStats) {
    cpuPercent.value = stats.cpuPercent
    memPercent.value = stats.memPercent
    memUsedGB.value = stats.memUsedGB
    memTotalGB.value = stats.memTotalGB
  }

  return { cpuPercent, memPercent, memUsedGB, memTotalGB, update }
})
