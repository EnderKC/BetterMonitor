<script setup lang="ts">
import { computed } from 'vue';
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, TitleComponent } from 'echarts/components';
import VChart from 'vue-echarts';

// 注册必要的ECharts组件
use([
  CanvasRenderer,
  LineChart,
  GridComponent,
  TooltipComponent,
  TitleComponent
]);

// 定义数据点类型
interface DataPoint {
  time: string;
  value: number;
}

// 定义props
interface Props {
  data?: DataPoint[];
  title?: string;
  height?: string;
}

const props = withDefaults(defineProps<Props>(), {
  data: () => [],
  title: 'CPU使用率',
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
    formatter: (params: any) => {
      const param = params[0];
      return `${param.name}<br/>${param.seriesName}: ${param.value}%`;
    }
  },
  xAxis: {
    type: 'category',
    data: props.data.map((item: DataPoint) => item.time),
    axisLabel: {
      rotate: 45,
      fontSize: 11
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
      data: props.data.map((item: DataPoint) => item.value),
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
    bottom: '15%',
    top: '15%',
    containLabel: true
  }
}));
</script>

<template>
  <div class="cpu-usage-chart-card">
    <div class="chart-container" :style="{ height: props.height }">
      <v-chart
        v-if="props.data.length > 0"
        class="chart"
        :option="chartOption"
        autoresize
      />
      <div v-else class="empty-chart">
        <span class="empty-text">暂无数据</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cpu-usage-chart-card {
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
