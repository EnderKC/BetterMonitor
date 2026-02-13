<script setup lang="ts">
defineOptions({
  name: 'PublicServerDetail'
});

import { ref, onMounted, onUnmounted, computed, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import request from '../../utils/request';
import { useServerStore } from '../../stores/serverStore';
import { useSettingsStore } from '../../stores/settingsStore';
import { useThemeStore } from '../../stores/theme';
import { storeToRefs } from 'pinia';
import { useUIStore } from '../../stores/uiStore';
// 导入图表组件
import CpuUsageChartCard from '../../components/server/monitor/CpuUsageChartCard.vue';
import MemoryUsageChartCard from '../../components/server/monitor/MemoryUsageChartCard.vue';
import DiskUsageChartCard from '../../components/server/monitor/DiskUsageChartCard.vue';
import NetworkTrafficChartCard from '../../components/server/monitor/NetworkTrafficChartCard.vue';
import ProcessCountChartCard from '../../components/server/monitor/ProcessCountChartCard.vue';
import TcpUdpChartCard from '../../components/server/monitor/TcpUdpChartCard.vue';

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));
const serverStore = useServerStore();
const settingsStore = useSettingsStore();
const themeStore = useThemeStore();

const { isDark } = storeToRefs(themeStore);
const uiStore = useUIStore();

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
  processes: DataPoint[];
  network: {
    in: DataPoint[];
    out: DataPoint[];
  };
  connections: {
    tcp: DataPoint[];
    udp: DataPoint[];
  };
};

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// WebSocket连接
let ws: WebSocket | null = null;
const wsConnected = ref(false);

// 详情/隐藏标签切换
const activeTab = ref<'detail' | 'hidden'>('detail');

// 历史数据时间范围选择
const historyHours = ref<number>(1);
const historyLoading = ref(false);

// 服务器监控数据
const monitorData = ref<MonitorDataType>({
  cpu: [],
  memory: [],
  disk: [],
  processes: [],
  network: {
    in: [],
    out: []
  },
  connections: {
    tcp: [],
    udp: []
  }
});

// 计算在线状态
const isServerOnline = computed(() => {
  return serverStore.isServerOnline(serverId.value);
});

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    // 使用公开WebSocket接口获取服务器信息
    // 注意：这里假设后端提供了公开访问的接口
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/servers/public/${serverId.value}/ws`;

    console.log('连接公开服务器详情WebSocket:', wsUrl);

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      console.log('公开服务器详情WebSocket连接成功');
      wsConnected.value = true;
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        console.log('收到服务器数据:', data);

        if (data.type === 'welcome' || data.type === 'server_info') {
          updateServerInfo(data);
        }

        if (data.type === 'monitor') {
          updateServerMonitorData(data.data || data);
        }
      } catch (error) {
        console.error('解析WebSocket消息失败:', error);
      }
    };

    ws.onerror = (error) => {
      console.error('WebSocket错误:', error);
      wsConnected.value = false;
      message.error('获取服务器信息失败');
    };

    ws.onclose = () => {
      console.log('WebSocket连接已关闭');
      wsConnected.value = false;
    };

  } catch (error) {
    console.error('获取服务器信息失败:', error);
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
    uiStore.stopLoading();
  }
};

// 更新服务器信息
const updateServerInfo = (data: any) => {
  // 提取服务器基本信息
  serverInfo.value = {
    id: data.server_id || serverId.value,
    name: data.name || serverInfo.value.name || `Server ${serverId.value}`,
    hostname: data.hostname || data.system_info?.hostname || '未知',
    status: data.status || 'unknown',
    online: (data.status || '').toLowerCase() === 'online',

    // 系统信息
    os: data.os || data.system_info?.os || '未知',
    arch: data.arch || data.system_info?.kernel_arch || '未知',
    cpu_model: data.cpu_model || data.system_info?.cpu_model || '未知',
    cpu_cores: data.cpu_cores || data.system_info?.cpu_cores || 0,

    // 容量信息
    memory_total: data.memory_total || 0,
    disk_total: data.disk_total || 0,
    swap_total: data.swap_total || 0,

    // 区域信息
    region: data.region || data.country_code || '未知',

    // 时间信息
    boot_time: data.boot_time || 0,
    last_seen: data.last_seen || Math.floor(Date.now() / 1000),
    uptime: data.uptime || 0,

    // 网络信息
    ip: data.ip || data.public_ip || '未知',

    // 描述信息
    description: data.description || '',
  };

  // 更新store
  serverStore.updateServerStatus(serverId.value, serverInfo.value.status);
};

// 更新服务器监控数据
const updateServerMonitorData = (data: any) => {
  if (!serverInfo.value) return;

  // 限制数组长度为30（保留最近30条数据）
  const maxDataPoints = 30;
  const currentTime = new Date().toLocaleTimeString();

  // 更新当前监控数据
  serverInfo.value = {
    ...serverInfo.value,

    // CPU
    cpu_usage: data.cpu_usage || 0,

    // 内存
    memory_used: data.memory_used || 0,
    memory_total: data.memory_total || serverInfo.value.memory_total || 0,

    // 磁盘
    disk_used: data.disk_used || 0,
    disk_total: data.disk_total || serverInfo.value.disk_total || 0,

    // Swap
    swap_used: data.swap_used || 0,
    swap_total: data.swap_total || serverInfo.value.swap_total || 0,

    // 负载
    load_avg_1: data.load_avg_1 || 0,
    load_avg_5: data.load_avg_5 || 0,
    load_avg_15: data.load_avg_15 || 0,

    // 网络
    network_in: data.network_in || 0,
    network_out: data.network_out || 0,
    network_in_total: data.network_in_total || 0,
    network_out_total: data.network_out_total || 0,

    // 时间
    last_seen: data.timestamp || Math.floor(Date.now() / 1000),
  };

  // 更新CPU数据
  if (data.cpu_usage !== undefined) {
    let cpuValue = Number(data.cpu_usage);
    if (cpuValue < 1 && cpuValue > 0) {
      cpuValue = cpuValue * 100;
    }
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

    if (data.memory_used <= 100) {
      memoryUsagePercent = Number(data.memory_used);
    } else if (data.memory_total && data.memory_total > 0) {
      const memoryUsed = Number(data.memory_used);
      const memoryTotal = Number(data.memory_total);
      if (!isNaN(memoryUsed) && !isNaN(memoryTotal)) {
        memoryUsagePercent = (memoryUsed / memoryTotal) * 100;
      }
    }

    monitorData.value.memory.push({
      time: currentTime,
      value: Math.min(Math.max(memoryUsagePercent, 0), 100)
    });

    if (monitorData.value.memory.length > maxDataPoints) {
      monitorData.value.memory.shift();
    }
  }

  // 更新磁盘数据
  if (data.disk_used !== undefined) {
    let diskUsagePercent = 0;

    if (data.disk_used <= 100) {
      diskUsagePercent = Number(data.disk_used);
    } else if (data.disk_total && data.disk_total > 0) {
      const diskUsed = Number(data.disk_used);
      const diskTotal = Number(data.disk_total);
      if (!isNaN(diskUsed) && !isNaN(diskTotal)) {
        diskUsagePercent = (diskUsed / diskTotal) * 100;
      }
    }

    monitorData.value.disk.push({
      time: currentTime,
      value: Math.min(Math.max(diskUsagePercent, 0), 100)
    });

    if (monitorData.value.disk.length > maxDataPoints) {
      monitorData.value.disk.shift();
    }
  }

  // 更新网络数据
  if (data.network_in !== undefined) {
    const networkIn = Number(data.network_in);
    if (!isNaN(networkIn)) {
      const networkInMB = networkIn / (1024 * 1024);

      monitorData.value.network.in.push({
        time: currentTime,
        value: networkInMB
      });

      if (monitorData.value.network.in.length > maxDataPoints) {
        monitorData.value.network.in.shift();
      }
    }
  }

  if (data.network_out !== undefined) {
    const networkOut = Number(data.network_out);
    if (!isNaN(networkOut)) {
      const networkOutMB = networkOut / (1024 * 1024);

      monitorData.value.network.out.push({
        time: currentTime,
        value: networkOutMB
      });

      if (monitorData.value.network.out.length > maxDataPoints) {
        monitorData.value.network.out.shift();
      }
    }
  }

  // 更新进程数数据 - 只有值大于0时才更新，避免用无效数据覆盖正常值
  if (data.processes !== undefined && data.processes > 0) {
    const processCount = Number(data.processes);
    if (!isNaN(processCount)) {
      monitorData.value.processes.push({
        time: currentTime,
        value: processCount
      });

      if (monitorData.value.processes.length > maxDataPoints) {
        monitorData.value.processes.shift();
      }
    }
  }

  // 更新TCP连接数数据 - 只有值大于0时才更新
  if (data.tcp_connections !== undefined && data.tcp_connections > 0) {
    const tcpCount = Number(data.tcp_connections);
    if (!isNaN(tcpCount)) {
      monitorData.value.connections.tcp.push({
        time: currentTime,
        value: tcpCount
      });

      if (monitorData.value.connections.tcp.length > maxDataPoints) {
        monitorData.value.connections.tcp.shift();
      }
    }
  }

  // 更新UDP连接数数据 - 只有值大于0时才更新
  if (data.udp_connections !== undefined && data.udp_connections > 0) {
    const udpCount = Number(data.udp_connections);
    if (!isNaN(udpCount)) {
      monitorData.value.connections.udp.push({
        time: currentTime,
        value: udpCount
      });

      if (monitorData.value.connections.udp.length > maxDataPoints) {
        monitorData.value.connections.udp.shift();
      }
    }
  }

  // 更新store
  serverStore.updateServerMonitorData(serverId.value, data);
};

// 格式化字节大小
const formatBytes = (bytes: number): string => {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i];
};

// 格式化使用量/总量
const formatUsedTotal = (used: number, total: number): string => {
  if (!total) return '-';
  const percentage = ((used / total) * 100).toFixed(1);
  return `${formatBytes(used)} / ${formatBytes(total)} (${percentage}%)`;
};

// 格式化时间
const formatTime = (timestamp: number): string => {
  if (!timestamp) return '未知';
  const date = new Date(timestamp * 1000);
  return date.toLocaleString();
};

// 格式化运行时间
const formatUptime = (seconds: number): string => {
  if (!seconds) return '未知';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);

  if (days > 0) return `${days}天 ${hours}小时`;
  if (hours > 0) return `${hours}小时 ${minutes}分`;
  return `${minutes}分钟`;
};

// 计算运行时间
const uptimeText = computed(() => {
  if (!serverInfo.value.boot_time) return '未知';
  const now = Math.floor(Date.now() / 1000);
  const uptime = now - serverInfo.value.boot_time;
  return formatUptime(uptime);
});

// 磁盘信息
const diskText = computed(() => {
  return formatUsedTotal(
    serverInfo.value.disk_used || 0,
    serverInfo.value.disk_total || 0
  );
});

// 内存信息
const memoryText = computed(() => {
  return formatUsedTotal(
    serverInfo.value.memory_used || 0,
    serverInfo.value.memory_total || 0
  );
});

// Swap信息
const swapText = computed(() => {
  return formatUsedTotal(
    serverInfo.value.swap_used || 0,
    serverInfo.value.swap_total || 0
  );
});

// Load信息
const loadText = computed(() => {
  const load1 = serverInfo.value.load_avg_1?.toFixed(2) || '0.00';
  const load5 = serverInfo.value.load_avg_5?.toFixed(2) || '0.00';
  const load15 = serverInfo.value.load_avg_15?.toFixed(2) || '0.00';
  return `1分钟: ${load1}, 5分钟: ${load5}, 15分钟: ${load15}`;
});

// 网络流量信息
const trafficText = computed(() => {
  const networkIn = formatBytes(serverInfo.value.network_in || 0);
  const networkOut = formatBytes(serverInfo.value.network_out || 0);
  return `上传: ${networkOut}/s, 下载: ${networkIn}/s`;
});

// 启动时间
const bootTimeText = computed(() => {
  return formatTime(serverInfo.value.boot_time || 0);
});

// 最后上报时间
const lastReportText = computed(() => {
  return formatTime(serverInfo.value.last_seen || 0);
});

// 清空监控数据
const clearMonitorData = () => {
  monitorData.value = {
    cpu: [],
    memory: [],
    disk: [],
    processes: [],
    network: { in: [], out: [] },
    connections: { tcp: [], udp: [] }
  };
};

// 获取历史监控数据
const fetchHistoryData = async () => {
  historyLoading.value = true;
  try {
    const response = await request.get(`/servers/public/${serverId.value}/monitor`, {
      params: { hours: historyHours.value }
    });

    if (response.data && Array.isArray(response.data)) {
      // 清空现有数据
      clearMonitorData();

      // 处理历史数据
      response.data.forEach((item: any) => {
        const time = new Date(item.timestamp || item.Timestamp).toLocaleTimeString();

        // CPU
        if (item.cpu_usage !== undefined || item.CPUUsage !== undefined) {
          let cpuValue = Number(item.cpu_usage ?? item.CPUUsage);
          if (cpuValue < 1 && cpuValue > 0) cpuValue = cpuValue * 100;
          monitorData.value.cpu.push({ time, value: Math.min(Math.max(cpuValue, 0), 100) });
        }

        // 内存
        const memUsed = item.memory_used ?? item.MemoryUsed ?? 0;
        const memTotal = item.memory_total ?? item.MemoryTotal ?? 0;
        if (memTotal > 0) {
          const memPercent = (memUsed / memTotal) * 100;
          monitorData.value.memory.push({ time, value: Math.min(Math.max(memPercent, 0), 100) });
        }

        // 磁盘
        const diskUsed = item.disk_used ?? item.DiskUsed ?? 0;
        const diskTotal = item.disk_total ?? item.DiskTotal ?? 0;
        if (diskTotal > 0) {
          const diskPercent = (diskUsed / diskTotal) * 100;
          monitorData.value.disk.push({ time, value: Math.min(Math.max(diskPercent, 0), 100) });
        }

        // 网络
        const networkIn = item.network_in ?? item.NetworkIn ?? 0;
        const networkOut = item.network_out ?? item.NetworkOut ?? 0;
        monitorData.value.network.in.push({ time, value: networkIn / (1024 * 1024) });
        monitorData.value.network.out.push({ time, value: networkOut / (1024 * 1024) });

        // 进程数
        const processes = item.processes ?? item.Processes ?? 0;
        if (processes > 0) {
          monitorData.value.processes.push({ time, value: processes });
        }

        // TCP/UDP
        const tcp = item.tcp_connections ?? item.TCPConnections ?? 0;
        const udp = item.udp_connections ?? item.UDPConnections ?? 0;
        if (tcp > 0) {
          monitorData.value.connections.tcp.push({ time, value: tcp });
        }
        if (udp > 0) {
          monitorData.value.connections.udp.push({ time, value: udp });
        }
      });

      console.log(`加载了 ${response.data.length} 条历史监控数据`);
    }
  } catch (error) {
    console.error('获取历史监控数据失败:', error);
  } finally {
    historyLoading.value = false;
  }
};

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

// 监听历史时间范围变化
watch(historyHours, async () => {
  await fetchHistoryData();
});

// 页面挂载时获取服务器信息
onMounted(async () => {
  await settingsStore.loadPublicSettings();
  // 先获取历史数据
  await fetchHistoryData();
  // 然后连接 WebSocket 接收实时数据
  await fetchServerInfo();
});

// 页面卸载时关闭WebSocket
onUnmounted(() => {
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }
});
</script>

<template>
  <div class="public-server-detail-container">
    <div class="detail-header">
      <a-page-header class="page-header" @back="router.push('/dashboard')">
        <template #title>
          <span class="gradient-title">
            {{ serverInfo.name }}
            <span v-if="serverInfo.hostname && serverInfo.hostname !== '未知'" class="hostname-tag">{{ serverInfo.hostname
            }}</span>
          </span>
        </template>
        <template #subTitle>
          <span class="sub-title">{{ serverInfo.ip }} • {{ serverInfo.os || '未知系统' }}</span>
        </template>
        <template #extra>
          <div class="header-actions">
            <a-tag :color="isServerOnline ? 'success' : 'error'" class="status-tag">
              {{ isServerOnline ? '运行中' : '已离线' }}
            </a-tag>
            <a-segmented v-model:value="historyHours" :options="[
              { label: '1小时', value: 1 },
              { label: '12小时', value: 12 },
              { label: '24小时', value: 24 }
            ]" size="middle" />
          </div>
        </template>
      </a-page-header>
    </div>

    <div class="detail-content">
      <a-spin :spinning="loading" tip="加载中...">
        <!-- 概览卡片网格 -->
        <div class="overview-grid">
          <!-- 状态与运行时间 -->
          <div class="overview-card">
            <p class="label">运行状态</p>
            <div class="status-value">
              <div class="status-dot" :class="{ online: isServerOnline }"></div>
              <h3>{{ isServerOnline ? '在线' : '离线' }}</h3>
            </div>
            <small>运行时间 {{ uptimeText }}</small>
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
                <span class="speed-value">{{ formatBytes(serverInfo.network_out) }}/s</span>
              </div>
              <div class="speed-item">
                <span class="arrow down">↓</span>
                <span class="speed-value">{{ formatBytes(serverInfo.network_in) }}/s</span>
              </div>
            </div>
            <small>总流量: {{ formatBytes(serverInfo.network_in_total + serverInfo.network_out_total) }}</small>
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
              <span class="sys-item">{{ serverInfo.os }}</span>
            </div>
            <small>{{ serverInfo.arch }} • {{ serverInfo.region !== '未知' ? serverInfo.region : serverInfo.hostname
              }}</small>
          </div>

          <!-- 描述 (全宽) -->
          <div class="overview-card full-width" v-if="serverInfo.description">
            <p class="label">备注</p>
            <p class="description-text">{{ serverInfo.description }}</p>
          </div>
        </div>



        <!-- 监控图表区域 -->
        <div class="monitor-cards-section">
          <div class="section-header">
            <h3 class="section-title">监控趋势</h3>
            <a-spin v-if="historyLoading" size="small" />
          </div>

          <div class="chart-grid">
            <!-- CPU 使用率图表 -->
            <div class="chart-card">
              <div class="chart-title">CPU 使用率</div>
              <div class="chart-content">
                <CpuUsageChartCard :data="monitorData.cpu" height="100%" />
              </div>
            </div>

            <!-- 内存使用图表 -->
            <div class="chart-card">
              <div class="chart-title">内存使用</div>
              <div class="chart-content">
                <MemoryUsageChartCard :data="monitorData.memory" height="100%" />
              </div>
            </div>

            <!-- 磁盘使用图表 -->
            <div class="chart-card">
              <div class="chart-title">磁盘使用</div>
              <div class="chart-content">
                <DiskUsageChartCard :data="monitorData.disk" height="100%" />
              </div>
            </div>

            <!-- 网络流量图表 -->
            <div class="chart-card">
              <div class="chart-title">网络流量</div>
              <div class="chart-content">
                <NetworkTrafficChartCard :data="monitorData.network" height="100%" />
              </div>
            </div>

            <!-- 进程数图表 -->
            <div class="chart-card">
              <div class="chart-title">进程数</div>
              <div class="chart-content">
                <ProcessCountChartCard :data="monitorData.processes" height="100%" />
              </div>
            </div>

            <!-- TCP/UDP连接数 -->
            <div class="chart-card">
              <div class="chart-title">连接数</div>
              <div class="chart-content">
                <TcpUdpChartCard :data="monitorData.connections" height="100%" />
              </div>
            </div>
          </div>
        </div>

        <!-- 状态提示 -->
        <div v-if="!isServerOnline" class="status-alert-container">
          <a-alert message="服务器离线" description="无法获取实时监控数据，请检查服务器状态。" type="error" show-icon />
        </div>

        <div v-if="isServerOnline && !wsConnected" class="status-alert-container">
          <a-alert message="连接断开" description="实时监控连接已断开，正在尝试重连..." type="info" show-icon />
        </div>
      </a-spin>
    </div>
  </div>
</template>

<style scoped>
.public-server-detail-container {
  min-height: 100vh;
  background: transparent;
}

.detail-header {
  background: var(--card-bg);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-bottom: 1px solid var(--card-border);
  padding: 0 24px;
  position: sticky;
  top: 0;
  z-index: 100;
}

.page-header {
  padding: 16px 0;
}

.gradient-title {
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-bold);
  background: linear-gradient(135deg, var(--primary-color), #4096ff);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.sub-title {
  font-size: var(--font-size-sm);
  font-family: "SF Mono", Menlo, monospace;
  color: var(--text-secondary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 16px;
}

.status-tag {
  font-weight: var(--font-weight-semibold);
  border-radius: 4px;
}

.detail-content {
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}

/* Overview Grid */
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
  border-color: rgba(22, 119, 255, 0.3);
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
  box-shadow: 0 0 8px rgba(255, 77, 79, 0.4);
}

.status-dot.online {
  background-color: var(--success-color);
  box-shadow: 0 0 8px rgba(82, 196, 26, 0.4);
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

.chart-content {
  flex: 1;
  min-height: 0;
}

.status-alert-container {
  margin-top: 24px;
}

@media (max-width: 768px) {
  .overview-grid {
    grid-template-columns: 1fr !important;
  }

  .chart-grid {
    grid-template-columns: 1fr !important;
  }
}

.hostname-tag {
  font-size: var(--font-size-md);
  font-weight: normal;
  color: var(--text-secondary);
  margin-left: 8px;
  background: var(--alpha-white-10);
  padding: 2px 8px;
  border-radius: 4px;
}

.description-text {
  color: var(--text-secondary);
  font-size: var(--font-size-md);
  line-height: 1.5;
  margin: 0;
  white-space: pre-wrap;
}

.full-width {
  grid-column: 1 / -1;
}
</style>


<style>
/* Dark Mode Overrides */
.dark .detail-header {
  background: rgba(20, 20, 20, 0.85);
  border-bottom: 1px solid var(--alpha-white-10);
}

.dark .overview-card,
.dark .chart-card {
  background: rgba(30, 30, 30, 0.6);
  border-color: var(--alpha-white-08);
}

.dark .overview-card:hover,
.dark .chart-card:hover {
  background: rgba(40, 40, 40, 0.8);
  border-color: rgba(22, 119, 255, 0.4);
}
</style>