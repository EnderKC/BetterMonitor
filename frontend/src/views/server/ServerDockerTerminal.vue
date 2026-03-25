<template>
  <div class="server-terminal-page">
    <a-page-header
      class="terminal-header"
      :title="`容器终端 - ${containerName}`"
      @back="router.push({ name: 'ServerDocker', params: { id: serverId } })"
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
          <a-button type="primary" danger @click="disconnect" :disabled="!connected">
            断开连接
          </a-button>
        </a-space>
      </template>
    </a-page-header>

    <div class="main-content">
      <div class="workspace-container">
        <div class="terminal-section">
          <div class="terminal-container-wrapper">
            <div class="terminal-wrapper">
              <TerminalView
                ref="terminalView"
                :socket-url="wsUrl"
                :session="sessionId"
                :container-id="containerId"
                :auto-create="false"
                @connected="onConnected"
                @disconnected="onDisconnected"
                @error="onError"
              />
            </div>
            
            <div class="session-controller">
              <div class="session-select-compact">
                <span v-if="connected" class="connection-status">
                  <a-tag color="processing" size="small">{{ sessionId }}</a-tag>
                  <a-tag color="success" size="small">已连接</a-tag>
                </span>
                <span v-else class="connection-status">
                  <a-tag color="default" size="small">未连接</a-tag>
                </span>
              </div>

              <div class="session-actions-compact">
                <a-space size="small">
                  <a-button size="small" type="primary" @click="reconnect" :disabled="connected" :loading="connecting">
                    连接
                  </a-button>
                  <a-button size="small" danger @click="disconnect" :disabled="!connected">
                    断开
                  </a-button>
                </a-space>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { getToken } from '../../utils/auth';
import { useUIStore } from '../../stores/uiStore';
import TerminalView from '../../components/server/TerminalView.vue';

const uiStore = useUIStore();

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
  uiStore.stopLoading();
});
</script>

<style scoped>
.server-terminal-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background-color: var(--body-bg);
  overflow: hidden;
}

.terminal-header {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-bottom: 1px solid var(--alpha-black-05);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03);
  z-index: 10;
  padding: 12px 24px;
}

.main-content {
  display: flex;
  flex: 1;
  overflow: hidden;
  padding: 16px;
  gap: 16px;
  position: relative;
}

.workspace-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

.terminal-section {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(var(--blur-md));
  border: 1px solid var(--alpha-black-05);
  border-radius: var(--radius-lg);
  padding: 16px;
  box-shadow: 0 8px 32px var(--alpha-black-05);
  overflow: hidden;
}

.terminal-container-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.terminal-wrapper {
  flex: 1;
  background: #1e1e1e;
  border-radius: var(--radius-md);
  overflow: hidden;
  padding: 12px;
  box-shadow: inset 0 0 20px var(--alpha-black-50);
  position: relative;
}

.session-controller {
  margin-top: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: var(--alpha-white-50);
  padding: 8px 12px;
  border-radius: var(--radius-md);
  border: 1px solid var(--alpha-black-05);
}

.session-select-compact,
.session-actions-compact {
  display: flex;
  align-items: center;
  gap: 12px;
}

.connection-status {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>

<style>
/* Dark Mode Global Overrides */
.dark .server-terminal-page {
  background-color: #1e1e1e;
}

.dark .terminal-header,
.dark .terminal-section {
  background: rgba(30, 30, 30, 0.7);
  border-color: var(--alpha-white-10);
}

.dark .session-controller {
  background: var(--alpha-black-20);
  border-color: var(--alpha-white-05);
}

.dark .ant-page-header-heading-title {
  color: #e0e0e0;
}
</style>
