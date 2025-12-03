<script setup lang="ts">
import { ref, watch, computed, onUnmounted } from 'vue';
import { message } from 'ant-design-vue';
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart, BarChart, PieChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent, TitleComponent } from 'echarts/components';
import VChart from 'vue-echarts';
import { getToken } from '@/utils/auth';
import type { LifeProbeDetails, SleepSegmentPoint } from '@/types/life';

use([CanvasRenderer, LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent, TitleComponent]);

const props = defineProps<{
  modelValue: boolean;
  probeId: number | null;
  publicMode?: boolean;
}>();

const emit = defineEmits(['update:modelValue']);

const loading = ref(false);
const details = ref<LifeProbeDetails | null>(null);
const errorMessage = ref('');
const detailWS = ref<WebSocket | null>(null);
const detailReconnectTimer = ref<number | null>(null);

const close = () => {
  emit('update:modelValue', false);
  cleanupDetailWS();
};

const cleanupDetailWS = () => {
  if (detailReconnectTimer.value !== null) {
    clearTimeout(detailReconnectTimer.value);
    detailReconnectTimer.value = null;
  }
  if (detailWS.value) {
    detailWS.value.onclose = null;
    detailWS.value.close();
    detailWS.value = null;
  }
};

const connectDetailWS = () => {
  if (!props.modelValue || !props.probeId) {
    return;
  }

  if (
    detailWS.value &&
    (detailWS.value.readyState === WebSocket.OPEN || detailWS.value.readyState === WebSocket.CONNECTING)
  ) {
    return;
  }

  if (detailReconnectTimer.value !== null) {
    clearTimeout(detailReconnectTimer.value);
    detailReconnectTimer.value = null;
  }

  loading.value = true;
  errorMessage.value = '';

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  let wsUrl = `${protocol}//${window.location.host}/api/life-probes/public/${props.probeId}/ws?hours=24&daily_days=7`;
  if (!props.publicMode) {
    const token = getToken();
    if (token) {
      wsUrl += `&token=${encodeURIComponent(token)}`;
    }
  }

  const ws = new WebSocket(wsUrl);
  detailWS.value = ws;

  ws.onopen = () => {
    loading.value = true;
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'life_probe_detail' && data.details) {
        details.value = data.details as LifeProbeDetails;
        loading.value = false;
        errorMessage.value = '';
      }
    } catch (err) {
      console.error('解析生命探针详情数据失败:', err);
    }
  };

  ws.onerror = (err) => {
    console.error('生命探针详情WebSocket错误:', err);
    errorMessage.value = '生命探针详情连接失败';
    message.error('生命探针详情连接失败');
  };

  ws.onclose = () => {
    detailWS.value = null;
    if (props.modelValue) {
      detailReconnectTimer.value = window.setTimeout(() => {
        detailReconnectTimer.value = null;
        connectDetailWS();
      }, 4000);
    } else {
      loading.value = false;
    }
  };
};

watch(
  () => props.modelValue,
  (visible) => {
    if (visible) {
      connectDetailWS();
    } else {
      details.value = null;
      loading.value = false;
      errorMessage.value = '';
      cleanupDetailWS();
    }
  }
);

watch(
  () => props.probeId,
  (id, old) => {
    if (props.modelValue && id && id !== old) {
      cleanupDetailWS();
      connectDetailWS();
    }
  }
);

onUnmounted(() => {
  cleanupDetailWS();
});

const summary = computed(() => details.value?.summary);

const latestHeartRate = computed(() => summary.value?.latest_heart_rate?.value ?? '--');
const latestHeartRateTime = computed(() => formatDate(summary.value?.latest_heart_rate?.time));
const stepsToday = computed(() => summary.value ? Math.round(summary.value.steps_today) : 0);
const batteryLevel = computed(() =>
  summary.value?.battery_level !== undefined && summary.value?.battery_level !== null
    ? `${Math.round((summary.value.battery_level || 0) * 100)}%`
    : '--'
);
const lastSync = computed(() => formatDate(summary.value?.last_sync_at));
const focusStatus = computed(() => {
  if (!summary.value?.focus_event) {
    return '未上报';
  }
  return summary.value.focus_event.is_focused ? '专注中' : '普通模式';
});

const heartRateOption = computed(() => {
  const data = [...(details.value?.heart_rates || [])].sort(sortByTime);
  if (!data.length) {
    return emptyLineOption('暂无心率数据');
  }
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 16, top: 32, bottom: 30 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.map((point) => formatTime(point.time)),
    },
    yAxis: {
      type: 'value',
      name: 'BPM',
      min: (value: any) => Math.max(50, Math.floor(value.min - 5)),
    },
    series: [
      {
        type: 'line',
        data: data.map((point) => point.value),
        smooth: true,
        areaStyle: { opacity: 0.2 },
        showSymbol: false,
        lineStyle: { width: 2 },
        color: '#ff4d4f',
      },
    ],
  };
});

const stepSegmentOption = computed(() => {
  const samples = [...(details.value?.step_samples || [])].sort((a, b) =>
    new Date(a.start).getTime() - new Date(b.start).getTime()
  );
  if (!samples.length) {
    return emptyBarOption('暂无步数数据');
  }
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
    grid: { left: 40, right: 16, top: 32, bottom: 30 },
    xAxis: {
      type: 'category',
      data: samples.map((sample) => formatTimePrecise(sample.start)),
      axisLabel: { rotate: 45 },
    },
    yAxis: { type: 'value', name: '步数' },
    series: [
      {
        type: 'bar',
        barWidth: 16,
        data: samples.map((sample) => Number(sample.value.toFixed(0))),
        itemStyle: {
          color: '#1677ff',
          borderRadius: [6, 6, 0, 0],
        },
      },
    ],
  };
});

const focusOption = computed(() => {
  const events = [...(details.value?.focus_events || [])].sort(sortByTime);
  if (!events.length) {
    return emptyLineOption('暂无专注模式历史');
  }
  return {
    tooltip: {
      trigger: 'axis',
      formatter: (params: any) => {
        const point = params[0];
        return `${point.axisValue}<br/>${point.value === 1 ? '专注模式' : '普通模式'}`;
      },
    },
    grid: { left: 40, right: 16, top: 32, bottom: 30 },
    xAxis: { type: 'category', data: events.map((evt) => formatTime(evt.time)) },
    yAxis: {
      type: 'value',
      min: 0,
      max: 1,
      interval: 1,
      axisLabel: {
        formatter: (value: number) => (value === 1 ? '专注' : '普通'),
      },
    },
    series: [
      {
        type: 'line',
        step: 'end',
        data: events.map((evt) => (evt.is_focused ? 1 : 0)),
        lineStyle: { color: '#722ed1', width: 2 },
        areaStyle: { opacity: 0.15 },
        symbol: 'circle',
        symbolSize: 8,
      },
    ],
  };
});

const dailyStepsOption = computed(() => {
  const totals = (summary.value?.daily_totals || []).filter(
    (item) => item.sample_type === 'steps_detailed'
  );
  if (!totals.length) {
    return emptyBarOption('暂无每日步数');
  }
  const sorted = [...totals].sort((a, b) => new Date(a.day).getTime() - new Date(b.day).getTime());
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 16, top: 32, bottom: 30 },
    xAxis: {
      type: 'category',
      data: sorted.map((item) => formatDay(item.day)),
    },
    yAxis: { type: 'value', name: '步数' },
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

const sleepOption = computed(() => {
  const overview = details.value?.sleep_overview;
  if (!overview || !overview.stage_durations) {
    return {
      title: { text: '暂无睡眠数据', left: 'center', top: 'middle', textStyle: { color: '#999' } },
      series: [],
    };
  }
  const entries = Object.entries(overview.stage_durations || {}).map(([stage, seconds]) => ({
    name: translateStage(stage),
    value: Number((seconds / 3600).toFixed(2)),
  }));

  if (!entries.length) {
    return {
      title: { text: '暂无睡眠数据', left: 'center', top: 'middle', textStyle: { color: '#999' } },
      series: [],
    };
  }

  return {
    tooltip: { trigger: 'item', formatter: '{b}: {c}h ({d}%)' },
    legend: { orient: 'vertical', left: 'left' },
    series: [
      {
        type: 'pie',
        radius: ['40%', '70%'],
        avoidLabelOverlap: false,
        label: { show: true, formatter: '{b}\n{c}h' },
        data: entries,
      },
    ],
  };
});

const sleepDailyOption = computed(() => {
  const grouped = groupSleepByDay(details.value?.sleep_segments || []);
  if (!grouped.length) {
    return emptyBarOption('暂无睡眠记录');
  }
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 16, top: 32, bottom: 30 },
    xAxis: { type: 'category', data: grouped.map((item) => item.label) },
    yAxis: { type: 'value', name: '小时' },
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

const screenUsageOption = computed(() => {
  const events = [...(details.value?.screen_events || [])].sort(
    (a, b) => new Date(a.time).getTime() - new Date(b.time).getTime()
  );
  if (!events.length) {
    return emptyLineOption('暂无屏幕事件');
  }

  const data: [number, number][] = [];
  let state = 0;
  events.forEach((evt, index) => {
    const timeValue = new Date(evt.time).getTime();
    if (index === 0) {
      data.push([timeValue, state]);
    }
    state = evt.action === 'unlock' ? 1 : 0;
    data.push([timeValue, state]);
  });

  return {
    tooltip: {
      trigger: 'axis',
      formatter: (params: any) => {
        const point = params[0];
        return `${new Date(point.data[0]).toLocaleString()}<br/>${point.data[1] === 1 ? '正在使用' : '锁屏'}`;
      },
    },
    grid: { left: 40, right: 16, top: 32, bottom: 30 },
    xAxis: { type: 'time' },
    yAxis: {
      type: 'value',
      min: 0,
      max: 1,
      axisLabel: { formatter: (val: number) => (val === 1 ? '使用中' : '锁屏') },
    },
    series: [
      {
        type: 'line',
        step: 'end',
        data,
        lineStyle: { color: '#fa8c16', width: 2 },
        areaStyle: { opacity: 0.2 },
        symbol: 'none',
      },
    ],
  };
});

const screenUsageSummary = computed(() => {
  const events = [...(details.value?.screen_events || [])].sort(
    (a, b) => new Date(a.time).getTime() - new Date(b.time).getTime()
  );
  let totalUnlockedMs = 0;
  let sessions = 0;
  let lastUnlock: Date | null = null;

  events.forEach((evt) => {
    const time = new Date(evt.time);
    if (evt.action === 'unlock') {
      lastUnlock = time;
      sessions += 1;
    } else if (evt.action === 'lock' && lastUnlock) {
      totalUnlockedMs += time.getTime() - lastUnlock.getTime();
      lastUnlock = null;
    }
  });

  return {
    hours: totalUnlockedMs / (1000 * 60 * 60),
    sessions,
  };
});

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

function translateStage(stage: string) {
  switch (stage) {
    case 'awake':
      return '清醒';
    case 'core':
      return '浅睡';
    case 'deep':
      return '深睡';
    case 'rem':
      return 'REM';
    case 'in_bed':
      return '卧床';
    default:
      return stage;
  }
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
  return {
    title: { text, left: 'center', top: 'middle', textStyle: { color: '#999', fontSize: 13 } },
    xAxis: { show: false },
    yAxis: { show: false },
    series: [],
  };
}

function emptyBarOption(text: string) {
  return {
    title: { text, left: 'center', top: 'middle', textStyle: { color: '#999', fontSize: 13 } },
    xAxis: { show: false },
    yAxis: { show: false },
    series: [],
  };
}
</script>

<template>
  <a-modal
    :open="modelValue"
    centered
    width="960px"
    class="life-detail-modal"
    :footer="null"
    destroy-on-close
    @cancel="close"
  >
    <template #title>
      <div class="modal-title">
        <span>{{ summary?.name || '生命探针详情' }}</span>
        <span class="subtitle">设备ID: {{ summary?.device_id }}</span>
      </div>
    </template>

    <a-spin :spinning="loading">
      <div v-if="errorMessage && !loading" class="error-state">{{ errorMessage }}</div>
      <template v-else>
        <div v-if="details" class="life-detail-body">
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
              <p class="label">专注状态</p>
              <h3>{{ focusStatus }}</h3>
              <small>最后更新 {{ summary?.focus_event ? formatDate(summary?.focus_event.time) : '--' }}</small>
            </div>
            <div class="overview-card">
              <p class="label">电量</p>
              <h3>{{ batteryLevel }}</h3>
              <small>最后同步 {{ lastSync }}</small>
            </div>
          </div>

          <div class="chart-grid">
            <div class="chart-card">
              <div class="chart-title">心率曲线</div>
              <VChart :option="heartRateOption" autoresize class="chart" />
            </div>
            <div class="chart-card">
              <div class="chart-title">步数区间</div>
              <VChart :option="stepSegmentOption" autoresize class="chart" />
            </div>
            <div class="chart-card">
              <div class="chart-title">专注模式历史</div>
              <VChart :option="focusOption" autoresize class="chart" />
            </div>
            <div class="chart-card">
              <div class="chart-title">每日步数</div>
              <VChart :option="dailyStepsOption" autoresize class="chart" />
            </div>
            <div class="chart-card full-width">
              <div class="chart-title">睡眠质量</div>
              <div class="sleep-summary">
                <div>
                  <p>睡眠时长</p>
                  <strong>{{ sleepDuration }}</strong>
                </div>
                <div>
                  <p>睡眠时段</p>
                  <strong>{{ sleepWindow }}</strong>
                </div>
              </div>
              <VChart :option="sleepOption" autoresize class="chart" />
            </div>
            <div class="chart-card">
              <div class="chart-title">每日睡眠时长</div>
              <VChart :option="sleepDailyOption" autoresize class="chart" />
            </div>
            <div class="chart-card">
              <div class="chart-title">屏幕使用</div>
              <div class="screen-summary">
                <div>
                  <p>解锁次数</p>
                  <strong>{{ screenUsageSummary.sessions }}</strong>
                </div>
                <div>
                  <p>总使用时长</p>
                  <strong>{{ screenUsageSummary.hours > 0 ? screenUsageSummary.hours.toFixed(1) + ' 小时' : '--' }}</strong>
                </div>
              </div>
              <VChart :option="screenUsageOption" autoresize class="chart" />
            </div>
          </div>
        </div>
        <a-empty v-else description="暂无详情数据" />
      </template>
    </a-spin>
  </a-modal>
</template>

<style scoped>
.life-detail-modal :deep(.ant-modal-body) {
  padding: 0;
}

.modal-title {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.modal-title .subtitle {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.45);
}

.life-detail-body {
  padding: 24px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.overview-card {
  background: #fff;
  border-radius: 16px;
  padding: 16px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.06);
}

.overview-card .label {
  font-size: 13px;
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 8px;
}

.overview-card h3 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.85);
}

.overview-card small {
  display: block;
  margin-top: 6px;
  color: rgba(0, 0, 0, 0.45);
}

.chart-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 16px;
}

.chart-card {
  background: #fff;
  border-radius: 20px;
  padding: 16px;
  box-shadow: 0 12px 32px rgba(15, 23, 42, 0.08);
}

.chart-card.full-width {
  grid-column: 1 / -1;
}

.chart-title {
  font-weight: 600;
  margin-bottom: 8px;
  color: rgba(0, 0, 0, 0.78);
}

.chart {
  width: 100%;
  height: 240px;
}

.sleep-summary {
  display: flex;
  gap: 32px;
  margin-bottom: 8px;
}

.sleep-summary p {
  margin: 0;
  font-size: 13px;
  color: rgba(0, 0, 0, 0.45);
}

.sleep-summary strong {
  display: block;
  margin-top: 4px;
  font-size: 16px;
}

.screen-summary {
  display: flex;
  gap: 32px;
  margin-bottom: 8px;
}

.screen-summary p {
  margin: 0;
  font-size: 13px;
  color: rgba(0, 0, 0, 0.45);
}

.screen-summary strong {
  display: block;
  margin-top: 4px;
  font-size: 16px;
}

.error-state {
  padding: 32px;
  text-align: center;
  color: #ff4d4f;
}
</style>
