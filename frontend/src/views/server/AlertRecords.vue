<template>
  <div class="alert-records-container">
    <a-card title="预警记录" :bordered="false">
      <a-row :gutter="16" style="margin-bottom: 16px">
        <a-col :span="6">
          <a-select 
            v-model:value="filters.server_id"
            style="width: 100%"
            placeholder="选择服务器"
            allowClear
            @change="handleFilterChange"
          >
            <a-select-option :value="0">全部服务器</a-select-option>
            <a-select-option v-for="server in servers" :key="server.id" :value="server.id">
              {{ server.name }}
            </a-select-option>
          </a-select>
        </a-col>
        <a-col :span="6">
          <a-select 
            v-model:value="filters.type"
            style="width: 100%"
            placeholder="预警类型"
            allowClear
            @change="handleFilterChange"
          >
            <a-select-option value="">全部类型</a-select-option>
            <a-select-option value="cpu">CPU 使用率</a-select-option>
            <a-select-option value="memory">内存使用率</a-select-option>
            <a-select-option value="network">网络流量</a-select-option>
            <a-select-option value="status">服务器状态</a-select-option>
          </a-select>
        </a-col>
        <a-col :span="6">
          <a-checkbox 
            v-model:checked="filters.unresolved"
            @change="handleFilterChange"
          >
            只显示未解决
          </a-checkbox>
        </a-col>
        <a-col :span="6" style="text-align: right">
          <a-button type="primary" @click="refreshRecords">刷新</a-button>
        </a-col>
      </a-row>

      <a-spin :spinning="loading.records">
        <a-table 
          :dataSource="alertRecords" 
          :columns="columns" 
          rowKey="id" 
          :pagination="paginationProps"
          @change="handleTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'alert_type'">
              <a-tag :color="getTypeColor(record.alert_type)">{{ getTypeName(record.alert_type) }}</a-tag>
            </template>
            <template v-if="column.key === 'value'">
              {{ getFormattedValue(record) }}
            </template>
            <template v-if="column.key === 'status'">
              <a-tag :color="record.resolved ? 'green' : 'red'">
                {{ record.resolved ? '已解决' : '未解决' }}
              </a-tag>
            </template>
            <template v-if="column.key === 'action'">
              <a-button 
                type="link" 
                size="small" 
                @click="resolveRecord(record.id)"
                :disabled="record.resolved"
              >
                标记为已解决
              </a-button>
            </template>
          </template>
        </a-table>
        <a-empty v-if="alertRecords.length === 0" description="暂无预警记录" />
      </a-spin>
    </a-card>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, computed, onMounted, reactive } from 'vue';
import { useAlertStore, useServerStore } from '@/stores';
import type { TablePaginationConfig } from 'ant-design-vue';

export default defineComponent({
  name: 'AlertRecords',
  
  setup() {
    const alertStore = useAlertStore();
    const serverStore = useServerStore();
    
    const filters = reactive({
      server_id: 0,
      type: '',
      unresolved: false,
      page: 1,
      limit: 10,
    });
    
    const columns = [
      {
        title: '服务器',
        dataIndex: 'server_name',
        key: 'server_name',
      },
      {
        title: '预警类型',
        dataIndex: 'alert_type',
        key: 'alert_type',
      },
      {
        title: '触发值',
        dataIndex: 'value',
        key: 'value',
      },
      {
        title: '阈值',
        dataIndex: 'threshold',
        key: 'threshold',
        render: (threshold: number, record: any) => getFormattedThreshold(record),
      },
      {
        title: '状态',
        key: 'status',
      },
      {
        title: '通知时间',
        dataIndex: 'notified_at',
        key: 'notified_at',
        render: (text: string) => new Date(text).toLocaleString(),
      },
      {
        title: '解决时间',
        key: 'resolved_at',
        render: (_, record: any) => record.resolved ? new Date(record.resolved_at).toLocaleString() : '-',
      },
      {
        title: '操作',
        key: 'action',
      },
    ];
    
    // 计算属性
    const loading = computed(() => alertStore.loading);
    const alertRecords = computed(() => alertStore.alertRecords);
    const servers = computed(() => serverStore.servers);
    const totalRecords = computed(() => alertStore.totalRecords);
    
    const paginationProps = computed(() => ({
      current: filters.page,
      pageSize: filters.limit,
      total: totalRecords.value,
      showSizeChanger: true,
      showTotal: (total: number) => `共 ${total} 条记录`,
    }));
    
    // 生命周期钩子
    onMounted(async () => {
      await serverStore.fetchServers();
      await fetchRecords();
    });
    
    // 方法
    const fetchRecords = async () => {
      await alertStore.fetchAlertRecords(filters);
    };
    
    const refreshRecords = () => {
      filters.page = 1; // 重置页码
      fetchRecords();
    };
    
    const handleFilterChange = () => {
      filters.page = 1; // 重置页码
      fetchRecords();
    };
    
    const handleTableChange = (pagination: TablePaginationConfig) => {
      filters.page = pagination.current || 1;
      filters.limit = pagination.pageSize || 10;
      fetchRecords();
    };
    
    const getTypeColor = (type: string) => {
      switch (type) {
        case 'cpu': return 'blue';
        case 'memory': return 'orange';
        case 'network': return 'green';
        case 'status': return 'purple';
        default: return 'default';
      }
    };
    
    const getTypeName = (type: string) => {
      switch (type) {
        case 'cpu': return 'CPU 使用率';
        case 'memory': return '内存使用率';
        case 'network': return '网络流量';
        case 'status': return '服务器状态';
        default: return type;
      }
    };
    
    const getFormattedValue = (record: any) => {
      switch (record.alert_type) {
        case 'cpu':
        case 'memory':
          return `${record.value.toFixed(2)}%`;
        case 'network':
          return `${record.value.toFixed(2)} MB/s`;
        case 'status':
          return record.value >= 1 ? '在线' : '离线';
        default:
          return record.value;
      }
    };
    
    const getFormattedThreshold = (record: any) => {
      switch (record.alert_type) {
        case 'cpu':
        case 'memory':
          return `${record.threshold}%`;
        case 'network':
          return `${record.threshold} MB/s`;
        case 'status':
          switch (record.threshold) {
            case 1: return '上线时';
            case 2: return '离线时';
            case 3: return '上下线';
            default: return record.threshold;
          }
        default:
          return record.threshold;
      }
    };
    
    const resolveRecord = async (id: number) => {
      try {
        await alertStore.resolveAlertRecord(id);
      } catch (error) {
        console.error('解决预警记录失败:', error);
      }
    };
    
    return {
      loading,
      filters,
      columns,
      alertRecords,
      servers,
      paginationProps,
      
      handleFilterChange,
      handleTableChange,
      refreshRecords,
      getTypeColor,
      getTypeName,
      getFormattedValue,
      getFormattedThreshold,
      resolveRecord,
    };
  },
});
</script>

<style scoped>
.alert-records-container {
  padding: 16px;
}
</style> 
