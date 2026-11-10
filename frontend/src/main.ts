import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { installMock } from './mock'
import { bootstrapTheme } from './theme'
import './style.css'

// 先把主题套上，避免首帧闪烁；真正的配置要等 init() 从后端读回来再覆盖。
bootstrapTheme()

// 在真正的 Wails 运行时里此调用会立即返回；
// 只有用浏览器直接打开 dev server 时才会注入假数据，便于单独调界面。
installMock()

createApp(App).use(router).mount('#app')
