<template>
  <Modal
    :visible="visible"
    :title="isEditing ? '编辑' + typeLabel : '创建' + typeLabel"
    :icon="formIcon"
    :closable="!saving"
    :max-width="'520px'"
    @close="onCancel"
    @update:visible="(v) => !v && onCancel()"
  >
    <form class="alarm-form" @submit.prevent="onSave">
      <!-- ─── Trigger Settings ─── -->
      <fieldset class="form-section">
        <legend class="form-section-title">{{ form.type === 'alarm_start' ? '何时触发' : '何时停止' }}</legend>

        <!-- Trigger mode toggle -->
        <div class="chip-group">
          <button
            type="button"
            class="chip"
            :class="{ active: form.trigger_mode === 'at_time' }"
            @click="form.trigger_mode = 'at_time'"
          >指定时刻</button>
          <button
            type="button"
            class="chip"
            :class="{ active: form.trigger_mode === 'countdown' }"
            @click="form.trigger_mode = 'countdown'"
          >倒计时</button>
          <button
            type="button"
            class="chip"
            :class="{ active: form.trigger_mode === 'song_count' }"
            @click="form.trigger_mode = 'song_count'"
          >播完N首歌</button>
        </div>

        <!-- Time input -->
        <div v-if="form.trigger_mode === 'at_time'" class="time-input-row">
          <label class="form-label">{{ form.type === 'alarm_start' ? '触发时刻' : '停止时刻' }}</label>
          <div class="time-input-group">
            <input
              v-model="triggerHour"
              type="number"
              min="0"
              max="23"
              class="form-input time-input"
              placeholder="HH"
              @input="onHourInput"
            />
            <span class="time-sep">:</span>
            <input
              v-model="triggerMinute"
              type="number"
              min="0"
              max="59"
              class="form-input time-input"
              placeholder="MM"
              @input="onMinuteInput"
            />
          </div>
        </div>

        <!-- Countdown input -->
        <div v-if="form.trigger_mode === 'countdown'" class="form-row">
          <div class="form-input-group">
            <input
              v-model.number="form.countdown_minutes"
              type="number"
              min="1"
              max="1440"
              class="form-input inline-input"
            />
            <span class="form-unit">分钟后{{ form.type === 'alarm_start' ? '触发' : '停止' }}</span>
          </div>
        </div>

        <!-- Song count input -->
        <div v-if="form.trigger_mode === 'song_count'" class="form-row">
          <div class="form-input-group">
            <input
              v-model.number="form.song_count"
              type="number"
              min="1"
              max="100"
              class="form-input inline-input"
            />
            <span class="form-unit">首歌后{{ form.type === 'alarm_start' ? '触发' : '停止' }}</span>
          </div>
        </div>
      </fieldset>

      <!-- ─── Play Content (alarm_start only) ─── -->
      <fieldset v-if="form.type === 'alarm_start'" class="form-section">
        <legend class="form-section-title">播放内容</legend>

        <div class="chip-group">
          <button
            type="button"
            class="chip"
            :class="{ active: form.content_mode === 'queue_start' }"
            @click="form.content_mode = 'queue_start'"
          >从头播放</button>
          <button
            type="button"
            class="chip"
            :class="{ active: form.content_mode === 'continue_current' }"
            @click="form.content_mode = 'continue_current'"
          >继续当前</button>
        </div>
      </fieldset>

      <!-- ─── Auto Stop (alarm_start only) ─── -->
      <fieldset v-if="form.type === 'alarm_start'" class="form-section">
        <legend class="form-section-title">自动停止</legend>

        <div class="chip-group">
          <button
            type="button"
            class="chip"
            :class="{ active: form.auto_stop_mode === 'no_limit' }"
            @click="form.auto_stop_mode = 'no_limit'"
          >不停止</button>
          <button
            type="button"
            class="chip"
            :class="{ active: form.auto_stop_mode === 'play_time' }"
            @click="form.auto_stop_mode = 'play_time'"
          >按时间</button>
          <button
            type="button"
            class="chip"
            :class="{ active: form.auto_stop_mode === 'song_count' }"
            @click="form.auto_stop_mode = 'song_count'"
          >按歌曲数</button>
        </div>

        <div v-if="form.auto_stop_mode === 'play_time'" class="form-row">
          <div class="form-input-group">
            <input
              v-model.number="form.auto_stop_value"
              type="number"
              min="1"
              class="form-input inline-input"
            />
            <span class="form-unit">分钟后停止</span>
          </div>
        </div>

        <div v-if="form.auto_stop_mode === 'song_count'" class="form-row">
          <div class="form-input-group">
            <input
              v-model.number="form.auto_stop_value"
              type="number"
              min="1"
              class="form-input inline-input"
            />
            <span class="form-unit">首歌后停止</span>
          </div>
        </div>
      </fieldset>

      <!-- ─── Advanced Options ─── -->
      <fieldset class="form-section advanced-section">
        <legend class="form-section-title">
          <button
            type="button"
            class="advanced-toggle"
            @click="showAdvanced = !showAdvanced"
          >
            <ChevronRight :size="14" :class="{ 'rotate-90': showAdvanced }" />
            高级选项
          </button>
        </legend>

        <div v-if="showAdvanced" class="advanced-content">
          <!-- Repeat (alarm_start only) -->
          <div v-if="form.type === 'alarm_start'" class="advanced-item">
            <label class="advanced-label">重复规则</label>
            <div class="chip-group">
              <button
                type="button"
                class="chip chip-sm"
                :class="{ active: form.repeat === 'once' }"
                @click="form.repeat = 'once'"
              >单次</button>
              <button
                type="button"
                class="chip chip-sm"
                :class="{ active: form.repeat === 'daily' }"
                @click="form.repeat = 'daily'"
              >每天</button>
              <button
                type="button"
                class="chip chip-sm"
                :class="{ active: form.repeat === 'weekday' }"
                @click="form.repeat = 'weekday'"
              >工作日</button>
              <button
                type="button"
                class="chip chip-sm"
                :class="{ active: form.repeat === 'custom' }"
                @click="form.repeat = 'custom'"
              >自定义</button>
            </div>

            <!-- Custom day picker -->
            <div v-if="form.repeat === 'custom'" class="day-picker">
              <button
                v-for="(label, idx) in dayLabels"
                :key="idx"
                type="button"
                class="day-chip"
                :class="{ active: form.repeat_days?.includes(idx) }"
                @click="toggleDay(idx)"
              >{{ label }}</button>
            </div>
          </div>

          <!-- Conflict Strategy (alarm_start only) -->
          <div v-if="form.type === 'alarm_start'" class="advanced-item">
            <label class="advanced-label">冲突时处理</label>
            <div class="chip-group">
              <button
                type="button"
                class="chip chip-sm"
                :class="{ active: form.conflict_strategy === 'queue' }"
                @click="form.conflict_strategy = 'queue'"
                title="插入队列头部，当前播完后切换"
              >排队等候</button>
              <button
                type="button"
                class="chip chip-sm"
                :class="{ active: form.conflict_strategy === 'replace' }"
                @click="form.conflict_strategy = 'replace'"
                title="立即停止当前，播放闹钟内容"
              >立即切换</button>
              <button
                type="button"
                class="chip chip-sm"
                :class="{ active: form.conflict_strategy === 'skip' }"
                @click="form.conflict_strategy = 'skip'"
                title="正在播放时不触发"
              >跳过本次</button>
            </div>
          </div>

          <!-- Fade -->
          <div class="advanced-item">
            <label class="advanced-label">
              <label class="fade-toggle">
                <input
                  type="checkbox"
                  v-model="fadeEnabled"
                />
                <span>{{ form.type === 'alarm_start' ? '淡入效果' : '淡出效果' }}</span>
              </label>
            </label>
            <div v-if="fadeEnabled" class="form-input-group">
              <input
                v-model.number="fadeSeconds"
                type="number"
                min="1"
                max="60"
                class="form-input inline-input"
              />
              <span class="form-unit">秒</span>
            </div>
          </div>
        </div>
      </fieldset>

      <!-- ─── Validation Error ─── -->
      <div v-if="validationError" class="validation-error">
        <AlertTriangle :size="16" />
        {{ validationError }}
      </div>

      <!-- ─── Actions ─── -->
      <div class="form-actions">
        <button type="button" class="btn" @click="onCancel" :disabled="saving">取消</button>
        <button type="submit" class="btn btn-primary" :disabled="saving">
          <template v-if="saving">
            <span class="spinner"></span> 保存中...
          </template>
          <template v-else>保存</template>
        </button>
      </div>
    </form>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, reactive, watch } from 'vue'
import type { Alarm } from '../types'
import { createDefaultAlarm } from '../types'
import { AlarmClock, Moon, AlertTriangle, ChevronRight } from '@lucide/vue'
import Modal from './Modal.vue'

const props = defineProps<{
  visible: boolean
  channelName: string
  editingAlarm: Alarm | null  // null = creating new
  initialType?: 'alarm_start' | 'sleep_timer'  // type hint for new alarms
}>()

const emit = defineEmits<{
  save: [alarm: Alarm]
  close: []
}>()

const dayLabels = ['日', '一', '二', '三', '四', '五', '六']

const saving = ref(false)
const validationError = ref<string | null>(null)
const showAdvanced = ref(false)

// Form state
const form = reactive<Alarm>(
  props.editingAlarm
    ? { ...props.editingAlarm }
    : createDefaultAlarm(props.channelName, 'alarm_start')
)

const isEditing = computed(() => props.editingAlarm !== null)

const typeLabel = computed(() => form.type === 'alarm_start' ? '定时播放' : '定时关闭')

const formIcon = computed(() => form.type === 'alarm_start' ? AlarmClock : Moon)

const fadeEnabled = ref(
  (form.type === 'alarm_start' && form.fade_in_seconds > 0) ||
  (form.type === 'sleep_timer' && form.fade_out_seconds > 0)
)

const fadeSeconds = ref(
  form.type === 'alarm_start' ? form.fade_in_seconds : form.fade_out_seconds
)

// Trigger time helper
const triggerHour = ref('')
const triggerMinute = ref('')

// Parse trigger time into hour/minute
watch([() => form.trigger_time, () => form.visible], () => {
  if (form.trigger_time && form.trigger_time.includes(':')) {
    const [h, m] = form.trigger_time.split(':')
    triggerHour.value = h
    triggerMinute.value = m
  } else {
    triggerHour.value = '07'
    triggerMinute.value = '00'
  }
})

// Sync hour/minute back to form.trigger_time
watch([triggerHour, triggerMinute], () => {
  const h = triggerHour.value.padStart(2, '0')
  const m = triggerMinute.value.padStart(2, '0')
  form.trigger_time = `${h}:${m}`
})

function onHourInput() {
  if (triggerHour.value.length >= 2) {
    // Auto-focus minute
  }
  let val = parseInt(triggerHour.value)
  if (isNaN(val)) val = 0
  if (val > 23) val = 23
  if (val < 0) val = 0
  triggerHour.value = String(val).padStart(2, '0')
}

function onMinuteInput() {
  let val = parseInt(triggerMinute.value)
  if (isNaN(val)) val = 0
  if (val > 59) val = 59
  if (val < 0) val = 0
  triggerMinute.value = String(val).padStart(2, '0')
}

function toggleDay(idx: number) {
  if (!form.repeat_days) {
    form.repeat_days = []
  }
  const i = form.repeat_days.indexOf(idx)
  if (i >= 0) {
    form.repeat_days.splice(i, 1)
  } else {
    form.repeat_days.push(idx)
  }
}

function validate(): boolean {
  validationError.value = null

  if (form.trigger_mode === 'at_time' && !form.trigger_time) {
    validationError.value = '请设置触发时刻'
    return false
  }

  if (form.trigger_mode === 'countdown' && (!form.countdown_minutes || form.countdown_minutes < 1)) {
    validationError.value = '请设置有效的倒计时分钟数'
    return false
  }

  if (form.trigger_mode === 'song_count' && (!form.song_count || form.song_count < 1)) {
    validationError.value = '请设置有效的歌曲数'
    return false
  }

  if (form.repeat === 'custom' && (!form.repeat_days || form.repeat_days.length === 0)) {
    validationError.value = '请选择至少一天'
    return false
  }

  return true
}

async function onSave() {
  if (!validate()) return

  saving.value = true
  validationError.value = null

  // Sync fade values
  if (fadeEnabled.value) {
    if (form.type === 'alarm_start') {
      form.fade_in_seconds = fadeSeconds.value
    } else {
      form.fade_out_seconds = fadeSeconds.value
    }
  } else {
    form.fade_in_seconds = 0
    form.fade_out_seconds = 0
  }

  emit('save', { ...form })
}

function onCancel() {
  if (saving.value) return
  emit('close')
}

// Reset form when opening
watch(() => props.visible, (val) => {
  if (val) {
    const defaultType = props.editingAlarm
      ? props.editingAlarm.type as 'alarm_start' | 'sleep_timer'
      : (props.initialType || 'alarm_start')
    const source = props.editingAlarm || createDefaultAlarm(props.channelName, defaultType)
    Object.assign(form, source)
    validationError.value = null
    saving.value = false
    showAdvanced.value = false
    fadeEnabled.value = (
      (form.type === 'alarm_start' && form.fade_in_seconds > 0) ||
      (form.type === 'sleep_timer' && form.fade_out_seconds > 0)
    )
    fadeSeconds.value = form.type === 'alarm_start' ? form.fade_in_seconds : form.fade_out_seconds
  }
})
</script>

<style scoped>
.alarm-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-section {
  border: none;
  padding: 0;
  margin: 0;
}

.form-section-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 8px;
  padding: 0;
}

/* Chip group */
.chip-group {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.chip {
  padding: 8px 16px;
  border: 1px solid var(--border);
  border-radius: 20px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.15s;
  white-space: nowrap;
}

.chip:hover {
  border-color: var(--accent);
  color: var(--text-primary);
}

.chip.active {
  background: var(--accent);
  color: white;
  border-color: var(--accent);
}

.chip-sm {
  padding: 5px 12px;
  font-size: 0.8rem;
}

/* Time input */
.time-input-row {
  margin-top: 8px;
}

.time-input-group {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
}

.time-input {
  width: 60px !important;
  text-align: center;
  font-variant-numeric: tabular-nums;
  font-size: 1.2rem !important;
  padding: 8px !important;
}

.time-sep {
  font-size: 1.2rem;
  font-weight: 600;
  color: var(--text-primary);
}

/* Form row */
.form-row {
  margin-top: 8px;
}

.form-label {
  display: block;
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.form-input {
  width: 100%;
  padding: 8px 12px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-primary);
  font-size: 0.9rem;
  outline: none;
  transition: border-color 0.2s;
}

.form-input:focus {
  border-color: var(--accent);
}

.form-input-group {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.form-unit {
  font-size: 0.85rem;
  color: var(--text-secondary);
  white-space: nowrap;
}

.inline-input {
  width: 70px !important;
  text-align: center;
}

/* Advanced section */
.advanced-section {
  border-top: 1px solid var(--border);
  padding-top: 12px;
  margin-top: 4px;
}

.advanced-toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: none;
  border: none;
  padding: 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-secondary);
  cursor: pointer;
  transition: color 0.15s;
}

.advanced-toggle:hover {
  color: var(--text-primary);
}

.advanced-toggle svg {
  transition: transform 0.2s;
}

.rotate-90 {
  transform: rotate(90deg);
}

.advanced-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}

.advanced-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.advanced-label {
  font-size: 0.8rem;
  color: var(--text-secondary);
}

/* Radio group */
.radio-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.radio-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.85rem;
  cursor: pointer;
  padding: 6px 0;
}

.radio-item input[type="radio"] {
  accent-color: var(--accent);
}

/* Day picker */
.day-picker {
  display: flex;
  gap: 6px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.day-chip {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 1px solid var(--border);
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s;
  display: flex;
  align-items: center;
  justify-content: center;
}

.day-chip:hover {
  border-color: var(--accent);
}

.day-chip.active {
  background: var(--accent);
  color: white;
  border-color: var(--accent);
}

.day-chip:hover {
  border-color: var(--accent);
}

.day-chip.active {
  background: var(--accent);
  color: white;
  border-color: var(--accent);
}

/* Fade toggle */
.fade-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  font-size: 0.85rem;
}

.fade-toggle input[type="checkbox"] {
  accent-color: var(--accent);
}

/* Validation */
.validation-error {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 14px;
  background: rgba(245, 158, 11, 0.1);
  border: 1px solid var(--warning);
  border-radius: var(--radius-sm);
  color: var(--warning);
  font-size: 0.8rem;
}

/* Actions */
.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}

/* Spinner */
.spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Responsive */
@media (max-width: 480px) {
  .chip-group {
    gap: 4px;
  }
  .chip {
    padding: 5px 10px;
    font-size: 0.75rem;
  }
}
</style>
