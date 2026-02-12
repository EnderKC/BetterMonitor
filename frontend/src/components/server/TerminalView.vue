<template>
  <div class="terminal-container" ref="terminalContainer">
    <div ref="terminalRef" class="terminal-element"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, nextTick } from 'vue';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';

const props = defineProps<{
  socketUrl: string;
  session?: string;
  theme?: 'light' | 'dark';
}>();

const emit = defineEmits<{
  (e: 'connected'): void;
  (e: 'disconnected'): void;
  (e: 'error', message: string): void;
}>();

const terminalRef = ref<HTMLElement | null>(null);
const terminalContainer = ref<HTMLElement | null>(null);
const terminal = ref<Terminal | null>(null);
const fitAddon = ref<FitAddon | null>(null);
const ws = ref<WebSocket | null>(null);
const connected = ref(false);

// Initialize Terminal
const initTerminal = () => {
  if (terminal.value) {
    terminal.value.dispose();
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
      selectionBackground: 'rgba(255, 255, 255, 0.3)',
    },
    allowTransparency: true,
  });

  const fit = new FitAddon();
  term.loadAddon(fit);

  terminal.value = term;
  fitAddon.value = fit;

  if (terminalRef.value) {
    term.open(terminalRef.value);
    fit.fit();
  }

  // Handle resize
  window.addEventListener('resize', handleResize);
};

// Connect WebSocket
const connect = () => {
  if (!props.socketUrl) return;

  if (ws.value) {
    ws.value.close();
  }

  try {
    const socket = new WebSocket(props.socketUrl);
    ws.value = socket;

    socket.onopen = () => {
      connected.value = true;
      emit('connected');

      // 先发送 create 命令在 Agent 端创建终端会话
      if (props.session) {
        socket.send(JSON.stringify({
          type: 'shell_command',
          payload: {
            type: 'create',
            data: '',
            session: props.session
          }
        }));

        // 延迟发送 resize，确保会话已创建
        setTimeout(() => {
          handleResize();
        }, 100);
      }

      // Handle input
      if (terminal.value) {
        terminal.value.onData((data) => {
          if (socket.readyState === WebSocket.OPEN) {
            if (!props.session) {
              console.warn('Input sent without session');
            }
            socket.send(JSON.stringify({
              type: 'shell_command',
              payload: {
                type: 'input',
                data: data,
                session: props.session || ''
              }
            }));
          }
        });
        terminal.value.focus();
      }
    };

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'output' || msg.type === 'shell_response') { // Support both formats
           if (terminal.value) {
             const content = msg.data || msg.output;
             if (typeof content === 'string') {
                terminal.value.write(content);
             }
           }
        } else if (msg.type === 'error') {
           emit('error', msg.message);
        } else if (msg.type === 'terminal_error') {
           // Agent断连等导致终端不可用
           emit('error', msg.message || 'Agent连接已断开，终端会话不可用');
           connected.value = false;
           emit('disconnected');
        }
      } catch (e) {
        // Handle raw text if not JSON
        if (terminal.value) {
            terminal.value.write(event.data);
        }
      }
    };

    socket.onclose = () => {
      connected.value = false;
      emit('disconnected');
    };

    socket.onerror = (err) => {
      console.error('WebSocket error:', err);
      emit('error', 'Connection error');
    };

  } catch (err) {
    console.error('Connection failed:', err);
    emit('error', 'Failed to establish connection');
  }
};

const handleResize = () => {
  if (!fitAddon.value || !terminal.value || !ws.value || !connected.value) return;

  try {
    fitAddon.value.fit();

    // 只有 session 存在时才发送 resize 命令
    if (!props.session) {
      console.warn('Resize skipped: no session provided');
      return;
    }

    const dims = {
      cols: terminal.value.cols,
      rows: terminal.value.rows
    };

    ws.value.send(JSON.stringify({
      type: 'shell_command',
      payload: {
        type: 'resize',
        data: JSON.stringify(dims),
        session: props.session
      }
    }));
  } catch (e) {
    console.warn('Resize failed:', e);
  }
};

const disconnect = () => {
  if (ws.value) {
    ws.value.close();
    ws.value = null;
  }
  connected.value = false;
};

const send = (data: any) => {
  if (ws.value && ws.value.readyState === WebSocket.OPEN) {
    ws.value.send(typeof data === 'string' ? data : JSON.stringify(data));
  }
};

// Expose methods
defineExpose({
  connect,
  disconnect,
  resize: handleResize,
  terminal,
  send
});

onMounted(() => {
  initTerminal();
  if (props.socketUrl) {
    connect();
  }
});

onUnmounted(() => {
  disconnect();
  window.removeEventListener('resize', handleResize);
  if (terminal.value) {
    terminal.value.dispose();
  }
});

watch(() => props.socketUrl, (newUrl) => {
  if (newUrl) {
    disconnect();
    connect();
  }
});
</script>

<style scoped>
.terminal-container {
  width: 100%;
  height: 100%;
  background: #1e1e1e;
  border-radius: var(--radius-sm);
  overflow: hidden;
  padding: 4px;
}

.terminal-element {
  width: 100%;
  height: 100%;
}
</style>
