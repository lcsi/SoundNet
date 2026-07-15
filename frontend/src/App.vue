<template>
  <div id="app-root" :class="currentTheme">
    <!-- Global header with theme switcher (only show on homepage) -->
    <header v-if="isHomePage" class="app-header">
      <div class="header-left">
        <h1><Music :size="28" /> 音乐播放器</h1>
      </div>
      <div class="header-right">
        <button class="btn btn-sm theme-btn" @click="cycleTheme" :title="'当前：' + themeLabel">
          <component :is="themeIcon" :size="18" />
        </button>
      </div>
    </header>
    <main class="app-main">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { Music, Moon, Leaf } from '@lucide/vue'

const route = useRoute()
const isHomePage = computed(() => route.path === '/')

// 可用主题列表
const THEMES = [
  { id: 'theme-dark', label: '默认暗红', icon: Moon },
  { id: 'theme-forest', label: '绿色森系', icon: Leaf },
  { id: 'theme-fresh', label: '清新黄绿', icon: Leaf },
]

// 当前主题
const currentTheme = ref(localStorage.getItem('app-theme') || THEMES[0].id)

// 当前主题的显示信息
const themeInfo = computed(() => THEMES.find(t => t.id === currentTheme.value) || THEMES[0])
const themeIcon = computed(() => themeInfo.value.icon)
const themeLabel = computed(() => themeInfo.value.label)

// 循环切换主题
function cycleTheme() {
  const idx = THEMES.findIndex(t => t.id === currentTheme.value)
  const next = THEMES[(idx + 1) % THEMES.length]
  applyTheme(next.id)
}

// 应用主题：class 同时加到 <html> 和 #app-root 上
function applyTheme(theme: string) {
  currentTheme.value = theme
  localStorage.setItem('app-theme', theme)
  document.documentElement.className = theme
}

// 初始化：将主题 class 同步到 <html>
applyTheme(currentTheme.value)

// 暴露给全局，方便从控制台切换
declare global {
  interface Window {
    __setTheme: (theme: string) => void
  }
}
window.__setTheme = applyTheme
</script>

<style scoped>
#app-root {
  max-width: 900px;
  margin: 0 auto;
  padding: 16px;
}

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 10px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
  gap: 12px;
}

.app-header h1 {
  font-size: 1.4rem;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 8px;
}

.app-main {
  min-height: calc(100vh - 100px);
}

.header-right {
  display: flex;
  align-items: center;
  gap: var(--space-3, 8px);
}

.theme-btn {
  font-size: 1.2rem;
  padding: 6px 12px;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.2s ease;
  line-height: 1;
  min-height: 44px;
  min-width: 44px;
}

.theme-btn:hover {
  background: var(--accent);
  transform: scale(1.05);
}

@media (max-width: 600px) {
  #app-root {
    padding: 0 10px;
  }

  .app-header {
    align-items: flex-start;
  }
}
</style>
