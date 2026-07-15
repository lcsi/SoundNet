import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'
import fs from 'fs'

// 自定义联动热更新插件
const nfsPrecisionHmrPlugin = () => ({
  name: 'nfs-precision-hmr',
  configureServer(server: ViteDevServer) {
    // 监听本地 app2 的变动
    server.watcher.on('change', async (filePath) => {
      // src/dir1是映射的volume，对平台不可见（就像node_modules一样，平台只能看到有个文件夹，看不到里面的文件），所以不能通过在平台修改该文件的方式触发热更新
      // vite热更新的逻辑是： 监听到文件变化  判断该文件是否需要通知浏览器  需要更新才会发送到浏览器
      // 这里只能检测到src/dir1中文件变化，所以肯定不会发送给浏览器，因为src/dir1里的文件不被任何文件引用
      // 即使项目中引用了src/dir1中的文件，比如App.vue中引入dir1中的一个组件，vite默认的热更新机制也只会更新src/dir1中的这个组件（vite会分析依赖），其他不更新
      // 所以这里只能自己实现热更新逻辑，维护src中所有文件的最后更新时间，触发热更新的时候判断文件最后更新时间是否改变，如果发生改变，就让编译缓存失效，vite就会触发热更新并发给浏览器
      
      if (!filePath.includes(path.join('src', 'dir1'))) return

      const updates: any[] = []

      // 遍历 Vite 依赖图中的所有缓存模块
      for (const [id, module] of server.moduleGraph.idToModuleMap.entries()) {
        // 过滤出属于 app1 的模块，且排除掉虚拟模块（不带绝对路径的）
        if (id && id.includes(path.join('src')) && fs.existsSync(id)) {
          try {
            // 1. 获取该 app1 文件在磁盘上的最新真实修改时间
            const stat = fs.statSync(id)
            const fileMtime = stat.mtimeMs

            // 2. 获取该模块在 Vite 内存中的“上一次热更新/编译时间”
            // 如果从未更新过，则取该模块被创建（首次编译）的时间
            const lastCompileTime = module.lastHMRTimestamp || (module as any).lastRequestTime || 0

            // 3. 核心对比：如果文件的磁盘修改时间 大于 上次编译时间，说明此文件有未同步的改动
            if (fileMtime > lastCompileTime) {
              // 强行将该模块的编译缓存标记为过期，迫使 Vite 下次读取新磁盘文件
              server.moduleGraph.invalidateModule(module)

              // 记录需要更新的模块信息
              updates.push({
                type: module.type === 'js' ? 'js-update' : 'css-update',
                path: module.url,
                acceptedPath: module.url,
                timestamp: Date.now(),
              })
              
              // 顺便同步更新模块的内部时间戳，防止单次保存时 app2 重复触发它
              module.lastHMRTimestamp = Date.now()
            }
          } catch (e) {
            // 防止文件被删除时 fs.statSync 报错崩溃
          }
        }
      }

      // 4. 只有真正变动的 app1 文件，才会通过 WebSocket 精准推送给浏览器
      if (updates.length > 0) {
        server.ws.send({
          type: 'update',
          updates: updates,
        })
      }
    })
  },
})

export default defineConfig({
  plugins: [
    vue(),
    nfsPrecisionHmrPlugin()
  ],
  server: {
    host: '0.0.0.0',
    allowedHosts: true,
    port: 5173,
    proxy: {
      '/ws': {
        target: 'ws://api:8080',
        ws: true,
      },
      '/api': {
        target: 'http://api:8080',
        changeOrigin: true,
      },
    },
  },
})
