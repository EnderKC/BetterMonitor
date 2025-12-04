<script setup lang="ts">
import { ref, watch, computed, onUnmounted } from 'vue';
import { message } from 'ant-design-vue';
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart, BarChart, PieChart, CustomChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent, TitleComponent, DataZoomComponent } from 'echarts/components';
import VChart from 'vue-echarts';
import { getToken } from '@/utils/auth';
import type { LifeProbeDetails, SleepSegmentPoint } from '@/types/life';
import * as echarts from 'echarts/core';
import { useThemeStore } from '@/stores/theme';
import { storeToRefs } from 'pinia';

use([CanvasRenderer, LineChart, BarChart, PieChart, CustomChart, GridComponent, TooltipComponent, LegendComponent, TitleComponent, DataZoomComponent]);

const props = defineProps<{
  modelValue: boolean;
  probeId: number | null;
  publicMode?: boolean;
}>();

const emit = defineEmits(['update:modelValue']);

const themeStore = useThemeStore();
const { isDark } = storeToRefs(themeStore);

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

const focusOption = computed(() => {
  const events = [...(details.value?.focus_events || [])].sort(sortByTime);
  if (!events.length) {
    return emptyLineOption('暂无专注模式历史');
  }
  const axisColor = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#333';
  const splitLineColor = isDark.value ? 'rgba(255, 255, 255, 0.1)' : '#eee';

  return {
    tooltip: {
      trigger: 'axis',
      formatter: (params: any) => {
        const point = params[0];
        return `${point.axisValue}<br/>${point.value === 1 ? '专注模式' : '普通模式'}`;
      },
    },
    grid: { left: 40, right: 16, top: 32, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      data: events.map((evt) => formatTime(evt.time)),
      axisLabel: {
        hideOverlap: true,
        color: axisColor
      },
      axisLine: { lineStyle: { color: isDark.value ? 'rgba(255, 255, 255, 0.2)' : '#333' } }
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 1,
      interval: 1,
      axisLabel: {
        formatter: (value: number) => (value === 1 ? '专注' : '普通'),
        color: axisColor
      },
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
        step: 'end',
        data: events.map((evt) => (evt.is_focused ? 1 : 0)),
        lineStyle: { color: '#722ed1', width: 2 },
        areaStyle: {
          opacity: 0.15,
          color: '#722ed1'
        },
        symbol: 'none',
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

const sleepOption = computed(() => {
  const segments = details.value?.sleep_segments || [];
  if (!segments.length) {
    return {
      title: { text: '暂无睡眠数据', left: 'center', top: 'middle', textStyle: { color: '#999' } },
      series: [],
    };
  }

  // Define categories and colors
  const categories = ['深睡', '浅睡', 'REM', '清醒'];
  const colors = ['#722ed1', '#1677ff', '#52c41a', '#faad14']; // Deep, Core, REM, Awake

  // Map stage codes to indices
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

const screenUsageOption = computed(() => {
  const events = [...(details.value?.screen_events || [])].sort(
    (a, b) => new Date(a.time).getTime() - new Date(b.time).getTime()
  );
  if (!events.length) {
    return emptyLineOption('暂无屏幕事件');
  }

  // Transform events into segments
  const segments = [];
  let lastTime = new Date(events[0].time).getTime();
  let isUnlocked = events[0].action === 'unlock';

  for (let i = 1; i < events.length; i++) {
    const currentTime = new Date(events[i].time).getTime();
    if (isUnlocked) {
      segments.push({
        start: lastTime,
        end: currentTime,
        state: 'Unlocked'
      });
    } else {
      segments.push({
        start: lastTime,
        end: currentTime,
        state: 'Locked'
      });
    }
    lastTime = currentTime;
    isUnlocked = events[i].action === 'unlock';
  }

  const data = segments.map(seg => ({
    value: [
      0, // Only one category row
      seg.start,
      seg.end,
      seg.state
    ],
    itemStyle: {
      color: seg.state === 'Unlocked' ? '#fa8c16' : '#f0f0f0'
    }
  }));

  const axisColor = isDark.value ? 'rgba(255, 255, 255, 0.45)' : '#333';

  return {
    tooltip: {
      formatter: (params: any) => {
        const v = params.value;
        const state = v[3];
        const start = new Date(v[1]).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        const end = new Date(v[2]).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        const duration = Math.round((v[2] - v[1]) / 60000);
        return `${state}<br/>${start} - ${end}<br/>${duration} 分钟`;
      }
    },
    grid: { left: 20, right: 20, top: 30, bottom: 30, containLabel: true },
    xAxis: {
      type: 'time',
      axisLabel: {
        hideOverlap: true,
        color: axisColor
      },
      axisLine: { lineStyle: { color: isDark.value ? 'rgba(255, 255, 255, 0.2)' : '#333' } },
      splitLine: { show: false }
    },
    yAxis: {
      type: 'category',
      data: ['状态'],
      show: false
    },
    series: [
      {
        type: 'custom',
        renderItem: (params: any, api: any) => {
          const categoryIndex = api.value(0);
          const start = api.coord([api.value(1), categoryIndex]);
          const end = api.coord([api.value(2), categoryIndex]);
          const height = api.size([0, 1])[1] * 0.4;

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
              fill: api.style().fill
            }
          };
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
  <a-modal :open="modelValue" width="80%" class="life-detail-modal" :footer="null" destroy-on-close @cancel="close">
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
                  <strong>{{ screenUsageSummary.hours > 0 ? screenUsageSummary.hours.toFixed(1) + ' 小时' : '--'
                    }}</strong>
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
.life-detail-modal :deep(.ant-modal) {
  padding-bottom: 0;
  top: 0;
  margin: 10vh auto;
}

.life-detail-modal :deep(.ant-modal-content) {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(24px) saturate(180%);
  -webkit-backdrop-filter: blur(24px) saturate(180%);
  border-radius: 24px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1), 0 0 20px rgba(66, 153, 225, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.5);
  padding: 0;
  overflow: hidden;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
}

.life-detail-modal :deep(.ant-modal-header) {
  background: transparent;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  padding: 24px 32px;
  flex-shrink: 0;
}

.life-detail-modal :deep(.ant-modal-body) {
  padding: 0;
  flex: 1;
  overflow-y: auto;
}

.life-detail-modal :deep(.ant-modal-close) {
  top: 24px;
  right: 24px;
}

.modal-title {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.modal-title span:first-child {
  font-size: 20px;
  font-weight: 700;
  background: linear-gradient(135deg, #2563eb, #7c3aed);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.modal-title .subtitle {
  font-size: 13px;
  color: rgba(0, 0, 0, 0.45);
  font-family: "SF Mono", Menlo, monospace;
}

.life-detail-body {
  padding: 32px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0) 0%, rgba(255, 255, 255, 0.5) 100%);
  min-height: 100%;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 24px;
  margin-bottom: 32px;
}

.overview-card {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px) saturate(180%);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  border-radius: 20px;
  padding: 24px;
  box-shadow: 0 8px 32px 0 rgba(31, 38, 135, 0.07);
  border: 1px solid rgba(255, 255, 255, 0.5);
  transition: transform 0.2s;
}

.overview-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 40px 0 rgba(31, 38, 135, 0.15);
}

.overview-card .label {
  font-size: 13px;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.overview-card h3 {
  margin: 0;
  font-size: 32px;
  font-weight: 700;
  color: rgba(0, 0, 0, 0.85);
  letter-spacing: -0.5px;
}

.overview-card small {
  display: block;
  margin-top: 8px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}

.chart-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(500px, 1fr));
  gap: 24px;
}

.chart-card {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px) saturate(180%);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  border-radius: 24px;
  padding: 24px;
  box-shadow: 0 8px 32px 0 rgba(31, 38, 135, 0.07);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.chart-card.full-width {
  grid-column: 1 / -1;
}

.chart-title {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 20px;
  color: rgba(0, 0, 0, 0.75);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.chart {
  width: 100%;
  height: 320px;
}

.sleep-summary,
.screen-summary {
  display: flex;
  gap: 48px;
  margin-bottom: 20px;
  padding: 20px;
  background: rgba(0, 0, 0, 0.02);
  border-radius: 16px;
}

.sleep-summary p,
.screen-summary p {
  margin: 0;
  font-size: 13px;
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 4px;
}

.sleep-summary strong,
.screen-summary strong {
  display: block;
  font-size: 24px;
  color: rgba(0, 0, 0, 0.85);
}

.error-state {
  padding: 48px;
  text-align: center;
  color: #ff4d4f;
  background: rgba(255, 77, 79, 0.05);
  border-radius: 16px;
  margin: 24px;
}
</style>

<style>
.dark .life-detail-modal .ant-modal-content {
  background: rgba(20, 20, 20, 0.85) !important;
  border: 1px solid rgba(255, 255, 255, 0.1) !important;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.3), 0 0 20px rgba(66, 153, 225, 0.1) !important;
}

.dark .life-detail-modal .ant-modal-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.06) !important;
}

.dark .life-detail-modal .ant-modal-close {
  color: rgba(255, 255, 255, 0.65) !important;
}

.dark .life-detail-modal .ant-modal-close:hover {
  color: rgba(255, 255, 255, 0.85) !important;
}

.dark .modal-title .subtitle {
  color: rgba(255, 255, 255, 0.45) !important;
}

.dark .life-detail-body {
  background: linear-gradient(180deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0.2) 100%) !important;
}

.dark .overview-card {
  background: rgba(30, 30, 30, 0.6) !important;
  border: 1px solid rgba(255, 255, 255, 0.1) !important;
  box-shadow: 0 8px 32px 0 rgba(0, 0, 0, 0.2) !important;
}

.dark .overview-card:hover {
  box-shadow: 0 12px 40px 0 rgba(0, 0, 0, 0.3) !important;
}

.dark .overview-card .label {
  color: rgba(255, 255, 255, 0.45) !important;
}

.dark .overview-card h3 {
  color: rgba(255, 255, 255, 0.85) !important;
}

.dark .overview-card small {
  color: rgba(255, 255, 255, 0.45) !important;
}

.dark .chart-card {
  background: rgba(30, 30, 30, 0.6) !important;
  border: 1px solid rgba(255, 255, 255, 0.1) !important;
  box-shadow: 0 8px 32px 0 rgba(0, 0, 0, 0.2) !important;
}

.dark .chart-title {
  color: rgba(255, 255, 255, 0.85) !important;
}

.dark .sleep-summary,
.dark .screen-summary {
  background: rgba(255, 255, 255, 0.05) !important;
}

.dark .sleep-summary p,
.dark .screen-summary p {
  color: rgba(255, 255, 255, 0.45) !important;
}

.dark .sleep-summary strong,
.dark .screen-summary strong {
  color: rgba(255, 255, 255, 0.85) !important;
}
</style>
