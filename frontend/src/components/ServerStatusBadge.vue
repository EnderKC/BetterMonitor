<template>
  <div :class="['status-badge', isOnline ? 'status-online' : 'status-offline']">
    <div class="status-indicator"></div>
    <span>{{ statusText }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useServerStore } from '../stores/serverStore';

// 定义props
const props = defineProps<{
  serverId: number;
}>();

// 使用服务器状态store
const serverStore = useServerStore();

// 计算服务器是否在线
const isOnline = computed(() => {
  return serverStore.isServerOnline(props.serverId);
});

// 计算服务器状态文本
const statusText = computed(() => {
  // 获取最近更新的状态
  const status = serverStore.getServerStatus(props.serverId);
  return status === 'online' ? '在线' : '离线';
});
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  padding: 4px 12px;
  border-radius: var(--radius-md);
  font-size: 14px;
  font-weight: 500;
}

.status-online {
  background-color: rgba(82, 196, 26, 0.1);
  color: var(--success-color);
  border: 1px solid rgba(82, 196, 26, 0.2);
}

.status-offline {
  background-color: rgba(255, 77, 79, 0.1);
  color: var(--error-color);
  border: 1px solid rgba(255, 77, 79, 0.2);
}

.status-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 8px;
}

.status-online .status-indicator {
  background-color: var(--success-color);
  box-shadow: 0 0 8px var(--success-color);
  animation: pulse 2s infinite;
}

.status-offline .status-indicator {
  background-color: var(--error-color);
}

@keyframes pulse {
  0% {
    opacity: 0.6;
    transform: scale(0.9);
  }
  50% {
    opacity: 1;
    transform: scale(1.1);
  }
  100% {
    opacity: 0.6;
    transform: scale(0.9);
  }
}
</style> 