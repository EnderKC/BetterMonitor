<script setup lang="ts">
defineOptions({
  name: 'ServerDetail'
});
import { ref, onMounted, onUnmounted, computed, nextTick, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Tabs } from 'ant-design-vue';
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
};

// 服务器监控数据
const monitorData = ref<MonitorDataType>({
  cpu: [],
  memory: [],
  disk: [],
  network: {
    in: [],
    out: []
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
    user_id: server.user_id
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
            </a-space>
          </div>
        </div>

        <div class="header-content">
          <div class="server-icon">
            <div class="icon-placeholder">{{ serverInfo.os ? serverInfo.os.charAt(0).toUpperCase() : 'S' }}</div>
          </div>
          <div class="server-title-area">
            <h1 class="server-title">{{ serverInfo.name }}</h1>
            <div class="server-meta">
              <span class="meta-item">{{ serverInfo.ip }}</span>
              <span class="meta-dot">•</span>
              <span class="meta-item">{{ serverInfo.os || '未知系统' }}</span>
              <span class="meta-dot">•</span>
              <span class="status-badge" :class="isServerOnline ? 'online' : 'offline'">
                {{ isServerOnline ? '运行中' : '已离线' }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- 内容区域 -->
      <div class="ios-content">
        <a-tabs default-active-key="info" class="ios-tabs" :animated="false">
          <a-tab-pane key="info" tab="概览">
            <div class="ios-card-grid">
              <!-- 基本信息卡片 -->
              <div class="ios-card glass-card">
                <div class="card-header">
                  <h3>基本信息</h3>
                </div>
                <div class="ios-list">
                  <div class="list-item">
                    <span class="label">ID</span>
                    <span class="value">{{ serverInfo.id }}</span>
                  </div>
                  <div class="list-divider"></div>
                  <div class="list-item">
                    <span class="label">主机名</span>
                    <span class="value">{{ serverInfo.hostname || '未知' }}</span>
                  </div>
                  <div class="list-divider"></div>
                  <div class="list-item">
                    <span class="label">系统版本</span>
                    <span class="value">{{ serverInfo.os_version || '未知' }}</span>
                  </div>
                  <div class="list-divider"></div>
                  <div class="list-item">
                    <span class="label">内核版本</span>
                    <span class="value">{{ serverInfo.kernel_version || '未知' }}</span>
                  </div>
                  <div class="list-divider"></div>
                  <div class="list-item">
                    <span class="label">最后在线</span>
                    <span class="value">{{ formatTime(serverInfo.last_seen) }}</span>
                  </div>
                </div>
              </div>

              <!-- 硬件配置卡片 -->
              <div class="ios-card glass-card">
                <div class="card-header">
                  <h3>硬件配置</h3>
                </div>
                <div class="ios-list">
                  <div class="list-item">
                    <span class="label">CPU型号</span>
                    <span class="value">{{ serverInfo.cpu_model || '未知' }}</span>
                  </div>
                  <div class="list-divider"></div>
                  <div class="list-item">
                    <span class="label">核心数</span>
                    <span class="value">{{ serverInfo.cpu_cores || '未知' }} 核</span>
                  </div>
                  <div class="list-divider"></div>
                  <div class="list-item">
                    <span class="label">内存总量</span>
                    <span class="value">{{ serverInfo.memory_total ? `${(serverInfo.memory_total / 1024 / 1024 /
                      1024).toFixed(2)} GB` : '未知' }}</span>
                  </div>
                  <div class="list-divider"></div>
                  <div class="list-item">
                    <span class="label">磁盘总量</span>
                    <span class="value">{{ serverInfo.disk_total ? `${(serverInfo.disk_total / 1024 / 1024 /
                      1024).toFixed(2)}
                      GB` : '未知' }}</span>
                  </div>
                </div>
              </div>

              <!-- 描述卡片 -->
              <div class="ios-card glass-card full-width">
                <div class="card-header">
                  <h3>备注描述</h3>
                </div>
                <div class="card-body">
                  <p class="description-text">{{ serverInfo.description || '暂无描述' }}</p>
                </div>
              </div>
            </div>
          </a-tab-pane>

          <a-tab-pane key="resource" tab="监控">
            <div class="monitor-panel" v-if="isServerOnline && wsConnected">
              <div class="ios-card-grid">
                <div class="ios-card glass-card chart-card">
                  <div class="card-header">
                    <h3>CPU 使用率</h3>
                  </div>
                  <div class="chart-container">
                    <v-chart class="chart" :option="cpuChartOption" autoresize />
                  </div>
                </div>

                <div class="ios-card glass-card chart-card">
                  <div class="card-header">
                    <h3>内存使用率</h3>
                  </div>
                  <div class="chart-container">
                    <v-chart class="chart" :option="memoryChartOption" autoresize />
                  </div>
                </div>

                <div class="ios-card glass-card chart-card">
                  <div class="card-header">
                    <h3>磁盘使用率</h3>
                  </div>
                  <div class="chart-container">
                    <v-chart class="chart" :option="diskChartOption" autoresize />
                  </div>
                </div>

                <div class="ios-card glass-card chart-card">
                  <div class="card-header">
                    <h3>网络流量</h3>
                  </div>
                  <div class="chart-container">
                    <v-chart class="chart" :option="networkChartOption" autoresize />
                  </div>
                </div>
              </div>
            </div>

            <!-- 状态提示 -->
            <div v-else class="status-alert-container">
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
          </a-tab-pane>
        </a-tabs>
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
  font-weight: 500;
  font-size: 15px;
  transition: opacity 0.2s;
}

.back-btn:hover {
  opacity: 0.7;
}

.back-arrow {
  font-size: 20px;
}

.header-content {
  display: flex;
  align-items: center;
  gap: 20px;
}

.server-icon {
  width: 64px;
  height: 64px;
  background: linear-gradient(135deg, #e0e0e0, #f5f5f5);
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-sm);
}

.icon-placeholder {
  font-size: 28px;
  font-weight: 600;
  color: var(--text-secondary);
}

.server-title-area {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.server-title {
  margin: 0;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: -0.5px;
  color: var(--text-primary);
}

.server-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary);
  font-size: 14px;
}

.meta-dot {
  font-size: 8px;
  opacity: 0.5;
}

.status-badge {
  padding: 2px 10px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 600;
}

.status-badge.online {
  background-color: rgba(52, 199, 89, 0.15);
  color: var(--success-color);
}

.status-badge.offline {
  background-color: rgba(255, 59, 48, 0.15);
  color: var(--error-color);
}

/* iOS Buttons */
.ios-btn {
  border: none;
  background: rgba(0, 0, 0, 0.05);
  color: var(--text-primary);
  font-weight: 500;
  box-shadow: none;
  transition: all 0.2s;
}

.ios-btn:hover {
  background: rgba(0, 0, 0, 0.1);
  color: var(--text-primary);
}

.ios-btn-primary {
  background: var(--primary-color);
  box-shadow: 0 2px 8px rgba(0, 122, 255, 0.3);
}

.ios-btn-primary:hover {
  background: var(--primary-hover);
}

/* iOS Content & Cards */
.ios-content {
  padding: 0 8px;
}

.ios-card-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 24px;
  margin-top: 20px;
}

.ios-card {
  padding: 0;
  overflow: hidden;
}

.full-width {
  grid-column: span 2;
}

.card-header {
  padding: 16px 24px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.card-header h3 {
  margin: 0;
  font-size: 17px;
  font-weight: 600;
  color: var(--text-primary);
}

.card-body {
  padding: 24px;
}

.description-text {
  color: var(--text-secondary);
  line-height: 1.6;
  margin: 0;
}

/* iOS List Style */
.ios-list {
  padding: 0 24px;
}

.list-item {
  display: flex;
  justify-content: space-between;
  padding: 16px 0;
  font-size: 15px;
}

.list-divider {
  height: 1px;
  background-color: rgba(0, 0, 0, 0.05);
}

.list-item .label {
  color: var(--text-secondary);
}

.list-item .value {
  color: var(--text-primary);
  font-weight: 500;
  font-family: -apple-system, BlinkMacSystemFont, "SF Mono", Menlo, monospace;
}

/* Charts */
.chart-card {
  height: 380px;
  display: flex;
  flex-direction: column;
}

.chart-container {
  flex: 1;
  padding: 16px;
  width: 100%;
}

.chart {
  width: 100%;
  height: 100%;
}

/* Tabs Customization */
:deep(.ant-tabs-nav) {
  margin-bottom: 0;
}

:deep(.ant-tabs-nav::before) {
  border-bottom: none;
}

:deep(.ant-tabs-tab) {
  padding: 8px 20px;
  margin: 0 4px 0 0;
  border-radius: 20px;
  transition: all 0.3s;
  font-size: 15px;
  color: var(--text-secondary);
}

:deep(.ant-tabs-tab-active) {
  background: rgba(0, 0, 0, 0.05);
}

:deep(.ant-tabs-tab-active .ant-tabs-tab-btn) {
  color: var(--text-primary);
  font-weight: 600;
  text-shadow: none;
}

:deep(.ant-tabs-ink-bar) {
  display: none;
}

/* Alerts */
.status-alert-container {
  margin-top: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ios-alert {
  background: rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(10px);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  gap: 16px;
  align-items: flex-start;
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.alert-icon {
  width: 24px;
  height: 24px;
  border-radius: 50%;
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
  font-weight: 600;
  color: var(--text-primary);
}

.alert-content p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 14px;
}

/* Responsive */
@media (max-width: 768px) {
  .ios-card-grid {
    grid-template-columns: 1fr;
  }

  .full-width {
    grid-column: span 1;
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
