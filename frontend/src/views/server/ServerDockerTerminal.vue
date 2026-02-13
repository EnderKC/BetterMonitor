<template>
  <div class="docker-terminal-page">
    <a-page-header
      :title="`容器终端 - ${containerName}`"
      @back="router.push({ name: 'ServerDocker', params: { id: serverId } })"
      class="glass-header"
    >
      <template #tags>
        <a-tag color="blue">容器: {{ containerId.slice(0, 12) }}</a-tag>
        <a-tag v-if="connected" color="success">已连接</a-tag>
        <a-tag v-else color="default">未连接</a-tag>
      </template>
      <template #extra>
        <a-space>
          <a-button @click="reconnect" :loading="connecting">
            重新连接
          </a-button>
          <a-button danger @click="disconnect" :disabled="!connected">
            断开
          </a-button>
        </a-space>
      </template>
    </a-page-header>

    <div class="terminal-wrapper">
      <TerminalView
        ref="terminalView"
        :socket-url="wsUrl"
        @connected="onConnected"
        @disconnected="onDisconnected"
        @error="onError"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { getToken } from '../../utils/auth';
import TerminalView from '../../components/server/TerminalView.vue';

const route = useRoute();
const router = useRouter();
const serverId = route.params.id as string;
const containerId = route.params.containerId as string;
const containerName = (route.query.name as string) || containerId;

const terminalView = ref<InstanceType<typeof TerminalView> | null>(null);
const connected = ref(false);
const connecting = ref(false);
const sessionId = ref<string>('');

const generateSessionId = () => `docker-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

const wsUrl = computed(() => {
  if (!sessionId.value) return '';
  const token = getToken();
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}/api/servers/${serverId}/ws?token=${token}&session=${sessionId.value}`;
});

const connect = () => {
  if (!containerId) {
    message.error('缺少容器ID');
    return;
  }
  
  connecting.value = true;
  sessionId.value = generateSessionId();
  
  // TerminalView will watch wsUrl and connect automatically
};

const disconnect = () => {
  sessionId.value = ''; // This will trigger TerminalView to disconnect
  connected.value = false;
};

const reconnect = () => {
  disconnect();
  setTimeout(() => {
    connect();
  }, 100);
};

const onConnected = () => {
  connected.value = true;
  connecting.value = false;
  
  if (terminalView.value) {
    terminalView.value.send({
      type: 'shell_command',
      payload: {
        container_id: containerId,
        type: 'create',
        session: sessionId.value,
        command: ['/bin/sh'],
      }
    });
  }
};

const onDisconnected = () => {
  connected.value = false;
  connecting.value = false;
};

const onError = (msg: string) => {
  message.error(msg);
  connecting.value = false;
};

onMounted(() => {
  connect();
});
</script>

<style scoped>
.docker-terminal-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: var(--body-bg);
}

.glass-header {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(var(--blur-md));
  border-bottom: 1px solid var(--alpha-black-05);
}

.terminal-wrapper {
  flex: 1;
  background: #1e1e1e;
  border-radius: var(--radius-md);
  margin: 16px;
  padding: 12px;
  min-height: 400px;
  box-shadow: 0 4px 12px var(--alpha-black-10);
  overflow: hidden;
}

</style>

<style>
.dark .docker-terminal-page {
  background-color: #1e1e1e;
}

.dark .glass-header {
  background: rgba(30, 30, 30, 0.7);
  border-bottom: 1px solid var(--alpha-white-05);
}

.dark .ant-page-header-heading-title {
  color: #e0e0e0;
}
</style>
