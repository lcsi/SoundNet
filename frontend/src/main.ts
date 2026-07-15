import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import ControlPage from './views/ControlPage.vue'
import PlayerPage from './views/PlayerPage.vue'
import ChannelDetail from './views/ChannelDetail.vue'

// 全局样式
import './styles/base.css'
import './styles/button.css'
import './styles/form.css'
import './styles/badge.css'
import './styles/card.css'
import './styles/utility.css'

// 主题
import './themes/dark.css'
import './themes/forest.css'
import './themes/fresh.css'

const routes = [
  { path: '/', name: 'control', component: ControlPage },
  { path: '/player', name: 'player', component: PlayerPage },
  { path: '/channel/:name', name: 'channel-detail', component: ChannelDetail },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

const app = createApp(App)
app.use(router)
app.mount('#app')
