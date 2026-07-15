<template>
  <Teleport to="body">
    <Transition name="modal-fade">
      <div
        v-if="visible"
        class="modal-overlay"
        @click.self="onBackdropClick"
        role="dialog"
        aria-modal="true"
        :aria-label="title || '对话框'"
      >
        <div
          ref="modalContentRef"
          class="modal-content"
          :class="[sizeClass]"
          :style="{ maxWidth: resolvedMaxWidth }"
          @keydown.escape="onClose"
        >
          <!-- Header -->
          <div v-if="title || closable || $slots.header" class="modal-header">
            <slot name="header">
              <h3 v-if="title" class="modal-title">
                <component :is="icon" v-if="icon" :size="20" />
                {{ title }}
              </h3>
              <button
                v-if="closable"
                class="btn-close"
                @click="onClose"
                aria-label="关闭"
              >
                <X :size="20" />
              </button>
            </slot>
          </div>

          <!-- Body -->
          <div class="modal-body">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="showFooter || $slots.footer" class="modal-footer">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { X } from '@lucide/vue'
import type { Component } from 'vue'

const props = withDefaults(defineProps<{
  visible: boolean
  maxWidth?: string
  closable?: boolean
  title?: string
  icon?: Component
  showFooter?: boolean
}>(), {
  maxWidth: '480px',
  closable: true,
  showFooter: false,
})

const emit = defineEmits<{
  close: []
  'update:visible': [value: boolean]
}>()

const modalContentRef = ref<HTMLElement | null>(null)

const sizeClass = computed(() => {
  if (props.maxWidth === 'full' || props.maxWidth === '96vh') return 'modal-fullscreen'
  if (props.maxWidth === 'compact' || props.maxWidth === '360px') return ''
  return ''
})

const resolvedMaxWidth = computed(() => {
  if (props.maxWidth === 'full' || props.maxWidth === '96vh' || props.maxWidth === 'compact') return undefined
  return props.maxWidth
})

function onClose() {
  if (!props.closable) return
  emit('close')
  emit('update:visible', false)
}

function onBackdropClick() {
  onClose()
}

// Focus trap: focus the modal content on open
watch(() => props.visible, (val) => {
  if (val) {
    // Lock body scroll
    document.body.style.overflow = 'hidden'
    document.body.style.overscrollBehavior = 'contain'
    // Focus the modal
    setTimeout(() => {
      modalContentRef.value?.focus()
    }, 100)
  } else {
    document.body.style.overflow = ''
    document.body.style.overscrollBehavior = ''
  }
})

onUnmounted(() => {
  document.body.style.overflow = ''
  document.body.style.overscrollBehavior = ''
})
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-content {
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
  border-radius: var(--radius);
  padding: 0;
  width: 94vw;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
  overscroll-behavior: contain;
  outline: none;
}

.modal-fullscreen {
  height: 96vh;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.modal-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 1rem;
  font-weight: 600;
  margin: 0;
  color: var(--text-primary);
}

.btn-close {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 6px;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 44px;
  min-height: 44px;
  transition: all 0.15s;
}

.btn-close:hover {
  background: var(--bg-primary);
  color: var(--text-primary);
}

.btn-close:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.modal-body {
  padding: 20px;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 16px 20px;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
}

/* Transitions */
.modal-fade-enter-active {
  transition: opacity 0.3s ease;
}
.modal-fade-enter-active .modal-content {
  transition: transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.modal-fade-leave-active {
  transition: opacity 0.2s ease;
}
.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}
.modal-fade-enter-from .modal-content {
  transform: scale(0.95);
}
.modal-fade-leave-to .modal-content {
  transform: scale(0.95);
}
</style>
