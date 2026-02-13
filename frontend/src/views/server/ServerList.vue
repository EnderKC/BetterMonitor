<script setup lang="ts">
defineOptions({
  name: 'ServerList'
});

import { ref, reactive, onMounted, h, computed, nextTick, watch, onActivated, onDeactivated } from 'vue';
import { useRouter } from 'vue-router';
import { message, Modal, Tag } from 'ant-design-vue';
import {
  PlusOutlined,
  EyeOutlined,
  EditOutlined,
  DeleteOutlined,
  KeyOutlined,
  CopyOutlined,
  HolderOutlined,
  SaveOutlined,
  CloseOutlined,
  MoreOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
import Sortable from 'sortablejs';
// 导入服务器状态Badge组件和store
import ServerStatusBadge from '../../components/ServerStatusBadge.vue';
import DeployAgentModal from '../../components/DeployAgentModal.vue';
import { useServerStore } from '../../stores/serverStore';
import { useUIStore } from '../../stores/uiStore';

import { ReloadOutlined } from '@ant-design/icons-vue';

const router = useRouter();
// 获取服务器状态store
const serverStore = useServerStore();
const uiStore = useUIStore();

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
  agent_type: 'full' as 'full' | 'monitor',
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
    title: '类型',
    key: 'agent_type',
    width: 90,
    customRender: ({ record }: { record: any }) => {
      const isMonitor = record.agent_type === 'monitor';
      return h(Tag, { color: isMonitor ? 'orange' : 'green' }, () => isMonitor ? '监控' : '全功能');
    }
  },
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
const fetchServers = async (force = false, showLoading = true) => {
  if (showLoading) {
    loading.value = true;
  }
  try {
    await serverStore.fetchServers(force);
  } catch (error) {
    console.error('获取服务器列表失败:', error);
    message.error('获取服务器列表失败');
  } finally {
    loading.value = false;
    uiStore.stopLoading();
  }
};

// 强制刷新
const handleRefresh = () => {
  fetchServers(true, true);
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
  formState.agent_type = record.agent_type || 'full'; // Fill current agent type

  formVisible.value = true;
};

// 重置表单
const resetForm = () => {
  formState.id = null;
  formState.name = '';
  formState.description = '';
  formState.agent_type = 'full';
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
          notes: formState.description,
          agent_type: formState.agent_type,
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

        // Check if agent type changed
        // We need to find the original record to compare, or trust formState was init correctly
        const originalServer = servers.value.find((s: any) => s.id === formState.id);
        if (originalServer && originalServer.agent_type !== formState.agent_type) {
          try {
            const switchRes = await request.post(`/servers/${formState.id}/switch-agent-type`, {
              target_agent_type: formState.agent_type
            });
            if (switchRes && switchRes.message) {
              message.success(switchRes.message);
            }
          } catch (switchErr) {
            console.error('切换 Agent 类型失败:', switchErr);
            message.error('基本信息已更新，但切换 Agent 类型失败');
          }
        } else {
          message.success('服务器更新成功');
        }
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

// 页面激活时静默刷新
onActivated(() => {
  fetchServers(false, false);
});

onDeactivated(() => {
  if (sortMode.value) {
    exitSortMode();
  }
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

              <a-dropdown :trigger="['click']">
                <a-button type="text" size="small" class="action-btn more-btn">
                  <MoreOutlined />
                </a-button>
                <template #overlay>
                  <a-menu class="action-menu glass-card">
                    <a-menu-item key="token" @click="viewToken(record)">
                      <KeyOutlined /> 令牌
                    </a-menu-item>
                    <a-menu-item key="edit" @click="showEditForm(record)">
                      <EditOutlined /> 编辑
                    </a-menu-item>
                    <a-menu-divider />
                    <a-menu-item key="delete" @click="handleDelete(record.id)" danger>
                      <DeleteOutlined /> 删除
                    </a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
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
    <a-modal :visible="formVisible" :title="null" :footer="null" :width="480" @cancel="handleCancel"
      class="glass-modal apple-modal">
      <div class="modal-header">
        <h3>{{ formTitle }}</h3>
        <p>配置您的服务器实例信息</p>
      </div>

      <a-form :model="formState" :rules="rules" ref="formRef" layout="vertical" class="apple-form">
        <a-form-item name="name" label="服务器名称">
          <a-input v-model:value="formState.name" placeholder="例如：生产环境-Web01" :maxLength="50" class="apple-input" />
        </a-form-item>

        <a-form-item name="description" label="备注">
          <a-textarea v-model:value="formState.description" placeholder="可选的服务器描述" :rows="3" :maxLength="500"
            class="apple-input" />
        </a-form-item>

        <a-form-item name="agent_type" label="Agent 类型">
          <div class="agent-type-selector">
            <div class="type-option" :class="{ active: formState.agent_type === 'full' }"
              @click="formState.agent_type = 'full'">
              <div class="type-header">
                <div class="radio-circle"></div>
                <span class="type-title">全功能版</span>
              </div>
              <div class="type-desc">包含监控、终端、文件管理、Docker、Nginx 等所有功能</div>
            </div>

            <div class="type-option" :class="{ active: formState.agent_type === 'monitor' }"
              @click="formState.agent_type = 'monitor'">
              <div class="type-header">
                <div class="radio-circle"></div>
                <span class="type-title">最小监控版</span>
              </div>
              <div class="type-desc">仅包含系统监控功能，轻量级运行</div>
            </div>
          </div>

          <div v-if="formMode === 'edit'" class="warning-box">
            <span class="warning-icon">⚠️</span>
            <div class="warning-content">
              <span class="warning-title">注意</span>
              <span>切换类型可能需要 Agent 在线以触发自动更新。如果 Agent 离线，请在上线后手动更新。</span>
            </div>
          </div>
        </a-form-item>

        <div class="modal-footer">
          <a-button @click="handleCancel" class="cancel-btn">取消</a-button>
          <a-button type="primary" :loading="formLoading" @click="handleSubmit" class="save-btn">保存</a-button>
        </div>
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
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-bold);
  margin-bottom: 8px;
}

.header-title p {
  color: var(--text-secondary);
  margin: 0;
}

.server-list-content {
  padding: 16px;
}

.modern-table :deep(.ant-table-wrapper),
.modern-table :deep(.ant-spin-nested-loading),
.modern-table :deep(.ant-spin-container),
.modern-table :deep(.ant-table) {
  background: transparent;
}

.modern-table :deep(.ant-table-thead > tr > th) {
  background: transparent;
  color: var(--text-secondary);
  font-weight: 500;
  border-bottom: 1px solid var(--border-subtle);
  padding: 16px 24px;
  /* Increased padding */
}

/* Row Styling */
.modern-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid var(--border-subtle);
  padding: 20px 24px;
  /* Increased padding for card-like feel */
  background: transparent;
  transition: background 0.3s ease;
}

.modern-table :deep(.ant-table-tbody > tr:hover > td) {
  background: var(--alpha-black-03);
}

.action-buttons {
  display: flex;
  gap: 8px;
  align-items: center;
}

.action-btn {
  border-radius: 8px;
  /* Rounded corners for buttons */
}

.more-btn {
  color: var(--text-secondary);
  font-size: 16px;
  padding: 0 4px;
}

.more-btn:hover {
  color: var(--text-primary);
  background-color: var(--alpha-black-05);
}

/* Dropdown Menu */
:deep(.action-menu) {
  border: 1px solid var(--border-subtle);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  padding: 4px;
  border-radius: 8px;
}

:deep(.action-menu .ant-dropdown-menu-item) {
  border-radius: 6px;
  padding: 6px 12px;
}

:deep(.action-menu .ant-dropdown-menu-item-danger:hover) {
  background-color: var(--error-bg);
  color: var(--error-color);
}

/* Apple-style Modal & Form */
.apple-modal .ant-modal-content {
  padding: 0;
  border-radius: 20px;
  overflow: hidden;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
}

/* Ensure body has no padding so we handle it */
.apple-modal .ant-modal-body {
  padding: 0;
}

/* Fix close button position */
.apple-modal .ant-modal-close {
  top: 16px;
  right: 16px;
  color: var(--text-secondary);
  transition: all 0.2s;
  background: transparent;
  border-radius: 50%;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.apple-modal .ant-modal-close:hover {
  background: var(--alpha-black-05);
  color: var(--text-primary);
}

.dark .apple-modal .ant-modal-close:hover {
  background: rgba(255, 255, 255, 0.1);
  color: var(--text-primary);
}

.modal-header {
  padding: 24px 24px 16px;
  background: var(--card-bg);
  /* Use theme var */
  /* Remove explicit border-bottom to let it blend or use a very subtle one */
}

.modal-header h3 {
  margin: 0 0 4px;
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
}

.modal-header p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.apple-form {
  padding: 0 24px 24px;
}

.apple-input {
  background: var(--input-bg);
  border: 1px solid transparent;
  border-radius: 10px;
  padding: 8px 12px;
  transition: all 0.2s ease;
  box-shadow: none !important;
}

.apple-input:hover {
  background: var(--input-bg);
  /* Keep same on hover, maybe slightly darker */
}

.apple-input:focus {
  background: var(--input-focus-bg);
  /* White or lighter */
  border-color: var(--primary-color);
  box-shadow: 0 0 0 3px var(--primary-light) !important;
}

/* Agent Type Selector */
.agent-type-selector {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.type-option {
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
  background: var(--card-bg);
}

.type-option:hover {
  border-color: var(--primary-color);
  background: var(--alpha-black-02);
}

.type-option.active {
  border-color: var(--primary-color);
  background: var(--primary-light);
}

.type-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.radio-circle {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid var(--text-hint);
  position: relative;
  transition: all 0.2s;
}

.type-option.active .radio-circle {
  border-color: var(--primary-color);
  background: var(--primary-color);
}

.type-option.active .radio-circle::after {
  content: '';
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 6px;
  height: 6px;
  background: white;
  border-radius: 50%;
}

.type-title {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 14px;
}

.type-desc {
  font-size: 12px;
  color: var(--text-secondary);
  margin-left: 24px;
  /* Align with text, skipping radio */
  line-height: 1.4;
}

/* Warning Box */
.warning-box {
  margin-top: 12px;
  background: var(--warning-bg);
  border-radius: 10px;
  padding: 10px 12px;
  display: flex;
  gap: 10px;
  border: 1px solid rgba(255, 149, 0, 0.2);
}

.warning-icon {
  font-size: 16px;
}

.warning-content {
  display: flex;
  flex-direction: column;
}

.warning-title {
  font-weight: 600;
  font-size: 12px;
  color: var(--warning-color);
  margin-bottom: 2px;
}

.warning-content span:not(.warning-title) {
  font-size: 12px;
  color: var(--text-secondary);
  opacity: 0.9;
}

/* Modal Footer */
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 24px;
}

.modal-footer button {
  height: 36px;
  padding: 0 20px;
  border-radius: 8px;
  font-weight: 500;
}

.cancel-btn {
  border: none;
  background: transparent;
  color: var(--text-secondary);
  box-shadow: none;
}

.cancel-btn:hover {
  color: var(--text-primary);
  background: var(--alpha-black-05);
}

.save-btn {
  background: var(--primary-color);
  box-shadow: 0 4px 12px var(--primary-light);
  border: none;
}

.save-btn:hover {
  background: var(--primary-hover);
  transform: translateY(-1px);
}
</style>

<style>
/* Dark Mode Overrides - targeted specifically */
.dark .modern-table :deep(.ant-table-thead > tr > th) {
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.dark .modern-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid var(--border-subtle);
}

.dark .modern-table :deep(.ant-table-tbody > tr:hover > td) {
  background: var(--alpha-white-05) !important;
}

.dark :deep(.glass-modal .ant-modal-content) {
  background: var(--card-bg);
  border: 1px solid var(--border-subtle);
}

.dark :deep(.glass-modal .ant-modal-header) {
  background: transparent;
  border-bottom: 1px solid var(--border-subtle);
}

.dark .more-btn:hover {
  background-color: var(--alpha-white-08);
  color: var(--text-primary);
}

.dark :deep(.action-menu) {
  background: var(--card-bg);
  border: 1px solid var(--border-subtle);
}
</style>
