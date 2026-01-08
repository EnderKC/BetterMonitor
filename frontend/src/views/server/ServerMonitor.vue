<script setup lang="ts">
import { ref, onMounted, onUnmounted, reactive, computed, nextTick, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import * as echarts from 'echarts/core';
import { LineChart } from 'echarts/charts';
import {
  TitleComponent,
  TooltipComponent,
  GridComponent,
  DataZoomComponent,
  LegendComponent
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import request from '../../utils/request';
// 导入服务器状态store
import { useServerStore } from '../../stores/serverStore';
// 导入设置store
import { useSettingsStore } from '../../stores/settingsStore';
import { ClockCircleOutlined } from '@ant-design/icons-vue';

// 注册必须的组件
echarts.use([
  TitleComponent,
  TooltipComponent,
  GridComponent,
  DataZoomComponent,
  LegendComponent,
  LineChart,
  CanvasRenderer
]);

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));
// 获取服务器状态store
const serverStore = useServerStore();
// 获取设置store
const settingsStore = useSettingsStore();

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);
const connecting = ref(false);

// 添加isOnline计算属性，使用store中的状态
const isOnline = computed(() => {
  // 优先使用store中的服务器状态
  return serverStore.isServerOnline(serverId.value);
});

// 添加getStatusType方法，与index.vue保持一致
const getStatusType = (online: boolean): string => {
  return online ? 'success' : 'error';
};

// 添加一个判断是否有数据的计算属性
const hasData = computed(() => {
  return monitorData.cpu.length > 0 ||
    monitorData.memory.length > 0 ||
    monitorData.disk.length > 0 ||
    monitorData.network.in.length > 0;
});

// 图表实例
const cpuChartRef = ref();
const memoryChartRef = ref();
const diskChartRef = ref();
const networkChartRef = ref();
const loadChartRef = ref();
let cpuChart: echarts.ECharts | null = null;
let memoryChart: echarts.ECharts | null = null;
let diskChart: echarts.ECharts | null = null;
let networkChart: echarts.ECharts | null = null;
let loadChart: echarts.ECharts | null = null;

// 监控数据
const monitorData = reactive({
  cpu: [] as { time: string; value: number }[],
  memory: [] as { time: string; value: number }[],
  disk: [] as { time: string; value: number }[],
  network: {
    in: [] as { time: string; value: number }[],
    out: [] as { time: string; value: number }[]
  },
  load: {
    load1: [] as { time: string; value: number }[],
    load5: [] as { time: string; value: number }[],
    load15: [] as { time: string; value: number }[]
  }
});

const clearMonitorData = () => {
  monitorData.cpu = [];
  monitorData.memory = [];
  monitorData.disk = [];
  monitorData.network.in = [];
  monitorData.network.out = [];
  monitorData.load.load1 = [];
  monitorData.load.load5 = [];
  monitorData.load.load15 = [];
};

// WebSocket连接
let ws: WebSocket | null = null;
const wsConnected = ref(false);
const reconnectCount = ref(0);
const maxReconnectAttempts = 5;
// 添加心跳定时器引用
let heartbeatInterval: number | null = null;
// 记录心跳失败次数
let heartbeatFailCount = 0;
const maxHeartbeatFails = 3;


// 获取历史监控数据
const fetchHistoricalData = async () => {
  if (!serverId.value) return;

  try {
    loading.value = true;
    clearMonitorData();

    // 计算时间范围
    const endTime = new Date();
    const startTime = new Date(endTime.getTime() - (settingsStore.chartHistoryHours * 60 * 60 * 1000));

    // 以非UTC格式构建日期参数，避免时区转换问题
    const startTimeStr = startTime.toISOString();
    const endTimeStr = endTime.toISOString();

    // 构建API请求URL，添加时间范围参数
    const url = `/servers/${serverId.value}/monitor`;
    const params = {
      start_time: startTimeStr,
      end_time: endTimeStr
    };

    console.log('发送历史监控数据请求:', { url, params, startTime, endTime });

    // 添加详细日志，记录完整请求信息
    const response = await request.get(url, { params });

    // 输出完整响应以便调试
    console.log('历史监控数据API完整响应:', JSON.stringify(response));
    console.log('响应类型:', typeof response);

    // 检查响应是否为空
    if (!response) {
      console.error('获取历史数据失败：响应为空');
      message.error('无法获取监控历史数据：服务器未返回数据');
      return;
    }

    let historicalData = [];

    // 由于axios拦截器已经返回了response.data，这里的response就是实际的数据
    if (Array.isArray(response)) {
      // 如果响应直接是数组
      console.log('响应数据是数组');
      historicalData = response;
    } else if (response && Array.isArray(response.data)) {
      // 如果响应是 { data: [...] } 格式
      console.log('响应数据是 { data: [...] } 格式');
      historicalData = response.data;
    } else if (response && typeof response === 'object') {
      // 如果响应是对象但没有data字段，尝试其他可能的字段
      console.log('响应数据是对象，检查可能的数据字段');
      const possibleFields = ['data', 'records', 'items', 'result', 'results', 'list', 'history'];
      for (const field of possibleFields) {
        if (Array.isArray(response[field])) {
          console.log(`找到可能的数据字段: ${field}`);
          historicalData = response[field];
          break;
        }
      }
    }

    console.log(`获取到 ${historicalData.length} 条历史监控数据:`, historicalData);

    if (historicalData.length === 0) {
      console.warn('没有历史监控数据，可能的原因：1. 时间范围内无数据 2. 数据格式不匹配 3. 服务器未返回数据');
      message.info('暂无监控历史数据（仅展示实时数据；需要 Agent 上报后才会产生历史记录）');
      updateCharts();
      return;
    }

    // 按时间排序数据
    const sortedData = [...historicalData].sort((a, b) => {
      const timeA = new Date(a.timestamp || a.created_at || a.time || 0).getTime();
      const timeB = new Date(b.timestamp || b.created_at || b.time || 0).getTime();
      return timeA - timeB;
    });

    console.log('排序后的数据:', sortedData);

    // 处理历史数据
    sortedData.forEach((entry) => {
      // 尝试获取时间戳字段，支持多种格式
      const rawTimestamp = entry.timestamp || entry.created_at || entry.time;
      if (!rawTimestamp) {
        console.warn('条目缺少时间戳字段:', entry);
        return; // 跳过没有时间戳的条目
      }

      // 格式化时间
      const timestamp = new Date(rawTimestamp);
      const timeStr = timestamp.toLocaleTimeString('zh-CN', { hour12: false });
      const dateStr = timestamp.toLocaleDateString('zh-CN');
      const fullTimeStr = `${dateStr} ${timeStr}`;

      // 记录原始值和处理后的值，帮助调试
      console.log('处理数据项:', {
        time: fullTimeStr,
        cpu_original: entry.cpu_usage,
        memory_original: entry.memory_used,
        memory_total: entry.memory_total,
        disk_original: entry.disk_used,
        disk_total: entry.disk_total,
        network_in: entry.network_in,
        network_out: entry.network_out
      });

      // CPU数据 - 支持多种字段名
      const cpuRawValue = entry.cpu_usage || entry.cpu || entry.cpu_percent || 0;
      let cpuValue = parseFloat((cpuRawValue).toFixed(2));
      // 如果CPU使用率小于1且大于0，认为是小数形式，转换为百分比
      if (cpuValue > 0 && cpuValue < 1) {
        cpuValue = cpuValue * 100;
      }
      monitorData.cpu.push({
        time: fullTimeStr,
        value: cpuValue
      });

      // 内存数据 - 确保使用正确的单位，支持多种字段名
      const memoryRawValue = entry.memory_used || entry.memory || entry.mem_used || entry.mem || 0;
      const memoryTotalValue = entry.memory_total || entry.mem_total || 0;

      let memoryPercent = 0;
      if (typeof memoryRawValue === 'number') {
        if (memoryRawValue <= 100) {
          memoryPercent = memoryRawValue;
        } else if (memoryTotalValue && memoryTotalValue > 0) {
          memoryPercent = (memoryRawValue / memoryTotalValue) * 100;
        }
      }
      monitorData.memory.push({
        time: fullTimeStr,
        value: parseFloat(memoryPercent.toFixed(2))
      });

      // 磁盘数据 - 确保使用正确的单位，支持多种字段名
      const diskRawValue = entry.disk_used || entry.disk || entry.disk_percent || 0;
      const diskTotalValue = entry.disk_total || 0;

      let diskPercent = 0;
      if (typeof diskRawValue === 'number') {
        if (diskRawValue <= 100) {
          diskPercent = diskRawValue;
        } else if (diskTotalValue && diskTotalValue > 0) {
          diskPercent = (diskRawValue / diskTotalValue) * 100;
        }
      }
      monitorData.disk.push({
        time: fullTimeStr,
        value: parseFloat(diskPercent.toFixed(2))
      });

      // 网络数据 - 转换为MB/s，支持多种字段名
      const networkInRaw = entry.network_in || entry.net_in || entry.rx_bytes || 0;
      const networkOutRaw = entry.network_out || entry.net_out || entry.tx_bytes || 0;

      const networkInMB = convertToMB(networkInRaw);
      const networkOutMB = convertToMB(networkOutRaw);

      monitorData.network.in.push({
        time: fullTimeStr,
        value: parseFloat(networkInMB.toFixed(2))
      });

      monitorData.network.out.push({
        time: fullTimeStr,
        value: parseFloat(networkOutMB.toFixed(2))
      });

      // 系统负载数据，支持多种字段名
      const load1 = entry.load_avg_1 || entry.load1 || entry.load_1 || 0;
      const load5 = entry.load_avg_5 || entry.load5 || entry.load_5 || 0;
      const load15 = entry.load_avg_15 || entry.load15 || entry.load_15 || 0;

      monitorData.load.load1.push({
        time: fullTimeStr,
        value: parseFloat((load1).toFixed(2))
      });

      monitorData.load.load5.push({
        time: fullTimeStr,
        value: parseFloat((load5).toFixed(2))
      });

      monitorData.load.load15.push({
        time: fullTimeStr,
        value: parseFloat((load15).toFixed(2))
      });
    });

    console.log('处理后的监控数据:', {
      cpu: monitorData.cpu.length,
      memory: monitorData.memory.length,
      disk: monitorData.disk.length,
      networkIn: monitorData.network.in.length,
      networkOut: monitorData.network.out.length,
      load: {
        load1: monitorData.load.load1.length,
        load5: monitorData.load.load5.length,
        load15: monitorData.load.load15.length
      }
    });

    // 更新图表
    updateCharts();

  } catch (error) {
    console.error('获取历史监控数据失败:', error);
    if (error.response) {
      console.error('错误响应:', error.response.status, error.response.data);
    }
    message.error('获取历史监控数据失败: ' + (error.message || '未知错误'));
  } finally {
    loading.value = false;
  }
};

// 添加工具函数：转换网络流量到MB
const convertToMB = (value: number): number => {
  if (!value) return 0;

  // 输入值为字节/秒，直接转换为MB/s
  return value / (1024 * 1024);
};

// 初始化图表
const initCharts = () => {
  // 使用nextTick确保DOM已经渲染
  nextTick(() => {
    try {
      console.log('初始化图表, DOM元素状态:', {
        cpuEl: !!cpuChartRef.value,
        memoryEl: !!memoryChartRef.value,
        diskEl: !!diskChartRef.value,
        networkEl: !!networkChartRef.value,
        loadEl: !!loadChartRef.value
      });

      // 初始化CPU图表
      if (cpuChartRef.value) {
        if (cpuChart) {
          cpuChart.dispose();
        }
        cpuChart = echarts.init(cpuChartRef.value);
      }

      // 初始化内存图表
      if (memoryChartRef.value) {
        if (memoryChart) {
          memoryChart.dispose();
        }
        memoryChart = echarts.init(memoryChartRef.value);
      }

      // 初始化磁盘图表
      if (diskChartRef.value) {
        if (diskChart) {
          diskChart.dispose();
        }
        diskChart = echarts.init(diskChartRef.value);
      }

      // 初始化网络图表
      if (networkChartRef.value) {
        if (networkChart) {
          networkChart.dispose();
        }
        networkChart = echarts.init(networkChartRef.value);
      }

      // 初始化负载图表
      if (loadChartRef.value) {
        if (loadChart) {
          loadChart.dispose();
        }
        loadChart = echarts.init(loadChartRef.value);
      }

      // 更新图表数据
      updateCharts();

      // 添加窗口大小变化监听
      window.removeEventListener('resize', handleResize);
      window.addEventListener('resize', handleResize);

    } catch (error) {
      console.error('初始化图表时出错:', error);
    }
  });
};

// 处理窗口大小变化
const handleResize = () => {
  cpuChart?.resize();
  memoryChart?.resize();
  diskChart?.resize();
  networkChart?.resize();
  loadChart?.resize();
};

// 更新图表数据
const updateCharts = () => {
  // 使用nextTick确保DOM已经渲染
  nextTick(() => {
    try {
      console.log('更新图表, 数据长度:', {
        cpu: monitorData.cpu.length,
        memory: monitorData.memory.length,
        disk: monitorData.disk.length,
        network: {
          in: monitorData.network.in.length,
          out: monitorData.network.out.length
        },
        load: {
          load1: monitorData.load.load1.length,
          load5: monitorData.load.load5.length,
          load15: monitorData.load.load15.length
        }
      });

      // CPU图表
      if (cpuChart) {
        const cpuOption = {
          title: { text: 'CPU使用率', left: 'center' },
          tooltip: {
            trigger: 'axis',
            formatter: '{b}<br />CPU: {c}%',
            axisPointer: {
              type: 'line',
              label: {
                backgroundColor: '#6a7985'
              }
            }
          },
          xAxis: {
            type: 'category',
            boundaryGap: false,
            data: monitorData.cpu.map(item => item.time),
            axisLabel: { rotate: 30 }
          },
          yAxis: {
            type: 'value',
            min: 0,
            max: 100,
            axisLabel: { formatter: '{value}%' }
          },
          series: [{
            name: 'CPU',
            type: 'line',
            data: monitorData.cpu.map(item => item.value),
            areaStyle: {
              color: {
                type: 'linear',
                x: 0, y: 0, x2: 0, y2: 1,
                colorStops: [
                  { offset: 0, color: 'rgba(0, 122, 255, 0.4)' },
                  { offset: 1, color: 'rgba(0, 122, 255, 0.05)' }
                ]
              }
            },
            lineStyle: { width: 3, color: '#007AFF' },
            itemStyle: { color: '#007AFF', borderWidth: 2, borderColor: '#fff' },
            smooth: true,
            symbol: 'circle',
            symbolSize: 8,
            showSymbol: false
          }],
          grid: { left: '3%', right: '4%', bottom: '10%', containLabel: true },
          dataZoom: [
            {
              type: 'inside',
              start: 0,
              end: 100
            },
            {
              type: 'slider',
              start: 0,
              end: 100
            }
          ]
        };
        cpuChart.setOption(cpuOption, true);
      }

      // 内存图表
      if (memoryChart) {
        const memoryOption = {
          title: { text: '内存使用率', left: 'center' },
          tooltip: {
            trigger: 'axis',
            formatter: '{b}<br />内存: {c}%',
            axisPointer: {
              type: 'line',
              label: {
                backgroundColor: '#6a7985'
              }
            }
          },
          xAxis: {
            type: 'category',
            boundaryGap: false,
            data: monitorData.memory.map(item => item.time),
            axisLabel: { rotate: 30 }
          },
          yAxis: {
            type: 'value',
            min: 0,
            max: 100,
            axisLabel: { formatter: '{value}%' }
          },
          series: [{
            name: '内存',
            type: 'line',
            data: monitorData.memory.map(item => item.value),
            areaStyle: {
              color: {
                type: 'linear',
                x: 0, y: 0, x2: 0, y2: 1,
                colorStops: [
                  { offset: 0, color: 'rgba(255, 149, 0, 0.4)' },
                  { offset: 1, color: 'rgba(255, 149, 0, 0.05)' }
                ]
              }
            },
            lineStyle: { width: 3, color: '#FF9500' },
            itemStyle: { color: '#FF9500', borderWidth: 2, borderColor: '#fff' },
            smooth: true,
            symbol: 'circle',
            symbolSize: 8,
            showSymbol: false
          }],
          grid: { left: '3%', right: '4%', bottom: '10%', containLabel: true },
          dataZoom: [
            {
              type: 'inside',
              start: 0,
              end: 100
            },
            {
              type: 'slider',
              start: 0,
              end: 100
            }
          ]
        };
        memoryChart.setOption(memoryOption, true);
      }

      // 磁盘图表
      if (diskChart) {
        const diskOption = {
          title: { text: '磁盘使用率', left: 'center' },
          tooltip: {
            trigger: 'axis',
            formatter: '{b}<br />磁盘: {c}%',
            axisPointer: {
              type: 'line',
              label: {
                backgroundColor: '#6a7985'
              }
            }
          },
          xAxis: {
            type: 'category',
            boundaryGap: false,
            data: monitorData.disk.map(item => item.time),
            axisLabel: { rotate: 30 }
          },
          yAxis: {
            type: 'value',
            min: 0,
            max: 100,
            axisLabel: { formatter: '{value}%' }
          },
          series: [{
            name: '磁盘',
            type: 'line',
            data: monitorData.disk.map(item => item.value),
            areaStyle: {
              color: {
                type: 'linear',
                x: 0, y: 0, x2: 0, y2: 1,
                colorStops: [
                  { offset: 0, color: 'rgba(52, 199, 89, 0.4)' },
                  { offset: 1, color: 'rgba(52, 199, 89, 0.05)' }
                ]
              }
            },
            lineStyle: { width: 3, color: '#34C759' },
            itemStyle: { color: '#34C759', borderWidth: 2, borderColor: '#fff' },
            smooth: true,
            symbol: 'circle',
            symbolSize: 8,
            showSymbol: false
          }],
          grid: { left: '3%', right: '4%', bottom: '10%', containLabel: true },
          dataZoom: [
            {
              type: 'inside',
              start: 0,
              end: 100
            },
            {
              type: 'slider',
              start: 0,
              end: 100
            }
          ]
        };
        diskChart.setOption(diskOption, true);
      }

      // 网络图表
      if (networkChart) {
        const networkOption = {
          title: { text: '网络流量 (MB/s)', left: 'center' },
          tooltip: {
            trigger: 'axis',
            axisPointer: {
              type: 'line',
              label: {
                backgroundColor: '#6a7985'
              }
            },
            formatter: function (params: any[]) {
              const time = params[0].name;
              let result = `${time}<br />`;
              params.forEach(param => {
                const color = param.color;
                const seriesName = param.seriesName;
                const value = param.value.toFixed(3); // 增加小数位数以显示更精确的值
                result += `<span style="display:inline-block;margin-right:5px;border-radius:10px;width:10px;height:10px;background-color:${color};"></span> ${seriesName}: ${value} MB/s<br />`;
              });
              return result;
            }
          },
          legend: {
            data: ['入站流量', '出站流量'],
            top: 30
          },
          xAxis: {
            type: 'category',
            boundaryGap: false,
            data: monitorData.network.in.map(item => item.time),
            axisLabel: { rotate: 30 }
          },
          yAxis: {
            type: 'value',
            axisLabel: { formatter: '{value} MB/s' },
            scale: true,
            min: 0
          },
          series: [
            {
              name: '入站流量',
              type: 'line',
              data: monitorData.network.in.map(item => item.value),
              areaStyle: {
                color: {
                  type: 'linear',
                  x: 0, y: 0, x2: 0, y2: 1,
                  colorStops: [
                    { offset: 0, color: 'rgba(0, 122, 255, 0.4)' },
                    { offset: 1, color: 'rgba(0, 122, 255, 0.05)' }
                  ]
                }
              },
              lineStyle: { width: 3, color: '#007AFF' },
              itemStyle: { color: '#007AFF', borderWidth: 2, borderColor: '#fff' },
              smooth: true,
              symbol: 'circle',
              symbolSize: 8,
              showSymbol: false
            },
            {
              name: '出站流量',
              type: 'line',
              data: monitorData.network.out.map(item => item.value),
              areaStyle: {
                color: {
                  type: 'linear',
                  x: 0, y: 0, x2: 0, y2: 1,
                  colorStops: [
                    { offset: 0, color: 'rgba(255, 149, 0, 0.4)' },
                    { offset: 1, color: 'rgba(255, 149, 0, 0.05)' }
                  ]
                }
              },
              lineStyle: { width: 3, color: '#FF9500' },
              itemStyle: { color: '#FF9500', borderWidth: 2, borderColor: '#fff' },
              smooth: true,
              symbol: 'circle',
              symbolSize: 8,
              showSymbol: false
            }
          ],
          grid: { left: '3%', right: '4%', bottom: '10%', containLabel: true },
          dataZoom: [
            {
              type: 'inside',
              start: 0,
              end: 100
            },
            {
              type: 'slider',
              start: 0,
              end: 100
            }
          ]
        };
        networkChart.setOption(networkOption, true);
      }

      // 系统负载图表
      if (loadChart) {
        const loadOption = {
          title: { text: '系统负载', left: 'center' },
          tooltip: {
            trigger: 'axis',
            axisPointer: {
              type: 'line',
              label: {
                backgroundColor: '#6a7985'
              }
            },
            formatter: function (params: any[]) {
              const time = params[0].name;
              let result = `${time}<br />`;
              params.forEach(param => {
                const color = param.color;
                const seriesName = param.seriesName;
                const value = param.value.toFixed(2);
                result += `<span style="display:inline-block;margin-right:5px;border-radius:10px;width:10px;height:10px;background-color:${color};"></span> ${seriesName}: ${value}<br />`;
              });
              return result;
            }
          },
          legend: {
            data: ['1分钟', '5分钟', '15分钟'],
            top: 30
          },
          xAxis: {
            type: 'category',
            boundaryGap: false,
            data: monitorData.load.load1.map(item => item.time),
            axisLabel: { rotate: 30 }
          },
          yAxis: {
            type: 'value',
            axisLabel: { formatter: '{value}' },
            scale: true,
            min: 0
          },
          series: [
            {
              name: '1分钟',
              type: 'line',
              data: monitorData.load.load1.map(item => item.value),
              lineStyle: { width: 3, color: '#007AFF' },
              itemStyle: { color: '#007AFF', borderWidth: 2, borderColor: '#fff' },
              smooth: true,
              symbol: 'circle',
              symbolSize: 8,
              showSymbol: false
            },
            {
              name: '5分钟',
              type: 'line',
              data: monitorData.load.load5.map(item => item.value),
              lineStyle: { width: 3, color: '#FF9500' },
              itemStyle: { color: '#FF9500', borderWidth: 2, borderColor: '#fff' },
              smooth: true,
              symbol: 'circle',
              symbolSize: 8,
              showSymbol: false
            },
            {
              name: '15分钟',
              type: 'line',
              data: monitorData.load.load15.map(item => item.value),
              lineStyle: { width: 3, color: '#34C759' },
              itemStyle: { color: '#34C759', borderWidth: 2, borderColor: '#fff' },
              smooth: true,
              symbol: 'circle',
              symbolSize: 8,
              showSymbol: false
            }
          ],
          grid: { left: '3%', right: '4%', bottom: '10%', containLabel: true },
          dataZoom: [
            {
              type: 'inside',
              start: 0,
              end: 100
            },
            {
              type: 'slider',
              start: 0,
              end: 100
            }
          ]
        };
        loadChart.setOption(loadOption, true);
      }
    } catch (error) {
      console.error('更新图表时出错:', error);
    }
  });
};

// 修改连接WebSocket方法
const connectWebSocket = () => {
  // 检查是否已有活跃连接
  if (ws && ws.readyState === WebSocket.OPEN) {
    console.log('WebSocket连接已存在且处于活跃状态，不需要重新连接');
    return;
  }

  // 获取token
  const token = localStorage.getItem('server_ops_token');
  if (!token) {
    message.error('未登录，无法获取实时数据');
    return;
  }

  // 关闭之前的连接（如果存在）
  if (ws && ws.readyState !== WebSocket.CLOSED) {
    console.log('关闭之前的WebSocket连接');
    // 确保在关闭连接时不会触发重连
    ws.onclose = null;
    try {
      ws.close();
    } catch (e) {
      console.error('关闭WebSocket时出错:', e);
    }
    ws = null;
  }

  // 清除之前的心跳定时器
  if (heartbeatInterval) {
    console.log('清除之前的心跳定时器');
    clearInterval(heartbeatInterval);
    heartbeatInterval = null;
  }

  // 重置心跳失败计数
  heartbeatFailCount = 0;

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  // 修正WebSocket URL，更明确地指定监控专用路径
  const wsUrl = `${protocol}//${window.location.host}/api/servers/${serverId.value}/monitor-ws?token=${encodeURIComponent(token)}`;

  console.log('正在连接监控WebSocket:', wsUrl);

  // 设置连接标志
  connecting.value = true;
  wsConnected.value = false;

  try {
    ws = new WebSocket(wsUrl);

    // 设置超时处理，如果10秒内没有连接成功则认为失败
    const connectionTimeout = setTimeout(() => {
      if (ws && ws.readyState !== WebSocket.OPEN) {
        console.log('WebSocket连接超时');
        if (ws) {
          // 确保在关闭连接时不会触发自动重连
          ws.onclose = null;
          ws.close();
          ws = null;
        }
        connecting.value = false;
        wsConnected.value = false;
        message.error('连接超时，请稍后重试');

        // 超时也尝试自动重连，但使用更长的延迟
        setTimeout(() => {
          if (!wsConnected.value && !connecting.value) {
            handleReconnect();
          }
        }, 5000);
      }
    }, 10000);

    ws.onopen = () => {
      clearTimeout(connectionTimeout);
      console.log('WebSocket连接成功打开');
      wsConnected.value = true;
      connecting.value = false;
      reconnectCount.value = 0; // 成功连接后重置重连计数
      message.success('实时监控已连接');

      // 服务器在线时更新服务器状态
      if (serverInfo.value && !isOnline.value) {
        serverInfo.value.status = 'online';
        // 同步更新到store
        serverStore.updateServerStatus(serverId.value, 'online');
      }

      // 添加心跳机制，每30秒发送一次心跳包
      if (heartbeatInterval) {
        clearInterval(heartbeatInterval);
      }

      // 获取系统设置中配置的UI刷新间隔，作为心跳间隔
      // 如果尚未加载设置或无法获取，则默认使用30秒
      const heartbeatMs = settingsStore.loaded ? settingsStore.getUiRefreshIntervalMs() : 30000;
      console.log('设置心跳间隔:', heartbeatMs, 'ms');

      heartbeatInterval = window.setInterval(() => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          console.log('发送心跳包');
          try {
            ws.send(JSON.stringify({
              type: 'heartbeat',
              timestamp: Date.now()
            }));
          } catch (error) {
            console.error('心跳发送失败:', error);
            heartbeatFailCount++;

            // 如果连续发送心跳失败次数达到上限，关闭连接并重连
            if (heartbeatFailCount >= maxHeartbeatFails) {
              console.log(`心跳失败${maxHeartbeatFails}次，关闭连接并尝试重连`);
              if (heartbeatInterval) {
                clearInterval(heartbeatInterval);
                heartbeatInterval = null;
              }

              if (ws) {
                ws.onclose = null; // 禁用onclose回调，避免触发自动重连
                ws.close();
                ws = null;
              }

              // 短暂延迟后重连
              setTimeout(() => {
                if (!wsConnected.value && !connecting.value) {
                  connectWebSocket();
                }
              }, 5000);
            }
          }
        } else {
          console.log('WebSocket已关闭，清除心跳定时器');
          if (heartbeatInterval) {
            clearInterval(heartbeatInterval);
            heartbeatInterval = null;
          }
        }
      }, heartbeatMs);
    };

    ws.onmessage = (event) => {
      try {
        console.log('收到WebSocket消息:', event.data);
        const data = JSON.parse(event.data);

        // 重置心跳失败计数（收到任何消息都视为连接正常）
        heartbeatFailCount = 0;

        // 更新监控数据
        if (data.type === 'monitor') {
          console.log('收到监控数据:', data.data);
          // 更新服务器状态（如果存在）
          if (data.data && data.data.status && serverInfo.value) {
            console.log('从监控数据更新服务器状态:', data.data.status);
            serverInfo.value.status = data.data.status;
            // 同步更新到store
            serverStore.updateServerStatus(serverId.value, data.data.status);
          } else if (data.status && serverInfo.value) {
            console.log('从监控消息根级别更新服务器状态:', data.status);
            serverInfo.value.status = data.status;
            // 同步更新到store
            serverStore.updateServerStatus(serverId.value, data.status);
          }

          // 检查图表是否已初始化，如果没有则初始化
          if (!cpuChart || !memoryChart || !diskChart || !networkChart || !loadChart) {
            console.log('图表未完全初始化，正在重新初始化...');
            initCharts();
          }

          // 处理嵌套的data字段
          if (data.data) {
            // 更新本地图表数据
            updateMonitorData(data.data);
            // 同步数据到store
            serverStore.updateServerMonitorData(serverId.value, data.data);
          } else {
            // 更新本地图表数据
            updateMonitorData(data);
            // 同步数据到store
            serverStore.updateServerMonitorData(serverId.value, data);
          }
        }
        // 处理欢迎消息，可能包含服务器信息
        else if (data.type === 'welcome') {
          console.log('收到欢迎消息:', data);
          serverInfo.value = {
            id: data.server_id || serverId.value,
            name: data.name || serverInfo.value.name,
            ip: data.ip || serverInfo.value.ip,
            status: data.status || serverInfo.value.status || 'unknown',
            last_heartbeat_time: data.last_seen ? new Date(data.last_seen * 1000) : serverInfo.value.last_heartbeat_time
          };
          serverStore.updateServerStatus(serverId.value, serverInfo.value.status);

          if (data.system_info) {
            serverStore.updateServerMonitorData(serverId.value, {
              system_info: data.system_info,
              status: serverInfo.value.status
            });
          }
        }
        // 处理心跳响应
        else if (data.type === 'heartbeat') {
          console.log('收到心跳响应');

          // 如果心跳包含状态信息，更新服务器状态
          if (data.status && serverInfo.value) {
            console.log('从心跳响应更新服务器状态:', data.status);
            serverInfo.value.status = data.status;
            // 同步更新到store
            serverStore.updateServerStatus(serverId.value, data.status);
          } else if (data.data && data.data.status && serverInfo.value) {
            console.log('从心跳响应data字段更新服务器状态:', data.data.status);
            serverInfo.value.status = data.data.status;
            // 同步更新到store
            serverStore.updateServerStatus(serverId.value, data.data.status);
          }

          // 检查心跳消息是否包含监控数据
          // 首先检查data字段
          if (data.data) {
            // 检查data字段中是否有必要的监控数据
            const hasMonitorData =
              data.data.cpu_usage !== undefined ||
              data.data.memory_used !== undefined ||
              data.data.disk_used !== undefined ||
              data.data.network_in !== undefined ||
              data.data.load_avg_1 !== undefined;

            if (hasMonitorData) {
              console.log('心跳消息的data字段包含监控数据，更新图表');
              updateMonitorData(data.data);
              // 同步数据到store
              serverStore.updateServerMonitorData(serverId.value, data.data);
            } else {
              console.log('心跳消息的data字段不包含必要的监控数据');
            }
          } else {
            // 检查心跳消息本身是否直接包含监控数据
            const hasDirectMonitorData =
              data.cpu_usage !== undefined ||
              data.memory_used !== undefined ||
              data.disk_used !== undefined ||
              data.network_in !== undefined ||
              data.load_avg_1 !== undefined;

            if (hasDirectMonitorData) {
              console.log('心跳消息本身包含监控数据，更新图表');
              updateMonitorData(data);
              // 同步数据到store
              serverStore.updateServerMonitorData(serverId.value, data);
            } else {
              console.log('心跳消息不包含监控数据');
            }
          }
        }
        // 处理无数据消息
        else if (data.type === 'no_data') {
          console.log('服务器没有监控数据:', data.message);
          message.warning('服务器没有监控数据，请检查agent是否正常运行');
        }
        // 处理错误消息
        else if (data.type === 'error') {
          console.error('服务器错误:', data.message);
          message.error(`服务器错误: ${data.message}`);
        }
      } catch (error) {
        console.error('解析WebSocket消息失败:', error, '原始消息:', event.data);
      }
    };

    ws.onerror = (error) => {
      clearTimeout(connectionTimeout);
      console.error('WebSocket发生错误:', error);
      wsConnected.value = false;
      connecting.value = false;
      if (serverInfo.value) {
        serverInfo.value.status = 'offline';
        // 同步更新到store
        serverStore.updateServerStatus(serverId.value, 'offline');
      }
      message.error('监控连接发生错误');
    };

    ws.onclose = (event) => {
      clearTimeout(connectionTimeout);
      wsConnected.value = false;
      connecting.value = false;
      console.log(`WebSocket连接已关闭，代码: ${event.code}, 原因: ${event.reason}`);
      if (serverInfo.value) {
        serverInfo.value.status = 'offline';
        // 同步更新到store
        serverStore.updateServerStatus(serverId.value, 'offline');
      }

      // 清除心跳定时器
      if (heartbeatInterval) {
        console.log('连接关闭，清除心跳定时器');
        clearInterval(heartbeatInterval);
        heartbeatInterval = null;
      }

      // 只有在非手动关闭的情况下才尝试重连
      if (event.code !== 1000 && event.code !== 1001) {
        setTimeout(() => {
          if (!wsConnected.value && !connecting.value) {
            handleReconnect();
          }
        }, 2000);
      } else {
        console.log('WebSocket连接正常关闭，不尝试重新连接');
      }
    };
  } catch (error) {
    console.error('创建WebSocket连接失败:', error);
    message.error('创建WebSocket连接失败');
    connecting.value = false;

    // 连接失败也尝试重连，但使用更长的延迟
    setTimeout(() => {
      if (!wsConnected.value && !connecting.value) {
        handleReconnect();
      }
    }, 5000);
  }
};

// 添加处理重连的函数
const handleReconnect = () => {
  // 使用更长的延迟和最大重试次数限制，避免过于频繁的重连
  if (reconnectCount.value < maxReconnectAttempts) {
    reconnectCount.value++;
    const delay = reconnectCount.value * 5000; // 延长重连间隔为5秒 * 重试次数
    console.log(`${reconnectCount.value}/${maxReconnectAttempts} 将在 ${delay / 1000} 秒后尝试重连...`);
    setTimeout(() => {
      if (!wsConnected.value && !connecting.value) { // 双重检查，确保还未连接成功才重连
        connectWebSocket();
      }
    }, delay);
  } else {
    console.log('已达到最大重连次数，不再自动重连');
    message.error('监控连接已断开，请手动重新连接');

    // 所有重连失败，将服务器状态标记为离线
    if (serverInfo.value) {
      serverInfo.value.status = 'offline';
      // 同步更新到store
      serverStore.updateServerStatus(serverId.value, 'offline');
    }
  }
};

// 添加重新连接WebSocket的方法，增加防抖动功能
let reconnectTimeout: number | null = null;
const reconnectWebSocket = () => {
  // 防止短时间内多次点击重连按钮
  if (reconnectTimeout) {
    clearTimeout(reconnectTimeout);
  }

  // 如果当前正在连接中，不进行操作
  if (connecting.value) {
    console.log('连接过程中，请勿重复操作');
    return;
  }

  connecting.value = true;

  // 关闭之前的WebSocket
  if (ws) {
    ws.onclose = null; // 防止触发自动重连
    ws.close();
    ws = null;
  }

  // 清除心跳定时器
  if (heartbeatInterval) {
    clearInterval(heartbeatInterval);
    heartbeatInterval = null;
  }

  // 重置重连计数
  reconnectCount.value = 0;
  heartbeatFailCount = 0;

  // 设置延迟避免立即重连可能导致的问题
  reconnectTimeout = window.setTimeout(() => {
    // 重新连接
    connectWebSocket();
    reconnectTimeout = null;
  }, 1000);
};

// 更新监控数据
const updateMonitorData = (data: any) => {
  console.log('更新监控数据:', data);

  // 获取当前时间
  const now = new Date();
  const currentTime = now.toLocaleString('zh-CN', { hour12: false });

  // 计算数据保留的时间范围（毫秒）
  const retentionTime = settingsStore.chartHistoryHours * 60 * 60 * 1000;
  const cutoffTime = now.getTime() - retentionTime;

  // 清理超出时间范围的数据
  const cleanOldData = (dataArray: { time: string; value: number }[]) => {
    const index = dataArray.findIndex(item => new Date(item.time).getTime() > cutoffTime);
    if (index > 0) {
      dataArray.splice(0, index);
    }
  };

  // 更新CPU数据
  if (data.cpu_usage !== undefined) {
    let cpuValue = Number(data.cpu_usage);
    if (cpuValue < 1 && cpuValue > 0) {
      cpuValue = cpuValue * 100;
    }
    const safeValue = isNaN(cpuValue) ? 0 : Math.min(Math.max(cpuValue, 0), 100);

    monitorData.cpu.push({
      time: currentTime,
      value: safeValue
    });
    cleanOldData(monitorData.cpu);
  }

  // 更新内存数据
  if (data.memory_used !== undefined) {
    let memoryPercent = 0;
    if (data.memory_used <= 100) {
      memoryPercent = data.memory_used;
    } else if (data.memory_total && data.memory_total > 0) {
      memoryPercent = (data.memory_used / data.memory_total) * 100;
    }

    monitorData.memory.push({
      time: currentTime,
      value: parseFloat(memoryPercent.toFixed(2))
    });
    cleanOldData(monitorData.memory);
  }

  // 更新磁盘数据
  if (data.disk_used !== undefined) {
    let diskPercent = 0;
    if (data.disk_used <= 100) {
      diskPercent = data.disk_used;
    } else if (data.disk_total && data.disk_total > 0) {
      diskPercent = (data.disk_used / data.disk_total) * 100;
    }

    monitorData.disk.push({
      time: currentTime,
      value: parseFloat(diskPercent.toFixed(2))
    });
    cleanOldData(monitorData.disk);
  }

  // 更新网络数据
  const addNetworkData = (field: 'in' | 'out', value: any) => {
    if (value === undefined) return;

    const numValue = Number(value);
    if (isNaN(numValue)) return;

    let mbValue = 0;
    if (numValue > 1000000) {
      mbValue = numValue / (1024 * 1024);
    } else if (numValue > 1000) {
      mbValue = numValue / 1024;
    } else {
      mbValue = numValue;
    }

    monitorData.network[field].push({
      time: currentTime,
      value: parseFloat(mbValue.toFixed(2))
    });
    cleanOldData(monitorData.network[field]);
  };

  addNetworkData('in', data.network_in);
  addNetworkData('out', data.network_out);

  // 更新系统负载数据
  const addLoadData = (loadField: 'load1' | 'load5' | 'load15', value: any) => {
    if (value === undefined) return;

    const numValue = Number(value);
    const safeValue = !isNaN(numValue) ? Math.max(0, numValue) : 0;

    monitorData.load[loadField].push({
      time: currentTime,
      value: parseFloat(safeValue.toFixed(2))
    });
    cleanOldData(monitorData.load[loadField]);
  };

  addLoadData('load1', data.load_avg_1);
  addLoadData('load5', data.load_avg_5);
  addLoadData('load15', data.load_avg_15);

  // 更新图表
  updateCharts();
};

// 添加数据键名格式规范化函数
const normalizeName = (data: any) => {
  const result: any = { ...data };

  // 检查常见的不同格式键名并统一
  const keyMappings: { [key: string]: string } = {
    'cpuUsage': 'cpu_usage',
    'cpu_percent': 'cpu_usage',
    'memoryUsed': 'memory_used',
    'memory_percent': 'memory_used',
    'diskUsed': 'disk_used',
    'disk_percent': 'disk_used',
    'networkIn': 'network_in',
    'networkOut': 'network_out',
    'loadAvg1': 'load_avg_1',
    'loadAvg5': 'load_avg_5',
    'loadAvg15': 'load_avg_15'
  };

  // 检查并统一键名
  Object.keys(keyMappings).forEach(altKey => {
    if (data[altKey] !== undefined && result[keyMappings[altKey]] === undefined) {
      console.log(`发现替代键名: ${altKey} -> ${keyMappings[altKey]}`);
      result[keyMappings[altKey]] = data[altKey];
    }
  });

  return result;
};

// 格式化内存大小
const formatMemorySize = (bytes: number): string => {
  if (!bytes) return '0 B';

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = bytes;

  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }

  return `${size.toFixed(2)} ${units[i]}`;
};

// 页面挂载时获取服务器信息并连接WebSocket
onMounted(async () => {
  console.log('组件挂载中...');

  // 先加载系统设置
  await settingsStore.loadPublicSettings();
  console.log('已加载设置，历史数据显示时间:', settingsStore.chartHistoryHours, '小时');

  // 获取历史监控数据
  await fetchHistoricalData();

  // 初始化图表
  initCharts();

  // 连接WebSocket获取实时数据
  connectWebSocket();
});

// 监听设置变化，当历史数据显示时间改变时重新加载数据
watch(() => settingsStore.chartHistoryHours, async (newValue, oldValue) => {
  if (newValue !== oldValue && newValue > 0) {
    console.log('历史数据显示时间设置已更改:', oldValue, '->', newValue, '小时');
    // 重新获取历史数据
    await fetchHistoricalData();
  }
});

watch(
  () => route.params.id,
  async (newValue, oldValue) => {
    if (newValue === oldValue) return;
    const nextServerId = Number(newValue);
    if (!nextServerId || nextServerId === serverId.value) return;

    console.log('监控页面切换服务器:', serverId.value, '->', nextServerId);

    if (ws) {
      ws.onclose = null;
      ws.close();
      ws = null;
    }

    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout);
      reconnectTimeout = null;
    }

    if (heartbeatInterval) {
      clearInterval(heartbeatInterval);
      heartbeatInterval = null;
    }

    wsConnected.value = false;
    connecting.value = false;
    reconnectCount.value = 0;
    heartbeatFailCount = 0;

    serverId.value = nextServerId;
    serverInfo.value = {};
    clearMonitorData();
    updateCharts();

    await fetchHistoricalData();
    initCharts();
    connectWebSocket();
  }
);

// 页面卸载时关闭WebSocket连接和移除事件监听
onUnmounted(() => {
  console.log('组件卸载，清理资源...');

  if (ws) {
    // 设置为null避免触发自动重连
    ws.onclose = null;
    ws.close();
    ws = null;
  }

  // 清除任何可能存在的定时器
  if (reconnectTimeout) {
    clearTimeout(reconnectTimeout);
    reconnectTimeout = null;
  }

  // 清除心跳定时器
  if (heartbeatInterval) {
    clearInterval(heartbeatInterval);
    heartbeatInterval = null;
  }

  window.removeEventListener('resize', handleResize);

  // 销毁图表实例
  cpuChart?.dispose();
  memoryChart?.dispose();
  diskChart?.dispose();
  networkChart?.dispose();
  loadChart?.dispose();

  // 设置图表实例为null避免内存泄漏
  cpuChart = null;
  memoryChart = null;
  diskChart = null;
  networkChart = null;
  loadChart = null;
});
</script>

<template>
  <div class="server-monitor-container">
    <a-spin :spinning="loading" tip="加载中...">
      <a-page-header title="服务器监控" :sub-title="serverInfo.name" @back="() => router.push(`/admin/servers/${serverId}`)">
        <template #tags>
          <a-tag :color="getStatusType(isOnline)">
            {{ isOnline ? '在线' : '离线' }}
          </a-tag>
          <a-tag v-if="wsConnected" color="blue">WebSocket已连接</a-tag>
          <a-tag v-if="settingsStore.chartHistoryHours" color="cyan">
            <template #icon>
              <ClockCircleOutlined />
            </template>
            显示 {{ settingsStore.chartHistoryHours }} 小时历史数据
          </a-tag>
        </template>
        <template #extra>
          <a-button type="primary" @click="reconnectWebSocket" :loading="connecting">
            {{ connecting ? '连接中...' : '重新连接' }}
          </a-button>
        </template>
      </a-page-header>

      <!-- 服务器离线提示 -->
      <a-alert v-if="!wsConnected && !isOnline && !loading" type="warning" show-icon banner style="margin-bottom: 16px"
        message="服务器当前离线，无法获取实时监控数据" description="请检查服务器是否启动，或者尝试重新连接。" />

      <div v-if="hasData || wsConnected" class="charts-grid">
        <div class="chart-card">
          <div class="chart-header">
            <h3>CPU使用率</h3>
          </div>
          <div ref="cpuChartRef" class="chart-container"></div>
        </div>

        <div class="chart-card">
          <div class="chart-header">
            <h3>内存使用率</h3>
          </div>
          <div ref="memoryChartRef" class="chart-container"></div>
        </div>

        <div class="chart-card">
          <div class="chart-header">
            <h3>磁盘使用率</h3>
          </div>
          <div ref="diskChartRef" class="chart-container"></div>
        </div>

        <div class="chart-card">
          <div class="chart-header">
            <h3>网络流量 (MB/s)</h3>
          </div>
          <div ref="networkChartRef" class="chart-container"></div>
        </div>

        <div class="chart-card" style="grid-column: 1 / -1;">
          <div class="chart-header">
            <h3>系统负载</h3>
          </div>
          <div ref="loadChartRef" class="chart-container"></div>
        </div>
      </div>

      <!-- 无数据状态显示 -->
      <div v-if="!hasData && !wsConnected && !loading" class="no-data-container">
        <a-empty description="暂无监控数据">
          <template #description>
            <span>暂无监控数据，服务器可能离线或未启动代理</span>
          </template>
          <a-button type="primary" @click="reconnectWebSocket">
            重新连接
          </a-button>
        </a-empty>
      </div>
    </a-spin>
  </div>
</template>

<style scoped>
.server-monitor-container {
  padding: 0;
  background: transparent;
}

.charts-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 24px;
  margin-top: 24px;
}

.chart-card {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
  transition: all 0.3s ease;
}

.chart-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.08);
}

.chart-header {
  margin-bottom: 20px;
  padding-bottom: 0;
  border-bottom: none;
  display: flex;
  justify-content: center;
}

.chart-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #1d1d1f;
  letter-spacing: -0.01em;
}

.chart-container {
  height: 300px;
  width: 100%;
}

.no-data-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 400px;
  background: rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(10px);
  border-radius: 16px;
  margin-top: 24px;
}

/* Responsive adjustments */
@media (max-width: 768px) {
  .charts-grid {
    grid-template-columns: 1fr;
  }
}
</style>

<style>
/* Dark Mode Adaptation - Global Styles */
.dark .chart-card {
  background: rgba(30, 30, 30, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
}

.dark .chart-card:hover {
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.3);
  background: rgba(40, 40, 40, 0.7);
}

.dark .chart-header h3 {
  color: #f5f5f7;
}

.dark .no-data-container {
  background: rgba(30, 30, 30, 0.4);
}

.dark .no-data-container .ant-empty-description {
  color: #a0a0a0;
}
</style>
