<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick, h } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal } from 'ant-design-vue';
import {
  ReloadOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  DeleteOutlined,
  PlusOutlined,
  EyeOutlined,
  SearchOutlined,
  SyncOutlined,
  DownloadOutlined,
  FileTextOutlined,
  FolderOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  ClockCircleOutlined,
  QuestionCircleOutlined,
  InfoCircleOutlined,
  CodeOutlined,
  DownOutlined,
  AppstoreOutlined,
  CloudServerOutlined,
  ContainerOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
import { getToken } from '../../utils/auth';
import { useServerStore } from '../../stores/serverStore';
import { useUIStore } from '../../stores/uiStore';
import Convert from 'ansi-to-html';

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));

// 获取服务器状态store
const serverStore = useServerStore();
const uiStore = useUIStore();

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// 标签页
const activeKey = ref('containers');

// 容器列表
const containers = ref<any[]>([]);
const containersLoading = ref(false);
const containerSearch = ref('');

// 镜像列表
const images = ref<any[]>([]);
const imagesLoading = ref(false);
const imageSearch = ref('');

// Compose列表
const composes = ref<any[]>([]);
const composesLoading = ref(false);

// 创建容器表单
const createContainerVisible = ref(false);
const containerForm = reactive({
  name: '',
  image: '',
  ports: [{ hostPort: '', containerPort: '' }],
  volumes: [{ hostPath: '', containerPath: '' }],
  envs: [{ key: '', value: '' }],
  command: '',
  restart: 'no',
  network: 'bridge'
});

// 拉取镜像表单
const pullImageVisible = ref(false);
const pullForm = ref('');
const pullLoading = ref(false);

// Compose表单
const composeFormVisible = ref(false);
const composeForm = reactive({
  name: '',
  content: ''
});

// 创建ANSI转HTML转换器实例
const ansiConverter = new Convert({
  newline: true,
  escapeXML: true,
  colors: {
    0: '#000',
    1: '#A00',
    2: '#0A0',
    3: '#A50',
    4: '#00A',
    5: '#A0A',
    6: '#0AA',
    7: '#AAA',
    8: '#555',
    9: '#F55',
    10: '#5F5',
    11: '#FF5',
    12: '#55F',
    13: '#F5F',
    14: '#5FF',
    15: '#FFF'
  }
});

// 计算服务器是否在线 (使用全局状态)
const isServerOnline = computed(() => {
  return serverStore.isServerOnline(serverId.value);
});

// 解析Docker容器状态
const parseContainerStatus = (status: string): string => {
  if (!status) return 'unknown';

  const lowerStatus = status.toLowerCase();

  if (lowerStatus.startsWith('up')) return 'running';
  if (lowerStatus.startsWith('exited')) return 'exited';
  if (lowerStatus.startsWith('created')) return 'created';
  if (lowerStatus.startsWith('paused')) return 'paused';
  if (lowerStatus.startsWith('restarting')) return 'restarting';
  if (lowerStatus.startsWith('removing')) return 'removing';
  if (lowerStatus.startsWith('dead')) return 'dead';

  return 'unknown';
};

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}`);
    if (response && response.server) {
      serverInfo.value = response.server;
      const status = response.server.status || 'offline';
      serverStore.updateServerStatus(serverId.value, status);
    } else {
      message.error('获取服务器数据失败');
    }
  } catch (error) {
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
    uiStore.stopLoading();
  }
};

// 获取容器列表
const fetchContainers = async () => {
  if (!isServerOnline.value) return;
  containersLoading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/containers`);
    if (Array.isArray(response)) {
      containers.value = response;
    } else if (response && response.data && Array.isArray(response.data)) {
      containers.value = response.data;
    } else if (response && response.containers) {
      containers.value = Array.isArray(response.containers) ? response.containers : [];
    } else {
      containers.value = [];
    }
  } catch (error) {
    containers.value = [];
  } finally {
    containersLoading.value = false;
  }
};

const openContainerTerminal = (container: any) => {
  const id = container.id || container.ID;
  if (!id) return message.warning('无法识别容器ID');
  const name = container.name || container.Names?.[0] || id;
  router.push({
    name: 'ServerDockerTerminal',
    params: { id: serverId.value, containerId: id },
    query: { name },
  });
};

const openContainerFile = (container: any) => {
  const id = container.id || container.ID;
  if (!id) return message.warning('无法识别容器ID');
  const name = container.name || container.Names?.[0] || id;
  router.push({
    name: 'ServerDockerFile',
    params: { id: serverId.value, containerId: id },
    query: { name },
  });
};

// 获取镜像列表
const fetchImages = async () => {
  if (!isServerOnline.value) return;
  imagesLoading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/images`);
    if (Array.isArray(response)) {
      images.value = response;
    } else if (response && response.data && Array.isArray(response.data)) {
      images.value = response.data;
    } else if (response && response.images) {
      images.value = Array.isArray(response.images) ? response.images : [];
    } else {
      images.value = [];
    }
  } catch (error) {
    images.value = [];
  } finally {
    imagesLoading.value = false;
  }
};

// 获取Compose列表
const fetchComposes = async () => {
  if (!isServerOnline.value) return;
  composesLoading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/composes`);
    if (Array.isArray(response)) {
      composes.value = response;
    } else if (response && response.data && Array.isArray(response.data)) {
      composes.value = response.data;
    } else if (response && response.composes) {
      composes.value = Array.isArray(response.composes) ? response.composes : [];
    } else {
      composes.value = [];
    }
  } catch (error) {
    composes.value = [];
  } finally {
    composesLoading.value = false;
  }
};

// 过滤容器列表
const filteredContainers = computed(() => {
  if (!containerSearch.value) return containers.value;
  return containers.value.filter(container =>
    container.name.toLowerCase().includes(containerSearch.value.toLowerCase()) ||
    container.image.toLowerCase().includes(containerSearch.value.toLowerCase()) ||
    container.id.includes(containerSearch.value) ||
    (container.state && container.state.toLowerCase().includes(containerSearch.value.toLowerCase())) ||
    (container.status && container.status.toLowerCase().includes(containerSearch.value.toLowerCase()))
  );
});

// 过滤镜像列表
const filteredImages = computed(() => {
  if (!imageSearch.value) return images.value;
  return images.value.filter(image =>
    (image.repository && image.repository.toLowerCase().includes(imageSearch.value.toLowerCase())) ||
    (image.tag && image.tag.toLowerCase().includes(imageSearch.value.toLowerCase())) ||
    (image.id && image.id.includes(imageSearch.value))
  );
});

// 容器操作
const startContainer = async (id: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  try {
    await request.post(`/servers/${serverId.value}/docker/containers/${id}/start`);
    message.success('容器已启动');
    fetchContainers();
  } catch (error) {
    message.error('启动容器失败');
  }
};

const stopContainer = async (id: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  try {
    await request.post(`/servers/${serverId.value}/docker/containers/${id}/stop`);
    message.success('容器已停止');
    fetchContainers();
  } catch (error) {
    message.error('停止容器失败');
  }
};

const restartContainer = async (id: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  try {
    await request.post(`/servers/${serverId.value}/docker/containers/${id}/restart`);
    message.success('容器已重启');
    fetchContainers();
  } catch (error) {
    message.error('重启容器失败');
  }
};

const removeContainer = (id: string, name: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  const container = containers.value.find(c => c.id === id);
  const isRunning = container && (container.state === 'running' || container.status.toLowerCase().includes('up'));

  if (isRunning) {
    Modal.confirm({
      title: '容器正在运行',
      content: h('div', [
        h('p', `容器 ${name} 当前正在运行中。`),
        h('p', '您可以选择:'),
        h('ul', { style: { paddingLeft: '20px', marginTop: '10px' } }, [
          h('li', '强制删除(会立即停止并删除容器)'),
          h('li', '先停止容器再删除')
        ])
      ]),
      okText: '强制删除',
      cancelText: '先停止',
      okType: 'danger',
      onOk: async () => {
        try {
          await request.delete(`/servers/${serverId.value}/docker/containers/${id}?force=true`);
          message.success('容器已强制删除');
          fetchContainers();
        } catch (error: any) {
          message.error(error.response?.data?.error || '强制删除容器失败');
        }
      },
      onCancel: async () => {
        try {
          await request.post(`/servers/${serverId.value}/docker/containers/${id}/stop`);
          message.info('容器已停止，正在删除...');
          await request.delete(`/servers/${serverId.value}/docker/containers/${id}`);
          message.success('容器已删除');
          fetchContainers();
        } catch (error: any) {
          message.error(error.response?.data?.error || '操作失败');
        }
      }
    });
  } else {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除容器 ${name} 吗？此操作不可恢复。`,
      okText: '确认',
      cancelText: '取消',
      okType: 'danger',
      onOk: async () => {
        try {
          await request.delete(`/servers/${serverId.value}/docker/containers/${id}`);
          message.success('容器已删除');
          fetchContainers();
        } catch (error: any) {
          message.error(error.response?.data?.error || '删除容器失败');
        }
      }
    });
  }
};

// ==================== WebSocket 连接管理 ====================
const ws = ref<WebSocket | null>(null);
const wsConnected = ref(false);

const connectWebSocket = () => {
  const token = getToken();
  if (!token) return;
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const url = `${protocol}//${host}/api/servers/${serverId.value}/ws?token=${encodeURIComponent(token)}`;

  const socket = new WebSocket(url);
  socket.onopen = () => { wsConnected.value = true; };
  socket.onclose = () => { wsConnected.value = false; ws.value = null; };
  socket.onerror = () => { wsConnected.value = false; };
  socket.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type === 'docker_logs_stream_data' && msg.stream_id === logStreamId.value) {
        onLogStreamData(msg.data?.logs || '');
      } else if (msg.type === 'docker_logs_stream_end' && msg.stream_id === logStreamId.value) {
        onLogStreamEnd(msg.data?.reason || '');
      }
    } catch { /* 忽略非 JSON 消息 */ }
  };
  ws.value = socket;
};

const disconnectWebSocket = () => {
  if (ws.value) {
    ws.value.close();
    ws.value = null;
  }
  wsConnected.value = false;
};

const sendWsMessage = (data: any) => {
  if (ws.value && ws.value.readyState === WebSocket.OPEN) {
    ws.value.send(JSON.stringify(data));
  }
};

// ==================== 实时日志流 ====================
const logDrawerVisible = ref(false);
const currentLogContainerId = ref('');
const currentLogContainerName = ref('');
const logStreamId = ref('');
const logLines = ref<string[]>([]);
const logStreaming = ref(false);
const logAutoScroll = ref(true);
const logContainerRef = ref<HTMLElement | null>(null);
const LOG_MAX_LINES = 5000;

// 日志级别颜色映射
const LOG_LEVEL_COLORS: Record<string, string> = {
  'FATAL': '#ff4d4f',
  'ERROR': '#ff4d4f',
  'WARN':  '#faad14',
  'WARNING': '#faad14',
  'INFO':  '#1890ff',
  'DEBUG': '#8c8c8c',
  'TRACE': '#595959',
};

// 检测日志行的级别
const detectLogLevel = (line: string): string | null => {
  // 匹配常见日志格式中的级别关键字（大小写不敏感）
  const match = line.match(/\b(FATAL|ERROR|WARN(?:ING)?|INFO|DEBUG|TRACE)\b/i);
  return match ? match[1].toUpperCase().replace('WARNING', 'WARN') : null;
};

// 将单行日志转为带高亮的 HTML
const renderLogLine = (line: string): string => {
  const escaped = ansiConverter.toHtml(line);
  const level = detectLogLevel(line);
  if (!level || !LOG_LEVEL_COLORS[level]) return escaped;
  const color = LOG_LEVEL_COLORS[level];
  return `<span style="display:inline-block;width:4px;height:1em;background:${color};border-radius:2px;margin-right:8px;vertical-align:middle;"></span>${escaped}`;
};

// 批量渲染缓冲
let pendingLines: string[] = [];
let flushTimer: ReturnType<typeof setTimeout> | null = null;

const flushPendingLines = () => {
  if (pendingLines.length === 0) return;
  const newLines = pendingLines.splice(0);
  const current = logLines.value;
  const combined = current.concat(newLines);
  // 环形缓冲：超过上限则丢弃旧行
  logLines.value = combined.length > LOG_MAX_LINES
    ? combined.slice(combined.length - LOG_MAX_LINES)
    : combined;
  if (logAutoScroll.value) {
    nextTick(scrollToBottom);
  }
};

const onLogStreamData = (logs: string) => {
  if (!logs) return;
  const lines = logs.split('\n').filter(l => l.length > 0);
  pendingLines.push(...lines);
  // 100ms 批量刷新
  if (!flushTimer) {
    flushTimer = setTimeout(() => {
      flushTimer = null;
      flushPendingLines();
    }, 100);
  }
};

const onLogStreamEnd = (reason: string) => {
  logStreaming.value = false;
  flushPendingLines();
  const hint = reason === 'container_stopped' ? '容器已停止' : reason;
  logLines.value.push(`\n--- 日志流已结束: ${hint} ---`);
  if (logAutoScroll.value) nextTick(scrollToBottom);
};

const scrollToBottom = () => {
  if (logContainerRef.value) {
    logContainerRef.value.scrollTop = logContainerRef.value.scrollHeight;
  }
};

// 滚动事件：检测用户是否手动滚动（非底部则暂停自动滚动）
const onLogScroll = () => {
  if (!logContainerRef.value) return;
  const el = logContainerRef.value;
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
  logAutoScroll.value = atBottom;
};

// 打开日志 Drawer
const viewContainerLogs = async (id: string, name: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');

  currentLogContainerId.value = id;
  currentLogContainerName.value = name;
  logLines.value = [];
  logStreaming.value = true;
  logAutoScroll.value = true;
  logDrawerVisible.value = true;

  // 确保 WebSocket 已连接
  if (!ws.value || ws.value.readyState !== WebSocket.OPEN) {
    connectWebSocket();
    // 等待连接就绪
    await new Promise<void>((resolve) => {
      const check = setInterval(() => {
        if (ws.value && ws.value.readyState === WebSocket.OPEN) {
          clearInterval(check);
          resolve();
        }
      }, 100);
      // 超时 5 秒
      setTimeout(() => { clearInterval(check); resolve(); }, 5000);
    });
  }

  // 发送 start 消息
  const streamId = crypto.randomUUID();
  logStreamId.value = streamId;
  sendWsMessage({
    type: 'docker_logs_stream',
    payload: {
      action: 'start',
      stream_id: streamId,
      container_id: id,
      tail: 200,
      timestamps: true,
    },
  });
};

// 关闭日志 Drawer
const closeLogDrawer = () => {
  // 发送 stop 消息
  if (logStreamId.value && logStreaming.value) {
    sendWsMessage({
      type: 'docker_logs_stream',
      payload: {
        action: 'stop',
        stream_id: logStreamId.value,
      },
    });
  }
  logStreaming.value = false;
  logStreamId.value = '';
  logDrawerVisible.value = false;
  pendingLines = [];
  if (flushTimer) { clearTimeout(flushTimer); flushTimer = null; }
};

// 清空日志
const clearLogs = () => {
  logLines.value = [];
};

// ==================== 镜像操作 ====================
const removeImage = (id: string, name: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  Modal.confirm({
    title: '确认删除',
    content: `确定要删除镜像 ${name} 吗？此操作不可恢复。`,
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await request.delete(`/servers/${serverId.value}/docker/images/${id}`);
        message.success('镜像已删除');
        fetchImages();
      } catch (error) {
        message.error('删除镜像失败');
      }
    }
  });
};

const pullImage = async () => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  if (!pullForm.value) return message.error('请输入要拉取的镜像名称');
  pullLoading.value = true;
  try {
    await request.post(`/servers/${serverId.value}/docker/images/pull`, { image: pullForm.value });
    message.success('镜像拉取任务已提交');
    pullImageVisible.value = false;
    pullForm.value = '';
    setTimeout(() => { fetchImages(); }, 3000);
  } catch (error) {
    message.error('拉取镜像失败');
  } finally {
    pullLoading.value = false;
  }
};

// Compose操作
const composeUp = async (name: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  try {
    await request.post(`/servers/${serverId.value}/docker/composes/${name}/up`);
    message.success('Compose服务已启动');
    fetchComposes();
  } catch (error) {
    message.error('启动Compose服务失败');
  }
};

const composeDown = async (name: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  try {
    await request.post(`/servers/${serverId.value}/docker/composes/${name}/down`);
    message.success('Compose服务已停止');
    fetchComposes();
  } catch (error) {
    message.error('停止Compose服务失败');
  }
};

const removeCompose = (name: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  Modal.confirm({
    title: '确认删除',
    content: `确定要删除Compose项目 ${name} 吗？此操作不可恢复。`,
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await request.delete(`/servers/${serverId.value}/docker/composes/${name}`);
        message.success('Compose项目已删除');
        fetchComposes();
      } catch (error) {
        message.error('删除Compose项目失败');
      }
    }
  });
};

const viewComposeConfig = async (name: string) => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/composes/${name}/config`);
    let config = '';
    if (typeof response === 'string') config = response;
    else if (response && typeof response.data === 'string') config = response.data;
    else if (response && response.config && typeof response.config === 'string') config = response.config;
    else config = '无配置数据或格式不正确';

    Modal.info({
      title: `Compose配置 - ${name}`,
      width: 800,
      content: h('div', [
        h('pre', {
          style: {
            maxHeight: '500px',
            overflow: 'auto',
            background: '#1e1e1e',
            padding: '16px',
            borderRadius: '8px',
            fontFamily: '"SF Mono", Menlo, monospace',
            color: '#d4d4d4',
            fontSize: '13px',
            lineHeight: '1.5'
          }
        }, config)
      ]),
      okText: '关闭',
      class: 'glass-modal'
    });
  } catch (error) {
    message.error('获取Compose配置失败');
  }
};

const createCompose = async () => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  if (!composeForm.name || !composeForm.content) return message.error('请填写完整信息');
  try {
    await request.post(`/servers/${serverId.value}/docker/composes`, composeForm);
    message.success('Compose项目已创建');
    composeFormVisible.value = false;
    composeForm.name = '';
    composeForm.content = '';
    fetchComposes();
  } catch (error) {
    message.error('创建Compose项目失败');
  }
};

// 容器表单操作
const addPortMapping = () => { containerForm.ports.push({ hostPort: '', containerPort: '' }); };
const removePortMapping = (index: number) => { containerForm.ports.splice(index, 1); };
const addVolumeMapping = () => { containerForm.volumes.push({ hostPath: '', containerPath: '' }); };
const removeVolumeMapping = (index: number) => { containerForm.volumes.splice(index, 1); };
const addEnvVar = () => { containerForm.envs.push({ key: '', value: '' }); };
const removeEnvVar = (index: number) => { containerForm.envs.splice(index, 1); };

const resetContainerForm = () => {
  containerForm.name = '';
  containerForm.image = '';
  containerForm.ports = [{ hostPort: '', containerPort: '' }];
  containerForm.volumes = [{ hostPath: '', containerPath: '' }];
  containerForm.envs = [{ key: '', value: '' }];
  containerForm.command = '';
  containerForm.restart = 'no';
  containerForm.network = 'bridge';
};

const createContainer = async () => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  if (!containerForm.name || !containerForm.image) return message.error('请填写必要信息');

  try {
    const ports = containerForm.ports.filter(p => p.hostPort && p.containerPort).map(p => `${p.hostPort}:${p.containerPort}`);
    const volumes = containerForm.volumes.filter(v => v.hostPath && v.containerPath).map(v => `${v.hostPath}:${v.containerPath}`);
    const env: Record<string, string> = {};
    containerForm.envs.forEach(item => { if (item.key) env[item.key] = item.value || ''; });

    await request.post(`/servers/${serverId.value}/docker/containers`, {
      name: containerForm.name,
      image: containerForm.image,
      ports, volumes, env,
      command: containerForm.command,
      restart: containerForm.restart,
      network: containerForm.network
    });

    message.success('容器创建成功');
    createContainerVisible.value = false;
    resetContainerForm();
    fetchContainers();
  } catch (error) {
    message.error('创建容器失败');
  }
};

// 辅助函数
const goBack = () => { router.push(`/admin/servers/${serverId.value}`); };
const refreshData = () => {
  if (!isServerOnline.value) return message.warning('服务器离线');
  if (activeKey.value === 'containers') fetchContainers();
  else if (activeKey.value === 'images') fetchImages();
  else if (activeKey.value === 'composes') fetchComposes();
};

const formatTime = (timestamp: string) => new Date(timestamp).toLocaleString();

// 截断路径显示（保留首尾，中间用...）
const truncatePath = (path: string, maxLength: number = 30): string => {
  if (!path || path.length <= maxLength) return path;
  const parts = path.split('/').filter(Boolean);
  if (parts.length <= 2) return path;
  // 保留第一个和最后一个目录
  return `/${parts[0]}/.../${parts[parts.length - 1]}`;
};

// 跳转到文件管理页面
const navigateToFileManager = (path: string) => {
  if (!path) return;
  router.push({
    name: 'ServerFile',
    params: { id: serverId.value },
    query: { path, from: 'docker' }
  });
};

const containerStatusText = (status: string) => {
  const s = parseContainerStatus(status);
  const map: Record<string, string> = {
    'running': '运行中', 'exited': '已停止', 'created': '已创建',
    'paused': '已暂停', 'restarting': '重启中', 'removing': '移除中', 'dead': '已死亡'
  };
  return map[s] || status;
};
const containerStatusColor = (status: string) => {
  const s = parseContainerStatus(status);
  const map: Record<string, string> = {
    'running': 'success', 'exited': 'error', 'created': 'processing',
    'paused': 'warning', 'restarting': 'warning', 'removing': 'default', 'dead': 'default'
  };
  return map[s] || 'default';
};
const isContainerActionable = (status: string) => !['removing', 'dead'].includes(parseContainerStatus(status));
const onTabChange = (key: string) => {
  activeKey.value = key;
  if (key === 'containers') fetchContainers();
  else if (key === 'images') fetchImages();
  else if (key === 'composes') fetchComposes();
};
const getContainerStatusIcon = (status: string) => {
  const s = parseContainerStatus(status);
  const map: Record<string, any> = {
    'running': CheckCircleOutlined, 'exited': CloseCircleOutlined, 'created': ClockCircleOutlined,
    'paused': PauseCircleOutlined, 'restarting': SyncOutlined, 'removing': DeleteOutlined, 'dead': ExclamationCircleOutlined
  };
  return h(map[s] || QuestionCircleOutlined);
};

onMounted(() => {
  fetchServerInfo().then(() => {
    if (isServerOnline.value) {
      fetchContainers();
      fetchImages();
      fetchComposes();
      connectWebSocket();
    }
  });
});

onUnmounted(() => {
  closeLogDrawer();
  disconnectWebSocket();
});
</script>

<template>
  <div class="docker-page">
    <a-page-header class="glass-header" :title="serverInfo.name || `服务器 ${serverId}`" sub-title="Docker管理"
      @back="goBack">
      <template #tags>
        <a-tag :color="isServerOnline ? 'success' : 'error'">
          {{ isServerOnline ? '在线' : '离线' }}
        </a-tag>
      </template>
      <template #extra>
        <a-button type="primary" @click="refreshData" class="glass-button">
          <template #icon>
            <ReloadOutlined />
          </template>
          刷新
        </a-button>
      </template>
    </a-page-header>

    <div class="main-content">
      <a-spin :spinning="loading">
        <a-alert v-if="!isServerOnline" type="warning" show-icon message="服务器当前离线，无法使用Docker管理功能"
          style="margin-bottom: 24px" class="glass-alert" />

        <div v-else class="glass-panel">
          <a-tabs v-model:activeKey="activeKey" @change="onTabChange" class="custom-tabs">
            <!-- 容器管理 -->
            <a-tab-pane key="containers">
              <template #tab>
                <span>
                  <ContainerOutlined /> 容器管理
                </span>
              </template>
              <div class="tab-content">
                <div class="toolbar">
                  <a-input-search v-model:value="containerSearch" placeholder="搜索容器..." style="width: 300px"
                    class="glass-input" />
                  <a-button type="primary" @click="createContainerVisible = true" class="action-button">
                    <template #icon>
                      <PlusOutlined />
                    </template>
                    创建容器
                  </a-button>
                </div>

                <a-table :dataSource="filteredContainers" :loading="containersLoading" :pagination="{ pageSize: 10 }"
                  rowKey="id" class="glass-table">
                  <a-table-column title="ID" dataIndex="id" width="120">
                    <template #default="{ text }"><span class="mono-text">{{ text.substring(0, 12) }}</span></template>
                  </a-table-column>
                  <a-table-column title="名称" dataIndex="name">
                    <template #default="{ text }"><span class="name-text">{{ text }}</span></template>
                  </a-table-column>
                  <a-table-column title="镜像" dataIndex="image">
                    <template #default="{ text }"><a-tag color="blue">{{ text }}</a-tag></template>
                  </a-table-column>
                  <a-table-column title="状态" dataIndex="status">
                    <template #default="{ text }">
                      <a-tag :color="containerStatusColor(text)">
                        <component :is="getContainerStatusIcon(text)" /> {{ containerStatusText(text) }}
                      </a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column title="端口" dataIndex="ports">
                    <template #default="{ text }">
                      <div v-if="text && text.length" class="ports-list">
                        <a-tag v-for="port in text" :key="port" class="port-tag">{{ port }}</a-tag>
                      </div>
                      <span v-else class="text-secondary">-</span>
                    </template>
                  </a-table-column>
                  <a-table-column title="操作" width="120">
                    <template #default="{ record }">
                      <a-dropdown>
                        <a-button type="link" size="small">操作
                          <DownOutlined />
                        </a-button>
                        <template #overlay>
                          <a-menu>
                            <a-menu-item v-if="parseContainerStatus(record.status) !== 'running'" key="start"
                              :disabled="!isContainerActionable(record.status)" @click="startContainer(record.id)">
                              <PlayCircleOutlined /> 启动
                            </a-menu-item>
                            <a-menu-item v-if="parseContainerStatus(record.status) === 'running'" key="stop"
                              :disabled="!isContainerActionable(record.status)" @click="stopContainer(record.id)">
                              <PauseCircleOutlined /> 停止
                            </a-menu-item>
                            <a-menu-item v-if="parseContainerStatus(record.status) === 'running'" key="restart"
                              :disabled="!isContainerActionable(record.status)" @click="restartContainer(record.id)">
                              <SyncOutlined /> 重启
                            </a-menu-item>
                            <a-menu-divider />
                            <a-menu-item key="terminal" @click="openContainerTerminal(record)">
                              <CodeOutlined /> 终端
                            </a-menu-item>
                            <a-menu-item key="file" @click="openContainerFile(record)">
                              <FolderOutlined /> 文件
                            </a-menu-item>
                            <a-menu-item key="logs" @click="viewContainerLogs(record.id, record.name)">
                              <EyeOutlined /> 日志
                            </a-menu-item>
                            <a-menu-divider />
                            <a-menu-item key="delete" danger
                              :disabled="parseContainerStatus(record.status) === 'removing'"
                              @click="removeContainer(record.id, record.name)">
                              <DeleteOutlined /> 删除
                            </a-menu-item>
                          </a-menu>
                        </template>
                      </a-dropdown>
                    </template>
                  </a-table-column>
                </a-table>
              </div>
            </a-tab-pane>

            <!-- 镜像管理 -->
            <a-tab-pane key="images">
              <template #tab>
                <span>
                  <AppstoreOutlined /> 镜像管理
                </span>
              </template>
              <div class="tab-content">
                <div class="toolbar">
                  <a-input-search v-model:value="imageSearch" placeholder="搜索镜像..." style="width: 300px"
                    class="glass-input" />
                  <a-button type="primary" @click="pullImageVisible = true" class="action-button">
                    <template #icon>
                      <DownloadOutlined />
                    </template>
                    拉取镜像
                  </a-button>
                </div>

                <a-table :dataSource="filteredImages" :loading="imagesLoading" :pagination="{ pageSize: 10 }"
                  rowKey="id" class="glass-table">
                  <a-table-column title="ID" dataIndex="id" width="120">
                    <template #default="{ text }"><span class="mono-text">{{ text.substring(0, 12) }}</span></template>
                  </a-table-column>
                  <a-table-column title="仓库" dataIndex="repository">
                    <template #default="{ text }"><span class="name-text">{{ text }}</span></template>
                  </a-table-column>
                  <a-table-column title="标签" dataIndex="tag">
                    <template #default="{ text }"><a-tag>{{ text }}</a-tag></template>
                  </a-table-column>
                  <a-table-column title="大小" dataIndex="size">
                    <template #default="{ text }">{{ (text / (1024 * 1024)).toFixed(2) }} MB</template>
                  </a-table-column>
                  <a-table-column title="创建时间" dataIndex="created">
                    <template #default="{ text }">{{ formatTime(text) }}</template>
                  </a-table-column>
                  <a-table-column title="操作">
                    <template #default="{ record }">
                      <a-button type="text" danger size="small"
                        @click="removeImage(record.id, `${record.repository}:${record.tag}`)">
                        <template #icon>
                          <DeleteOutlined />
                        </template> 删除
                      </a-button>
                    </template>
                  </a-table-column>
                </a-table>
              </div>
            </a-tab-pane>

            <!-- Compose管理 -->
            <a-tab-pane key="composes">
              <template #tab>
                <span>
                  <CloudServerOutlined /> Compose管理
                </span>
              </template>
              <div class="tab-content">
                <div class="toolbar">
                  <div class="spacer"></div>
                  <a-button type="primary" @click="composeFormVisible = true" class="action-button">
                    <template #icon>
                      <PlusOutlined />
                    </template>
                    创建项目
                  </a-button>
                </div>

                <a-table :dataSource="composes" :loading="composesLoading" :pagination="{ pageSize: 10 }" rowKey="name"
                  class="glass-table">
                  <a-table-column title="名称" dataIndex="name">
                    <template #default="{ text }"><span class="name-text">{{ text }}</span></template>
                  </a-table-column>
                  <a-table-column title="状态" dataIndex="status">
                    <template #default="{ text }">
                      <a-tag :color="text === 'running' ? 'success' : 'error'">
                        {{ text === 'running' ? '运行中' : '已停止' }}
                      </a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column title="容器数" dataIndex="container_count" />
                  <a-table-column title="工作目录" dataIndex="working_dir">
                    <template #default="{ text }">
                      <a-tooltip v-if="text" :title="`点击跳转到文件管理: ${text}`">
                        <a @click="navigateToFileManager(text)" class="working-dir-link">
                          <FolderOutlined style="margin-right: 4px" />
                          {{ truncatePath(text) }}
                        </a>
                      </a-tooltip>
                      <span v-else class="text-muted">-</span>
                    </template>
                  </a-table-column>
                  <a-table-column title="更新时间" dataIndex="updated_at">
                    <template #default="{ text }">{{ formatTime(text) }}</template>
                  </a-table-column>
                  <a-table-column title="操作">
                    <template #default="{ record }">
                      <a-space>
                        <a-button v-if="record.status !== 'running'" type="link" size="small"
                          @click="composeUp(record.name)">
                          启动
                        </a-button>
                        <a-button v-if="record.status === 'running'" type="link" danger size="small"
                          @click="composeDown(record.name)">
                          停止
                        </a-button>
                        <a-button type="link" size="small" @click="viewComposeConfig(record.name)">配置</a-button>
                        <a-popconfirm title="确定要删除此项目吗？" ok-text="删除" cancel-text="取消"
                          @confirm="removeCompose(record.name)">
                          <a-button type="link" danger size="small">删除</a-button>
                        </a-popconfirm>
                      </a-space>
                    </template>
                  </a-table-column>
                </a-table>
              </div>
            </a-tab-pane>
          </a-tabs>
        </div>
      </a-spin>
    </div>

    <!-- 模态框组件 (保持原有逻辑，仅添加样式类) -->
    <a-modal v-model:visible="pullImageVisible" title="拉取镜像" @ok="pullImage" :confirmLoading="pullLoading"
      :maskClosable="false" class="glass-modal">
      <a-form layout="vertical">
        <a-form-item label="镜像名称" required>
          <a-input v-model:value="pullForm" placeholder="例如：nginx:latest" @pressEnter="pullImage" />
          <div class="form-help">格式：repository:tag (默认latest)</div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="composeFormVisible" title="创建Compose项目" width="700px" @ok="createCompose"
      :maskClosable="false" class="glass-modal">
      <a-form layout="vertical">
        <a-form-item label="项目名称" required>
          <a-input v-model:value="composeForm.name" placeholder="输入项目名称" />
        </a-form-item>
        <a-form-item label="docker-compose.yml内容" required>
          <a-textarea v-model:value="composeForm.content" placeholder="输入YAML内容" :rows="15"
            :autoSize="{ minRows: 15, maxRows: 25 }" class="code-textarea" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="createContainerVisible" title="创建容器" width="700px" @ok="createContainer"
      :maskClosable="false" class="glass-modal">
      <a-form layout="vertical">
        <a-form-item label="容器名称" required>
          <a-input v-model:value="containerForm.name" placeholder="输入容器名称" />
        </a-form-item>
        <a-form-item label="镜像" required>
          <a-input-group compact>
            <a-select style="width: 75%" v-model:value="containerForm.image" placeholder="选择或输入镜像" show-search
              :filter-option="false" @search="(v: string) => containerForm.image = v">
              <a-select-option v-for="image in images" :key="image.id" :value="`${image.repository}:${image.tag}`">
                {{ image.repository }}:{{ image.tag }}
              </a-select-option>
            </a-select>
            <a-button style="width: 25%" @click="fetchImages" :loading="imagesLoading">
              <ReloadOutlined />
            </a-button>
          </a-input-group>
        </a-form-item>
        <!-- 端口映射等复杂表单项保持原样，样式由全局CSS控制 -->
        <a-divider orientation="left">端口映射</a-divider>
        <div v-for="(port, index) in containerForm.ports" :key="'port-' + index" class="form-row">
          <a-input v-model:value="port.hostPort" placeholder="主机端口" style="width: 40%" />
          <span class="separator">:</span>
          <a-input v-model:value="port.containerPort" placeholder="容器端口" style="width: 40%" />
          <a-button v-if="index === 0" type="primary" shape="circle" size="small" @click="addPortMapping">
            <PlusOutlined />
          </a-button>
          <a-button v-else type="danger" shape="circle" size="small" @click="removePortMapping(index)">
            <DeleteOutlined />
          </a-button>
        </div>

        <a-divider orientation="left">数据卷</a-divider>
        <div v-for="(volume, index) in containerForm.volumes" :key="'volume-' + index" class="form-row">
          <a-input v-model:value="volume.hostPath" placeholder="主机路径" style="width: 40%" />
          <span class="separator">:</span>
          <a-input v-model:value="volume.containerPath" placeholder="容器路径" style="width: 40%" />
          <a-button v-if="index === 0" type="primary" shape="circle" size="small" @click="addVolumeMapping">
            <PlusOutlined />
          </a-button>
          <a-button v-else type="danger" shape="circle" size="small" @click="removeVolumeMapping(index)">
            <DeleteOutlined />
          </a-button>
        </div>

        <a-divider orientation="left">环境变量</a-divider>
        <div v-for="(env, index) in containerForm.envs" :key="'env-' + index" class="form-row">
          <a-input v-model:value="env.key" placeholder="变量名" style="width: 40%" />
          <span class="separator">=</span>
          <a-input v-model:value="env.value" placeholder="变量值" style="width: 40%" />
          <a-button v-if="index === 0" type="primary" shape="circle" size="small" @click="addEnvVar">
            <PlusOutlined />
          </a-button>
          <a-button v-else type="danger" shape="circle" size="small" @click="removeEnvVar(index)">
            <DeleteOutlined />
          </a-button>
        </div>

        <a-divider orientation="left">高级设置</a-divider>
        <a-form-item label="启动命令">
          <a-input v-model:value="containerForm.command" placeholder="可选" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="重启策略">
              <a-select v-model:value="containerForm.restart">
                <a-select-option value="no">不自动重启</a-select-option>
                <a-select-option value="always">总是重启</a-select-option>
                <a-select-option value="on-failure">失败时重启</a-select-option>
                <a-select-option value="unless-stopped">除非手动停止</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="网络模式">
              <a-select v-model:value="containerForm.network">
                <a-select-option value="bridge">桥接(bridge)</a-select-option>
                <a-select-option value="host">主机(host)</a-select-option>
                <a-select-option value="none">无网络(none)</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>

    <!-- 实时日志 Drawer -->
    <a-drawer
      v-model:open="logDrawerVisible"
      :title="`容器日志 - ${currentLogContainerName}`"
      placement="right"
      :width="720"
      :destroyOnClose="true"
      class="log-drawer"
      @close="closeLogDrawer"
    >
      <template #extra>
        <a-space>
          <a-tag :color="logStreaming ? 'green' : 'default'">
            {{ logStreaming ? '实时' : '已停止' }}
          </a-tag>
          <a-tooltip :title="logAutoScroll ? '自动滚动已开启' : '自动滚动已暂停（滚动到底部恢复）'">
            <a-button size="small" :type="logAutoScroll ? 'primary' : 'default'" @click="logAutoScroll = !logAutoScroll; if(logAutoScroll) scrollToBottom()">
              <template #icon><DownOutlined /></template>
            </a-button>
          </a-tooltip>
          <a-button size="small" @click="clearLogs">清空</a-button>
        </a-space>
      </template>

      <div
        ref="logContainerRef"
        class="log-container"
        @scroll="onLogScroll"
      >
        <div
          v-for="(line, idx) in logLines"
          :key="idx"
          class="log-line"
          v-html="renderLogLine(line)"
        />
        <div v-if="logLines.length === 0 && logStreaming" class="log-placeholder">
          等待日志数据...
        </div>
        <div v-if="!logStreaming && logLines.length === 0" class="log-placeholder">
          暂无日志数据
        </div>
      </div>
    </a-drawer>
  </div>
</template>

<style scoped>
.docker-page {
  padding: 0;
  min-height: 100%;
  background: transparent;
}

.glass-header {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(var(--blur-md));
  border-bottom: 1px solid var(--alpha-black-05);
  margin-bottom: 16px;
}

.main-content {
  padding: 0 24px 24px;
}

.glass-panel {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  border: 1px solid var(--alpha-white-30);
  box-shadow: 0 8px 32px var(--alpha-black-05);
  padding: 24px;
  min-height: 600px;
}

.custom-tabs :deep(.ant-tabs-nav) {
  margin-bottom: 24px;
}

.custom-tabs :deep(.ant-tabs-tab) {
  padding: 8px 16px;
  border-radius: var(--radius-sm);
  transition: all 0.3s;
}

.custom-tabs :deep(.ant-tabs-tab-active) {
  background: var(--info-bg);
}

.toolbar {
  display: flex;
  justify-content: space-between;
  margin-bottom: 16px;
  gap: 16px;
}

.glass-input {
  background: var(--alpha-white-50);
  border-radius: var(--radius-sm);
}

.glass-table :deep(.ant-table) {
  background: transparent;
}

.glass-table :deep(.ant-table-thead > tr > th) {
  background: var(--alpha-black-02);
  font-weight: var(--font-weight-semibold);
}

.glass-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid var(--alpha-black-03);
}

.glass-table :deep(.ant-table-tbody > tr:hover > td) {
  background: var(--alpha-black-02);
}

.mono-text {
  font-family: "SF Mono", Menlo, monospace;
  color: #666;
}

.name-text {
  font-weight: var(--font-weight-medium);
  color: var(--primary-color);
}

.text-secondary {
  color: #999;
}

.text-muted {
  color: #999;
  font-style: italic;
}

.working-dir-link {
  color: var(--primary-color);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: var(--font-size-xs);
  transition: color 0.2s;
}

.working-dir-link:hover {
  color: #40a9ff;
  text-decoration: underline;
}

.form-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.separator {
  color: #999;
  font-weight: bold;
}

.form-help {
  font-size: var(--font-size-xs);
  color: #999;
  margin-top: 4px;
}

.code-textarea {
  font-family: "SF Mono", Menlo, monospace;
  background: var(--alpha-black-02);
}

/* 日志 Drawer 样式 */
.log-container {
  height: calc(100vh - 120px);
  overflow-y: auto;
  background: #1e1e1e;
  padding: 12px 16px;
  border-radius: 8px;
  font-family: "SF Mono", SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #d4d4d4;
}

.log-line {
  white-space: pre-wrap;
  word-break: break-all;
  padding: 1px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
}

.log-line:hover {
  background: rgba(255, 255, 255, 0.05);
}

.log-placeholder {
  color: #666;
  text-align: center;
  padding: 40px 0;
  font-style: italic;
}
</style>

<style>
/* Dark Mode Global Overrides */
.dark .glass-header {
  background: rgba(30, 30, 30, 0.7);
  border-bottom-color: var(--alpha-white-05);
}

.dark .glass-panel {
  background: rgba(30, 30, 30, 0.7);
  border-color: var(--alpha-white-05);
}

.dark .glass-input {
  background: var(--alpha-black-20);
  border-color: var(--alpha-white-10);
  color: #fff;
}

.dark .glass-table .ant-table-thead>tr>th {
  background: var(--alpha-white-05);
  color: #e6e6e6;
  border-bottom-color: var(--alpha-white-05);
}

.dark .glass-table .ant-table-tbody>tr>td {
  border-bottom-color: var(--alpha-white-05);
  color: #e6e6e6;
}

.dark .glass-table .ant-table-tbody>tr:hover>td {
  background: var(--alpha-white-05);
}

.dark .mono-text {
  color: #aaa;
}

.dark .name-text {
  color: #177ddc;
}

.dark .working-dir-link {
  color: #177ddc;
}

.dark .working-dir-link:hover {
  color: #3c9ae8;
}

.dark .text-muted {
  color: #666;
}

.dark .code-textarea {
  background: #1e1e1e;
  color: #d4d4d4;
  border-color: var(--alpha-white-10);
}

.dark .custom-tabs .ant-tabs-tab-active {
  background: rgba(23, 125, 220, 0.2);
}

.dark .custom-tabs .ant-tabs-tab {
  color: #aaa;
}

.dark .custom-tabs .ant-tabs-tab-active .ant-tabs-tab-btn {
  color: #177ddc;
}

/* Glass Modal Styles */
.glass-modal .ant-modal-content {
  background: var(--alpha-white-80);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  box-shadow: 0 20px 50px var(--alpha-black-10);
  border: 1px solid var(--alpha-white-50);
}

.glass-modal .ant-modal-header {
  background: transparent;
  border-bottom: 1px solid var(--alpha-black-05);
  border-radius: var(--radius-lg) 16px 0 0;
}

.glass-modal .ant-modal-title {
  font-weight: var(--font-weight-semibold);
}

.glass-modal .ant-input,
.glass-modal .ant-select-selector,
.glass-modal .ant-input-number {
  border-radius: var(--radius-sm);
  background: var(--alpha-white-50);
  border-color: var(--alpha-black-10);
}

.glass-modal .ant-btn {
  border-radius: var(--radius-sm);
}

/* Dark Mode Modal */
.dark .glass-modal .ant-modal-content {
  background: rgba(40, 40, 40, 0.8);
  border-color: var(--alpha-white-10);
  box-shadow: 0 20px 50px var(--alpha-black-30);
}

.dark .glass-modal .ant-modal-header {
  border-bottom-color: var(--alpha-white-05);
}

.dark .glass-modal .ant-modal-title {
  color: #e0e0e0;
}

.dark .glass-modal .ant-modal-close {
  color: #aaa;
}

.dark .glass-modal .ant-input,
.dark .glass-modal .ant-select-selector,
.dark .glass-modal .ant-input-number {
  background: var(--alpha-black-20);
  border-color: var(--alpha-white-10);
  color: #e0e0e0;
}

.dark .glass-modal .ant-form-item-label>label {
  color: #ccc;
}

/* 日志 Drawer 全局样式 */
.log-drawer .ant-drawer-body {
  padding: 12px;
  background: #141414;
}

.dark .log-drawer .ant-drawer-content {
  background: #1a1a1a;
}

.dark .log-drawer .ant-drawer-header {
  background: #1a1a1a;
  border-bottom-color: rgba(255, 255, 255, 0.08);
}

.dark .log-drawer .ant-drawer-header .ant-drawer-title {
  color: #e0e0e0;
}

.dark .log-drawer .ant-drawer-close {
  color: #aaa;
}
</style>
