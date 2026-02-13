<script setup lang="ts">
defineOptions({
  name: 'ServerDetail'
});
import { ref, onMounted, onUnmounted, computed, nextTick, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Tabs, Modal } from 'ant-design-vue';
import request from '../../utils/request';
// 导入ECharts组件
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, TitleComponent, LegendComponent } from 'echarts/components';
import VChart from 'vue-echarts';
// 导入服务器状态store
import { useServerStore } from '../../stores/serverStore';
// 导入设置store
import { useSettingsStore } from '../../stores/settingsStore';
import { useUIStore } from '../../stores/uiStore';
import { ClockCircleOutlined, DownOutlined } from '@ant-design/icons-vue';

// 注册必要的ECharts组件
use([
  CanvasRenderer,
  LineChart,
  GridComponent,
  TooltipComponent,
  TitleComponent,
  LegendComponent
]);

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));
// 获取服务器状态store
const serverStore = useServerStore();
// 添加设置存储
const settingsStore = useSettingsStore();
const uiStore = useUIStore();

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// WebSocket连接
let ws: WebSocket | null = null;
const wsConnected = ref(false);

// 心跳定时器
let heartbeatTimer: number | null = null;
// 记录尝试次数
let heartbeatFailureCount = 0;
const maxHeartbeatFailures = 3;

// 添加WebSocket专用重连机制
let reconnectAttempts = 0;
const maxReconnectAttempts = 5;
const reconnectDelay = 3000; // 3秒

// 定义数据点类型
type DataPoint = {
  time: string;
  value: number;
};

// 定义监控数据类型
type MonitorDataType = {
  cpu: DataPoint[];
  memory: DataPoint[];
  disk: DataPoint[];
  network: {
    in: DataPoint[];
    out: DataPoint[];
  };
  processes: DataPoint[];
  connections: {
    tcp: DataPoint[];
    udp: DataPoint[];
  };
};

// 服务器监控数据
const monitorData = ref<MonitorDataType>({
  cpu: [],
  memory: [],
  disk: [],
  network: {
    in: [],
    out: []
  },
  processes: [],
  connections: {
    tcp: [],
    udp: []
  }
});

// 重连机制的参数
const reconnectCount = ref(0);
const reconnectInterval = 3000; // 3秒

// 保存定时器引用
const refreshIntervalRef = ref<number | null>(null);

// 使用计算属性获取服务器在线状态
const isServerOnline = computed(() => {
  // 优先使用store中的状态
  return serverStore.isServerOnline(serverId.value);
});

// 是否为监控模式服务器（隐藏操作类 Tab）
const isMonitorOnly = computed(() => {
  return serverInfo.value?.agent_type === 'monitor';
});

// 切换 Agent 类型（full ↔ monitor）
const switchingAgentType = ref(false);
const switchAgentType = () => {
  const currentType = serverInfo.value?.agent_type || 'full';
  const targetType = currentType === 'monitor' ? 'full' : 'monitor';
  const targetLabel = targetType === 'monitor' ? '最小监控版' : '全功能版';
  const currentLabel = currentType === 'monitor' ? '最小监控版' : '全功能版';

  Modal.confirm({
    title: '切换 Agent 类型',
    content: `确定将此服务器的 Agent 从「${currentLabel}」切换为「${targetLabel}」吗？切换后 Agent 将自动下载对应版本的二进制并重启。`,
    okText: '确认切换',
    cancelText: '取消',
    onOk: async () => {
      switchingAgentType.value = true;
      try {
        const res = await request.post(`/servers/${serverId.value}/switch-agent-type`, {
          target_agent_type: targetType,
        });
        // 更新本地状态
        serverInfo.value.agent_type = targetType;
        serverStore.updateServerMonitorData(serverId.value, { agent_type: targetType });
        if (res?.upgrade_dispatched) {
          message.success(`Agent 类型切换指令已下发，正在切换为${targetLabel}`);
        } else {
          message.warning(res?.message || '类型已更新，但 Agent 离线，需手动重装');
        }
      } catch (error: any) {
        message.error(error?.response?.data?.error || '切换 Agent 类型失败');
      } finally {
        switchingAgentType.value = false;
      }
    },
  });
};

// 更新服务器信息并解析系统信息
const updateServerInfo = (server: any) => {
  console.log('🔄 updateServerInfo被调用');
  console.log('传入的server对象:', server);
  console.log('调用堆栈:', new Error().stack);

  // 处理系统信息 JSON
  let systemInfo: Record<string, any> = {};
  if (server.system_info) {
    try {
      // 判断系统信息是否已经是对象
      if (typeof server.system_info === 'object') {
        systemInfo = server.system_info;
        console.log('系统信息已经是对象:', systemInfo);
      } else {
        systemInfo = JSON.parse(server.system_info);
        console.log('解析系统信息字符串:', systemInfo);
      }
    } catch (error) {
      console.error('解析系统信息失败:', error);
      systemInfo = {};
    }
  }

  console.log('最终系统信息对象:', systemInfo);
  console.log('原始服务器数据:', server);
  console.log('原始last_heartbeat:', server.last_heartbeat);

  // 获取状态信息
  const status = server.status || 'offline';
  // 更新全局状态store
  serverStore.updateServerStatus(serverId.value, status);

  // 处理时间字段
  let lastSeenTimestamp = null;
  if (server.last_heartbeat) {
    try {
      lastSeenTimestamp = new Date(server.last_heartbeat).getTime() / 1000;
      console.log('解析后的时间戳:', lastSeenTimestamp, '对应时间:', new Date(lastSeenTimestamp * 1000).toLocaleString());
    } catch (error) {
      console.error('时间解析失败:', error);
    }
  }

  // 更新服务器信息，适配后端API返回的字段名
  serverInfo.value = {
    id: server.ID, // 大写ID
    name: server.name,
    ip: server.ip, // 改为ip而非ip_address
    port: server.port,
    online: server.online || status === 'online', // 优先使用online字段
    last_seen: lastSeenTimestamp, // 使用处理后的时间戳
    description: server.description, // 改为description而非notes
    // 直接使用API返回的系统信息字段
    os: server.os || systemInfo.os || '未知',
    arch: server.arch || systemInfo.kernel_arch || '未知',
    cpu_cores: server.cpu_cores || systemInfo.cpu_cores || '未知',
    cpu_model: server.cpu_model || systemInfo.cpu_model || '未知',
    memory_total: server.memory_total || systemInfo.memory_total || 0,
    disk_total: server.disk_total || systemInfo.disk_total || 0,
    // 添加系统信息字段，使用更健壮的错误处理
    hostname: systemInfo.hostname || '未知',
    os_version: (systemInfo.platform && systemInfo.platform_version) ?
      `${systemInfo.platform} ${systemInfo.platform_version}` :
      (systemInfo.os_version || '未知'),
    kernel_version: systemInfo.kernel_version || '未知',
    tags: server.tags || '',
    user_id: server.user_id,
    agent_type: server.agent_type || server.AgentType || 'full',
  };

  console.log('处理后的服务器信息:', serverInfo.value);
};

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    const response = await request.get(`/servers/${serverId.value}`);
    console.log('获取到服务器信息响应:', response);
    if (response && (response as any).server) {
      // 处理返回的服务器数据
      updateServerInfo((response as any).server);
    } else {
      console.warn('服务器信息API返回格式异常:', response);
      serverInfo.value = {};
      message.warning('获取服务器信息返回格式异常');
    }
  } catch (error) {
    console.error('获取服务器信息失败:', error);
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
  }
};

// 格式化时间戳
const formatTime = (timestamp: number): string => {
  if (!timestamp) return '未知';
  const date = new Date(timestamp * 1000);
  return date.toLocaleString();
};

// 节流刷新变量
let lastRefreshTime = 0;
const refreshThrottleMs = 10000; // 10秒内最多刷新一次

// 节流的刷新函数
const throttledRefreshServerInfo = () => {
  const now = Date.now();
  if (now - lastRefreshTime >= refreshThrottleMs) {
    lastRefreshTime = now;
    refreshServerInfo();
  }
};

// 定期刷新服务器信息的函数
const refreshServerInfo = async () => {
  console.log('定期刷新服务器信息...');
  try {
    const response = await request.get(`/servers/${serverId.value}`);
    if (response.data && response.data.server) {
      // 获取当前WebSocket连接状态
      const wasOnline = serverInfo.value.online;
      const server = response.data.server;

      // 使用更新函数处理服务器信息，保留WebSocket状态
      updateServerInfo(server);

      // 如果WebSocket已经确认在线，保持在线状态
      if (wasOnline && wsConnected.value) {
        serverInfo.value.online = true;
        serverStore.updateServerStatus(serverId.value, 'online');
      }

      console.log('服务器信息已刷新, 状态:', serverInfo.value.online ? '在线' : '离线',
        '最后心跳:', serverInfo.value.last_seen ? new Date(serverInfo.value.last_seen * 1000).toLocaleString() : '未知');
    }
  } catch (error) {
    console.error('刷新服务器信息失败:', error);
  }
};

// 获取历史监控数据
const fetchHistoricalData = async () => {
  if (!serverId.value) return;

  try {
    loading.value = true;
    const response = await request.get(`/servers/${serverId.value}/monitor`);

    console.log('历史监控数据API响应:', response);

    // 检查响应数据格式 - axios拦截器已经返回了response.data
    if (!response) {
      console.error('获取历史数据失败：响应格式无效');
      return;
    }

    // 使用正确的数据路径 - response就是返回的数据，其中的data字段才是历史数据数组
    const historicalData = response.data || [];
    console.log(`获取到 ${historicalData.length} 条历史监控数据`);

    if (historicalData.length === 0) {
      console.warn('没有历史监控数据');
      return;
    }

    // 清空现有数据
    monitorData.value.cpu = [];
    monitorData.value.memory = [];
    monitorData.value.disk = [];
    monitorData.value.network.in = [];
    monitorData.value.network.out = [];

    // 处理历史数据
    historicalData.forEach((entry) => {
      // 格式化时间
      const timestamp = new Date(entry.timestamp);
      const timeStr = timestamp.toLocaleTimeString('zh-CN', { hour12: false });
      const dateStr = timestamp.toLocaleDateString('zh-CN');
      const fullTimeStr = `${dateStr} ${timeStr}`;

      // CPU数据
      const cpuValue = parseFloat(entry.cpu_usage.toFixed(2));
      monitorData.value.cpu.push({
        time: fullTimeStr,
        value: cpuValue
      });

      // 内存数据 - 处理不同格式的memory_used
      let memoryPercent = 0;
      if (typeof entry.memory_used === 'number') {
        // 如果memory_used是数值，判断是百分比还是字节
        if (entry.memory_used <= 100) {
          // 小于或等于100认为是百分比
          memoryPercent = entry.memory_used;
        } else if (entry.memory_total && entry.memory_total > 0) {
          // 否则认为是字节数，计算百分比
          memoryPercent = (entry.memory_used / entry.memory_total) * 100;
        }
      }
      monitorData.value.memory.push({
        time: fullTimeStr,
        value: parseFloat(memoryPercent.toFixed(2))
      });

      // 磁盘数据 - 处理不同格式的disk_used
      let diskPercent = 0;
      if (typeof entry.disk_used === 'number') {
        // 如果disk_used是数值，判断是百分比还是字节
        if (entry.disk_used <= 100) {
          // 小于或等于100认为是百分比
          diskPercent = entry.disk_used;
        } else if (entry.disk_total && entry.disk_total > 0) {
          // 否则认为是字节数，计算百分比
          diskPercent = (entry.disk_used / entry.disk_total) * 100;
        }
      }
      monitorData.value.disk.push({
        time: fullTimeStr,
        value: parseFloat(diskPercent.toFixed(2))
      });

      // 网络数据 - 转换为MB/s（字节转MB）
      const networkInMB = entry.network_in / (1024 * 1024);
      const networkOutMB = entry.network_out / (1024 * 1024);

      monitorData.value.network.in.push({
        time: fullTimeStr,
        value: parseFloat(networkInMB.toFixed(3))
      });

      monitorData.value.network.out.push({
        time: fullTimeStr,
        value: parseFloat(networkOutMB.toFixed(3))
      });
    });

  } catch (error) {
    console.error('获取历史监控数据失败:', error);
    message.error('获取历史监控数据失败');
  } finally {
    loading.value = false;
  }
};

// 页面加载时获取服务器信息和监控数据
onMounted(async () => {
  // 先加载系统设置
  await settingsStore.loadPublicSettings();
  console.log('已加载设置，历史数据显示时间:', settingsStore.chartHistoryHours, '小时');

  // 获取服务器信息
  await fetchServerInfo();

  // 获取历史监控数据
  await fetchHistoricalData();

  // 数据加载完成，关闭全局骨架屏
  uiStore.stopLoading();

  // 连接WebSocket获取实时数据
  connectWebSocket();

  // 设置定期刷新服务器信息，每30秒刷新一次以更新最后在线时间
  refreshIntervalRef.value = window.setInterval(() => {
    refreshServerInfo();
  }, 30000); // 30秒刷新一次

  console.log('已设置定期刷新机制，每30秒更新服务器信息');
});

// 监听设置变化，当历史数据显示时间改变时重新加载数据
watch(() => settingsStore.chartHistoryHours, async (newValue, oldValue) => {
  if (newValue !== oldValue && newValue > 0) {
    console.log('历史数据显示时间设置已更改:', oldValue, '->', newValue, '小时');
    // 重新获取历史监控数据
    await fetchHistoricalData();
  }
});

// 页面卸载时清理资源
onUnmounted(() => {
  // 清除定时刷新
  if (refreshIntervalRef.value) {
    window.clearInterval(refreshIntervalRef.value);
    refreshIntervalRef.value = null;
  }

  // 清除心跳定时器
  if (heartbeatTimer !== null) {
    window.clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }

  // 关闭WebSocket连接
  if (ws) {
    console.log('组件卸载，关闭WebSocket连接');
    ws.onclose = null; // 防止触发重连
    ws.close();
    ws = null;
  }
});

// 导航到其他功能页面
const navigateTo = (path: string) => {
  router.push(`/admin/servers/${serverId.value}/${path}`);
};

// 为ECharts创建计算属性
const cpuChartOption = computed(() => ({
  title: {
    text: 'CPU使用率',
    left: 'center'
  },
  tooltip: {
    trigger: 'axis'
  },
  xAxis: {
    type: 'category',
    data: monitorData.value.cpu.map((item: DataPoint) => item.time),
    axisLabel: {
      rotate: 45
    }
  },
  yAxis: {
    type: 'value',
    min: 0,
    max: 100,
    axisLabel: {
      formatter: '{value}%'
    }
  },
  series: [
    {
      name: 'CPU使用率',
      type: 'line',
      data: monitorData.value.cpu.map((item: DataPoint) => item.value),
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#2F54EB'
      },
      smooth: true
    }
  ],
  grid: {
    left: '3%',
    right: '4%',
    bottom: '10%',
    top: '15%',
    containLabel: true
  }
}));

const memoryChartOption = computed(() => ({
  title: {
    text: '内存使用率',
    left: 'center'
  },
  tooltip: {
    trigger: 'axis'
  },
  xAxis: {
    type: 'category',
    data: monitorData.value.memory.map((item: DataPoint) => item.time),
    axisLabel: {
      rotate: 45
    }
  },
  yAxis: {
    type: 'value',
    min: 0,
    max: 100,
    axisLabel: {
      formatter: '{value}%'
    }
  },
  series: [
    {
      name: '内存使用率',
      type: 'line',
      data: monitorData.value.memory.map((item: DataPoint) => item.value),
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#52C41A'
      },
      smooth: true
    }
  ],
  grid: {
    left: '3%',
    right: '4%',
    bottom: '10%',
    top: '15%',
    containLabel: true
  }
}));

const diskChartOption = computed(() => ({
  title: {
    text: '磁盘使用率',
    left: 'center'
  },
  tooltip: {
    trigger: 'axis'
  },
  xAxis: {
    type: 'category',
    data: monitorData.value.disk.map((item: DataPoint) => item.time),
    axisLabel: {
      rotate: 45
    }
  },
  yAxis: {
    type: 'value',
    min: 0,
    max: 100,
    axisLabel: {
      formatter: '{value}%'
    }
  },
  series: [
    {
      name: '磁盘使用率',
      type: 'line',
      data: monitorData.value.disk.map((item: DataPoint) => item.value),
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#FA8C16'
      },
      smooth: true
    }
  ],
  grid: {
    left: '3%',
    right: '4%',
    bottom: '10%',
    top: '15%',
    containLabel: true
  }
}));

const networkChartOption = computed(() => ({
  title: {
    text: '网络流量 (MB/s)',
    left: 'center'
  },
  tooltip: {
    trigger: 'axis',
    formatter: function (params: any[]) {
      const time = params[0].name;
      let result = `${time}<br />`;
      params.forEach(param => {
        const color = param.color;
        const seriesName = param.seriesName;
        const value = parseFloat(param.value).toFixed(3); // 增加小数位数以显示更精确的值
        result += `<span style="display:inline-block;margin-right:5px;border-radius:10px;width:10px;height:10px;background-color:${color};"></span> ${seriesName}: ${value} MB/s<br />`;
      });
      return result;
    }
  },
  legend: {
    data: ['入站流量', '出站流量'],
    top: '30px'
  },
  xAxis: {
    type: 'category',
    data: monitorData.value.network.in.map((item: DataPoint) => item.time),
    axisLabel: {
      rotate: 45
    }
  },
  yAxis: {
    type: 'value',
    axisLabel: {
      formatter: '{value} MB/s'
    },
    scale: true, // 启用scale使Y轴自适应数据
    min: 0 // 确保从0开始
  },
  series: [
    {
      name: '入站流量',
      type: 'line',
      data: monitorData.value.network.in.map((item: DataPoint) => item.value),
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#13C2C2'
      },
      smooth: true
    },
    {
      name: '出站流量',
      type: 'line',
      data: monitorData.value.network.out.map((item: DataPoint) => item.value),
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#F5222D'
      },
      smooth: true
    }
  ],
  grid: {
    left: '3%',
    right: '4%',
    bottom: '10%',
    top: '70px',
    containLabel: true
  }
}));

// 添加一个计算属性来判断是否有监控数据
const hasMonitorData = computed(() => {
  return monitorData.value.cpu.length > 0 ||
    monitorData.value.memory.length > 0 ||
    monitorData.value.disk.length > 0 ||
    monitorData.value.network.in.length > 0;
});

// 计算当前实时数据
const currentCpuUsage = computed(() => {
  const data = monitorData.value.cpu;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentMemoryUsage = computed(() => {
  const data = monitorData.value.memory;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentDiskUsage = computed(() => {
  const data = monitorData.value.disk;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentNetworkIn = computed(() => {
  const data = monitorData.value.network.in;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentNetworkOut = computed(() => {
  const data = monitorData.value.network.out;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentProcesses = computed(() => {
  const data = monitorData.value.processes;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentTcpConnections = computed(() => {
  const data = monitorData.value.connections.tcp;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentUdpConnections = computed(() => {
  const data = monitorData.value.connections.udp;
  return data.length > 0 ? data[data.length - 1].value : 0;
});

const currentNetworkTotal = computed(() => {
  return currentNetworkIn.value + currentNetworkOut.value;
});

// 格式化字节
const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

// 格式化运行时间
const uptimeText = computed(() => {
  if (!serverInfo.value.last_seen) return '未知';
  // 这里假设last_seen是最后心跳时间，不是启动时间。
  // 如果没有启动时间字段，我们只能显示最后在线时间。
  // ServerDetail.vue 似乎没有 boot_time。
  // 我们暂时显示 "最后在线: " + formatTime(serverInfo.value.last_seen)
  return formatTime(serverInfo.value.last_seen);
});

// 建立WebSocket连接获取实时监控数据
const connectWebSocket = () => {
  // 获取token
  const token = localStorage.getItem('server_ops_token');
  if (!token) {
    message.error('未登录，无法获取实时数据');
    return;
  }

  // 关闭之前的连接并清除定时器
  if (ws) {
    console.log('关闭之前的WebSocket连接');
    // 移除事件处理函数防止触发重连
    ws.onclose = null;
    ws.close();
    ws = null;
  }

  // 清除之前的心跳定时器
  if (heartbeatTimer) {
    console.log('清除之前的心跳定时器');
    window.clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }

  // 重置心跳失败计数
  heartbeatFailureCount = 0;

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';

  // 修正WebSocket URL，确保与后端路由匹配
  const wsUrl = `${protocol}//${window.location.host}/api/servers/${serverId.value}/ws?token=${encodeURIComponent(token)}`;

  console.log('正在连接WebSocket:', wsUrl);

  try {
    ws = new WebSocket(wsUrl);

    // 设置超时处理，如果10秒内没有连接成功则认为失败
    const connectionTimeout = setTimeout(() => {
      if (ws && ws.readyState !== WebSocket.OPEN) {
        console.log('WebSocket连接超时');
        if (ws) {
          ws.close();
        }
        wsConnected.value = false;
        message.error('连接超时，请稍后重试');

        // 尝试重连
        handleReconnect();
      }
    }, 10000);

    // 监听连接打开事件
    ws.onopen = () => {
      clearTimeout(connectionTimeout);
      console.log('WebSocket连接成功');
      wsConnected.value = true;
      reconnectAttempts = 0; // 成功连接后重置重连计数
      message.success('实时监控已连接');

      // WebSocket连接成功时，更新服务器状态为在线
      serverStore.updateServerStatus(serverId.value, 'online');

      // 同时更新本地状态
      if (!serverInfo.value.online) {
        console.log('WebSocket连接成功，更新服务器状态为在线');
        serverInfo.value.online = true;
      }

      // 设置心跳定时器
      heartbeatTimer = window.setInterval(() => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          console.log('发送心跳包');
          try {
            ws.send(JSON.stringify({
              type: 'heartbeat',
              timestamp: Date.now()
            }));
          } catch (error) {
            console.error('发送心跳失败:', error);
            heartbeatFailureCount++;

            // 如果连续多次发送心跳失败，关闭连接并尝试重连
            if (heartbeatFailureCount >= maxHeartbeatFailures) {
              console.log(`心跳发送失败${maxHeartbeatFailures}次，关闭连接并重连`);
              if (heartbeatTimer !== null) {
                window.clearInterval(heartbeatTimer);
                heartbeatTimer = null;
              }
              // 关闭连接并重新连接
              if (ws) {
                ws.close();
              }
              // 短暂延迟后重新连接
              setTimeout(connectWebSocket, 3000);
            }
          }
        } else {
          // WebSocket已关闭，清理心跳定时器
          console.log('WebSocket已关闭，清除心跳定时器');
          if (heartbeatTimer !== null) {
            window.clearInterval(heartbeatTimer);
            heartbeatTimer = null;
          }
          wsConnected.value = false;
        }
      }, 30000); // 30秒发送一次心跳，减少频率
    };

    ws.onmessage = (event) => {
      try {
        // 收到消息，重置心跳失败计数
        heartbeatFailureCount = 0;

        const data = JSON.parse(event.data);

        // 更新监控数据
        if (data.type === 'monitor') {
          console.log('收到监控数据:', data);

          // 提取状态信息
          let status = '';
          if (data.data && data.data.status) {
            status = data.data.status;
          } else if (data.status) {
            status = data.status;
          }

          // 如果有状态信息，更新全局store
          if (status) {
            serverStore.updateServerStatus(serverId.value, status);
            // 更新本地状态
            serverInfo.value.online = status === 'online';
          }

          // 收到监控数据时，触发服务器信息刷新以获取最新的数据库时间
          throttledRefreshServerInfo();

          // 检查是否有嵌套的data字段
          if (data.data) {
            updateMonitorData(data.data);
            // 同步数据到store
            serverStore.updateServerMonitorData(serverId.value, data.data);
          } else {
            updateMonitorData(data);
            // 同步数据到store
            serverStore.updateServerMonitorData(serverId.value, data);
          }
        }
        // 处理心跳消息
        else if (data.type === 'heartbeat') {
          console.log('收到心跳消息:', data);
          // 收到心跳消息，更新连接状态
          wsConnected.value = true;

          // 提取状态信息
          let status = '';
          if (data.data && data.data.status) {
            status = data.data.status;
          } else if (data.status) {
            status = data.status;
          }

          // 如果有状态信息，更新全局store
          if (status) {
            serverStore.updateServerStatus(serverId.value, status);
            // 更新本地状态
            serverInfo.value.online = status === 'online';
          } else {
            // 心跳成功表示服务器在线
            serverStore.updateServerStatus(serverId.value, 'online');
            serverInfo.value.online = true;
          }

          // 收到心跳消息时，触发服务器信息刷新以获取最新的数据库时间
          throttledRefreshServerInfo();

          // 处理心跳消息中的监控数据
          let hasMonitorData = false;

          if (data.data) {
            console.log('心跳消息中包含监控数据:', data.data);
            // 检查是否包含有效的监控数据
            const hasData = data.data.cpu_usage !== undefined ||
              data.data.memory_used !== undefined ||
              data.data.disk_used !== undefined ||
              data.data.network_in !== undefined;

            if (hasData) {
              updateMonitorData(data.data);
              // 同步数据到store
              serverStore.updateServerMonitorData(serverId.value, data.data);
              hasMonitorData = true;
            } else {
              console.log('心跳消息data字段中没有有效的监控数据');
            }
          }
          // 检查心跳消息本身是否包含监控数据
          else if (
            data.cpu_usage !== undefined ||
            data.memory_used !== undefined ||
            data.disk_used !== undefined ||
            data.network_in !== undefined
          ) {
            console.log('心跳消息本身包含监控数据:', data);
            updateMonitorData(data);
            // 同步数据到store
            serverStore.updateServerMonitorData(serverId.value, data);
            hasMonitorData = true;
          }
        }
        // 处理欢迎消息
        else if (data.type === 'welcome') {
          console.log('WebSocket欢迎消息:', data.message);

          // 提取状态信息
          if (data.status) {
            // 更新全局store
            serverStore.updateServerStatus(serverId.value, data.status);
            // 更新本地状态
            serverInfo.value.online = data.status === 'online';
          } else {
            // 收到欢迎消息，服务器应该是在线的
            serverStore.updateServerStatus(serverId.value, 'online');
            serverInfo.value.online = true;
          }

          // 如果欢迎消息中包含系统信息，尝试解析并更新
          if (data.system_info) {
            console.log('欢迎消息中包含系统信息:', data.system_info);

            // 更新系统信息对象
            try {
              let welcomeSystemInfo;
              if (typeof data.system_info === 'object') {
                welcomeSystemInfo = data.system_info;
              } else {
                welcomeSystemInfo = JSON.parse(data.system_info);
              }

              // 直接更新系统信息字段，而不是重新调用updateServerInfo
              // 这样可以避免覆盖last_heartbeat等重要字段
              if (welcomeSystemInfo.hostname) {
                serverInfo.value.hostname = welcomeSystemInfo.hostname;
              }
              if (welcomeSystemInfo.platform && welcomeSystemInfo.platform_version) {
                serverInfo.value.os_version = `${welcomeSystemInfo.platform} ${welcomeSystemInfo.platform_version}`;
              }
              if (welcomeSystemInfo.kernel_version) {
                serverInfo.value.kernel_version = welcomeSystemInfo.kernel_version;
              }

              console.log('已更新系统信息字段，保留了last_heartbeat');

              // 同步系统信息到store
              serverStore.updateServerMonitorData(serverId.value, {
                system_info: data.system_info,
                status: data.status || 'online'
              });
            } catch (error) {
              console.error('解析欢迎消息中的系统信息失败:', error);
            }
          }
        }
      } catch (error) {
        console.error('解析WebSocket消息失败:', error);
      }
    };

    ws.onerror = (error) => {
      console.error('WebSocket错误:', error);
      wsConnected.value = false;
      // WebSocket错误不一定意味着服务器离线，这里不更新状态
      message.error('监控连接发生错误');

      // 在错误时尝试重连
      handleReconnect();
    };

    // 添加onclose处理
    ws.onclose = (event) => {
      console.log(`WebSocket连接已关闭，代码: ${event.code}, 原因: ${event.reason}`);
      wsConnected.value = false;

      // 清除心跳定时器
      if (heartbeatTimer) {
        clearInterval(heartbeatTimer);
        heartbeatTimer = null;
      }

      // 尝试重连
      handleReconnect();
    };
  } catch (error) {
    console.error('建立WebSocket连接失败:', error);
    wsConnected.value = false;
    message.error('建立WebSocket连接失败，请稍后重试');

    // 在连接失败时尝试重连
    handleReconnect();
  }
};

// 添加重连处理函数
const handleReconnect = () => {
  if (reconnectAttempts < maxReconnectAttempts) {
    reconnectAttempts++;
    const delay = reconnectDelay * reconnectAttempts; // 逐渐增加延迟
    console.log(`尝试第 ${reconnectAttempts}/${maxReconnectAttempts} 次重连，将在 ${delay / 1000} 秒后重试...`);

    setTimeout(() => {
      connectWebSocket();
    }, delay);
  } else {
    console.log('已达到最大重连次数，不再自动重连');
    message.warning('监控连接已断开，请刷新页面重试');

    // 重连失败，标记服务器为离线
    serverStore.updateServerStatus(serverId.value, 'offline');
    if (serverInfo.value) {
      serverInfo.value.online = false;
    }
  }
};

// 更新监控数据
const updateMonitorData = (data: any) => {
  console.log('更新监控数据:', data);
  // 限制数组长度为30（保留最近30条数据）
  const maxDataPoints = 30;
  const currentTime = new Date().toLocaleTimeString();

  // 更新CPU数据
  if (data.cpu_usage !== undefined) {
    // 处理CPU使用率小数值(0-1范围)
    let cpuValue = Number(data.cpu_usage);
    if (cpuValue < 1 && cpuValue > 0) {
      cpuValue = cpuValue * 100;
    }
    // 确保CPU值在0-100之间
    const safeValue = isNaN(cpuValue) ? 0 : Math.min(Math.max(cpuValue, 0), 100);

    monitorData.value.cpu.push({
      time: currentTime,
      value: safeValue
    });

    if (monitorData.value.cpu.length > maxDataPoints) {
      monitorData.value.cpu.shift();
    }
  }

  // 更新内存数据
  if (data.memory_used !== undefined) {
    let memoryUsagePercent = 0;

    // 如果memory_used是百分比（<=100）则直接使用
    if (data.memory_used <= 100) {
      memoryUsagePercent = Number(data.memory_used);
    }
    // 如果提供了字节数和总量，计算百分比
    else if (data.memory_total && data.memory_total > 0) {
      const memoryUsed = Number(data.memory_used);
      const memoryTotal = Number(data.memory_total);
      if (!isNaN(memoryUsed) && !isNaN(memoryTotal)) {
        memoryUsagePercent = (memoryUsed / memoryTotal) * 100;
      }
    }

    monitorData.value.memory.push({
      time: currentTime,
      value: Math.min(Math.max(memoryUsagePercent, 0), 100) // 确保值在0-100范围内
    });

    if (monitorData.value.memory.length > maxDataPoints) {
      monitorData.value.memory.shift();
    }
  }

  // 更新磁盘数据
  if (data.disk_used !== undefined) {
    let diskUsagePercent = 0;

    // 如果disk_used是百分比（<=100）则直接使用
    if (data.disk_used <= 100) {
      diskUsagePercent = Number(data.disk_used);
    }
    // 如果提供了字节数和总量，计算百分比
    else if (data.disk_total && data.disk_total > 0) {
      const diskUsed = Number(data.disk_used);
      const diskTotal = Number(data.disk_total);
      if (!isNaN(diskUsed) && !isNaN(diskTotal)) {
        diskUsagePercent = (diskUsed / diskTotal) * 100;
      }
    }

    monitorData.value.disk.push({
      time: currentTime,
      value: Math.min(Math.max(diskUsagePercent, 0), 100) // 确保值在0-100范围内
    });

    if (monitorData.value.disk.length > maxDataPoints) {
      monitorData.value.disk.shift();
    }
  }

  // 更新网络数据
  if (data.network_in !== undefined) {
    const networkIn = Number(data.network_in);
    if (isNaN(networkIn)) return;

    // 输入值为字节/秒，直接转换为MB/s
    const networkInMB = networkIn / (1024 * 1024);

    monitorData.value.network.in.push({
      time: currentTime,
      value: networkInMB
    });

    if (monitorData.value.network.in.length > maxDataPoints) {
      monitorData.value.network.in.shift();
    }
  }

  if (data.network_out !== undefined) {
    const networkOut = Number(data.network_out);
    if (isNaN(networkOut)) return;

    // 输入值为字节/秒，直接转换为MB/s
    const networkOutMB = networkOut / (1024 * 1024);

    monitorData.value.network.out.push({
      time: currentTime,
      value: networkOutMB
    });

    if (monitorData.value.network.out.length > maxDataPoints) {
      monitorData.value.network.out.shift();
    }
  }

  // 更新进程数
  if (data.processes !== undefined) {
    monitorData.value.processes.push({
      time: currentTime,
      value: Number(data.processes)
    });
    if (monitorData.value.processes.length > maxDataPoints) {
      monitorData.value.processes.shift();
    }
  }

  // 更新TCP连接数
  if (data.tcp_connections !== undefined) {
    monitorData.value.connections.tcp.push({
      time: currentTime,
      value: Number(data.tcp_connections)
    });
    if (monitorData.value.connections.tcp.length > maxDataPoints) {
      monitorData.value.connections.tcp.shift();
    }
  }

  // 更新UDP连接数
  if (data.udp_connections !== undefined) {
    monitorData.value.connections.udp.push({
      time: currentTime,
      value: Number(data.udp_connections)
    });
    if (monitorData.value.connections.udp.length > maxDataPoints) {
      monitorData.value.connections.udp.shift();
    }
  }

  // 添加初始数据点，如果没有数据的话
  if (monitorData.value.cpu.length === 0) {
    monitorData.value.cpu.push({ time: currentTime, value: 0 });
  }
  if (monitorData.value.memory.length === 0) {
    monitorData.value.memory.push({ time: currentTime, value: 0 });
  }
  if (monitorData.value.disk.length === 0) {
    monitorData.value.disk.push({ time: currentTime, value: 0 });
  }
  if (monitorData.value.network.in.length === 0) {
    monitorData.value.network.in.push({ time: currentTime, value: 0 });
  }
  if (monitorData.value.network.out.length === 0) {
    monitorData.value.network.out.push({ time: currentTime, value: 0 });
  }
  if (monitorData.value.processes.length === 0) {
    monitorData.value.processes.push({ time: currentTime, value: 0 });
  }
  if (monitorData.value.connections.tcp.length === 0) {
    monitorData.value.connections.tcp.push({ time: currentTime, value: 0 });
  }
  if (monitorData.value.connections.udp.length === 0) {
    monitorData.value.connections.udp.push({ time: currentTime, value: 0 });
  }

  console.log('监控数据更新完成:', monitorData.value);
};
</script>

<template>
  <div class="server-detail-container">
    <a-spin :spinning="loading">
      <!-- 顶部导航栏 -->
      <div class="ios-header glass-card">
        <div class="header-top">
          <div class="back-btn" @click="router.push('/admin/servers')">
            <span class="back-arrow">←</span>
            <span class="back-text">服务器列表</span>
          </div>
          <div class="header-actions">
            <a-space>
              <a-button type="primary" shape="round" class="ios-btn-primary"
                @click="navigateTo('monitor')">监控</a-button>
              <template v-if="!isMonitorOnly">
                <a-button shape="round" class="ios-btn" @click="navigateTo('terminal')">终端</a-button>
                <a-button shape="round" class="ios-btn" @click="navigateTo('file')">文件</a-button>
                <a-dropdown>
                  <template #overlay>
                    <a-menu class="ios-menu">
                      <a-menu-item @click="navigateTo('process')">进程管理</a-menu-item>
                      <a-menu-item @click="navigateTo('docker')">Docker容器</a-menu-item>
                      <a-menu-item @click="navigateTo('nginx')">网站管理</a-menu-item>
                    </a-menu>
                  </template>
                  <a-button shape="round" class="ios-btn">更多
                    <DownOutlined />
                  </a-button>
                </a-dropdown>
              </template>
            </a-space>
          </div>
        </div>

        <div class="header-content">
          <div class="server-icon">
            <div class="icon-placeholder">{{ serverInfo.os ? serverInfo.os.charAt(0).toUpperCase() : 'S' }}</div>
          </div>
          <div class="server-title-area">
            <h1 class="server-title">
              {{ serverInfo.name }}
              <span v-if="serverInfo.hostname && serverInfo.hostname !== '未知'" class="hostname-tag">{{
                serverInfo.hostname
              }}</span>
            </h1>
            <div class="server-meta">
              <span class="meta-item">{{ serverInfo.ip }}</span>
              <span class="meta-dot">•</span>
              <span class="meta-item">{{ serverInfo.os || '未知系统' }}</span>
              <span class="meta-dot">•</span>
              <span class="status-badge" :class="isServerOnline ? 'online' : 'offline'">
                {{ isServerOnline ? '运行中' : '已离线' }}
              </span>
              <span class="meta-dot">•</span>
              <span
                v-if="isMonitorOnly"
                class="status-badge"
                style="background: rgba(255, 149, 0, 0.12); color: #ff9500;"
              >监控模式</span>
              <span
                v-else
                class="status-badge"
                style="background: rgba(52, 199, 89, 0.12); color: #34c759;"
              >全功能</span>
              <span
                class="switch-agent-type-btn"
                :class="{ disabled: switchingAgentType }"
                @click="!switchingAgentType && switchAgentType()"
              >{{ switchingAgentType ? '切换中...' : '切换' }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 内容区域 -->
      <div class="ios-content">
        <!-- 概览卡片网格 -->
        <div class="overview-grid">
          <!-- 状态与运行时间 -->
          <div class="overview-card">
            <p class="label">运行状态</p>
            <div class="status-value">
              <div class="status-dot" :class="{ online: isServerOnline }"></div>
              <h3>{{ isServerOnline ? '在线' : '离线' }}</h3>
            </div>
            <small>最后在线 {{ uptimeText }}</small>
          </div>

          <!-- CPU -->
          <div class="overview-card">
            <p class="label">CPU</p>
            <h3>{{ currentCpuUsage.toFixed(1) }}%</h3>
            <small>{{ serverInfo.cpu_cores }} 核 • {{ serverInfo.cpu_model || 'Unknown' }}</small>
          </div>

          <!-- 内存 -->
          <div class="overview-card">
            <p class="label">内存</p>
            <h3>{{ currentMemoryUsage.toFixed(1) }}%</h3>
            <small>总量: {{ serverInfo.memory_total ? formatBytes(serverInfo.memory_total) : '未知' }}</small>
          </div>

          <!-- 磁盘 -->
          <div class="overview-card">
            <p class="label">磁盘</p>
            <h3>{{ currentDiskUsage.toFixed(1) }}%</h3>
            <small>总量: {{ serverInfo.disk_total ? formatBytes(serverInfo.disk_total) : '未知' }}</small>
          </div>

          <!-- 网络 -->
          <div class="overview-card">
            <p class="label">网络速率</p>
            <div class="network-speeds">
              <div class="speed-item">
                <span class="arrow up">↑</span>
                <span class="speed-value">{{ currentNetworkOut.toFixed(2) }} MB/s</span>
              </div>
              <div class="speed-item">
                <span class="arrow down">↓</span>
                <span class="speed-value">{{ currentNetworkIn.toFixed(2) }} MB/s</span>
              </div>
            </div>
            <small>实时速率</small>
          </div>

          <!-- 进程数 -->
          <div class="overview-card">
            <p class="label">进程数</p>
            <h3>{{ currentProcesses }}</h3>
            <small>活跃进程</small>
          </div>

          <!-- TCP连接 -->
          <div class="overview-card">
            <p class="label">TCP连接</p>
            <h3>{{ currentTcpConnections }}</h3>
            <small>建立连接</small>
          </div>

          <!-- UDP连接 -->
          <div class="overview-card">
            <p class="label">UDP连接</p>
            <h3>{{ currentUdpConnections }}</h3>
            <small>活跃连接</small>
          </div>

          <!-- 系统信息 -->
          <div class="overview-card">
            <p class="label">系统信息</p>
            <div class="system-info-row">
              <span class="sys-item">{{ serverInfo.os_version || serverInfo.os }}</span>
            </div>
            <small>{{ serverInfo.arch }} • {{ serverInfo.hostname }}</small>
          </div>

          <!-- 描述 (全宽) -->
          <div class="overview-card full-width" v-if="serverInfo.description">
            <p class="label">备注</p>
            <p class="description-text">{{ serverInfo.description }}</p>
          </div>
        </div>



        <!-- 状态提示 -->
        <div v-if="!isServerOnline || !wsConnected" class="status-alert-container">
          <div class="ios-alert" v-if="!isServerOnline">
            <div class="alert-icon warning">!</div>
            <div class="alert-content">
              <h4>服务器离线</h4>
              <p>无法获取实时监控数据，请检查服务器状态。</p>
            </div>
          </div>

          <div class="ios-alert" v-if="isServerOnline && !wsConnected">
            <div class="alert-icon info">i</div>
            <div class="alert-content">
              <h4>连接断开</h4>
              <p>实时监控连接已断开，<a @click="connectWebSocket">点击重连</a></p>
            </div>
          </div>
        </div>
      </div>
    </a-spin>
  </div>
</template>

<style scoped>
.server-detail-container {
  padding: 0;
  max-width: 1400px;
  margin: 0 auto;
}

/* iOS Header Style */
.ios-header {
  padding: 24px 32px;
  margin-bottom: 24px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.header-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.back-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  color: var(--primary-color);
  font-weight: var(--font-weight-medium);
  font-size: 15px;
  transition: opacity 0.2s;
}

.back-btn:hover {
  opacity: 0.7;
}

.back-arrow {
  font-size: var(--font-size-2xl);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 20px;
}

.server-icon {
  width: 64px;
  height: 64px;
  background: linear-gradient(135deg, var(--alpha-black-05), var(--alpha-black-02));
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-sm);
}

.icon-placeholder {
  font-size: var(--font-size-4xl);
  font-weight: var(--font-weight-semibold);
  color: var(--text-secondary);
}

.server-title-area {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.server-title {
  margin: 0;
  font-size: var(--font-size-4xl);
  font-weight: var(--font-weight-bold);
  letter-spacing: -0.5px;
  color: var(--text-primary);
}

.server-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary);
  font-size: var(--font-size-md);
}

.meta-dot {
  font-size: 8px;
  opacity: 0.5;
}

.status-badge {
  padding: 2px 10px;
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-semibold);
}

.status-badge.online {
  background-color: var(--success-bg);
  color: var(--success-color);
}

.status-badge.offline {
  background-color: var(--error-bg);
  color: var(--error-color);
}

.switch-agent-type-btn {
  margin-left: 6px;
  padding: 1px 8px;
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  color: var(--primary-color, #007aff);
  background: var(--alpha-black-05);
  cursor: pointer;
  transition: opacity 0.2s;
  user-select: none;
}

.switch-agent-type-btn:hover {
  opacity: 0.7;
}

.switch-agent-type-btn.disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* iOS Buttons */
.ios-btn {
  border: none;
  background: var(--alpha-black-05);
  color: var(--text-primary);
  font-weight: var(--font-weight-medium);
  box-shadow: none;
  transition: all 0.2s;
}

.ios-btn:hover {
  background: var(--alpha-black-10);
  color: var(--text-primary);
}

.ios-btn-primary {
  background: var(--primary-color);
  box-shadow: var(--btn-primary-shadow);
}

.ios-btn-primary:hover {
  background: var(--primary-hover);
}

/* iOS Content & Cards */
.ios-content {
  padding: 0 8px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
  gap: 20px;
  margin-bottom: 32px;
}

.overview-card {
  background: var(--card-bg);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  padding: 20px;
  box-shadow: 0 4px 24px -1px var(--alpha-black-05);
  border: 1px solid var(--card-border);
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
  display: flex;
  flex-direction: column;
}

.overview-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 32px -4px var(--alpha-black-10);
  border-color: var(--primary-light);
}

.overview-card.full-width {
  grid-column: 1 / -1;
}

.overview-card .label {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-secondary);
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.overview-card h3 {
  margin: 0;
  font-size: var(--font-size-4xl);
  font-weight: var(--font-weight-bold);
  color: var(--text-primary);
  letter-spacing: -0.5px;
  font-family: "SF Mono", Menlo, monospace;
}

.overview-card small {
  display: block;
  margin-top: auto;
  padding-top: 8px;
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
  white-space: normal;
  overflow: visible;
}

.description-text {
  color: var(--text-secondary);
  line-height: 1.6;
  margin: 0;
  font-size: var(--font-size-md);
}

/* Specific Card Styles */
.status-value {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-dot {
  width: 12px;
  height: 12px;
  border-radius: var(--radius-circle);
  background-color: var(--error-color);
  box-shadow: 0 0 8px var(--error-bg);
}

.status-dot.online {
  background-color: var(--success-color);
  box-shadow: 0 0 8px var(--success-bg);
}

.network-speeds {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.speed-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.speed-value {
  font-family: "SF Mono", Menlo, monospace;
  font-weight: var(--font-weight-semibold);
  font-size: var(--font-size-lg);
  color: var(--text-primary);
}

.arrow {
  font-size: var(--font-size-xs);
  font-weight: bold;
}

.arrow.up {
  color: var(--success-color);
}

.arrow.down {
  color: var(--primary-color);
}

/* Monitor Section */
.monitor-cards-section {
  margin-top: 32px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.section-title {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
  margin: 0;
}

.chart-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  gap: 24px;
}

.chart-card {
  background: var(--card-bg);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  padding: 20px;
  box-shadow: 0 4px 24px -1px var(--alpha-black-05);
  border: 1px solid var(--card-border);
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
  height: 320px;
  display: flex;
  flex-direction: column;
}

.chart-card:hover {
  box-shadow: 0 12px 32px -4px var(--alpha-black-10);
}

.chart-title {
  font-size: 15px;
  font-weight: var(--font-weight-semibold);
  margin-bottom: 16px;
  color: var(--text-primary);
}

.chart-container {
  flex: 1;
  width: 100%;
  min-height: 0;
}

.chart {
  width: 100%;
  height: 100%;
}

/* Alerts */
.status-alert-container {
  margin-top: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ios-alert {
  background: var(--alpha-white-60);
  backdrop-filter: blur(var(--blur-sm));
  border-radius: var(--radius-md);
  padding: 16px;
  display: flex;
  gap: 16px;
  align-items: flex-start;
  border: 1px solid var(--alpha-black-05);
}

.alert-icon {
  width: 24px;
  height: 24px;
  border-radius: var(--radius-circle);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: bold;
  color: white;
  flex-shrink: 0;
}

.alert-icon.warning {
  background-color: var(--warning-color);
}

.alert-icon.info {
  background-color: var(--primary-color);
}

.alert-content h4 {
  margin: 0 0 4px 0;
  font-size: 15px;
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
}

.alert-content p {
  margin: 0;
  color: var(--text-secondary);
  font-size: var(--font-size-md);
}

@media (max-width: 768px) {
  .overview-grid {
    grid-template-columns: 1fr !important;
  }

  .chart-grid {
    grid-template-columns: 1fr !important;
  }

  .header-top {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }

  .header-actions {
    width: 100%;
    overflow-x: auto;
    padding-bottom: 4px;
  }
}
</style>

<style>
/* Dark Mode Overrides */

/* --- Header --- */
.dark .ios-header {
  background: var(--card-bg);
  border-bottom: 1px solid var(--border-default);
}

/* --- Server Icon --- */
.dark .server-icon {
  background: linear-gradient(135deg, var(--alpha-white-10), var(--alpha-white-05));
  box-shadow: var(--shadow-sm);
}

.dark .icon-placeholder {
  color: var(--text-secondary);
}

/* --- Cards --- */
.dark .overview-card,
.dark .chart-card {
  background: var(--card-bg);
  border-color: var(--card-border);
}

.dark .overview-card:hover,
.dark .chart-card:hover {
  background: var(--alpha-white-08);
  border-color: var(--primary-light);
}

/* --- Buttons --- */
.dark .ios-btn {
  background: var(--alpha-white-08);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
}

.dark .ios-btn:hover {
  background: var(--alpha-white-15);
  color: var(--text-primary);
}

.dark .ios-btn-primary {
  background: var(--primary-color);
  border-color: transparent;
  box-shadow: var(--btn-primary-shadow);
}

.dark .ios-btn-primary:hover {
  background: var(--primary-hover);
}

/* --- Status Dots --- */
.dark .status-dot {
  box-shadow: 0 0 8px var(--error-bg);
}

.dark .status-dot.online {
  box-shadow: 0 0 8px var(--success-bg);
}

/* --- Alerts --- */
.dark .ios-alert {
  background: var(--card-bg);
  border-color: var(--card-border);
}

.dark .ios-alert a {
  color: var(--primary-color);
}

/* --- Text Overrides --- */
.dark .back-btn {
  color: var(--primary-color);
}
</style>
