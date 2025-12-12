<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue';
import { message, Modal } from 'ant-design-vue';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  EyeOutlined,
  ReloadOutlined
} from '@ant-design/icons-vue';
import request from '@/utils/request';
import LifeProbeDetailModal from '@/components/LifeProbeDetailModal.vue';
import type { LifeProbeSummary } from '@/types/life';
import { getToken } from '@/utils/auth';

const loading = ref(true);
const probes = ref<LifeProbeSummary[]>([]);

const modalVisible = ref(false);
const submitting = ref(false);
const editingId = ref<number | null>(null);
const detailVisible = ref(false);
const selectedProbeId = ref<number | null>(null);
const lifeWS = ref<WebSocket | null>(null);
const lifeHeartbeatTimer = ref<number | null>(null);
const lifeReconnectTimer = ref<number | null>(null);

const form = reactive({
  name: '',
  device_id: '',
  description: '',
  tags: '',
  allow_public_view: true
});

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '设备ID', dataIndex: 'device_id', key: 'device_id' },
  { title: '今日步数', dataIndex: 'steps_today', key: 'steps_today' },
  { title: '当前心率', dataIndex: 'latest_heart_rate', key: 'latest_heart_rate' },
  { title: '专注状态', dataIndex: 'focus_event', key: 'focus_event' },
  { title: '最后同步', dataIndex: 'last_sync_at', key: 'last_sync_at' },
  { title: '操作', key: 'action' }
];

const clearLifeHeartbeat = () => {
  if (lifeHeartbeatTimer.value !== null) {
    clearInterval(lifeHeartbeatTimer.value);
    lifeHeartbeatTimer.value = null;
  }
};

const cleanupLifeWS = () => {
  clearLifeHeartbeat();
  if (lifeReconnectTimer.value !== null) {
    clearTimeout(lifeReconnectTimer.value);
    lifeReconnectTimer.value = null;
  }
  if (lifeWS.value) {
    lifeWS.value.onclose = null;
    lifeWS.value.close();
    lifeWS.value = null;
  }
};

const connectLifeProbeListWS = () => {
  if (
    lifeWS.value &&
    (lifeWS.value.readyState === WebSocket.OPEN || lifeWS.value.readyState === WebSocket.CONNECTING)
  ) {
    return;
  }

  const token = getToken();
  if (!token) {
    message.error('请先登录');
    loading.value = false;
    return;
  }

  if (lifeReconnectTimer.value !== null) {
    clearTimeout(lifeReconnectTimer.value);
    lifeReconnectTimer.value = null;
  }

  loading.value = true;

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/api/life-probes/public/ws?token=${encodeURIComponent(token)}`;
  const ws = new WebSocket(wsUrl);
  lifeWS.value = ws;

  ws.onopen = () => {
    clearLifeHeartbeat();
    lifeHeartbeatTimer.value = window.setInterval(() => {
      if (lifeWS.value && lifeWS.value.readyState === WebSocket.OPEN) {
        try {
          lifeWS.value.send(JSON.stringify({ type: 'heartbeat', timestamp: Date.now() }));
        } catch (error) {
          console.error('生命探针列表心跳失败:', error);
        }
      } else {
        clearLifeHeartbeat();
      }
    }, 25000);
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'life_probe_list' && Array.isArray(data.life_probes)) {
        probes.value = data.life_probes;
        loading.value = false;
      }
    } catch (error) {
      console.error('解析生命探针列表数据失败:', error);
    }
  };

  ws.onerror = (error) => {
    console.error('生命探针列表WebSocket错误:', error);
    message.error('生命探针列表连接失败');
    loading.value = false;
  };

  ws.onclose = () => {
    clearLifeHeartbeat();
    lifeWS.value = null;
    if (lifeReconnectTimer.value !== null) {
      clearTimeout(lifeReconnectTimer.value);
    }
    lifeReconnectTimer.value = window.setTimeout(() => {
      lifeReconnectTimer.value = null;
      connectLifeProbeListWS();
    }, 5000);
  };
};

onMounted(() => {
  connectLifeProbeListWS();
});

onUnmounted(() => {
  cleanupLifeWS();
});

const openCreateModal = () => {
  editingId.value = null;
  Object.assign(form, {
    name: '',
    device_id: '',
    description: '',
    tags: '',
    allow_public_view: true
  });
  modalVisible.value = true;
};

const openEditModal = (probe: LifeProbeSummary) => {
  editingId.value = probe.id;
  Object.assign(form, {
    name: probe.name,
    device_id: probe.device_id,
    description: probe.description || '',
    tags: probe.tags || '',
    allow_public_view: probe.allow_public_view
  });
  modalVisible.value = true;
};

const handleSubmit = async () => {
  if (!form.name) {
    message.error('请输入探针名称');
    return;
  }
  if (!form.device_id) {
    message.error('请输入设备ID');
    return;
  }

  submitting.value = true;
  const payload = {
    name: form.name,
    device_id: form.device_id,
    description: form.description,
    tags: form.tags,
    allow_public_view: form.allow_public_view
  };

  try {
    if (editingId.value) {
      await request.put(`/life-probes/${editingId.value}`, payload);
      message.success('生命探针已更新');
    } else {
      await request.post('/life-probes', payload);
      message.success('生命探针已创建');
    }
    modalVisible.value = false;
  } catch (error) {
    console.error('保存生命探针失败:', error);
  } finally {
    submitting.value = false;
  }
};

const handleDelete = (probe: LifeProbeSummary) => {
  Modal.confirm({
    title: '删除生命探针',
    content: `确认删除 ${probe.name} 吗？该操作将清空该探针的所有监控数据。`,
    okType: 'danger',
    okText: '删除',
    cancelText: '取消',
    async onOk() {
      try {
        await request.delete(`/life-probes/${probe.id}`);
        message.success('生命探针已删除');
      } catch (error) {
        console.error('删除生命探针失败:', error);
        message.error('删除生命探针失败');
      }
    }
  });
};

const handleRefresh = () => {
  cleanupLifeWS();
  connectLifeProbeListWS();
  message.success('已刷新生命探针列表');
};

const openDetail = (probeId: number) => {
  selectedProbeId.value = probeId;
  detailVisible.value = true;
};

const totalSteps = computed(() =>
  probes.value.reduce((sum, probe) => sum + (probe.steps_today || 0), 0)
);

const focusBadge = (probe: LifeProbeSummary) => {
  if (!probe.focus_event) return '未上报';
  return probe.focus_event.is_focused ? '专注中' : '普通模式';
};
</script>

<template>
  <div class="life-page">
    <div class="page-header">
      <div>
        <h1>生命探针</h1>
        <p>统一管理 LifeLogger 设备，查看数据接入情况</p>
      </div>
      <div class="actions">
        <a-button @click="handleRefresh">
          <template #icon>
            <ReloadOutlined />
          </template>
          刷新
        </a-button>
        <a-button type="primary" @click="openCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          添加生命探针
        </a-button>
      </div>
    </div>

    <div class="stats-bar ">
      <div class="stat-item">
        <p>探针数量</p>
        <h3>{{ probes.length }}</h3>
      </div>
      <div class="stat-item">
        <p>今日累计步数</p>
        <h3>{{ Math.round(totalSteps).toLocaleString() }}</h3>
      </div>
    </div>

    <a-table :data-source="probes" :loading="loading" :columns="columns" row-key="id" class="life-table glass-card">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'latest_heart_rate'">
          {{ record.latest_heart_rate ? record.latest_heart_rate.value : '--' }}
        </template>
        <template v-else-if="column.key === 'focus_event'">
          <a-tag :color="record.focus_event?.is_focused ? 'green' : 'blue'">{{ focusBadge(record) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'last_sync_at'">
          {{ record.last_sync_at ? new Date(record.last_sync_at).toLocaleString() : '--' }}
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-tooltip title="查看详情">
              <a-button size="small" @click="openDetail(record.id)">
                <EyeOutlined />
              </a-button>
            </a-tooltip>
            <a-tooltip title="编辑">
              <a-button size="small" @click="openEditModal(record)">
                <EditOutlined />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button size="small" danger @click="handleDelete(record)">
                <DeleteOutlined />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
        <template v-else>
          {{ record[column.dataIndex] }}
        </template>
      </template>
    </a-table>

    <a-modal :open="modalVisible" :title="editingId ? '编辑生命探针' : '添加生命探针'" :confirm-loading="submitting"
      @cancel="modalVisible = false" @ok="handleSubmit">
      <a-form layout="vertical">
        <a-form-item label="探针名称" required>
          <a-input v-model:value="form.name" placeholder="例如：王小明 · iPhone" />
        </a-form-item>
        <a-form-item label="设备ID" required>
          <a-input v-model:value="form.device_id" placeholder="LifeLogger 客户端中的 Device ID" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="form.description" rows="3" />
        </a-form-item>
        <a-form-item label="标签">
          <a-input v-model:value="form.tags" placeholder="例如：家人, Apple Watch" />
        </a-form-item>
        <a-form-item>
          <a-switch v-model:checked="form.allow_public_view" /> <span class="switch-label">允许在探针页面展示</span>
        </a-form-item>
      </a-form>
    </a-modal>

    <LifeProbeDetailModal v-model="detailVisible" :probe-id="selectedProbeId" />
  </div>
</template>

<style scoped>
.life-page {
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 0;
  font-size: 26px;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.85);
}

.page-header p {
  margin: 4px 0 0;
  color: rgba(0, 0, 0, 0.45);
}

.actions {
  display: flex;
  gap: 12px;
}

.stats-bar {
  display: flex;
  gap: 16px;
  margin-bottom: 24px;
}

.stat-item {
  flex: 1;
  background: var(--card-bg) !important;
  backdrop-filter: blur(20px) saturate(180%);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  border: 1px solid rgba(255, 255, 255, 0.5);
  border-radius: 20px;
  padding: 20px;
  box-shadow: 0 8px 32px 0 rgba(31, 38, 135, 0.07);
  transition: all 0.3s ease;
}

.stat-item:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 40px 0 rgba(31, 38, 135, 0.15);
}

.stat-item p {
  margin: 0 0 6px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 14px;
}

.stat-item h3 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.85);
}

/* Table Styling */
.life-table {
  background: var(--card-bg) !important;
  backdrop-filter: blur(20px) saturate(180%);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  border-radius: 20px;
  padding: 16px;
  box-shadow: 0 8px 32px 0 rgba(31, 38, 135, 0.07);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.life-table :deep(.ant-table) {
  background: transparent;
}

.life-table :deep(.ant-table-thead > tr > th) {
  background: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  font-weight: 600;
}

.life-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.life-table :deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(255, 255, 255, 0.4) !important;
}

.life-table :deep(.ant-pagination-item-active) {
  background: transparent;
  border-color: #1890ff;
}

.life-table :deep(.ant-pagination-item a) {
  color: rgba(0, 0, 0, 0.85);
}

.switch-label {
  margin-left: 8px;
  color: rgba(0, 0, 0, 0.45);
}


</style>

<style>
.dark .page-header h1 {
  color: rgba(255, 255, 255, 0.85);
}

.dark .page-header p {
  color: rgba(255, 255, 255, 0.45);
}

.dark .stat-item {
  background: var(--card-bg) !important;
  border-color: rgba(255, 255, 255, 0.1);
  box-shadow: 0 8px 32px 0 rgba(0, 0, 0, 0.2);
}

.dark .stat-item:hover {
  box-shadow: 0 12px 40px 0 rgba(0, 0, 0, 0.3);
}

.dark .stat-item p {
  color: rgba(255, 255, 255, 0.45);
}

.dark .stat-item h3 {
  color: rgba(255, 255, 255, 0.85);
}

.dark .life-table {
  background: var(--card-bg) !important;
  border-color: rgba(255, 255, 255, 0.1);
  box-shadow: 0 8px 32px 0 rgba(0, 0, 0, 0.2);
}

.dark .life-table .ant-table-thead > tr > th {
  background: rgba(255, 255, 255, 0.05);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.85);
}

.dark .life-table .ant-table-tbody > tr > td {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.85);
}

.dark .life-table .ant-table-tbody > tr:hover > td {
  background: rgba(255, 255, 255, 0.08) !important;
}

.dark .life-table .ant-pagination-item-active {
  border-color: #177ddc;
}

.dark .life-table .ant-pagination-item a {
  color: rgba(255, 255, 255, 0.85);
}

.dark .switch-label {
  color: rgba(255, 255, 255, 0.45);
}
</style>
