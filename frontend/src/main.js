import './polyfills/uint8-base64.js'
import './polyfills/promise-with-resolvers.js'
import { createApp } from 'vue'
import { addCollection } from '@iconify/vue'
import lucideIcons from '@iconify-json/lucide/icons.json'
import App from './App.vue'
import router from './router'
import './index.css'
import ui from '@nuxt/ui/vue-plugin'
import { i18n } from './i18n.js'

// 离线内联 lucide 图标集：Nuxt UI 的 UIcon 默认在运行时从 api.iconify.design 拉取
// SVG，国内网络常不可达，导致图标加载失败。带文字的按钮尚能看到、能点，但纯图标
// 按钮（如移动端顶栏的汉堡菜单）会完全空白——「看不到但能点出菜单」即由此而来。
// 构建期把本地 @iconify-json/lucide 全量注册进来，所有图标不再依赖网络。
addCollection(lucideIcons)

const app = createApp(App)

app.use(i18n)
app.use(router)
app.use(ui)
app.mount('#app')
