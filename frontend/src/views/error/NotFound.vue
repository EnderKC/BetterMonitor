<template>
  <div class="not-found">
    <h1>404</h1>
    <h2>页面未找到</h2>
    <p>抱歉，您访问的页面不存在。</p>
    <a-button type="primary" @click="goHome">返回首页</a-button>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router';

const router = useRouter();

const goHome = () => {
  router.push('/');
};
</script>

<style scoped>
.not-found {
  height: 100vh;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  background-color: #f0f2f5;
}

h1 {
  font-size: 120px;
  margin: 0;
  color: var(--primary-color);
}

h2 {
  font-size: var(--font-size-3xl);
  margin: 8px 0 16px;
}

p {
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 24px;
}
</style> 