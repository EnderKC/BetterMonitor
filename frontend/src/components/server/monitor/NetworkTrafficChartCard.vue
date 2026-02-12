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

// 定义网络数据类型
interface NetworkData {
  in: DataPoint[];
  out: DataPoint[];
}

// 定义props
interface Props {
  data?: NetworkData;
  title?: string;
  height?: string;
}

const props = withDefaults(defineProps<Props>(), {
  data: () => ({ in: [], out: [] }),
  title: '网络流量',
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
        const value = parseFloat(param.value).toFixed(3);
        result += `<span style="display:inline-block;margin-right:5px;border-radius:10px;width:10px;height:10px;background-color:${color};"></span> ${seriesName}: ${value} MB/s<br/>`;
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
    data: props.data.in.map((item: DataPoint) => item.time),
    axisLabel: {
      rotate: 45,
      fontSize: 11
    }
  },
  yAxis: {
    type: 'value',
    axisLabel: {
      formatter: '{value} MB/s'
    },
    scale: true,
    min: 0
  },
  series: [
    {
      name: '入站流量',
      type: 'line',
      data: props.data.in.map((item: DataPoint) => item.value),
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
      data: props.data.out.map((item: DataPoint) => item.value),
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
    bottom: '15%',
    top: '70px',
    containLabel: true
  }
}));

const hasData = computed(() => {
  return props.data.in.length > 0 || props.data.out.length > 0;
});
</script>

<template>
  <div class="network-traffic-chart-card">
    <div class="chart-container" :style="{ height: props.height }">
      <v-chart
        v-if="hasData"
        class="chart"
        :option="chartOption"
        autoresize
      />
      <div v-else class="empty-chart">
        <span class="empty-icon">🌐</span>
        <span class="empty-text">暂无数据</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.network-traffic-chart-card {
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
