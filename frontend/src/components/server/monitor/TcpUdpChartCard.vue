<script setup lang="ts">
import { computed } from 'vue';
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, TitleComponent, LegendComponent } from 'echarts/components';
import VChart from 'vue-echarts';

// 注册必要的ECharts组件
use([
  CanvasRenderer,
  LineChart,
  GridComponent,
  TooltipComponent,
  TitleComponent,
  LegendComponent
]);

// 定义数据点类型
interface DataPoint {
  time: string;
  value: number;
}

// 定义TCP/UDP数据类型
interface TcpUdpData {
  tcp: DataPoint[];
  udp: DataPoint[];
}

// 定义props
interface Props {
  data?: TcpUdpData;
  title?: string;
  height?: string;
}

const props = withDefaults(defineProps<Props>(), {
  data: () => ({ tcp: [], udp: [] }),
  title: 'TCP / UDP 连接数',
  height: '280px'
});

// 图表配置
const chartOption = computed(() => ({
  title: {
    text: props.title,
    left: 'center',
    textStyle: {
      fontSize: 14,
      fontWeight: 600
    }
  },
  tooltip: {
    trigger: 'axis',
    formatter: (params: any[]) => {
      const time = params[0].name;
      let result = `${time}<br/>`;
      params.forEach(param => {
        const color = param.color;
        const seriesName = param.seriesName;
        const value = Math.round(param.value);
        result += `<span style="display:inline-block;margin-right:5px;border-radius:10px;width:10px;height:10px;background-color:${color};"></span> ${seriesName}: ${value} 个<br/>`;
      });
      return result;
    }
  },
  legend: {
    data: ['TCP连接', 'UDP连接'],
    top: '30px'
  },
  xAxis: {
    type: 'category',
    data: props.data.tcp.map((item: DataPoint) => item.time),
    axisLabel: {
      rotate: 45,
      fontSize: 11
    }
  },
  yAxis: {
    type: 'value',
    minInterval: 1,
    axisLabel: {
      formatter: '{value}'
    },
    scale: true,
    min: 0
  },
  series: [
    {
      name: 'TCP连接',
      type: 'line',
      data: props.data.tcp.map((item: DataPoint) => item.value),
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#1890FF'
      },
      smooth: true
    },
    {
      name: 'UDP连接',
      type: 'line',
      data: props.data.udp.map((item: DataPoint) => item.value),
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
    bottom: '15%',
    top: '70px',
    containLabel: true
  }
}));

const hasData = computed(() => {
  return props.data.tcp.length > 0 || props.data.udp.length > 0;
});
</script>

<template>
  <div class="tcp-udp-chart-card">
    <div class="chart-container" :style="{ height: props.height }">
      <v-chart
        v-if="hasData"
        class="chart"
        :option="chartOption"
        autoresize
      />
      <div v-else class="empty-chart">
        <span class="empty-icon">🔌</span>
        <span class="empty-text">暂无数据</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tcp-udp-chart-card {
  width: 100%;
  height: 100%;
}

.chart-container {
  width: 100%;
  position: relative;
}

.chart {
  width: 100%;
  height: 100%;
}

.empty-chart {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--text-secondary);
  background: var(--alpha-black-02);
  border-radius: var(--radius-sm);
}

.empty-icon {
  font-size: 48px;
  opacity: 0.5;
}

.empty-text {
  font-size: var(--font-size-md);
}
</style>
