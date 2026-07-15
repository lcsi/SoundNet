<template>
  <div class="volume-control">
    <div class="volume-row">
      <span class="volume-label"><Volume2 :size="20" /></span>
      <input
        type="range"
        min="0"
        max="100"
        :value="volume"
        @input.stop="$emit('update:volume', Number(($event.target as HTMLInputElement).value))"
      />
      <span class="volume-value">{{ volume }}%</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Volume2 } from '@lucide/vue'

defineProps<{
  volume: number
}>()

defineEmits<{
  'update:volume': [value: number]
}>()
</script>

<style scoped>
.volume-control {
  width: 100%;
}

.volume-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.volume-label {
  font-size: 1.2rem;
}

.volume-row input[type="range"] {
  flex: 1;
}

.volume-value {
  min-width: 45px;
  text-align: right;
  font-size: 0.85rem;
  color: var(--text-secondary);
}
</style>
