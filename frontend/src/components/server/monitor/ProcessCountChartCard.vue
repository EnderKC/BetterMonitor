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
  title: '进程数',
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
      return `${param.name}<br/>${param.seriesName}: ${param.value} 个`;
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
    minInterval: 1,
    axisLabel: {
      formatter: '{value}'
    }
  },
  series: [
    {
      name: '进程数',
      type: 'line',
      data: props.data.map((item: DataPoint) => item.value),
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#722ED1'
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
  <div class="process-count-chart-card">
    <div class="chart-container" :style="{ height: props.height }">
      <v-chart
        v-if="props.data.length > 0"
        class="chart"
        :option="chartOption"
        autoresize
      />
      <div v-else class="empty-chart">
        <span class="empty-icon">📈</span>
        <span class="empty-text">暂无数据</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.process-count-chart-card {
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
  background: rgba(0, 0, 0, 0.02);
  border-radius: 8px;
}

.empty-icon {
  font-size: 48px;
  opacity: 0.5;
}

.empty-text {
  font-size: 14px;
}
</style>
