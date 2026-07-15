<template>
  <div class="player-controls">
    <div class="controls-row">
      <button class="btn btn-sm" @click="$emit('prev')" :disabled="!hasQueue" title="上一曲">
        <SkipBack :size="18" />
      </button>
      <button
        class="btn btn-primary btn-play"
        @click="togglePlay"
        :disabled="!hasQueue"
        :title="playing ? '暂停' : '播放'"
      >
        <component :is="playing ? Pause : Play" :size="24" />
      </button>
      <button class="btn btn-sm" @click="$emit('next')" :disabled="!hasQueue" title="下一曲">
        <SkipForward :size="18" />
      </button>
    </div>
    <div class="playback-mode-row">
      <button
        v-for="mode in modeList"
        :key="mode.value"
        class="btn btn-sm mode-btn"
        :class="{ active: currentMode === mode.value }"
        @click="$emit('set-playback-mode', mode.value)"
        :title="mode.label"
      >
        <component :is="mode.icon" :size="16" />
        <span class="mode-label">{{ mode.label }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { PlaybackModes, PlaybackModeLabels } from '../types'
import type { PlaybackMode } from '../types'
import { SkipBack, Play, Pause, SkipForward, Repeat, ListOrdered, Shuffle } from '@lucide/vue'

const props = defineProps<{
  playing: boolean
  hasQueue: boolean
  currentMode: PlaybackMode
}>()

const emit = defineEmits<{
  play: []
  pause: []
  resume: []
  next: []
  prev: []
  'set-playback-mode': [mode: PlaybackMode]
}>()

const modeList = [
  { value: PlaybackModes.SEQUENTIAL, label: PlaybackModeLabels[PlaybackModes.SEQUENTIAL], icon: ListOrdered },
  { value: PlaybackModes.LOOP, label: PlaybackModeLabels[PlaybackModes.LOOP], icon: Repeat },
  { value: PlaybackModes.SHUFFLE, label: PlaybackModeLabels[PlaybackModes.SHUFFLE], icon: Shuffle },
]

function togglePlay() {
  if (props.playing) {
    emit('pause')
  } else {
    emit('resume')
  }
}
</script>

<style scoped>
.player-controls {
  width: 100%;
}

.controls-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-bottom: 8px;
}

.btn-play {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  font-size: 1.3rem;
  padding: 0;
}

.playback-mode-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.mode-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  font-size: 0.8rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--bg-primary);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.2s;
}

.mode-btn:hover {
  background: var(--bg-card);
  color: var(--text-primary);
}

.mode-btn.active {
  background: var(--accent);
  color: white;
  border-color: var(--accent);
}

.mode-label {
  font-size: 0.75rem;
}
</style>
