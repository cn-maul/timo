import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface SystemStats {
  cpuPercent: number
  memPercent: number
  memUsedGB: number
  memTotalGB: number
  netDownKBps: number
  netUpKBps: number
  localIP: string
  diskReadKBps: number
  diskWriteKBps: number
}

export const useSystemStore = defineStore('system', () => {
  const cpuPercent = ref(0)
  const memPercent = ref(0)
  const memUsedGB = ref(0)
  const memTotalGB = ref(0)
  const netDownKBps = ref(0)
  const netUpKBps = ref(0)
  const localIP = ref('')
  const diskReadKBps = ref(0)
  const diskWriteKBps = ref(0)

  function update(stats: SystemStats) {
    cpuPercent.value = stats.cpuPercent
    memPercent.value = stats.memPercent
    memUsedGB.value = stats.memUsedGB
    memTotalGB.value = stats.memTotalGB
    netDownKBps.value = stats.netDownKBps
    netUpKBps.value = stats.netUpKBps
    localIP.value = stats.localIP
    diskReadKBps.value = stats.diskReadKBps || 0
    diskWriteKBps.value = stats.diskWriteKBps || 0
  }

  return {
    cpuPercent, memPercent, memUsedGB, memTotalGB,
    netDownKBps, netUpKBps, localIP,
    diskReadKBps, diskWriteKBps,
    update
  }
})
