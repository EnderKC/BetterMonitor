<script setup lang="ts">
import { ref, reactive, onMounted, h, computed } from 'vue';
import { useRouter } from 'vue-router';
import { message, Modal } from 'ant-design-vue';
import { PlusOutlined, EyeOutlined, EditOutlined, DeleteOutlined, KeyOutlined, CopyOutlined } from '@ant-design/icons-vue';
import request from '../../utils/request';
// 导入服务器状态Badge组件和store
import ServerStatusBadge from '../../components/ServerStatusBadge.vue';
import { useServerStore } from '../../stores/serverStore';

import { ReloadOutlined } from '@ant-design/icons-vue';

const router = useRouter();
// 获取服务器状态store
const serverStore = useServerStore();

// 数据状态
const loading = ref(false);
// 使用计算属性从store获取服务器列表
const servers = computed(() => serverStore.getAllServers);

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

// 表单验证规则
const rules = {
  name: [
    { required: true, message: '请输入服务器名称', trigger: 'blur' },
  ],
};

// 定义表格列
const columns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: 'IP地址', dataIndex: 'ip', key: 'ip' },
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
const viewToken = (server: any) => {
  let releaseRepo = '';
  const dashboardUrl = window.location.origin;

  const copyToClipboard = (text, type) => {
    navigator.clipboard.writeText(text).then(() => {
      message.success(`${type}命令已复制到剪贴板`);
    }).catch(err => {
      console.error('复制失败:', err);
      message.error('复制失败，请手动复制');
    });
  };

  request.get('/public/settings').then((res) => {
    releaseRepo = res?.agent_release_repo || 'your-org/better-monitor-agent';

    Modal.info({
      title: '服务器令牌',
      content: h('div', {}, [
        h('p', '请保存以下令牌信息，用于安装Agent到服务器：'),
        h('div', { class: 'token-info' }, [
          h('p', [
            h('strong', 'ID: '),
            h('span', { class: 'token-value' }, server.id || server.ID)
          ]),
          h('p', [
            h('strong', '密钥: '),
            h('span', { class: 'token-value' }, server.secret_key)
          ])
        ]),
        h('div', { class: 'token-help' }, [
          h('p', { style: 'font-weight: bold; margin-top: 16px' }, '安装步骤：'),
          h('ol', [
            h('li', [
              '访问 ',
              h('a', {
                href: `https://github.com/${releaseRepo}/releases/latest`,
                target: '_blank'
              }, 'Agent发布页面'),
              '，下载对应系统的二进制文件'
            ]),
            h('li', '授予执行权限，例如：chmod +x better-monitor-agent'),
            h('li', '使用以下参数运行Agent：')
          ]),
          h('div', { style: 'position: relative; margin-top: 16px;' }, [
            h('div', { style: 'display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px;' }, [
              h('span', { style: 'font-weight: bold;' }, '启动命令:'),
              h('a-button', {
                type: 'primary',
                size: 'small',
                shape: 'round',
                onClick: () => copyToClipboard(
                  `./better-monitor-agent --server ${dashboardUrl} --server-id ${server.id || server.ID} --secret-key ${server.secret_key}`,
                  '启动'
                )
              }, [
                h(CopyOutlined),
                ' 复制命令'
              ])
            ]),
            h('pre', { class: 'install-command' },
              `./better-monitor-agent --server ${dashboardUrl} --server-id ${server.id || server.ID} --secret-key ${server.secret_key}`
            )
          ])
        ])
      ]),
      okText: '我已保存',
      width: 700,
      class: 'token-modal'
    });
  }).catch(err => {
    console.error('获取设置失败:', err);
    message.warning('无法获取发布仓库信息，请参考文档手动安装');
  });
};

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
        <a-button @click="handleRefresh" class="glow-effect" style="margin-right: 8px;">
          <template #icon>
            <ReloadOutlined />
          </template>
          刷新
        </a-button>
        <a-button type="primary" @click="showAddForm" class="glow-effect">
          <template #icon>
            <PlusOutlined />
          </template>
          添加服务器
        </a-button>
      </div>
    </div>

    <div class="server-list-content glass-card">
      <a-table :dataSource="servers" :columns="columns" :loading="loading" :pagination="{ pageSize: 10 }" rowKey="id"
        class="modern-table">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'action'">
            <div class="action-buttons">
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

:deep(.glass-modal .ant-modal-content) {
  background: var(--card-bg);
  backdrop-filter: blur(15px);
  -webkit-backdrop-filter: blur(15px);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  border: 1px solid var(--card-border);
}

:deep(.token-modal .ant-modal-content) {
  background: var(--card-bg);
  backdrop-filter: blur(15px);
  -webkit-backdrop-filter: blur(15px);
}

:deep(.token-info) {
  margin: 16px 0;
  padding: 16px;
  background-color: rgba(0, 0, 0, 0.02);
  border-radius: var(--radius-md);
  border: 1px solid rgba(0, 0, 0, 0.06);
}

:deep(.token-value) {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-weight: 500;
}

:deep(.token-help) {
  margin-top: 16px;
}

:deep(.install-command) {
  margin: 8px 0;
  padding: 16px;
  background-color: #272822;
  color: #f8f8f2;
  border-radius: var(--radius-md);
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
  border: 1px solid rgba(0, 0, 0, 0.3);
  line-height: 1.6;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.1);
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
.dark .modern-table .ant-table-thead > tr > th {
  background: #2d2d2d;
  color: #ccc;
  border-bottom: 1px solid #333;
}

.dark .modern-table .ant-table-tbody > tr > td {
  border-bottom: 1px solid #333;
  color: #ccc;
}

.dark .modern-table .ant-table-tbody > tr:hover > td {
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

.dark .token-modal .ant-modal-content {
  background: #252526;
  color: #e0e0e0;
}

.dark .token-info {
  background-color: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
}
</style>
