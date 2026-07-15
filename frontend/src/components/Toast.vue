<template>
  <Teleport to="body">
    <Transition name="toast-slide">
      <div v-if="visible" class="toast" :class="[type]">
        <component :is="iconComponent" :size="16" />
        <span>{{ message }}</span>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { CheckCircle, AlertCircle, Info } from '@lucide/vue'
import type { Component } from 'vue'

const props = withDefaults(defineProps<{
  message: string
  type?: 'success' | 'error' | 'info'
  duration?: number
}>(), {
  type: 'success',
  duration: 2000,
})

const visible = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

const iconComponent = computed<Component>(() => {
  switch (props.type) {
    case 'success': return CheckCircle
    case 'error': return AlertCircle
    default: return Info
  }
})

function show() {
  if (timer) clearTimeout(timer)
  visible.value = true
  timer = setTimeout(() => {
    visible.value = false
  }, props.duration)
}

watch(() => props.message, (val) => {
  if (val) show()
})

defineExpose({ show })
</script>

<style scoped>
.toast {
  position: fixed;
  top: 20px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px;
  border-radius: var(--radius);
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 0.85rem;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
  z-index: 2000;
  pointer-events: none;
  white-space: nowrap;
}

.toast.success {
  border-left: 3px solid #22c55e;
}

.toast.success svg {
  color: #22c55e;
}

.toast.error {
  border-left: 3px solid var(--accent);
}

.toast.error svg {
  color: var(--accent);
}

.toast.info {
  border-left: 3px solid #3b82f6;
}

.toast.info svg {
  color: #3b82f6;
}

.toast-slide-enter-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.toast-slide-leave-active {
  transition: all 0.2s ease;
}
.toast-slide-enter-from,
.toast-slide-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(-12px);
}
</style>
