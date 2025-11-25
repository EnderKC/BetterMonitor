<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';
import { getToken } from '../../utils/auth';

const route = useRoute();
const router = useRouter();
const serverId = route.params.id as string;
const containerId = route.params.containerId as string;
const containerName = (route.query.name as string) || containerId;

const terminalRef = ref<HTMLElement | null>(null);
const terminal = ref<Terminal | null>(null);
const fitAddon = ref<FitAddon | null>(null);
const ws = ref<WebSocket | null>(null);
const sessionId = ref<string>('');
const connected = ref(false);
const connecting = ref(false);
let inputDisposable: { dispose: () => void } | null = null;

const generateSessionId = () => `docker-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

const initTerminal = () => {
  if (terminal.value) {
    terminal.value.dispose();
    terminal.value = null;
  }
  const term = new Terminal({
    cursorBlink: true,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    fontSize: 14,
    lineHeight: 1.2,
    theme: {
      background: '#1e1e1e',
      foreground: '#ffffff',
      cursor: '#ffffff',
    },
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  terminal.value = term;
  fitAddon.value = fit;
  if (terminalRef.value) {
    term.open(terminalRef.value);
    fit.fit();
  }
};

const detachInput = () => {
  if (inputDisposable) {
    try {
      inputDisposable.dispose();
    } catch (err) {
      console.warn('移除终端输入监听失败:', err);
    }
    inputDisposable = null;
  }
};

const sendShellCommand = (payload: Record<string, any>) => {
  if (ws.value && ws.value.readyState === WebSocket.OPEN) {
    ws.value.send(JSON.stringify({
      type: 'shell_command',
      payload: {
        container_id: containerId,
        ...payload,
      },
    }));
  }
};

const connectTerminal = () => {
  if (!containerId) {
    message.error('缺少容器ID');
    return;
  }
  const token = getToken();
  if (!token) {
    message.error('未登录，无法连接');
    router.push('/login');
    return;
  }
  sessionId.value = generateSessionId();
  connecting.value = true;
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/api/servers/${serverId}/ws?token=${token}&session=${sessionId.value}`;
  const socket = new WebSocket(wsUrl);
  ws.value = socket;

  socket.onopen = () => {
    connecting.value = false;
    connected.value = true;
    sendShellCommand({
      type: 'create',
      session: sessionId.value,
      container_id: containerId,
      command: ['/bin/sh'],
    });

    detachInput();
    if (terminal.value) {
      inputDisposable = terminal.value.onData((data) => {
        sendShellCommand({
          type: 'input',
          data,
          session: sessionId.value,
        });
      });
      terminal.value.focus();
      setTimeout(() => {
        handleResize();
      }, 100);
    }
  };

  socket.onerror = () => {
    connecting.value = false;
    message.error('容器终端连接错误');
  };

  socket.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (!data) return;
      if ((data.session && data.session !== sessionId.value)) {
        return;
      }
      if (data.type === 'shell_response') {
        if (terminal.value && typeof data.data === 'string') {
          terminal.value.write(data.data);
        }
      } else if (data.type === 'shell_error') {
        message.error(data.error || '容器终端错误');
      } else if (data.type === 'shell_close') {
        connected.value = false;
        message.info('容器终端已关闭');
      } else if (data.type === 'error') {
        message.error(data.message || '容器终端发生错误');
      }
    } catch (err) {
      console.error('解析容器终端消息失败:', err);
    }
  };

  socket.onclose = () => {
    connected.value = false;
    detachInput();
  };
};

const disconnectTerminal = () => {
  if (ws.value) {
    try {
      sendShellCommand({ type: 'close', session: sessionId.value });
      ws.value.onclose = null;
      ws.value.close();
    } catch (err) {
      console.warn('关闭容器终端连接失败:', err);
    }
    ws.value = null;
  }
  connected.value = false;
  detachInput();
};

const handleResize = () => {
  if (fitAddon.value) {
    try {
      fitAddon.value.fit();
    } catch (err) {
      console.warn('调整终端大小失败:', err);
    }
  }
  if (terminal.value && connected.value) {
    const dimensions = {
      cols: terminal.value.cols,
      rows: terminal.value.rows,
    };
    sendShellCommand({
      type: 'resize',
      session: sessionId.value,
      data: JSON.stringify(dimensions),
    });
  }
};

const reconnect = () => {
  disconnectTerminal();
  initTerminal();
  connectTerminal();
};

onMounted(() => {
  initTerminal();
  connectTerminal();
  window.addEventListener('resize', handleResize);
});

onUnmounted(() => {
  disconnectTerminal();
  window.removeEventListener('resize', handleResize);
  if (terminal.value) {
    terminal.value.dispose();
    terminal.value = null;
  }
  fitAddon.value = null;
});
</script>

<template>
  <div class="docker-terminal-page">
    <a-page-header
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
          <a-button danger @click="disconnectTerminal" :disabled="!connected">
            断开
          </a-button>
        </a-space>
      </template>
    </a-page-header>

    <div class="terminal-wrapper">
      <div ref="terminalRef" class="terminal"></div>
    </div>
  </div>
</template>

<style scoped>
.docker-terminal-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: #f5f5f5;
}

/* Page Header Dark Mode */

.terminal-wrapper {
  flex: 1;
  background: #1e1e1e;
  border-radius: 8px;
  margin: 16px;
  padding: 12px;
  min-height: 400px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}

.terminal {
  width: 100%;
  height: 100%;
}
</style>
<style>
.dark .docker-terminal-page {
  background-color: #252526;
}

.dark .ant-page-header {
  background-color: #252526;
  border-bottom: 1px solid #333;
}

.dark .ant-page-header-heading-title {
  color: #e0e0e0;
}

.dark .ant-page-header-back-button {
  color: #ccc;
}

.dark .terminal-wrapper {
  background-color: #282c34;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}
</style>
