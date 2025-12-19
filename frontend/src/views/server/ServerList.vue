<script setup lang="ts">
import { ref, reactive, onMounted, h, computed, nextTick, watch } from 'vue';
import { useRouter } from 'vue-router';
import { message, Modal, Tag } from 'ant-design-vue';
import { PlusOutlined, EyeOutlined, EditOutlined, DeleteOutlined, KeyOutlined, CopyOutlined, HolderOutlined, SaveOutlined, CloseOutlined } from '@ant-design/icons-vue';
import request from '../../utils/request';
import Sortable from 'sortablejs';
// 导入服务器状态Badge组件和store
import ServerStatusBadge from '../../components/ServerStatusBadge.vue';
import DeployAgentModal from '../../components/DeployAgentModal.vue';
import { useServerStore } from '../../stores/serverStore';

import { ReloadOutlined } from '@ant-design/icons-vue';

const router = useRouter();
// 获取服务器状态store
const serverStore = useServerStore();

// 数据状态
const loading = ref(false);
// 使用计算属性从store获取服务器列表
const servers = computed(() => serverStore.getAllServers);

// 排序模式相关状态
const sortMode = ref(false); // 是否处于排序模式
const sortableInstance = ref<Sortable | null>(null); // Sortable 实例
const localServerOrder = ref<any[]>([]); // 本地服务器列表副本（用于拖拽）
const savingOrder = ref(false); // 保存中状态
const tableRef = ref<any>(null); // 表格引用
const normalPagination = { pageSize: 10 };
const tablePagination = computed(() => (sortMode.value ? false : normalPagination));

// 表单状态
const formVisible = ref(false);
const formLoading = ref(false);
const formMode = ref<'create' | 'edit'>('create');
const formTitle = ref('添加服务器');
const formRef = ref();
const formState = reactive({
  id: null,
  name: '',
  description: '',
});

// 部署 Agent 弹窗状态
const deployModalVisible = ref(false);
const currentDeployServer = ref<any>(null);
const agentReleaseRepo = ref('EnderKC/BetterMonitor');

// 表单验证规则
const rules = {
  name: [
    { required: true, message: '请输入服务器名称', trigger: 'blur' },
  ],
};

const normalizeIpForDisplay = (raw: string) => {
  const trimmed = raw.trim();
  // WebSocket RemoteAddr 可能会带上 IPv6 方括号，如 "[2001:db8::1]"
  if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
    return trimmed.slice(1, -1).trim();
  }
  return trimmed;
};

const splitIpList = (value: unknown): string[] => {
  if (typeof value !== 'string') return [];
  return value
    .split(/[,\s]+/)
    .map((ip) => normalizeIpForDisplay(ip))
    .filter((ip) => ip !== '');
};

// 定义表格列
const columns = [
  { title: '', key: 'dragHandle', width: 50 }, // 拖拽手柄列
  { title: '名称', dataIndex: 'name', key: 'name' },
  {
    title: '出口IP',
    key: 'ip',
    customRender: ({ record }: { record: any }) => {
      const tags = [];
      const seen = new Set<string>();

      // 仅显示出口/公网 IP（public_ip）；如果同时存在 IPv4 和 IPv6，会以多个 Tag 展示
      splitIpList(record.public_ip).forEach((ip) => {
        const key = ip.toLowerCase();
        if (seen.has(key)) return;
        seen.add(key);
        tags.push(h(Tag, { color: 'blue' }, () => ip));
      });
      if (tags.length === 0) return '-';

      return h('div', { style: 'display: flex; flex-direction: column; gap: 4px; align-items: flex-start;' }, tags);
    }
  },

  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    customRender: ({ record }: { record: any }) => {
      return h(ServerStatusBadge, { serverId: record.id });
    }
  },
  {
    title: '最后更新',
    dataIndex: 'lastUpdate',
    key: 'lastUpdate',
    customRender: ({ text }: { text: number }) => text ? new Date(text).toLocaleString() : '未知'
  },
  { title: '操作', key: 'action' }
];

// 获取服务器列表
const fetchServers = async (force = false) => {
  loading.value = true;
  try {
    await serverStore.fetchServers(force);
  } catch (error) {
    console.error('获取服务器列表失败:', error);
    message.error('获取服务器列表失败');
  } finally {
    loading.value = false;
  }
};

// 强制刷新
const handleRefresh = () => {
  fetchServers(true);
  message.success('已刷新服务器列表');
};

// 打开添加服务器表单
const showAddForm = () => {
  formMode.value = 'create';
  formTitle.value = '添加服务器';
  resetForm();
  formVisible.value = true;
};

// 打开编辑服务器表单
const showEditForm = (record: any) => {
  formMode.value = 'edit';
  formTitle.value = '编辑服务器';

  formState.id = record.id;
  formState.name = record.name;
  formState.description = record.notes || '';

  formVisible.value = true;
};

// 重置表单
const resetForm = () => {
  formState.id = null;
  formState.name = '';
  formState.description = '';
};

// 关闭表单
const handleCancel = () => {
  formVisible.value = false;
  resetForm();
};

// 提交表单
const handleSubmit = () => {
  formRef.value.validate().then(async () => {
    formLoading.value = true;

    try {
      let response;
      if (formMode.value === 'create') {
        // 创建服务器
        response = await request.post('/servers', {
          name: formState.name,
          notes: formState.description
        });
        message.success('服务器添加成功');

        // 显示服务器令牌
        console.log('创建服务器响应:', response);
        if (response && response.server) {
          viewToken(response.server);
        }
      } else {
        // 更新服务器
        await request.put(`/servers/${formState.id}/update`, {
          name: formState.name,
          notes: formState.description
        });
        message.success('服务器更新成功');
      }

      // 刷新列表
      fetchServers();
      // 关闭表单
      formVisible.value = false;
      resetForm();
    } catch (error) {
      console.error('保存服务器失败:', error);
      message.error('保存服务器失败');
    } finally {
      formLoading.value = false;
    }
  });
};

// 删除服务器
const handleDelete = (id: number) => {
  Modal.confirm({
    title: '确认删除',
    content: '确定要删除这个服务器吗？该操作不可恢复。',
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await serverStore.deleteServer(id);
        message.success('服务器已删除');
        // 不需要 fetchServers()，因为 store 已经更新了
      } catch (error) {
        console.error('删除服务器失败:', error);
        message.error('删除服务器失败');
      }
    },
  });
};

// 查看服务器详情
const viewServer = (id: number) => {
  router.push(`/admin/servers/${id}`);
};

// 查看服务器令牌
const viewToken = async (server: any) => {
  currentDeployServer.value = server;

  try {
    const res = await request.get('/public/settings');
    if (res?.agent_release_repo) {
      agentReleaseRepo.value = res.agent_release_repo;
    }
  } catch (err) {
    console.error('获取设置失败:', err);
    // 即使获取失败也显示弹窗，使用默认值
  }

  deployModalVisible.value = true;
};

// 进入排序模式
const enterSortMode = () => {
  sortMode.value = true;
  // 复制当前服务器列表到本地状态
  localServerOrder.value = [...servers.value];

  // 在下一个 tick 初始化 Sortable
  nextTick(() => {
    const tableBody = document.querySelector('.ant-table-tbody') as HTMLElement;
    if (tableBody && !sortableInstance.value) {
      sortableInstance.value = new Sortable(tableBody, {
        animation: 150,
        handle: '.drag-handle', // 只能通过拖拽手柄拖动
        ghostClass: 'sortable-ghost',
        onEnd: (evt: any) => {
          // 更新本地顺序
          const oldIndex = evt.oldIndex;
          const newIndex = evt.newIndex;
          if (oldIndex !== undefined && newIndex !== undefined) {
            const movedItem = localServerOrder.value.splice(oldIndex, 1)[0];
            localServerOrder.value.splice(newIndex, 0, movedItem);
          }
        },
      });
    }
  });
};

// 退出排序模式
const exitSortMode = () => {
  sortMode.value = false;
  // 销毁 Sortable 实例
  if (sortableInstance.value) {
    sortableInstance.value.destroy();
    sortableInstance.value = null;
  }
  // 重置本地服务器列表
  localServerOrder.value = [];
};

// 保存排序
const saveOrder = async () => {
  savingOrder.value = true;
  try {
    // 提取服务器 ID 列表
    const orderedIds = localServerOrder.value.map((server: any) => server.id);

    // 调用 store 的 reorderServers 方法
    await serverStore.reorderServers(orderedIds);

    message.success('服务器顺序已更新');

    // 退出排序模式并刷新列表
    exitSortMode();
    await fetchServers(true);
  } catch (error) {
    console.error('保存服务器顺序失败:', error);
    message.error('保存服务器顺序失败');
  } finally {
    savingOrder.value = false;
  }
};

// 取消排序
const cancelSort = () => {
  Modal.confirm({
    title: '确认取消',
    content: '确定要取消排序吗？所有未保存的更改将丢失。',
    okText: '确认',
    cancelText: '取消',
    onOk: () => {
      exitSortMode();
      message.info('已取消排序');
    },
  });
};

// 当前显示的服务器列表（排序模式下使用本地列表，否则使用 store 的列表）
const displayServers = computed(() => {
  return sortMode.value ? localServerOrder.value : servers.value;
});

// 页面加载时获取数据
onMounted(() => {
  fetchServers();
});
</script>

<template>
  <div class="server-list-container">
    <div class="page-header glass-card">
      <div class="header-title">
        <h2 class="gradient-text">服务器管理</h2>
        <p>管理您的所有实例</p>
      </div>

      <div class="header-actions">
        <!-- 排序模式控制按钮 -->
        <template v-if="!sortMode">
          <a-button @click="handleRefresh" class="glow-effect" style="margin-right: 8px;">
            <template #icon>
              <ReloadOutlined />
            </template>
            刷新
          </a-button>
          <a-button @click="enterSortMode" class="glow-effect" style="margin-right: 8px;" v-if="servers.length > 0">
            <template #icon>
              <HolderOutlined />
            </template>
            调整顺序
          </a-button>
          <a-button type="primary" @click="showAddForm" class="glow-effect">
            <template #icon>
              <PlusOutlined />
            </template>
            添加服务器
          </a-button>
        </template>
        <template v-else>
          <a-button type="primary" @click="saveOrder" :loading="savingOrder" class="glow-effect"
            style="margin-right: 8px;">
            <template #icon>
              <SaveOutlined />
            </template>
            保存顺序
          </a-button>
          <a-button @click="cancelSort" :disabled="savingOrder">
            <template #icon>
              <CloseOutlined />
            </template>
            取消
          </a-button>
        </template>
      </div>
    </div>

    <div class="server-list-content glass-card">
      <a-table :dataSource="displayServers" :columns="columns" :loading="loading" :pagination="tablePagination"
        rowKey="id" class="modern-table" ref="tableRef">
        <template #bodyCell="{ column, record }">
          <!-- 拖拽手柄列 -->
          <template v-if="column.key === 'dragHandle'">
            <div class="drag-handle" v-if="sortMode" style="cursor: move;">
              <HolderOutlined style="font-size: 18px; color: #999;" />
            </div>
          </template>

          <template v-if="column.key === 'action'">
            <div class="action-buttons" v-if="!sortMode">
              <a-button type="primary" size="small" @click="viewServer(record.id)" class="action-btn glow-effect">
                <template #icon>
                  <EyeOutlined />
                </template>
                查看
              </a-button>

              <a-button type="default" size="small" @click="viewToken(record)" class="action-btn">
                <template #icon>
                  <KeyOutlined />
                </template>
                令牌
              </a-button>

              <a-button type="default" size="small" @click="showEditForm(record)" class="action-btn">
                <template #icon>
                  <EditOutlined />
                </template>
                编辑
              </a-button>

              <a-button type="primary" danger size="small" @click="handleDelete(record.id)"
                class="action-btn glow-effect-error">
                <template #icon>
                  <DeleteOutlined />
                </template>
                删除
              </a-button>
            </div>
            <div v-else style="color: #999; font-size: 12px;">
              拖动行调整顺序
            </div>
          </template>
        </template>

        <template #emptyText>
          <div class="empty-content">
            <a-empty description="暂无服务器" />
            <a-button type="primary" @click="showAddForm" class="glow-effect">添加第一台服务器</a-button>
          </div>
        </template>
      </a-table>
    </div>

    <!-- 添加/编辑服务器表单 -->
    <a-modal :visible="formVisible" :title="formTitle" :confirmLoading="formLoading" @cancel="handleCancel"
      @ok="handleSubmit" okText="保存" cancelText="取消" width="500px" class="glass-modal">
      <a-form :model="formState" :rules="rules" ref="formRef" layout="vertical">
        <a-form-item name="name" label="服务器名称">
          <a-input v-model:value="formState.name" placeholder="输入一个易于识别的名称" :maxLength="50" />
        </a-form-item>

        <a-form-item name="description" label="备注">
          <a-textarea v-model:value="formState.description" placeholder="可选的服务器描述" :rows="4" :maxLength="500" />
        </a-form-item>
      </a-form>
    </a-modal>
    <!-- 部署 Agent 弹窗 -->
    <DeployAgentModal v-model:visible="deployModalVisible" :server="currentDeployServer"
      :agent-release-repo="agentReleaseRepo" />
  </div>
</template>

<style scoped>
.server-list-container {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  margin-bottom: 16px;

}

.header-title h2 {
  font-size: 24px;
  font-weight: 700;
  margin-bottom: 8px;
}

.header-title p {
  color: var(--text-secondary);
  margin: 0;
}

.server-list-content {
  padding: 16px;
}

.modern-table {
  width: 100%;
}

.modern-table :deep(.ant-table-thead > tr > th) {
  font-weight: 600;
}

.action-buttons {
  display: flex;
  gap: 8px;
}

.action-btn {
  transition: var(--transition);
}

.action-btn:hover {
  transform: translateY(-2px);
}

.empty-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px 0;
}

.empty-content button {
  margin-top: 16px;
}

/* 排序拖拽相关样式 */
.sortable-ghost {
  opacity: 0.4;
  background: var(--primary-color);
}

.drag-handle {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 8px;
}

.drag-handle:hover {
  color: var(--primary-color);
}

:deep(.glass-modal .ant-modal-content) {
  background: var(--card-bg);
  backdrop-filter: blur(15px);
  -webkit-backdrop-filter: blur(15px);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  border: 1px solid var(--card-border);
}



/* 响应式调整 */
@media (max-width: 768px) {
  .server-list-container {
    padding: 0;
  }

  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
    padding: 16px;
  }

  .server-list-content {
    padding: 16px;
  }

  .action-buttons {
    flex-wrap: wrap;
  }
}
</style>

<style>
.dark .modern-table .ant-table-thead>tr>th {
  background: #2d2d2d;
  color: #ccc;
  border-bottom: 1px solid #333;
}

.dark .modern-table .ant-table-tbody>tr>td {
  border-bottom: 1px solid #333;
  color: #ccc;
}

.dark .modern-table .ant-table-tbody>tr:hover>td {
  background: #2a2d2e !important;
}

.dark .glass-modal .ant-modal-content {
  background: #252526;
  border: 1px solid #333;
  color: #e0e0e0;
}

.dark .glass-modal .ant-modal-header {
  background: #252526;
  border-bottom: 1px solid #333;
}

.dark .glass-modal .ant-modal-title {
  color: #e0e0e0;
}

.dark .glass-modal .ant-modal-close {
  color: #ccc;
}
</style>
