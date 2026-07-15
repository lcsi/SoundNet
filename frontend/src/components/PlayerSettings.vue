<template>
  <Modal
    :visible="visible"
    title="播放器设置"
    :icon="Settings"
    :max-width="'400px'"
    show-footer
    @close="cancel"
    @update:visible="(v) => emit('update:visible', v)"
  >
    <div class="form-group">
      <label>名称</label>
      <input
        v-model="form.name"
        type="text"
        placeholder="播放器名称"
      />
    </div>

    <div class="form-group">
      <label>备注</label>
      <input
        v-model="form.note"
        type="text"
        placeholder="备注信息"
      />
    </div>

    <div class="form-group">
      <label>歌曲缓存目录</label>
      <input
        v-model="form.settings.cache_dir"
        type="text"
        placeholder="留空使用系统默认目录"
      />
      <span class="form-hint">原生播放器下载歌曲的本地存储路径</span>
    </div>

    <div class="form-group">
      <label>初始音量</label>
      <div class="volume-slider">
        <input
          v-model.number="form.settings.initial_volume"
          type="range"
          min="0"
          max="100"
        />
        <span class="volume-value">{{ form.settings.initial_volume }}%</span>
      </div>
    </div>

    <template #footer>
      <button class="btn" @click="cancel">取消</button>
      <button class="btn btn-primary" @click="save" :disabled="saving">
        保存
      </button>
      <button class="btn btn-accent" @click="refreshConfig" :disabled="saving">
        刷新配置
      </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Settings } from '@lucide/vue'
import Modal from './Modal.vue'
import type { PlayerInfo } from '../types'

interface PlayerSettings {
  cache_dir: string
  initial_volume: number
}

const props = defineProps<{
  visible: boolean
  player: PlayerInfo | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'saved'): void
}>()

const saving = ref(false)

const form = ref({
  name: '',
  note: '',
  settings: {
    cache_dir: '',
    initial_volume: 80,
  } as PlayerSettings,
})

// Watch for player changes and populate form
watch(
  () => props.player,
  (player) => {
    if (player) {
      form.value.name = player.name || ''
      form.value.note = player.note || ''
      form.value.settings = {
        cache_dir: player.settings?.cache_dir || '',
        initial_volume: player.settings?.initial_volume ?? 80,
      }
    }
  },
  { immediate: true }
)

function cancel() {
  emit('update:visible', false)
}

async function save() {
  if (!props.player) return
  saving.value = true
  try {
    const resp = await fetch(`/api/players/${props.player.id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: form.value.name,
        note: form.value.note,
        settings: form.value.settings,
      }),
    })
    if (resp.ok) {
      emit('saved')
      emit('update:visible', false)
    }
  } catch (e) {
    console.error('Failed to save player settings:', e)
  } finally {
    saving.value = false
  }
}

async function refreshConfig() {
  if (!props.player) return
  saving.value = true
  try {
    // First save the settings
    const saveResp = await fetch(`/api/players/${props.player.id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: form.value.name,
        note: form.value.note,
        settings: form.value.settings,
      }),
    })

    if (saveResp.ok) {
      // Then trigger refresh_config
      await fetch(`/api/players/${props.player.id}/refresh-config`, {
        method: 'POST',
      })
      emit('saved')
      emit('update:visible', false)
    }
  } catch (e) {
    console.error('Failed to refresh player config:', e)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 16px;
}

.form-group:last-child {
  margin-bottom: 0;
}

.form-group label {
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--text-primary);
}

.form-group input[type="text"] {
  width: 100%;
  padding: 10px 14px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-primary);
  font-size: 0.9rem;
  outline: none;
  transition: border-color 0.2s;
}

.form-group input[type="text"]:focus {
  border-color: var(--accent);
}

.form-hint {
  font-size: 0.75rem;
  color: var(--text-secondary);
}

.volume-slider {
  display: flex;
  align-items: center;
  gap: 12px;
}

.volume-slider input[type="range"] {
  flex: 1;
}

.volume-value {
  min-width: 40px;
  text-align: right;
  font-size: 0.9rem;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
}

.btn-accent {
  background: var(--success, #22c55e);
  color: white;
  border-color: var(--success, #22c55e);
}

.btn-accent:hover {
  background: var(--success-hover, #16a34a);
}
</style>
