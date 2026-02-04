import { createApp } from 'vue'
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'
import './style.css'
import App from './App.vue'
import router from './router'
// 导入createPinia创建Pinia实例
import { createPinia } from 'pinia'

// 防止钱包扩展冲突
try {
  // 检查是否已经有ethereum对象，如果因为某些扩展出现问题，捕获错误
  window.ethereum = window.ethereum || {};
} catch (error) {
  console.warn('Unable to set ethereum object:', error);
}

// 创建Pinia实例
const pinia = createPinia()

const app = createApp(App)
app.use(Antd)
// 注册Pinia
app.use(pinia)
app.use(router)
app.mount('#app')
