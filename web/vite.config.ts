import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { readdirSync, readFileSync, statSync, unlinkSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { constants, gzipSync } from 'node:zlib'

// 构建后保留 dist/.gitkeep，保证未构建前端时 Go 的 go:embed 仍可编译
const keepGitkeep = {
  name: 'keep-gitkeep',
  closeBundle() {
    writeFileSync(resolve(import.meta.dirname, 'dist/.gitkeep'), '')
  },
}

// 构建后为 assets 下的文本资源生成 .gz 并删除原文件，管理端直接返回压缩版本，免去每次请求实时压缩。
// 只生成 gzip：浏览器经 HTTP 访问（如局域网 IP）时不支持 br，gzip 两种场景都能用。
// 二进制只嵌入 .gz（文本资源约少 3/4 体积）；少数不接受 gzip 的请求由服务端现场解压返回。
const precompress = {
  name: 'precompress-gzip',
  closeBundle() {
    const dir = resolve(import.meta.dirname, 'dist/assets')
    for (const name of readdirSync(dir)) {
      const file = join(dir, name)
      // 小文件压缩收益很小，不生成
      if (!/\.(js|css|svg|json|txt)$/.test(name) || statSync(file).size < 1024) {
        continue
      }
      writeFileSync(file + '.gz', gzipSync(readFileSync(file), { level: constants.Z_BEST_COMPRESSION }))
      unlinkSync(file)
    }
  },
}

export default defineConfig({
  plugins: [vue(), keepGitkeep, precompress],
  // 资源使用相对路径，页面位于子路径（如飞牛 fnOS 统一网关的 /app/gately/）时也能加载
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              // 每种语言的文案合并为一个文件，首屏少发约百个请求（经飞牛统一网关等反向代理时尤其明显）
              name: (id) => {
                const m = /[\\/]src[\\/]locales[\\/]([^\\/]+)[\\/][^\\/]+\.json$/.exec(id)
                return m ? `locale-${m[1]}` : null
              },
            },
          ],
        },
      },
    },
  },
  server: {
    proxy: {
      // 保留浏览器的 Host：后端按 Origin 与 Host 是否一致拒绝跨源写请求，改写 Host 会让开发环境无法登录；ws 供终端等 WebSocket 使用
      // 后端地址可用环境变量 GATELY_API 覆盖，便于同时调试多个实例
      '/api': { target: process.env.GATELY_API || 'http://127.0.0.1:16680', changeOrigin: false, ws: true },
    },
  },
})
