<script setup lang="ts">
import { ref, computed, watch, onUnmounted, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart, BarChart, CustomChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent, TitleComponent } from 'echarts/components';
import VChart from 'vue-echarts';
import { getToken } from '@/utils/auth';
import type { LifeProbeDetails, SleepSegmentPoint } from '@/types/life';
import * as echarts from 'echarts/core';
import { useThemeStore } from '@/stores/theme';
import { storeToRefs } from 'pinia';
import axios from 'axios';

use([CanvasRenderer, LineChart, BarChart, CustomChart, GridComponent, TooltipComponent, LegendComponent, TitleComponent]);

const route = useRoute();
const router = useRouter();
const themeStore = useThemeStore();
const { isDark } = storeToRefs(themeStore);

// 获取probe ID和公开访问模式
const probeId = computed(() => Number(route.params.id));
const publicMode = computed(() => !route.path.startsWith('/admin'));

// 时间范围选择
type TimeRangeKey = '24h' | '7d' | '30d';
const timeRange = ref<TimeRangeKey>('24h');
const loading = ref(false);
const details = ref<LifeProbeDetails | null>(null);
const errorMessage = ref('');

// 计算时间范围参数
const rangeParams = computed(() => {
  switch (timeRange.value) {
    case '7d':
      return { hours: 24 * 7, daily_days: 7 };
    case '30d':
      return { hours: 24 * 30, daily_days: 30 };
    case '24h':
    default:
      return { hours: 24, daily_days: 7 };
  }
});

// 获取详情数据（带请求竞态控制）
let requestSeq = 0; // 请求序列号
const fetchDetails = async () => {
  if (!probeId.value) return;

  const currentSeq = ++requestSeq; // 生成新的序列号
  loading.value = true;
  errorMessage.value = '';

  try {
    const token = getToken();
    const isAuthenticated = !!token;

    // 认证用户使用私有接口，匿名用户使用公开接口
    const apiPath = isAuthenticated
      ? `/api/life-probes/${probeId.value}/details`
      : `/api/life-probes/public/${probeId.value}/details`;

    const headers: any = {};
    if (isAuthenticated) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await axios.get(apiPath, {
      params: rangeParams.value,
      headers,
    });

    // 检查是否为最新请求，避免竞态
    if (currentSeq !== requestSeq) {
      return;
    }

    details.value = response.data as LifeProbeDetails;
  } catch (error: any) {
    // 同样检查是否为最新请求
    if (currentSeq !== requestSeq) {
      return;
    }

    const msg = error?.response?.data?.error || error?.message || '获取生命探针详情失败';
    errorMessage.value = msg;
    message.error(msg);
    console.error('获取生命探针详情失败:', error);
  } finally {
    if (currentSeq === requestSeq) {
      loading.value = false;
    }
  }
};

// 监听路由参数和时间范围变化
watch([probeId, timeRange], () => {
  fetchDetails();
}, { immediate: true });

// 返回按钮（带fallback）
const onBack = () => {
  if (window.history.length > 1) {
    router.back();
  } else {
    // 无历史栈时跳转到dashboard
    router.push({ name: 'Dashboard' });
  }
};

// 计算属性
const summary = computed(() => details.value?.summary);
const pageTitle = computed(() => summary.value?.name || '生命探针详情');

const latestHeartRate = computed(() => summary.value?.latest_heart_rate?.value ?? '--');
const latestHeartRateTime = computed(() => formatDate(summary.value?.latest_heart_rate?.time));
const stepsToday = computed(() => summary.value ? Math.round(summary.value.steps_today) : 0);
const batteryLevel = computed(() =>
  summary.value?.battery_level !== undefined && summary.value?.battery_level !== null
    ? `${Math.round((summary.value.battery_level || 0) * 100)}%`
    : '--'
);
const lastSync = computed(() => formatDate(summary.value?.last_sync_at));

// 心率图表配置
const heartRateOption = computed(() => {
  const data = [...(details.value?.heart_rates || [])].sort(sortByTime);
  if (!data.length) {
    return emptyLineOption('暂无心率数据');
  }
  const axisColor = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#333';
  const splitLineColor = isDark.value ? 'rgba(255, 255, 255, 0.1)' : '#eee';

  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 50, right: 20, top: 30, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.map((point) => formatTime(point.time)),
      axisLabel: {
        hideOverlap: true,
        color: axisColor
      },
      axisLine: { lineStyle: { color: isDark.value ? 'rgba(255, 255, 255, 0.2)' : '#333' } }
    },
    yAxis: {
      type: 'value',
      name: 'BPM',
      min: (value: any) => Math.max(40, Math.floor(value.min - 5)),
      nameTextStyle: { color: axisColor },
      axisLabel: { color: axisColor },
      splitLine: {
        lineStyle: {
          type: 'dashed',
          opacity: 0.3,
          color: splitLineColor
        }
      }
    },
    series: [
      {
        type: 'line',
        data: data.map((point) => point.value),
        smooth: true,
        areaStyle: {
          opacity: 0.2,
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(255, 77, 79, 0.5)' },
            { offset: 1, color: 'rgba(255, 77, 79, 0.01)' }
          ])
        },
        showSymbol: false,
        lineStyle: { width: 2 },
        color: '#ff4d4f',
      },
    ],
  };
});

// 步数区间图表配置
const stepSegmentOption = computed(() => {
  const samples = [...(details.value?.step_samples || [])].sort((a, b) =>
    new Date(a.start).getTime() - new Date(b.start).getTime()
  );
  if (!samples.length) {
    return emptyBarOption('暂无步数数据');
  }
  const axisColor = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#333';
  const splitLineColor = isDark.value ? 'rgba(255, 255, 255, 0.1)' : '#eee';

  return {
    tooltip: {
      trigger: 'axis',
      formatter(params: any) {
        const first = Array.isArray(params) ? params[0] : params;
        const sample = samples[first?.dataIndex ?? 0];
        if (!sample) {
          return '';
        }
        const range = `${formatTimePrecise(sample.start)} - ${formatTimePrecise(sample.end)}`;
        const value = Number(sample.value.toFixed(0));
        return `${range}<br/>步数：${value}`;
      },
    },
    grid: { left: 40, right: 16, top: 32, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      data: samples.map((sample) => formatTimePrecise(sample.start)),
      axisLabel: {
        rotate: 0,
        hideOverlap: true,
        formatter: (value: string) => value.substring(0, 5), // HH:mm
        color: axisColor
      },
      axisLine: { lineStyle: { color: isDark.value ? 'rgba(255, 255, 255, 0.2)' : '#333' } }
    },
    yAxis: {
      type: 'value',
      name: '步数',
      nameTextStyle: { color: axisColor },
      axisLabel: { color: axisColor },
      splitLine: {
        lineStyle: {
          type: 'dashed',
          opacity: 0.3,
          color: splitLineColor
        }
      }
    },
    series: [
      {
        type: 'bar',
        barMaxWidth: 30,
        data: samples.map((sample) => Number(sample.value.toFixed(0))),
        itemStyle: {
          color: '#1677ff',
          borderRadius: [4, 4, 0, 0],
        },
      },
    ],
  };
});

// 每日步数图表配置
const dailyStepsOption = computed(() => {
  const totals = (summary.value?.daily_totals || []).filter(
    (item) => item.sample_type === 'steps_detailed'
  );
  if (!totals.length) {
    return emptyBarOption('暂无每日步数');
  }
  const sorted = [...totals].sort((a, b) => new Date(a.day).getTime() - new Date(b.day).getTime());
  const axisColor = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#333';

  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 16, top: 32, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      data: sorted.map((item) => formatDay(item.day)),
      axisLabel: {
        hideOverlap: true,
        color: axisColor
      },
      axisLine: { lineStyle: { color: isDark.value ? 'rgba(255, 255, 255, 0.2)' : '#333' } }
    },
    yAxis: {
      type: 'value',
      name: '步数',
      nameTextStyle: { color: axisColor },
      axisLabel: { color: axisColor },
      splitLine: {
        lineStyle: {
          type: 'dashed',
          opacity: 0.3,
          color: isDark.value ? 'rgba(255, 255, 255, 0.1)' : '#eee'
        }
      }
    },
    series: [
      {
        type: 'bar',
        barWidth: 20,
        itemStyle: { color: '#52c41a', borderRadius: [6, 6, 0, 0] },
        data: sorted.map((item) => Math.round(item.total)),
      },
    ],
  };
});

// 睡眠质量图表配置（阶段分布）
const sleepOption = computed(() => {
  const segments = details.value?.sleep_segments || [];
  if (!segments.length) {
    return {
      title: { text: '暂无睡眠数据', left: 'center', top: 'middle', textStyle: { color: '#999' } },
      series: [],
    };
  }

  const categories = ['深睡', '浅睡', 'REM', '清醒'];
  const colors = ['#722ed1', '#1677ff', '#52c41a', '#faad14'];

  const getStageIndex = (stage: string) => {
    switch (stage) {
      case 'deep': return 0;
      case 'core': return 1;
      case 'rem': return 2;
      case 'awake': return 3;
      default: return -1;
    }
  };

  const data = segments.map(seg => {
    const idx = getStageIndex(seg.stage);
    if (idx === -1) return null;
    return {
      value: [
        idx,
        new Date(seg.start_time).getTime(),
        new Date(seg.end_time).getTime(),
        seg.duration
      ],
      itemStyle: {
        color: colors[idx]
      }
    };
  }).filter(Boolean);

  const startTime = data.length > 0 ? data[0]!.value[1] : null;
  const endTime = data.length > 0 ? data[data.length - 1]!.value[2] : null;

  const axisColor = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#333';
  const splitLineColor = isDark.value ? 'rgba(255, 255, 255, 0.1)' : '#eee';

  return {
    tooltip: {
      formatter: (params: any) => {
        const v = params.value;
        const stage = categories[v[0]];
        const start = new Date(v[1]).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        const end = new Date(v[2]).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        const duration = Math.round(v[3] / 60);
        return `${stage}<br/>${start} - ${end}<br/>${duration} 分钟`;
      }
    },
    grid: { left: 60, right: 20, top: 30, bottom: 30, containLabel: true },
    xAxis: {
      type: 'time',
      min: startTime,
      max: endTime,
      axisLabel: {
        formatter: (val: number) => new Date(val).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        hideOverlap: true,
        color: axisColor
      },
      axisLine: { lineStyle: { color: isDark.value ? 'rgba(255, 255, 255, 0.2)' : '#333' } },
      splitLine: { show: false }
    },
    yAxis: {
      type: 'category',
      data: categories,
      axisLabel: { color: axisColor },
      splitLine: {
        show: true,
        lineStyle: {
          type: 'dashed',
          opacity: 0.3,
          color: splitLineColor
        }
      }
    },
    series: [
      {
        type: 'custom',
        renderItem: (params: any, api: any) => {
          const categoryIndex = api.value(0);
          const start = api.coord([api.value(1), categoryIndex]);
          const end = api.coord([api.value(2), categoryIndex]);
          const height = api.size([0, 1])[1] * 0.6;

          const rectShape = echarts.graphic.clipRectByRect({
            x: start[0],
            y: start[1] - height / 2,
            width: end[0] - start[0],
            height: height
          }, {
            x: params.coordSys.x,
            y: params.coordSys.y,
            width: params.coordSys.width,
            height: params.coordSys.height
          });

          return rectShape && {
            type: 'rect',
            transition: ['shape'],
            shape: rectShape,
            style: {
              fill: api.style().fill,
              opacity: 0.8
            }
          };
        },
        itemStyle: {
          opacity: 0.8,
          borderRadius: 4
        },
        encode: {
          x: [1, 2],
          y: 0
        },
        data: data
      }
    ]
  };
});

// 每日睡眠时长图表配置
const sleepDailyOption = computed(() => {
  const grouped = groupSleepByDay(details.value?.sleep_segments || []);
  if (!grouped.length) {
    return emptyBarOption('暂无睡眠记录');
  }
  const axisColor = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#333';

  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 16, top: 32, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      data: grouped.map((item) => item.label),
      axisLabel: {
        hideOverlap: true,
        color: axisColor
      },
      axisLine: { lineStyle: { color: isDark.value ? 'rgba(255, 255, 255, 0.2)' : '#333' } }
    },
    yAxis: {
      type: 'value',
      name: '小时',
      nameTextStyle: { color: axisColor },
      axisLabel: { color: axisColor },
      splitLine: {
        lineStyle: {
          type: 'dashed',
          opacity: 0.3,
          color: isDark.value ? 'rgba(255, 255, 255, 0.1)' : '#eee'
        }
      }
    },
    series: [
      {
        type: 'bar',
        data: grouped.map((item) => Number(item.hours.toFixed(2))),
        barWidth: 20,
        itemStyle: { color: '#52c41a', borderRadius: [6, 6, 0, 0] },
      },
    ],
  };
});

// 睡眠概览信息
const sleepWindow = computed(() => {
  const overview = details.value?.sleep_overview;
  if (!overview || !overview.start_time || !overview.end_time) {
    return '--';
  }
  return `${formatDate(overview.start_time)} - ${formatDate(overview.end_time)}`;
});

const sleepDuration = computed(() => {
  const overview = details.value?.sleep_overview;
  if (!overview) {
    return '--';
  }
  return formatDuration(overview.total_duration);
});

// 睡眠质量评级（好/中/差）
const sleepQualityRating = computed(() => {
  const overview = details.value?.sleep_overview;
  if (!overview || !overview.total_duration) {
    return { label: '--', color: '#999', level: 'unknown' };
  }

  const hours = overview.total_duration / 3600;

  // 根据睡眠时长评级
  if (hours >= 7 && hours <= 9) {
    return { label: '好', color: '#52c41a', level: 'good' };
  } else if ((hours >= 6 && hours < 7) || (hours > 9 && hours <= 10)) {
    return { label: '中', color: '#faad14', level: 'medium' };
  } else {
    return { label: '差', color: '#ff4d4f', level: 'poor' };
  }
});

// 工具函数
function formatDate(value?: string | null) {
  if (!value) return '--';
  return new Date(value).toLocaleString();
}

function formatTime(value: string) {
  const date = new Date(value);
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatTimePrecise(value: string) {
  const date = new Date(value);
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function formatDay(value: string) {
  const date = new Date(value);
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

function formatDuration(seconds?: number) {
  if (!seconds || seconds <= 0) {
    return '--';
  }
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) {
    return `${hours}小时${minutes}分`;
  }
  return `${minutes}分钟`;
}

function sortByTime(a: { time: string }, b: { time: string }) {
  return new Date(a.time).getTime() - new Date(b.time).getTime();
}

function groupSleepByDay(segments: SleepSegmentPoint[]) {
  const map = new Map<string, number>();
  segments.forEach((segment) => {
    const start = new Date(segment.start_time);
    const end = new Date(segment.end_time);
    if (isNaN(start.getTime()) || isNaN(end.getTime())) {
      return;
    }
    splitIntervalByDay(start, end).forEach((part) => {
      map.set(part.day, (map.get(part.day) || 0) + part.duration / 3600);
    });
  });
  return Array.from(map.entries())
    .map(([label, hours]) => ({ label, hours }))
    .sort((a, b) => new Date(a.label).getTime() - new Date(b.label).getTime());
}

function splitIntervalByDay(start: Date, end: Date) {
  const parts: { day: string; duration: number }[] = [];
  let current = start;
  while (current < end) {
    const dayStart = new Date(current.getFullYear(), current.getMonth(), current.getDate());
    const nextDay = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000);
    const currentEnd = nextDay < end ? nextDay : end;
    parts.push({
      day: `${dayStart.getMonth() + 1}/${dayStart.getDate()}`,
      duration: (currentEnd.getTime() - current.getTime()) / 1000,
    });
    current = currentEnd;
  }
  return parts;
}

function emptyLineOption(text: string) {
  const color = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#999';
  return {
    title: { text, left: 'center', top: 'middle', textStyle: { color, fontSize: 13 } },
    xAxis: { show: false },
    yAxis: { show: false },
    series: [],
  };
}

function emptyBarOption(text: string) {
  const color = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#999';
  return {
    title: { text, left: 'center', top: 'middle', textStyle: { color, fontSize: 13 } },
    xAxis: { show: false },
    yAxis: { show: false },
    series: [],
  };
}
</script>

<template>
  <div class="life-probe-detail">
    <div class="detail-header">
      <a-page-header class="page-header" :title="pageTitle" :sub-title="`设备ID: ${summary?.device_id || '--'}`"
        @back="onBack">
        <template #extra>
          <a-radio-group v-model:value="timeRange" button-style="solid" size="middle">
            <a-radio-button value="24h">24小时</a-radio-button>
            <a-radio-button value="7d">7天</a-radio-button>
            <a-radio-button value="30d">30天</a-radio-button>
          </a-radio-group>
        </template>
      </a-page-header>
    </div>

    <div class="detail-content">
      <a-spin :spinning="loading" tip="加载中...">
        <div v-if="errorMessage && !loading" class="error-state">
          <a-alert type="error" show-icon :message="errorMessage" description="请检查网络连接或稍后重试" />
        </div>
        <template v-else>
          <div v-if="details" class="life-detail-body">
            <!-- 概览卡片 -->
            <div class="overview-grid">
              <div class="overview-card">
                <p class="label">当前心率</p>
                <h3>{{ latestHeartRate }}<span v-if="latestHeartRate !== '--'"> BPM</span></h3>
                <small>更新于 {{ latestHeartRateTime }}</small>
              </div>
              <div class="overview-card">
                <p class="label">今日步数</p>
                <h3>{{ stepsToday.toLocaleString() }}</h3>
                <small>同步时间 {{ lastSync }}</small>
              </div>
              <div class="overview-card">
                <p class="label">睡眠时长</p>
                <div style="display: flex; align-items: baseline; gap: 12px;">
                  <h3>{{ sleepDuration }}</h3>
                  <a-tag v-if="sleepQualityRating.level !== 'unknown'" :color="sleepQualityRating.color"
                    style="font-size: 14px; font-weight: 600; padding: 4px 12px; border-radius: 6px;">
                    {{ sleepQualityRating.label }}
                  </a-tag>
                </div>
                <small>睡眠时段 {{ sleepWindow }}</small>
              </div>
              <div class="overview-card">
                <p class="label">电量</p>
                <h3>{{ batteryLevel }}</h3>
                <small>最后同步 {{ lastSync }}</small>
              </div>
            </div>

            <!-- 图表网格 -->
            <div class="chart-grid">
              <!-- 心率曲线 -->
              <div class="chart-card">
                <div class="chart-title">心率曲线</div>
                <VChart :option="heartRateOption" autoresize class="chart" />
              </div>

              <!-- 步数区间 -->
              <div class="chart-card">
                <div class="chart-title">步数区间</div>
                <VChart :option="stepSegmentOption" autoresize class="chart" />
              </div>

              <!-- 每日步数 -->
              <div class="chart-card">
                <div class="chart-title">每日步数</div>
                <VChart :option="dailyStepsOption" autoresize class="chart" />
              </div>

              <!-- 每日睡眠时长 -->
              <div class="chart-card">
                <div class="chart-title">每日睡眠时长</div>
                <VChart :option="sleepDailyOption" autoresize class="chart" />
              </div>

              <!-- 睡眠质量（占满宽度） -->
              <div class="chart-card full-width">
                <div class="chart-title">睡眠阶段分布</div>
                <VChart :option="sleepOption" autoresize class="chart" />
              </div>
            </div>
          </div>
          <a-empty v-else description="暂无详情数据" style="margin: 48px 0;" />
        </template>
      </a-spin>
    </div>
  </div>
</template>

<style scoped>
.life-probe-detail {
  min-height: 100vh;
  background: transparent;
}

.detail-header {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-bottom: 1px solid var(--card-border);
  padding: 0 24px;
  position: sticky;
  top: 0;
  z-index: 100;
}

.page-header {
  padding: 16px 0;
}

.page-header :deep(.ant-page-header-heading-title) {
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, #ff4d4f, #ff7875);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.page-header :deep(.ant-page-header-heading-sub-title) {
  font-size: 13px;
  font-family: "SF Mono", Menlo, monospace;
  color: var(--text-secondary);
}

.detail-content {
  padding: 24px;
}

.error-state {
  max-width: 600px;
  margin: 48px auto;
}

.life-detail-body {
  max-width: 1400px;
  margin: 0 auto;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.overview-card {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: var(--radius-lg);
  padding: 16px;
  box-shadow: 0 4px 24px -1px rgba(0, 0, 0, 0.05);
  border: 1px solid var(--card-border);
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
}

.overview-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 32px -4px rgba(0, 0, 0, 0.1);
  border-color: rgba(255, 77, 79, 0.3);
}

.overview-card .label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.overview-card h3 {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.5px;
  font-family: "SF Mono", Menlo, monospace;
}

.overview-card small {
  display: block;
  margin-top: 8px;
  color: var(--text-secondary);
  font-size: 12px;
}

.chart-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
}

@media (max-width: 768px) {
  .overview-grid {
    grid-template-columns: 1fr !important;
  }

  .chart-grid {
    grid-template-columns: 1fr !important;
  }
}

.chart-card {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: var(--radius-lg);
  padding: 16px;
  box-shadow: 0 4px 24px -1px rgba(0, 0, 0, 0.05);
  border: 1px solid var(--card-border);
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
}

.chart-card:hover {
  box-shadow: 0 12px 32px -4px rgba(0, 0, 0, 0.1);
}

.chart-card.full-width {
  grid-column: 1 / -1;
}

.chart-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 12px;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.chart {
  width: 100%;
  height: 240px;
}
</style>

<style>
/* Dark mode 样式 */
.dark .detail-header {
  background: rgba(20, 20, 20, 0.85);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.dark .overview-card,
.dark .chart-card {
  background: rgba(30, 30, 30, 0.6);
  border-color: rgba(255, 255, 255, 0.08);
}

.dark .overview-card:hover,
.dark .chart-card:hover {
  background: rgba(40, 40, 40, 0.8);
  border-color: rgba(22, 119, 255, 0.4);
}
</style>
