<template>
  <div class="play-queue">
    <div v-if="queue.length === 0" class="empty">队列为空，去搜索音乐吧</div>

    <div v-else ref="queueListRef" class="queue-list">
      <div
        v-for="(song, index) in queue"
        :key="song.id + '-' + index"
        :class="['queue-item', { active: index === currentIndex }]"
        draggable="true"
        @dragstart="onDragStart(index)"
        @dragover.prevent="onDragOver(index)"
        @drop="onDrop(index)"
        @dragend="onDragEnd"
      >
        <div class="queue-index" @click="$emit('play-index', index)">
          <template v-if="index === currentIndex"><Play :size="12" /></template>
          <template v-else>{{ index + 1 }}</template>
        </div>
        <div class="queue-song" @click="$emit('play-index', index)">
          <div class="queue-title">{{ song.title }}</div>
          <div class="queue-artist">{{ song.artist }}</div>
        </div>
        <div class="queue-duration">{{ formatTime(song.duration) }}</div>
        <button
          class="btn-add-to-channel"
          @click="$emit('add-to-channel', song)"
          title="添加到频道"
        >
          <Star :size="14" />
        </button>
        <button
          class="btn-remove"
          @click="$emit('remove', index)"
          title="移除"
        >
          <X :size="14" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import type { Song } from '../types'
import { Play, X, Star } from '@lucide/vue'

const props = defineProps<{
  queue: Song[]
  currentIndex: number
}>()

const emit = defineEmits<{
  remove: [index: number]
  reorder: [from: number, to: number]
  'play-index': [index: number]
  'add-to-channel': [song: Song]
}>()

const queueListRef = ref<HTMLElement | null>(null)

function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

// Drag and drop
const dragIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)

function onDragStart(index: number) {
  dragIndex.value = index
}

function onDragOver(index: number) {
  dragOverIndex.value = index
}

function onDrop(index: number) {
  if (dragIndex.value !== null && dragIndex.value !== index) {
    emit('reorder', dragIndex.value, index)
  }
  dragIndex.value = null
  dragOverIndex.value = null
}

function onDragEnd() {
  dragIndex.value = null
  dragOverIndex.value = null
}

// Auto-scroll to active item when currentIndex changes
watch(() => props.currentIndex, (index) => {
  if (index < 0) return
  nextTick(() => {
    if (!queueListRef.value) return
    const activeEl = queueListRef.value.querySelector('.queue-item.active') as HTMLElement | null
    if (activeEl) {
      activeEl.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    }
  })
})
</script>

<style scoped>
.play-queue {
  width: 100%;
}

.empty {
  text-align: center;
  color: var(--text-secondary);
  padding: 20px;
  font-size: 0.9rem;
}

.queue-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  overflow-y: auto;
  flex: 1;
}

.queue-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  cursor: grab;
  transition: background 0.2s ease;
  min-height: 48px;
}

.queue-item:hover {
  background: var(--bg-card);
}

.queue-item.active {
  background: rgba(233, 69, 96, 0.15);
  border-left: 3px solid var(--accent);
}

.queue-index {
  width: 28px;
  text-align: center;
  font-size: 0.8rem;
  color: var(--text-secondary);
  cursor: pointer;
  flex-shrink: 0;
}

.queue-item.active .queue-index {
  color: var(--accent);
}

.queue-song {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}

.queue-title {
  font-size: 0.85rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.queue-artist {
  font-size: 0.75rem;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.queue-duration {
  font-size: 0.75rem;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.btn-remove {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 8px;
  font-size: 0.8rem;
  flex-shrink: 0;
  min-width: 44px;
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  transition: all 0.15s ease;
}

.btn-remove:hover {
  color: var(--accent);
  background: rgba(233, 69, 96, 0.1);
}

.btn-add-to-channel {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 8px;
  font-size: 0.8rem;
  flex-shrink: 0;
  min-width: 44px;
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  transition: all 0.15s ease;
}

.btn-add-to-channel:hover {
  color: var(--accent);
  background: rgba(233, 69, 96, 0.1);
}
</style>
