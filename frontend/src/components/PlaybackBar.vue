<template>
  <div class="playback-bar-root">
    <!-- Progress line -->
    <div
      class="playback-progress-line"
      :class="{ visible: !!activeSong, dragging: isSeeking }"
      ref="progressLineRef"
      @mousedown="onSeekStart"
      @touchstart.prevent="onSeekStart"
    >
      <div class="playback-progress-fill" :style="{ width: seekPreviewProgress + '%' }"></div>
      <div class="playback-progress-handle" :style="{ left: seekPreviewProgress + '%' }"></div>
      <div class="playback-progress-tooltip" :style="{ left: seekPreviewProgress + '%' }">{{ seekPreviewTime }}</div>
    </div>

    <!-- Fixed bottom bar -->
    <div class="card fixed-playback-bar">
      <div class="fixed-playback-inner">
        <!-- Left: current song info -->
        <div class="playback-song-info" :class="{ empty: !activeSong }">
          <template v-if="activeSong">
            <img
              v-if="activeSong.cover"
              :src="activeSong.cover"
              class="playback-cover clickable-cover"
              @click="emit('showLyrics')"
            />
            <div class="playback-song-text" @click="emit('showLyrics')">
              <span class="playback-title">{{ activeSong.title || '未知歌曲' }}</span>
              <span class="playback-artist">{{ activeSong.artist || '' }}</span>
            </div>
          </template>
          <span v-else class="playback-no-song">等待播放...</span>
        </div>

        <!-- Center: transport controls -->
        <div class="playback-transport">
          <button
            class="btn btn-sm transport-btn"
            @click="emit('prev')"
            :disabled="!hasQueue"
            title="上一曲"
          ><SkipBack :size="18" /></button>
          <button
            class="btn btn-primary play-btn"
            @click="emit('togglePlay')"
            :disabled="!hasQueue"
            :title="isPlaying ? '暂停' : '播放'"
          ><component :is="isPlaying ? Pause : Play" :size="24" /></button>
          <button
            class="btn btn-sm transport-btn"
            @click="emit('next')"
            :disabled="!hasQueue"
            title="下一曲"
          ><SkipForward :size="18" /></button>
        </div>

        <!-- Right: mode + volume -->
        <div class="playback-extras">
          <!-- Playback mode -->
          <div class="mode-btn-wrap" ref="modeWrapRef">
            <button
              class="btn btn-sm mode-btn"
              @click="showModePopover = !showModePopover; if (showModePopover) restartPopoverTimer('mode'); else clearPopoverTimer('mode')"
              :title="currentModeLabel"
            ><component :is="currentModeIcon" :size="18" /></button>
            <div
              v-if="showModePopover"
              class="mode-popover"
              @mouseenter="restartPopoverTimer('mode')"
              @mouseleave="restartPopoverTimer('mode')"
            >
              <button
                v-for="mode in modeOptions"
                :key="mode.value"
                class="mode-option"
                :class="{ active: playbackMode === mode.value }"
                @click="showModePopover = false; clearPopoverTimer('mode'); emit('update:playbackMode', mode.value)"
              >
                <component :is="mode.icon" :size="16" />
                <span>{{ mode.label }}</span>
              </button>
            </div>
          </div>

          <!-- Volume button with popover -->
          <div class="volume-btn-wrap" ref="volumeWrapRef">
            <button
              class="btn btn-sm"
              @click="showVolumePopover = !showVolumePopover; if (showVolumePopover) restartPopoverTimer('volume'); else clearPopoverTimer('volume')"
              title="音量"
            ><Volume2 :size="18" /></button>
            <div
              v-if="showVolumePopover"
              class="volume-popover"
              @wheel.stop
              @mouseenter.stop="restartPopoverTimer('volume')"
              @mouseleave.stop="restartPopoverTimer('volume')"
            >
              <input
                type="range"
                min="0"
                max="99"
                :value="masterVolume"
                @input.stop="restartPopoverTimer('volume'); emit('update:masterVolume', Number(($event.target as HTMLInputElement).value))"
                @mousedown.stop
                @touchstart.stop
              />
              <span class="volume-popover-value">{{ masterVolume }}%</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import type { Song, PlaybackMode } from '../types'
import { PlaybackModes } from '../types'
import { formatTime } from '../utils/format'
import { SkipBack, Play, Pause, SkipForward, Repeat, ListOrdered, Shuffle, Volume2, AlarmClock } from '@lucide/vue'
import CountdownBadge from './CountdownBadge.vue'
import type { CountdownBadgeTimer } from '../types'

const props = defineProps<{
  activeSong: Song | null
  isPlaying: boolean
  hasQueue: boolean
  playbackMode: PlaybackMode
  masterVolume: number
  playerProgress: number
  songDuration: number
}>()

const emit = defineEmits<{
  prev: []
  next: []
  togglePlay: []
  'update:playbackMode': [mode: PlaybackMode]
  'update:masterVolume': [volume: number]
  seek: [position: number]
  showLyrics: []
}>()

// ─── Seek (progress bar click / drag) ───────────────
const isSeeking = ref(false)
const seekPreviewRatio = ref<number | null>(null)
const progressLineRef = ref<HTMLElement | null>(null)

const seekPreviewProgress = computed(() => {
  if (seekPreviewRatio.value !== null && props.songDuration) {
    return seekPreviewRatio.value * 100
  }
  if (!props.songDuration) return 0
  return (props.playerProgress / props.songDuration) * 100
})

const seekPreviewTime = computed(() => {
  if (seekPreviewRatio.value !== null && props.songDuration) {
    return formatTime(seekPreviewRatio.value * props.songDuration)
  }
  return formatTime(props.playerProgress)
})

function ratioFromEvent(e: MouseEvent | TouchEvent): number {
  const el = progressLineRef.value
  if (!el) return 0
  const rect = el.getBoundingClientRect()
  const clientX = 'touches' in e && e.touches.length
    ? e.touches[0].clientX
    : (e as MouseEvent).clientX
  let ratio = (clientX - rect.left) / rect.width
  ratio = Math.min(1, Math.max(0, ratio))
  return ratio
}

function onSeekStart(e: MouseEvent | TouchEvent) {
  if (!props.songDuration) return
  isSeeking.value = true
  seekPreviewRatio.value = ratioFromEvent(e)

  window.addEventListener('mousemove', onSeekMove)
  window.addEventListener('mouseup', onSeekEnd)
  window.addEventListener('touchmove', onSeekMove, { passive: false })
  window.addEventListener('touchend', onSeekEnd)
}

function onSeekMove(e: MouseEvent | TouchEvent) {
  if (!isSeeking.value) return
  e.preventDefault?.()
  seekPreviewRatio.value = ratioFromEvent(e)
}

function onSeekEnd() {
  if (!isSeeking.value) return
  isSeeking.value = false
  window.removeEventListener('mousemove', onSeekMove)
  window.removeEventListener('mouseup', onSeekEnd)
  window.removeEventListener('touchmove', onSeekMove)
  window.removeEventListener('touchend', onSeekEnd)

  const ratio = seekPreviewRatio.value
  seekPreviewRatio.value = null
  if (ratio === null || !props.songDuration) return
  emit('seek', Math.round(ratio * props.songDuration))
}

// ─── Mode ─────────────────────────────────────────────
const modeOptions = [
  { value: PlaybackModes.LOOP, label: '循环', icon: Repeat },
  { value: PlaybackModes.SEQUENTIAL, label: '顺序', icon: ListOrdered },
  { value: PlaybackModes.SHUFFLE, label: '随机', icon: Shuffle },
]

const currentModeIcon = computed(() => {
  const icons: Record<PlaybackMode, any> = {
    [PlaybackModes.LOOP]: Repeat,
    [PlaybackModes.SEQUENTIAL]: ListOrdered,
    [PlaybackModes.SHUFFLE]: Shuffle,
  }
  return icons[props.playbackMode] || Repeat
})

const currentModeLabel = computed(() => {
  const labels: Record<PlaybackMode, string> = {
    [PlaybackModes.LOOP]: '循环',
    [PlaybackModes.SEQUENTIAL]: '顺序',
    [PlaybackModes.SHUFFLE]: '随机',
  }
  return labels[props.playbackMode] || '循环'
})

// ─── Popover auto-close timers ──────────────────────
const showModePopover = ref(false)
const showVolumePopover = ref(false)
const modeWrapRef = ref<HTMLElement | null>(null)
const volumeWrapRef = ref<HTMLElement | null>(null)

let volumeTimer: ReturnType<typeof setTimeout> | null = null
let modeTimer: ReturnType<typeof setTimeout> | null = null

function clearPopoverTimer(type: 'volume' | 'mode') {
  const timer = type === 'volume' ? volumeTimer : modeTimer
  if (timer !== null) {
    clearTimeout(timer)
    if (type === 'volume') volumeTimer = null
    else modeTimer = null
  }
}

function restartPopoverTimer(type: 'volume' | 'mode') {
  clearPopoverTimer(type)
  const timer = setTimeout(() => {
    if (type === 'volume') showVolumePopover.value = false
    else showModePopover.value = false
  }, 1500)
  if (type === 'volume') volumeTimer = timer
  else modeTimer = timer
}

function stopPopoverTimers() {
  clearPopoverTimer('volume')
  clearPopoverTimer('mode')
}

// ─── Click outside handler ──────────────────────────
function onDocumentClick(e: MouseEvent) {
  if (volumeWrapRef.value && !volumeWrapRef.value.contains(e.target as Node)) {
    showVolumePopover.value = false
    clearPopoverTimer('volume')
  }
  if (modeWrapRef.value && !modeWrapRef.value.contains(e.target as Node)) {
    showModePopover.value = false
    clearPopoverTimer('mode')
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
  stopPopoverTimers()
  window.removeEventListener('mousemove', onSeekMove)
  window.removeEventListener('mouseup', onSeekEnd)
  window.removeEventListener('touchmove', onSeekMove)
  window.removeEventListener('touchend', onSeekEnd)
})
</script>

<style scoped>
/* ── Root (wrapper for Vue scoping) ───────────────── */
.playback-bar-root {
  /* no layout styles needed — children are position: fixed */
}

/* ── Progress line at top of playback bar ────────── */
.playback-progress-line {
  position: fixed;
  bottom: 16px;
  left: 20px;
  right: 20px;
  height: 2px;
  background: var(--border);
  z-index: 201;
  opacity: 0;
  transition: opacity 0.3s, height 0.15s;
  cursor: pointer;
  background-clip: content-box;
  touch-action: none;
}
.playback-progress-line.visible {
  opacity: 1;
}
.playback-progress-line:hover,
.playback-progress-line.dragging {
  height: 6px;
}
.playback-progress-fill {
  height: 2px;
  background: var(--accent);
  transition: width 0.3s linear;
  pointer-events: none;
}
.playback-progress-handle {
  position: absolute;
  top: 50%;
  width: 20px;
  height: 20px;
  margin-left: -10px;
  border-radius: 50%;
  background: var(--accent);
  transform: translateY(-50%) scale(0);
  transition: transform 0.15s ease;
  pointer-events: none;
  box-shadow: 0 0 6px rgba(0, 0, 0, 0.5);
  touch-action: none;
}
.playback-progress-line:hover .playback-progress-handle,
.playback-progress-line.dragging .playback-progress-handle {
  transform: translateY(-50%) scale(1);
}
.playback-progress-tooltip {
  position: absolute;
  bottom: calc(100% + 6px);
  transform: translateX(-50%);
  background: var(--bg-primary);
  border: 1px solid var(--border);
  color: var(--text-primary);
  font-size: 0.75rem;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.15s;
  font-variant-numeric: tabular-nums;
}
.playback-progress-line:hover .playback-progress-tooltip,
.playback-progress-line.dragging .playback-progress-tooltip {
  opacity: 1;
}

/* ── Fixed bottom bar ──────────────────────────────── */
.fixed-playback-bar {
  position: fixed;
  bottom: 10px;
  left: 10px;
  right: 10px;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border);
  padding: 12px 16px;
  z-index: 200;
  box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.15);
}
.fixed-playback-inner {
  max-width: 960px;
  margin: 0 auto;
  display: flex;
  align-items: center;
  gap: 12px;
}

/* Left: song info */
.playback-song-info {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 10px;
}
.playback-song-info.empty {
  flex: 0 1 auto;
}
.playback-cover {
  width: 40px;
  height: 40px;
  border-radius: 6px;
  object-fit: cover;
  flex-shrink: 0;
}
.playback-song-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.playback-title {
  font-size: 0.85rem;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.playback-artist {
  font-size: 0.7rem;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.playback-no-song {
  font-size: 0.85rem;
  color: var(--text-secondary);
}

/* Center: transport controls */
.playback-transport {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
.transport-btn {
  font-size: 1.1rem;
  padding: 6px 10px;
}
.play-btn {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  font-size: 1.2rem;
  padding: 0;
}

/* Right: extras */
.playback-extras {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.mode-btn {
  font-size: 1.1rem;
  padding: 6px 8px;
}

/* Mode popover */
.mode-btn-wrap {
  position: relative;
}
.mode-popover {
  position: absolute;
  bottom: calc(100% + 8px);
  right: 0;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
  z-index: 1010;
  min-width: 90px;
}
.mode-option {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  border-radius: var(--radius-sm);
  font-size: 0.85rem;
  white-space: nowrap;
  transition: all 0.15s;
}
.mode-option:hover {
  background: var(--bg-card);
  color: var(--text-primary);
}
.mode-option.active {
  color: var(--accent);
  font-weight: 500;
}

/* Volume popover */
.volume-btn-wrap {
  position: relative;
}
.volume-popover {
  position: absolute;
  bottom: calc(100% + 8px);
  right: 0;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 12px 16px;
  display: flex;
  align-items: center;
  gap: 10px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
  z-index: 1010;
  min-width: 180px;
}
.volume-popover input[type="range"] {
  -webkit-appearance: none;
  appearance: none;
  width: 100px;
  height: 4px;
  background: var(--border);
  border-radius: 2px;
  outline: none;
  cursor: pointer;
  flex: 1;
}
.volume-popover input[type="range"]::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--accent);
  cursor: pointer;
}
.volume-popover input[type="range"]::-moz-range-thumb {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--accent);
  cursor: pointer;
  border: none;
}
.volume-popover-value {
  font-size: 0.85rem;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
  min-width: 36px;
  text-align: right;
}

/* ── Clickable cover ───────────────────────────────── */
.clickable-cover {
  cursor: pointer;
  transition: transform 0.2s ease;
}
.clickable-cover:hover {
  transform: scale(1.08);
}
</style>
