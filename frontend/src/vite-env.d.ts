/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

// 扩展 vue-router 的 RouteMeta 类型
import 'vue-router';

declare module 'vue-router' {
  interface RouteMeta {
    title?: string;
    requiresAuth?: boolean;
    admin?: boolean;
  }
}

// 全局类型扩展
declare global {
  interface Window {
    ethereum?: any;
  }
}

export {};
