<script setup lang="ts">
defineOptions({
  name: 'Dashboard'
});

import { ref, onMounted, onUnmounted, onActivated, onDeactivated, computed } from 'vue';

import { message } from 'ant-design-vue';
import { useRouter } from 'vue-router';
import { getToken } from '../../utils/auth';
import { useSettingsStore } from '../../stores/settingsStore';
import { useUIStore } from '../../stores/uiStore';
import type { LifeProbeSummary } from '@/types/life';
import {
  DesktopOutlined,
  GlobalOutlined,
  ClockCircleOutlined,
  CheckCircleFilled,
  CloseCircleFilled,
  CloudServerOutlined,
  DisconnectOutlined,
  ArrowUpOutlined,
  ArrowDownOutlined,
  ThunderboltOutlined,
  LinkOutlined,
  DatabaseOutlined,
  CloudSyncOutlined,
  CalendarOutlined,
  SyncOutlined,
  LineChartOutlined,
  CodeOutlined,
  AppleOutlined,
  WindowsOutlined,
  AndroidOutlined,
  HeartFilled,
  MobileOutlined,
  RestOutlined
} from '@ant-design/icons-vue';

// 添加router
const router = useRouter();
// 判断用户是否登录
const isLoggedIn = computed(() => !!getToken());
// 获取系统设置
const settingsStore = useSettingsStore();
const uiStore = useUIStore();

// 登录或进入控制台
const goToLoginOrAdmin = () => {
  if (isLoggedIn.value) {
    router.push('/admin/servers');
  } else {
    router.push('/login');
  }
};

// 服务器列表
const servers = ref<any[]>([]);
// 加载状态
const loading = ref(true);
// WebSocket连接状态
const wsConnections = ref<{ [key: string]: WebSocket | null }>({});
// 重连次数
const reconnectCounts = ref<{ [key: string]: number }>({});
const serverListWS = ref<WebSocket | null>(null);
const serverListHeartbeatTimer = ref<number | null>(null);
const serverListReconnectTimer = ref<number | null>(null);

// 生命探针数据
const lifeProbes = ref<LifeProbeSummary[]>([]);
const lifeLoading = ref(true);
const LIFE_STEP_GOAL = 10000;
const lifeProbesWS = ref<WebSocket | null>(null);
const lifeHeartbeatTimer = ref<number | null>(null);
const lifeReconnectTimer = ref<number | null>(null);

// 获取所有服务器的状态（通过公开WebSocket）
const fetchServers = () => {
  // 如果已有连接且处于打开或正在连接状态，则无需重新建立
  if (
    serverListWS.value &&
    (serverListWS.value.readyState === WebSocket.OPEN ||
      serverListWS.value.readyState === WebSocket.CONNECTING)
  ) {
    return;
  }

  // 清理计划中的重连
  if (serverListReconnectTimer.value !== null) {
    clearTimeout(serverListReconnectTimer.value);
    serverListReconnectTimer.value = null;
  }

  loading.value = true;

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  let wsUrl = `${protocol}//${window.location.host}/api/servers/public/ws`;

  // 如果用户已登录，添加token参数
  const token = getToken();
  if (token) {
    wsUrl += `?token=${encodeURIComponent(token)}`;
  }

  const ws = new WebSocket(wsUrl);
  serverListWS.value = ws;

  const connectionTimeout = window.setTimeout(() => {
    if (ws.readyState !== WebSocket.OPEN) {
      console.log('公开服务器WebSocket连接超时，主动关闭');
      ws.close();
    }
  }, 10000);

  const clearHeartbeat = () => {
    if (serverListHeartbeatTimer.value !== null) {
      clearInterval(serverListHeartbeatTimer.value);
      serverListHeartbeatTimer.value = null;
    }
  };

  const scheduleReconnect = () => {
    if (serverListReconnectTimer.value !== null) {
      clearTimeout(serverListReconnectTimer.value);
    }
    serverListReconnectTimer.value = window.setTimeout(() => {
      serverListReconnectTimer.value = null;
      fetchServers();
    }, 5000);
  };

  ws.onopen = () => {
    clearTimeout(connectionTimeout);
    console.log('公开服务器WebSocket连接成功');

    clearHeartbeat();
    serverListHeartbeatTimer.value = window.setInterval(() => {
      if (serverListWS.value && serverListWS.value.readyState === WebSocket.OPEN) {
        try {
          serverListWS.value.send(JSON.stringify({
            type: 'heartbeat',
            timestamp: Date.now()
          }));
        } catch (error) {
          console.error('公开服务器WebSocket心跳发送失败:', error);
        }
      } else {
        clearHeartbeat();
      }
    }, 25000);
  };

  ws.onerror = (error) => {
    console.error('公开服务器WebSocket错误:', error);
    if (servers.value.length === 0) {
      message.error('获取服务器状态失败');
      loading.value = false;
    }
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);

      if (data.type === 'server_list' && Array.isArray(data.servers)) {
        loading.value = false;
        uiStore.stopLoading();

        const processedServers = data.servers.map((server: any) => {
          const status = server.status || 'offline';
          const isOnline = status.toLowerCase() === 'online';
          const rawPublicIP = typeof server.public_ip === 'string' ? server.public_ip.trim() : '';
          const fallbackIP = server.ip || 'Unknown';
          const displayIP = rawPublicIP || fallbackIP;
          return {
            id: server.id,
            name: server.name || 'Unknown',
            status,
            online: isOnline,
            last_seen: server.last_seen || null,
            ip: fallbackIP,
            public_ip: rawPublicIP,
            display_ip: displayIP,
            os: server.os || '未知',
            cpu_usage: parseFloat(server.cpu_usage) || 0,
            memory_used: parseFloat(server.memory_used) || 0,
            memory_total: parseFloat(server.memory_total) || 0,
            disk_used: parseFloat(server.disk_used) || 0,
            disk_total: parseFloat(server.disk_total) || 0,
            load_avg_1: parseFloat(server.load_avg_1) || 0,
            load_avg_5: parseFloat(server.load_avg_5) || 0,
            load_avg_15: parseFloat(server.load_avg_15) || 0,
            cpu_cores: server.cpu_cores || 1,
            country_code: server.country_code || '',
            swap_used: parseFloat(server.swap_used) || 0,
            swap_total: parseFloat(server.swap_total) || 0,
            boot_time: parseInt(server.boot_time) || 0,
            network_in: parseFloat(server.network_in) || 0,
            network_out: parseFloat(server.network_out) || 0,
            network_in_total: parseFloat(server.network_in_total) || 0,
            network_out_total: parseFloat(server.network_out_total) || 0,
            latency: parseFloat(server.latency) || 0,
            packet_loss: parseFloat(server.packet_loss) || 0
          };
        });

        const onlineIds = new Set(processedServers.filter(s => s.online).map(s => s.id));

        // 关闭下线或已移除服务器的连接
        Object.keys(wsConnections.value).forEach(key => {
          const id = Number(key);
          if (!onlineIds.has(id)) {
            const conn = wsConnections.value[id];
            if (conn) {
              conn.onclose = null;
              conn.close();
            }
            delete wsConnections.value[id];
            delete reconnectCounts.value[id];
          }
        });

        servers.value = processedServers;

        // 为在线服务器确保存在连接
        onlineIds.forEach(id => {
          connectWebSocket(id as number);
        });
      } else {
        console.warn('收到未知的公开WebSocket消息:', data);
      }
    } catch (error) {
      console.error('解析公开WebSocket消息失败:', error);
    }
  };

  ws.onclose = () => {
    clearTimeout(connectionTimeout);
    clearHeartbeat();
    serverListWS.value = null;
    if (servers.value.length === 0) {
      loading.value = false;
    }
    console.log('公开服务器WebSocket连接已关闭，准备重连');
    scheduleReconnect();
  };
};

const clearLifeHeartbeat = () => {
  if (lifeHeartbeatTimer.value !== null) {
    clearInterval(lifeHeartbeatTimer.value);
    lifeHeartbeatTimer.value = null;
  }
};

const scheduleLifeReconnect = () => {
  if (lifeReconnectTimer.value !== null) {
    clearTimeout(lifeReconnectTimer.value);
  }
  lifeReconnectTimer.value = window.setTimeout(() => {
    lifeReconnectTimer.value = null;
    connectLifeProbesWS();
  }, 5000);
};

const connectLifeProbesWS = () => {
  if (
    lifeProbesWS.value &&
    (lifeProbesWS.value.readyState === WebSocket.OPEN ||
      lifeProbesWS.value.readyState === WebSocket.CONNECTING)
  ) {
    return;
  }

  if (lifeReconnectTimer.value !== null) {
    clearTimeout(lifeReconnectTimer.value);
    lifeReconnectTimer.value = null;
  }

  lifeLoading.value = true;

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  let wsUrl = `${protocol}//${window.location.host}/api/life-probes/public/ws`;
  const token = getToken();
  if (token) {
    wsUrl += `?token=${encodeURIComponent(token)}`;
  }

  const ws = new WebSocket(wsUrl);
  lifeProbesWS.value = ws;

  ws.onopen = () => {
    clearLifeHeartbeat();
    lifeHeartbeatTimer.value = window.setInterval(() => {
      if (lifeProbesWS.value && lifeProbesWS.value.readyState === WebSocket.OPEN) {
        try {
          lifeProbesWS.value.send(JSON.stringify({ type: 'heartbeat', timestamp: Date.now() }));
        } catch (error) {
          console.error('生命探针WebSocket心跳发送失败:', error);
        }
      } else {
        clearLifeHeartbeat();
      }
    }, 25000);
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'life_probe_list' && Array.isArray(data.life_probes)) {
        lifeProbes.value = data.life_probes;
        lifeLoading.value = false;
      }
    } catch (error) {
      console.error('解析生命探针数据失败:', error);
    }
  };

  ws.onerror = (error) => {
    console.error('生命探针WebSocket错误:', error);
    if (!lifeProbes.value.length) {
      message.error('生命探针数据连接失败');
    }
  };

  ws.onclose = () => {
    clearLifeHeartbeat();
    lifeProbesWS.value = null;
    scheduleLifeReconnect();
  };
};

// 为每个在线服务器建立WebSocket连接
const connectWebSocket = (serverId: number) => {
  // 如果已存在连接，且是打开状态，则不需再次连接
  if (wsConnections.value[serverId] && wsConnections.value[serverId]?.readyState === WebSocket.OPEN) {
    return;
  }

  // 关闭之前的连接（如果存在）
  if (wsConnections.value[serverId]) {
    wsConnections.value[serverId]!.onclose = null;
    wsConnections.value[serverId]!.close();
    wsConnections.value[serverId] = null;
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  // 使用新的公开WebSocket接口
  const wsUrl = `${protocol}//${window.location.host}/api/servers/public/${serverId}/ws`;

  try {
    const ws = new WebSocket(wsUrl);
    wsConnections.value[serverId] = ws;

    // 设置超时处理
    const connectionTimeout = setTimeout(() => {
      if (ws.readyState !== WebSocket.OPEN) {
        ws.close();
      }
    }, 10000);

    ws.onopen = () => {
      clearTimeout(connectionTimeout);
      reconnectCounts.value[serverId] = 0;

      // 添加心跳机制，每30秒发送一次心跳包
      const heartbeatInterval = setInterval(() => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          try {
            ws.send(JSON.stringify({
              type: 'heartbeat',
              timestamp: Date.now()
            }));
          } catch (error) {
            clearInterval(heartbeatInterval);
          }
        } else {
          clearInterval(heartbeatInterval);
        }
      }, 30000);

      // 在连接关闭时清除心跳
      ws.addEventListener('close', () => {
        clearInterval(heartbeatInterval);
      });
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);

        // 更新欢迎消息中的系统信息
        if (data.type === 'welcome') {
          updateServerStatus(serverId, true);
        }

        // 更新监控数据
        if (data.type === 'monitor') {
          if (data.data) {
            updateServerMonitorData(serverId, data.data);
          } else {
            updateServerMonitorData(serverId, data);
          }
        }

        // Agent离线通知
        if (data.type === 'agent_offline') {
          updateServerStatus(serverId, false);
        }

        // Agent升级状态推送
        if (data.type === 'agent_upgrade_status') {
          const status = data.status;
          if (status === 'completed' || status === 'success') {
            message.success(`服务器 ${serverId} Agent升级完成`);
          } else if (status === 'failed' || status === 'error') {
            message.error(`服务器 ${serverId} Agent升级失败: ${data.message || '未知错误'}`);
          }
        }
      } catch (error) {
        console.error(`解析服务器 ${serverId} WebSocket消息失败:`, error);
      }
    };

    ws.onerror = (error) => {
      clearTimeout(connectionTimeout);
    };

    ws.onclose = () => {
      clearTimeout(connectionTimeout);

      // 尝试重连, 但次数更有限
      if (!reconnectCounts.value[serverId]) {
        reconnectCounts.value[serverId] = 0;
      }

      if (reconnectCounts.value[serverId] < 3) { // 减少重连次数到3次
        reconnectCounts.value[serverId]++;
        setTimeout(() => {
          connectWebSocket(serverId);
        }, 5000); // 延长重连间隔到5秒
      } else {
        // 更新服务器状态为离线
        updateServerStatus(serverId, false);
      }
    };
  } catch (error) {
    console.error(`创建服务器 ${serverId} WebSocket连接失败:`, error);
  }
};

// 更新服务器状态
const updateServerStatus = (serverId: number, isOnline: boolean) => {
  const serverIndex = servers.value.findIndex(server => server.id === serverId);
  if (serverIndex !== -1) {
    servers.value[serverIndex].online = isOnline;
    servers.value[serverIndex].status = isOnline ? 'online' : 'offline';
  }
};

// 更新服务器监控数据
const updateServerMonitorData = (serverId: number, data: any) => {
  const serverIndex = servers.value.findIndex(server => server.id === serverId);
  if (serverIndex !== -1) {
    const server = servers.value[serverIndex];

    // 更新CPU使用率
    if (data.cpu_usage !== undefined) {
      let cpuValue = Number(data.cpu_usage);
      if (cpuValue < 1 && cpuValue > 0) {
        cpuValue = cpuValue * 100;
      }
      server.cpu_usage = cpuValue;
    }

    // 更新内存使用情况
    if (data.memory_used !== undefined) {
      if (data.memory_used <= 100) {
        server.memory_used = data.memory_used;
      } else if (data.memory_total && data.memory_total > 0) {
        server.memory_used = data.memory_used;
        server.memory_total = data.memory_total;
      }
    }

    if (data.memory_total !== undefined) {
      server.memory_total = data.memory_total;
    }

    // 更新磁盘使用情况
    if (data.disk_used !== undefined) {
      if (data.disk_used <= 100) {
        server.disk_used = data.disk_used;
      } else {
        server.disk_used = data.disk_used;
      }
    }

    if (data.disk_total !== undefined) {
      server.disk_total = data.disk_total;
    }

    // 更新系统负载
    if (data.load_avg_1 !== undefined) server.load_avg_1 = data.load_avg_1;
    if (data.load_avg_5 !== undefined) server.load_avg_5 = data.load_avg_5;
    if (data.load_avg_15 !== undefined) server.load_avg_15 = data.load_avg_15;

    // 更新Swap
    if (data.swap_used !== undefined) server.swap_used = data.swap_used;
    if (data.swap_total !== undefined) server.swap_total = data.swap_total;

    // 更新启动时间
    if (data.boot_time !== undefined) server.boot_time = data.boot_time;

    // 更新网络
    if (data.network_in !== undefined) server.network_in = data.network_in;
    if (data.network_out !== undefined) server.network_out = data.network_out;

    // 更新总流量
    if (data.network_in_total !== undefined) server.network_in_total = data.network_in_total;
    if (data.network_out_total !== undefined) server.network_out_total = data.network_out_total;

    // 更新延迟和丢包率
    if (data.latency !== undefined) server.latency = data.latency;
    if (data.packet_loss !== undefined) server.packet_loss = data.packet_loss;

    // 如果有状态信息，更新状态
    if (data.status !== undefined) {
      server.status = data.status;
      server.online = data.status.toLowerCase() === 'online';
    }

    // 更新最后更新时间
    server.last_seen = data.timestamp || Math.floor(Date.now() / 1000);
  }
};

// 定时刷新数据
let timer: number | null = null;

// 重置刷新定时器
const resetRefreshTimer = () => {
  if (timer !== null) {
    clearInterval(timer);
    timer = null;
  }

  const refreshInterval = settingsStore.getUiRefreshIntervalMs();
  timer = window.setInterval(() => {
    fetchServers();
    connectLifeProbesWS();
  }, refreshInterval);
};

const startDashboard = async () => {
  await settingsStore.loadPublicSettings();
  fetchServers();
  connectLifeProbesWS();
  resetRefreshTimer();
};

const stopDashboard = () => {
  if (timer !== null) {
    clearInterval(timer);
    timer = null;
  }

  Object.values(wsConnections.value).forEach(ws => {
    if (ws) {
      ws.onclose = null;
      ws.close();
    }
  });
  wsConnections.value = {};

  if (serverListHeartbeatTimer.value !== null) {
    clearInterval(serverListHeartbeatTimer.value);
    serverListHeartbeatTimer.value = null;
  }
  if (serverListReconnectTimer.value !== null) {
    clearTimeout(serverListReconnectTimer.value);
    serverListReconnectTimer.value = null;
  }
  if (serverListWS.value) {
    serverListWS.value.onclose = null;
    serverListWS.value.close();
    serverListWS.value = null;
  }

  clearLifeHeartbeat();
  if (lifeReconnectTimer.value !== null) {
    clearTimeout(lifeReconnectTimer.value);
    lifeReconnectTimer.value = null;
  }
  if (lifeProbesWS.value) {
    lifeProbesWS.value.onclose = null;
    lifeProbesWS.value.close();
    lifeProbesWS.value = null;
  }
};

onMounted(() => {
  startDashboard();
});

onActivated(() => {
  startDashboard();
});

onDeactivated(() => {
  stopDashboard();
});

onUnmounted(() => {
  stopDashboard();
});

// 格式化文件大小
const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i];
};

// 计算百分比值
const getPercentage = (used: number, total: number) => {
  if (!total || total <= 0) return 0;
  const percentage = (used / total) * 100;
  return percentage > 100 ? 100 : parseFloat(percentage.toFixed(1));
};

// 获取负载百分比
const getLoadPercentage = (load: number, cores: number) => {
  if (!cores || cores <= 0) return 0;
  const percentage = (load / cores) * 100;
  return percentage > 100 ? 100 : parseFloat(percentage.toFixed(1));
};

// 获取进度条颜色
const getProgressColor = (percentage: number) => {
  if (percentage >= 90) return '#ff4d4f';
  if (percentage >= 70) return '#faad14';
  return '#1677ff';
};

// 格式化时间为"多久之前"
const formatTimeAgo = (timestamp: number) => {
  if (!timestamp) return '未知';

  const now = Math.floor(Date.now() / 1000);
  const diff = now - timestamp;

  if (diff < 60) return `${diff}秒前`;
  if (diff < 3600) return `${Math.floor(diff / 60)}分前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;

  const date = new Date(timestamp * 1000);
  return date.toLocaleDateString();
};

// 获取国家旗帜Emoji
const getFlagEmoji = (countryCode: string) => {
  if (!countryCode) return '';
  const codePoints = countryCode
    .toUpperCase()
    .split('')
    .map(char => 127397 + char.charCodeAt(0));
  return String.fromCodePoint(...codePoints);
};

const getFlagTooltip = (server: any) => {
  if (server?.public_ip) {
    return `出口IP: ${server.public_ip}`;
  }
  if (server?.ip && server.ip !== 'Unknown') {
    return `IP: ${server.ip}`;
  }
  return '出口IP未知';
};

// 格式化网络速率
const formatSpeed = (bytesPerSec: number) => {
  return formatBytes(bytesPerSec) + '/s';
};

// 格式化运行时间
const formatUptime = (bootTime: number) => {
  if (!bootTime) return '未知';
  const now = Math.floor(Date.now() / 1000);
  const uptime = now - bootTime;

  const days = Math.floor(uptime / 86400);
  const hours = Math.floor((uptime % 86400) / 3600);
  const minutes = Math.floor((uptime % 3600) / 60);

  if (days > 0) return `${days}天 ${hours}小时`;
  if (hours > 0) return `${hours}小时 ${minutes}分`;
  return `${minutes}分钟`;
};

// 计算离线服务器数量
const offlineServersCount = computed(() => {
  return servers.value.filter(s => !s.online).length;
});

// 计算总带宽
const totalBandwidth = computed(() => {
  return servers.value.reduce((total, server) => {
    return total + (server.network_in || 0) + (server.network_out || 0);
  }, 0);
});

// 格式化带宽
const formatBandwidth = (bytesPerSec: number) => {
  if (bytesPerSec === 0) return '0 B/s';
  const k = 1024;
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s'];
  const i = Math.floor(Math.log(bytesPerSec) / Math.log(k));
  return (bytesPerSec / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i];
};
const getLifeHeartRate = (probe: LifeProbeSummary) => {
  if (!probe.latest_heart_rate) return '--';
  return Math.round(probe.latest_heart_rate.value);
};

const formatLifeTime = (value?: string) => {
  if (!value) return '尚未同步';
  return new Date(value).toLocaleString();
};

const getBatteryLevel = (probe: LifeProbeSummary) => {
  if (probe.battery_level === undefined || probe.battery_level === null) return '--';
  return `${Math.round(probe.battery_level * 100)}%`;
};

const getStepsProgress = (probe: LifeProbeSummary) => {
  if (!probe.steps_today) return 0;
  return Math.min(100, Math.round((probe.steps_today / LIFE_STEP_GOAL) * 100));
};

const openLifeDetail = (probeId: number) => {
  router.push({ name: 'LifeProbeDetail', params: { id: probeId } });
};

const openServerDetail = (serverId: number) => {
  router.push({ name: 'PublicServerDetail', params: { id: serverId } });
};

// 获取简短的CPU型号
const getShortCpuModel = (model: string) => {
  if (!model) return '';

  // 移除常见的多余文字
  let short = model
    .replace(/Intel\(R\)/gi, '')
    .replace(/Core\(TM\)/gi, '')
    .replace(/CPU/gi, '')
    .replace(/Processor/gi, '')
    .replace(/@.*/, '') // 移除频率信息
    .trim();

  // 压缩多个空格
  short = short.replace(/\s+/g, ' ');

  // 如果太长，截断
  if (short.length > 20) {
    return short.substring(0, 20) + '...';
  }

  return short;
};

// 格式化紧凑配置信息
const formatCompactConfig = (server: any) => {
  const cores = server.cpu_cores ? `${server.cpu_cores}C` : '?C';

  // 格式化内存，去掉 'B' 后缀，例如 '7.8 GB' -> '7.8 G'
  const mem = server.memory_total ? formatBytes(server.memory_total).replace('B', '') : '?G';

  // 格式化磁盘
  const disk = server.disk_total ? formatBytes(server.disk_total).replace('B', '') : '?G';

  return `${cores} ${mem} / ${disk}`;
};

// 获取简短的Device ID
const getShortDeviceId = (id: string) => {
  if (!id) return 'Unknown';
  if (id.length <= 12) return id;
  return id.substring(0, 6) + '...' + id.substring(id.length - 4);
};

// 估算距离 (km)
const estimateDistance = (steps: number) => {
  if (!steps) return '0.0';
  return (steps * 0.0007).toFixed(1); // 假设一步0.7米
};

// 估算卡路里 (kcal)
const estimateCalories = (steps: number) => {
  if (!steps) return '0';
  return Math.round(steps * 0.04).toLocaleString(); // 假设一步0.04千卡
};

// 获取同步延迟显示
const getSyncLatency = (lastSync?: string) => {
  if (!lastSync) return '--';
  const diff = Date.now() - new Date(lastSync).getTime();
  if (diff < 60000) return '刚刚';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分前`;
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分前`;
  return `${Math.floor(diff / 3600000)}小时前`;
};

const isProbeOffline = (probe: LifeProbeSummary) => {
  if (!probe.last_sync_at) return true;
  const diff = Date.now() - new Date(probe.last_sync_at).getTime();
  return diff > 6 * 60 * 60 * 1000; // 6 hours
};

const getSleepDurationLabel = (probe: LifeProbeSummary) => {
  if (!probe.sleep_duration) return '--';
  const hours = Math.floor(probe.sleep_duration / 3600);
  const minutes = Math.floor((probe.sleep_duration % 3600) / 60);
  return `${hours}h ${minutes}m`;
};

const getSleepProgress = (probe: LifeProbeSummary) => {
  if (!probe.sleep_duration) return 0;
  // Assuming 10 hours is the goal (36000 seconds)
  return Math.min(100, (probe.sleep_duration / 36000) * 100);
};
</script>

<template>
  <div class="dashboard-container">
    <div class="dashboard-header">
      <div class="dashboard-title">
        <h1 class="gradient-text">Better Monitor</h1>
        <p>实时服务器状态监控</p>
      </div>

      <a-button type="primary" @click="goToLoginOrAdmin">
        {{ isLoggedIn ? '进入控制台' : '登录系统' }}
      </a-button>
    </div>

    <div class="dashboard-content">
      <!-- 顶部统计卡片 -->
      <div class="stats-overview">
        <div class="stat-card">
          <div class="stat-icon server-total-icon">
            <CloudServerOutlined />
          </div>
          <div class="stat-info">
            <div class="stat-value">{{ servers.length }}</div>
            <div class="stat-label">服务器总数</div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon server-offline-icon">
            <DisconnectOutlined />
          </div>
          <div class="stat-info">
            <div class="stat-value">{{ offlineServersCount }}</div>
            <div class="stat-label">离线服务器</div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon bandwidth-icon">
            <GlobalOutlined />
          </div>
          <div class="stat-info">
            <div class="stat-value">{{ formatBandwidth(totalBandwidth) }}</div>
            <div class="stat-label">实时总带宽</div>
          </div>
        </div>
      </div>

      <a-spin :spinning="loading && lifeLoading" tip="加载中...">
        <div v-if="servers.length === 0 && lifeProbes.length === 0 && !loading && !lifeLoading" class="empty-wrapper">
          <a-empty description="暂无监控设备" />
        </div>

        <div v-else class="servers-grid">
          <!-- Life Probes -->
          <div v-for="probe in lifeProbes" :key="'life-' + probe.id" class="server-card glass-card life-probe-card"
            @click="openLifeDetail(probe.id)">
            <!-- 头部信息 -->
            <div class="card-header">
              <div class="header-left">
                <span class="flag-icon">❤️</span>
                <div class="header-text">
                  <h3 class="server-name" :title="probe.name">{{ probe.name }}</h3>
                </div>
              </div>
              <div class="header-right">
                <div class="status-dot" :class="{ online: !isProbeOffline(probe) }"></div>
              </div>
            </div>

            <!-- 信息条 (Device ID only) -->
            <div class="server-info-bar">
              <span class="info-cpu" :title="probe.device_id">
                <MobileOutlined /> {{ getShortDeviceId(probe.device_id) }}
              </span>
            </div>

            <!-- 核心指标网格 -->
            <div class="metrics-grid-compact life-metrics-grid">
              <!-- 心率 -->
              <div class="metric-compact">
                <div class="metric-row">
                  <span class="metric-icon heart-icon-active">
                    <HeartFilled />
                  </span>
                  <span class="metric-name">心率</span>
                  <span class="metric-val">{{ getLifeHeartRate(probe) }} BPM</span>
                </div>
                <div class="progress-bar-bg">
                  <div class="progress-bar-fill"
                    :style="{ width: Math.min(100, (probe.latest_heart_rate?.value || 0) / 200 * 100) + '%', background: '#ff4d4f' }">
                  </div>
                </div>
              </div>

              <!-- 步数 -->
              <div class="metric-compact">
                <div class="metric-row">
                  <span class="metric-icon" style="color: #faad14">
                    <ThunderboltOutlined />
                  </span>
                  <span class="metric-name">步数</span>
                  <span class="metric-val">{{ Math.round(probe.steps_today).toLocaleString() }}</span>
                </div>
                <div class="progress-bar-bg">
                  <div class="progress-bar-fill"
                    :style="{ width: getStepsProgress(probe) + '%', background: '#faad14' }"></div>
                </div>
              </div>

              <!-- 电量 -->
              <div class="metric-compact">
                <div class="metric-row">
                  <span class="metric-icon" style="color: #52c41a">
                    <MobileOutlined />
                  </span>
                  <span class="metric-name">电量</span>
                  <span class="metric-val">{{ getBatteryLevel(probe) }}</span>
                </div>
                <div class="progress-bar-bg">
                  <div class="progress-bar-fill"
                    :style="{ width: (probe.battery_level || 0) * 100 + '%', background: '#52c41a' }"></div>
                </div>
              </div>

              <!-- 睡眠 -->
              <div class="metric-compact">
                <div class="metric-row">
                  <span class="metric-icon" style="color: #722ed1">
                    <RestOutlined />
                  </span>
                  <span class="metric-name">睡眠</span>
                  <span class="metric-val">{{ getSleepDurationLabel(probe) }}</span>
                </div>
                <div class="progress-bar-bg">
                  <div class="progress-bar-fill"
                    :style="{ width: getSleepProgress(probe) + '%', background: '#722ed1' }"></div>
                </div>
              </div>
            </div>

            <!-- 活动数据 (模仿网络部分) -->
            <div class="network-section">
              <div class="net-item">
                <span class="net-val success">{{ estimateDistance(probe.steps_today) }} km</span>
              </div>
              <div class="net-item">
                <span class="net-val primary">{{ estimateCalories(probe.steps_today) }} kcal</span>
              </div>
            </div>

            <div class="network-total-row">
              <div class="net-total-item">
                <span title="预估距离">今日里程</span>
              </div>
              <div class="net-total-item">
                <span title="预估消耗">今日消耗</span>
              </div>
            </div>

            <a-divider style="margin: 12px 0;" />

            <!-- 同步状态 (模仿延迟部分) -->
            <div class="latency-section">
              <div class="latency-item">
                <ClockCircleOutlined />
                <span class="latency-label">同步延迟</span>
                <span class="latency-val">{{ getSyncLatency(probe.last_sync_at) }}</span>
              </div>
              <div class="latency-item">
                <span class="latency-label">目标达成</span>
                <span class="latency-val">{{ getStepsProgress(probe) }}%</span>
              </div>
            </div>

            <!-- 目标进度条 (模仿延迟条) -->
            <div class="latency-bar-bg">
              <div class="latency-bar-fill"
                :style="{ width: getStepsProgress(probe) + '%', background: getStepsProgress(probe) >= 100 ? '#52c41a' : '#1677ff' }">
              </div>
            </div>

            <a-divider style="margin: 12px 0;" />

            <!-- 底部信息 -->
            <div class="card-footer-compact">
              <div class="footer-item">
                <span v-if="isProbeOffline(probe)" style="color: var(--error-color); font-weight: bold;">可能似了</span>
                <span v-else>{{ probe.device_id ? '已连接' : '未连接' }}</span>
              </div>
              <div class="footer-item">
                <SyncOutlined :spin="false" />
                <span>{{ formatLifeTime(probe.last_sync_at) }}</span>
              </div>
            </div>
          </div>

          <!-- Server Probes -->
          <div v-for="server in servers" :key="server.id" class="server-card glass-card server-probe-card"
            @click="openServerDetail(server.id)">
            <!-- 头部信息 -->
            <div class="card-header">
              <div class="header-left">
                <span v-if="server.country_code" class="flag-icon" :title="getFlagTooltip(server)">{{
                  getFlagEmoji(server.country_code) }}</span>
                <span v-else class="flag-icon" :title="getFlagTooltip(server)">🏳️</span>
                <div class="header-text">
                  <h3 class="server-name" :title="server.name">{{ server.name }}</h3>
                  <a v-if="server.public_ip || server.ip" :href="'http://' + (server.public_ip || server.ip)"
                    target="_blank" class="server-link" @click.stop>
                    <LinkOutlined />
                  </a>
                  <div class="server-ip-wrapper"
                    v-if="server.display_ip && (isLoggedIn || !server.display_ip.includes('*'))">
                    <a-tooltip :title="server.display_ip">
                      <span class="server-ip-tag">{{ server.display_ip }}</span>
                    </a-tooltip>
                  </div>
                </div>
              </div>
              <div class="header-right">
                <span class="os-icon" :title="server.os">
                  <AppleOutlined
                    v-if="server.os.toLowerCase().includes('darwin') || server.os.toLowerCase().includes('mac')" />
                  <WindowsOutlined v-else-if="server.os.toLowerCase().includes('windows')" />
                  <AndroidOutlined v-else-if="server.os.toLowerCase().includes('android')" />
                  <CodeOutlined v-else />
                </span>
                <div class="status-dot" :class="{ online: server.online }"></div>
              </div>
            </div>

            <!-- 服务器信息条 (CPU型号 + 配置) -->
            <div class="server-info-bar">
              <span class="info-cpu" v-if="server.cpu_model" :title="server.cpu_model">
                <DesktopOutlined /> {{ getShortCpuModel(server.cpu_model) }}
              </span>
              <span class="info-divider" v-if="server.cpu_model">|</span>
              <span class="info-config">{{ formatCompactConfig(server) }}</span>
            </div>

            <!-- 监控指标网格 (2列) -->
            <div class="metrics-grid-compact">
              <!-- CPU -->
              <div class="metric-compact">
                <div class="metric-row">
                  <span class="metric-icon">
                    <DesktopOutlined />
                  </span>
                  <span class="metric-name">CPU</span>
                  <span class="metric-val">{{ server.cpu_usage.toFixed(1) }}%</span>
                </div>
                <div class="progress-bar-bg">
                  <div class="progress-bar-fill"
                    :style="{ width: server.cpu_usage + '%', background: getProgressColor(server.cpu_usage) }"></div>
                </div>
              </div>

              <!-- 内存 -->
              <div class="metric-compact">
                <div class="metric-row">
                  <span class="metric-icon">
                    <DatabaseOutlined />
                  </span>
                  <span class="metric-name">内存</span>
                  <span class="metric-val">{{ getPercentage(server.memory_used, server.memory_total) }}%</span>
                </div>
                <div class="progress-bar-bg">
                  <div class="progress-bar-fill"
                    :style="{ width: getPercentage(server.memory_used, server.memory_total) + '%', background: '#d4b106' }">
                  </div>
                </div>
              </div>

              <!-- 磁盘 -->
              <div class="metric-compact">
                <div class="metric-row">
                  <span class="metric-icon">
                    <DatabaseOutlined />
                  </span>
                  <span class="metric-name">磁盘</span>
                  <span class="metric-val">{{ getPercentage(server.disk_used, server.disk_total) }}%</span>
                </div>
                <div class="progress-bar-bg">
                  <div class="progress-bar-fill"
                    :style="{ width: getPercentage(server.disk_used, server.disk_total) + '%', background: '#fa8c16' }">
                  </div>
                </div>
              </div>
            </div>

            <!-- 网络速率 -->
            <div class="network-section">
              <div class="net-item">
                <ArrowDownOutlined class="down-icon" />
                <span class="net-val success">{{ formatSpeed(server.network_in) }}</span>
              </div>
              <div class="net-item">
                <ArrowUpOutlined class="up-icon" />
                <span class="net-val primary">{{ formatSpeed(server.network_out) }}</span>
              </div>
            </div>

            <div class="network-total-row">
              <div class="net-total-item">
                <ArrowDownOutlined style="color: var(--success-color);" />
                <span title="总下载流量">{{ formatBytes(server.network_in_total || 0) }}</span>
              </div>
              <div class="net-total-item">
                <ArrowUpOutlined style="color: var(--primary-color);" />
                <span title="总上传流量">{{ formatBytes(server.network_out_total || 0) }}</span>
              </div>
            </div>

            <a-divider style="margin: 12px 0;" />

            <!-- 延迟与丢包 -->
            <div class="latency-section">
              <div class="latency-item">
                <ClockCircleOutlined />
                <span class="latency-label">延迟</span>
                <span class="latency-val">{{ server.latency !== undefined ? server.latency.toFixed(1) + ' ms' : '-- ms'
                }}</span>
              </div>
              <div class="latency-item">
                <LineChartOutlined />
                <span class="latency-label">丢包率</span>
                <span class="latency-val">{{ server.packet_loss !== undefined && server.packet_loss > 0 ?
                  server.packet_loss.toFixed(1) + '%' : '0.0%'
                }}</span>
              </div>
            </div>

            <!-- 延迟条 -->
            <div class="latency-bar-bg">
              <div class="latency-bar-fill"
                :style="{ width: Math.min(100, server.latency > 0 ? (server.latency / 200 * 100) : 0) + '%', background: server.latency > 100 ? '#ff4d4f' : server.latency > 50 ? '#faad14' : '#52c41a' }">
              </div>
            </div>

            <a-divider style="margin: 12px 0;" />

            <!-- 底部信息 (移除到期时间) -->
            <div class="card-footer-compact">
              <div class="footer-item">
                <!-- 占位或显示其他信息 -->
              </div>
              <div class="footer-item">
                <SyncOutlined :spin="server.online" />
                <span>在线: {{ formatUptime(server.boot_time) }}</span>
              </div>
            </div>
          </div>
        </div>
      </a-spin>

      <!-- Life Section Removed and Merged into Grid -->
    </div>

    <div class="dashboard-footer">
      <span>Better Monitor</span> &copy; {{ new Date().getFullYear() }}
    </div>
  </div>
</template>

<style scoped>
.dashboard-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 24px;
  background: transparent;
}

.dashboard-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 32px;
  padding: 0 8px;
}

.dashboard-title h1 {
  font-size: var(--font-size-4xl);
  margin-bottom: 4px;
  font-weight: var(--font-weight-bold);
  letter-spacing: -0.5px;
}

.dashboard-title p {
  font-size: 15px;
  color: var(--text-secondary);
  margin: 0;
}

.dashboard-content {
  flex: 1;
}

.servers-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 24px;
}

.server-card {
  padding: 20px;
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--card-bg);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
  border: 1px solid var(--card-border);
  box-shadow: 0 4px 24px -1px var(--alpha-black-05);
}

.server-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 32px -4px var(--alpha-black-10);
  border-color: var(--alpha-white-60);
}

/* Header */
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.flag-icon {
  font-size: var(--font-size-2xl);
  line-height: 1;
}

.header-text {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

/* IP wrapper 承担flex收缩责任 */
.server-ip-wrapper {
  flex-shrink: 1;
  min-width: 0;
  max-width: 150px;
  display: flex;
}

/* 确保tooltip根节点也能正确收缩 */
.server-ip-wrapper>* {
  min-width: 0;
  max-width: 100%;
}

.server-name {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
  white-space: nowrap;
  margin: 0;
  /* 优先显示完整服务器名字 */
  flex-shrink: 0;
  min-width: 0;
  /* 只在极端情况下才压缩名字 */
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.server-ip-tag {
  font-size: var(--font-size-xs);
  color: var(--text-secondary);
  background: var(--alpha-black-04);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: "SF Mono", Menlo, monospace;
  /* 空间不足时压缩IP显示 */
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: help;
}


.server-link {
  color: var(--text-secondary);
  font-size: var(--font-size-md);
  display: flex;
  align-items: center;
  transition: color 0.2s;
  flex-shrink: 0;
}

.server-link:hover {
  color: var(--primary-color);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.os-icon {
  font-size: var(--font-size-xl);
  color: var(--error-color);
  display: flex;
  align-items: center;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-circle);
  background-color: var(--error-color);
  box-shadow: 0 0 0 2px rgba(255, 59, 48, 0.2);
}

.status-dot.online {
  background-color: var(--success-color);
  box-shadow: 0 0 0 2px rgba(52, 199, 89, 0.2);
}

/* Metrics Grid */
.metrics-grid-compact {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 24px;
  margin-bottom: 20px;
}

.metric-compact {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.server-info-bar {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-bottom: 20px;
  padding: 8px 12px;
  background: var(--alpha-black-02);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
}

.info-cpu {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: var(--font-weight-medium);
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.info-divider {
  color: var(--border-color);
  opacity: 0.5;
}

.info-config {
  font-family: "SF Mono", Menlo, monospace;
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
}

.cpu-model-tag {
  font-size: var(--font-size-xs);
  color: var(--text-secondary);
  background: var(--alpha-black-04);
  padding: 1px 6px;
  border-radius: 4px;
  margin-left: auto;
  /* 推到右边 */
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.config-compact-row {
  display: flex;
}

.config-value {
  font-size: var(--font-size-sm);
  background: var(--alpha-black-04);
  padding: 4px 10px;
  border-radius: var(--radius-xs);
  color: var(--text-primary);
  font-family: "SF Mono", Menlo, monospace;
  font-weight: var(--font-weight-semibold);
  width: 100%;
  text-align: center;
}



.metric-row {
  display: flex;
  align-items: center;
  font-size: var(--font-size-sm);
}

.metric-icon {
  color: var(--text-secondary);
  margin-right: 6px;
  display: flex;
  align-items: center;
}

.metric-name {
  color: var(--text-secondary);
  flex: 1;
}

.metric-val {
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
  font-family: "SF Mono", Menlo, monospace;
}

.progress-bar-bg {
  height: 6px;
  background: rgba(0, 0, 0, 0.06);
  border-radius: 3px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease;
}

/* Network Section */
.network-section {
  display: flex;
  justify-content: space-between;
  margin-bottom: 12px;
}

.net-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--font-size-sm);
}

.down-icon {
  color: var(--success-color);
}

.up-icon {
  color: var(--primary-color);
}

.net-val {
  font-family: "SF Mono", Menlo, monospace;
  font-weight: var(--font-weight-semibold);
}

.net-val.success {
  color: var(--success-color);
}

.net-val.primary {
  color: var(--primary-color);
}

.network-total-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: var(--font-size-xs);
  color: var(--text-secondary);
}

.net-total-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

/* Latency Section */
.latency-section {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: var(--font-size-sm);
}

.latency-item {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
}

.latency-val {
  color: var(--text-primary);
  font-weight: var(--font-weight-medium);
  font-family: "SF Mono", Menlo, monospace;
}

.latency-bar-bg {
  height: 4px;
  background: rgba(0, 0, 0, 0.06);
  border-radius: 2px;
  overflow: hidden;
  margin-bottom: 8px;
}

.latency-bar-fill {
  height: 100%;
  border-radius: 2px;
}

/* Footer */
.card-footer-compact {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
  margin-top: auto;
  padding-top: 16px;
  border-top: 1px solid var(--alpha-black-05);
}

.footer-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.footer-item span {
  color: var(--primary-color);
}

.dashboard-footer {
  text-align: center;
  padding: 24px 0;
  color: var(--text-hint);
  font-size: var(--font-size-xs);
}





.stats-overview {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 24px;
  margin-bottom: 32px;
}

.stat-card {
  background: var(--card-bg);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  padding: 24px;
  display: flex;
  align-items: center;
  border: 1px solid var(--card-border);
  box-shadow: 0 4px 24px -1px var(--alpha-black-05);
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 32px -4px var(--alpha-black-10);
  border-color: var(--alpha-white-60);
}

.stat-icon {
  width: 56px;
  height: 56px;
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-4xl);
  margin-right: 20px;
  flex-shrink: 0;
}

.server-total-icon {
  background: linear-gradient(135deg, var(--primary-light), var(--primary-bg));
  color: var(--primary-color);
}

.server-offline-icon {
  background: linear-gradient(135deg, rgba(255, 59, 48, 0.1), rgba(255, 59, 48, 0.05));
  color: var(--error-color);
}

.bandwidth-icon {
  background: linear-gradient(135deg, var(--success-bg), rgba(52, 199, 89, 0.05));
  color: var(--success-color);
}

.stat-info {
  flex: 1;
  min-width: 0;
}

.stat-value {
  font-size: var(--font-size-4xl);
  font-weight: var(--font-weight-bold);
  color: var(--text-primary);
  line-height: 1.2;
  margin-bottom: 4px;
  font-family: "SF Mono", Menlo, monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.stat-label {
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
  font-weight: var(--font-weight-medium);
}

.country-flag {
  font-size: var(--font-size-4xl);
  line-height: 1;
}

.metric-item.full-width {
  grid-column: span 2;
}

.uptime {
  display: flex;
  align-items: center;
  gap: 6px;
}



.life-section {
  margin-top: 48px;
}

.life-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 20px;
}

.life-card {
  padding: 20px;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.life-card:hover {
  transform: translateY(-4px);
}

.life-count {
  color: var(--text-secondary);
  font-weight: var(--font-weight-medium);
}

.life-card-header h3 {
  margin: 0;
  font-size: var(--font-size-xl);
}

.life-card-header p {
  margin: 4px 0 0;
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
}

.life-metrics {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin: 16px 0;
}

.life-metric {
  display: flex;
  gap: 8px;
  align-items: center;
}

.life-metric p {
  margin: 0;
  font-size: var(--font-size-xs);
  color: var(--text-secondary);
}

.life-metric h4 {
  margin: 2px 0 0;
  font-size: var(--font-size-xl);
}

.life-metric-icon {
  width: 34px;
  height: 34px;
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-lg);
  color: #fff;
}

.life-probe-card {
  cursor: pointer;
  border: 1px solid var(--error-bg);
}

.life-probe-card:hover {
  border-color: rgba(255, 77, 79, 0.5);
  box-shadow: 0 12px 32px -4px var(--error-bg);
}

.server-probe-card {
  cursor: pointer;
}

.server-probe-card:hover {
  transform: translateY(-6px);
  box-shadow: 0 16px 40px -6px var(--info-bg);
}

.life-metrics-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 24px;
  margin-bottom: 20px;
}

.heart-icon-active {
  color: var(--error-color);
  animation: heartbeat 1.5s ease-in-out infinite;
}

@keyframes heartbeat {
  0% {
    transform: scale(1);
  }

  14% {
    transform: scale(1.3);
  }

  28% {
    transform: scale(1);
  }

  42% {
    transform: scale(1.3);
  }

  70% {
    transform: scale(1);
  }
}

.focus-tag {
  margin: 0;
  border-radius: var(--radius-xs);
  font-weight: var(--font-weight-medium);
}

.life-metric-icon.heart {
  background: linear-gradient(135deg, #2563eb, #7c3aed);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.life-metric-icon.steps {
  background: linear-gradient(135deg, var(--primary-color), #69c0ff);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.life-metric-icon.focus {
  background: linear-gradient(135deg, #722ed1, #b37feb);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.life-progress .progress-label {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-xs);
  color: var(--text-secondary);
  margin-bottom: 6px;
}

.life-meta {
  margin-top: 12px;
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-xs);
  color: var(--text-secondary);
}
</style>

<style>
.dark .server-ip-tag {
  background: var(--alpha-white-08);
}

.dark .cpu-model-tag {
  background: var(--alpha-white-08);
}

.dark .server-info-bar {
  background: rgba(255, 255, 255, 0.04);
}

.dark .config-value {
  background: var(--alpha-white-08);
}

/* Dark mode specific adjustments */
.dark .server-card,
.dark .stat-card {
  background: rgba(30, 30, 30, 0.6);
  border-color: var(--alpha-white-08);
}

.dark .server-card:hover,
.dark .stat-card:hover {
  background: rgba(40, 40, 40, 0.8);
  border-color: var(--alpha-white-15);
}
</style>
