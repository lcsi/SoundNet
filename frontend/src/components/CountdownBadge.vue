<template>
  <div
    class="countdown-badge"
    :class="badgeClass"
    role="button"
    :aria-label="ariaLabel"
    tabindex="0"
    @click="$emit('openAlarmPanel')"
    @keydown.enter="$emit('openAlarmPanel')"
    @keydown.space.prevent="$emit('openAlarmPanel')"
    :title="tooltipText"
  >
    <!-- Alarm start countdown -->
    <span v-if="alarmTimer" class="badge-item">
      <AlarmClock :size="16" />
      <span class="badge-time" :class="{ imminent: isImminent }">{{ alarmDisplay }}</span>
    </span>

    <!-- Sleep timer countdown -->
    <span v-if="sleepTimer" class="badge-item">
      <Moon :size="16" />
      <span class="badge-time" :class="{ imminent: isSleepImminent }">{{ sleepDisplay }}</span>
    </span>

    <!-- No active timers — show simple icon -->
    <span v-if="!alarmTimer && !sleepTimer" class="badge-item badge-inactive">
      <AlarmClock :size="16" />
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import type { CountdownBadgeTimer } from '../types'
import { AlarmClock, Moon } from '@lucide/vue'

const props = defineProps<{
  alarmTimer: CountdownBadgeTimer | null
  sleepTimer: CountdownBadgeTimer | null
}>()

defineEmits<{
  openAlarmPanel: []
}>()

// Local countdown offsets
const elapsed = ref(0)
let interval: ReturnType<typeof setInterval> | null = null

// Reset elapsed when props change
watch([() => props.alarmTimer, () => props.sleepTimer], () => {
  elapsed.value = 0
})

onMounted(() => {
  elapsed.value = 0
  interval = setInterval(() => {
    elapsed.value++
  }, 1000)
})

onUnmounted(() => {
  if (interval) clearInterval(interval)
})

function getRemaining(timer: CountdownBadgeTimer | null): number {
  if (!timer) return 0
  return Math.max(0, timer.remaining_seconds - elapsed.value)
}

function getDisplay(timer: CountdownBadgeTimer | null): string {
  if (!timer) return ''
  const remaining = getRemaining(timer)
  if (timer.trigger_time && remaining > 300) {
    // Show static trigger time when > 5 minutes remaining
    return timer.trigger_time
  }
  return formatRemaining(remaining)
}

function isImminentCheck(timer: CountdownBadgeTimer | null): boolean {
  if (!timer) return false
  return getRemaining(timer) <= 300
}

const alarmRemaining = computed(() => getRemaining(props.alarmTimer))
const sleepRemaining = computed(() => getRemaining(props.sleepTimer))

const isImminent = computed(() => isImminentCheck(props.alarmTimer))
const isSleepImminent = computed(() => isImminentCheck(props.sleepTimer))

const alarmDisplay = computed(() => getDisplay(props.alarmTimer))
const sleepDisplay = computed(() => getDisplay(props.sleepTimer))

const badgeClass = computed(() => {
  if (props.alarmTimer || props.sleepTimer) return 'badge-active'
  return 'badge-inactive'
})

const ariaLabel = computed(() => {
  const parts: string[] = ['闹钟管理']
  if (props.alarmTimer) {
    parts.push(`定时播放 ${alarmDisplay.value}`)
  }
  if (props.sleepTimer) {
    parts.push(`睡眠定时 ${sleepDisplay.value}`)
  }
  return parts.join('，')
})

const tooltipText = computed(() => {
  const parts: string[] = ['闹钟管理']
  if (props.alarmTimer && props.alarmTimer.trigger_time && alarmRemaining.value > 300) {
    parts.push(`定时播放 ${props.alarmTimer.trigger_time} · 剩余 ${formatRemaining(alarmRemaining.value)}`)
  } else if (props.alarmTimer) {
    parts.push(`定时播放 · 剩余 ${formatRemaining(alarmRemaining.value)}`)
  }
  if (props.sleepTimer) {
    parts.push(`睡眠定时 · 剩余 ${formatRemaining(sleepRemaining.value)}`)
  }
  if (parts.length === 1) {
    parts.push('点击设置')
  }
  return parts.join('\n')
})

function formatRemaining(seconds: number): string {
  if (seconds <= 0) return '00:00'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) {
    return `${h}小时${m}分`
  }
  if (m > 0 && seconds > 300) {
    return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  }
  // < 5 minutes: show MM:SS
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}
</script>

<style scoped>
.countdown-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s ease;
  font-size: 0.8rem;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  user-select: none;
  outline: none;
  min-height: 32px;
}

.countdown-badge:hover {
  background: var(--bg-card);
}

.countdown-badge:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.countdown-badge.badge-inactive {
  color: var(--text-secondary);
  opacity: 0.6;
}

.countdown-badge.badge-active {
  color: var(--accent);
}

.badge-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.badge-item + .badge-item {
  margin-left: 4px;
  padding-left: 4px;
}

.badge-time {
  white-space: nowrap;
}

.badge-time.imminent {
  color: var(--warning, #f59e0b);
  animation: blink-warning 1s ease-in-out infinite;
}

@keyframes blink-warning {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

@media (prefers-reduced-motion: reduce) {
  .badge-time.imminent {
    animation: none;
    opacity: 0.7;
  }
}
</style>
