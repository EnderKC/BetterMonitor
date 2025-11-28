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
  const dashboardUrl = window.location.origin;
  const secretKey = server.secret_key || server.SecretKey || '';
  const serverId = server.id || server.ID;

  const copyToClipboard = (text, type) => {
    navigator.clipboard.writeText(text).then(() => {
      message.success(`${type}命令已复制`);
    }).catch(err => {
      console.error('复制失败:', err);
      message.error('复制失败，请手动复制');
    });
  };

  request.get('/public/settings').then((res) => {
    const releaseRepo = res?.agent_release_repo || 'EnderKC/BetterMonitor';

    Modal.info({
      title: '部署 Agent',
      icon: h(KeyOutlined),
      content: h('div', { class: 'deploy-modal-content' }, [
        h('p', { class: 'deploy-desc' }, '请选择一种方式将 Agent 安装到您的服务器：'),

        // 区块1：服务器信息
        h('div', { class: 'deploy-block' }, [
          h('div', { class: 'block-title' }, '服务器信息'),
          h('div', { class: 'token-grid' }, [
            h('div', { class: 'token-item' }, [
              h('span', { class: 'token-label' }, 'Server ID'),
              h('div', { class: 'token-value-box' }, [
                h('code', { class: 'token-value' }, serverId),
                h(CopyOutlined, {
                  class: 'token-copy-icon',
                  onClick: () => copyToClipboard(String(serverId), 'Server ID')
                })
              ])
            ]),
            h('div', { class: 'token-item' }, [
              h('span', { class: 'token-label' }, 'Secret Key'),
              h('div', { class: 'token-value-box' }, [
                h('code', { class: 'token-value' }, secretKey || '未找到密钥'),
                h(CopyOutlined, {
                  class: 'token-copy-icon',
                  onClick: () => copyToClipboard(secretKey, 'Secret Key')
                })
              ])
            ])
          ])
        ]),

        // 区块2：一键安装
        h('div', { class: 'deploy-block recommended-block' }, [
          h('div', { class: 'block-header' }, [
            h('div', { class: 'block-title' }, '方案一：一键安装'),
            h('span', { class: 'recommend-badge' }, '推荐')
          ]),
          h('p', { class: 'block-desc' }, '在目标服务器上执行以下命令即可自动完成安装和启动：'),
          h('div', { class: 'command-box' }, [
            h('pre', { class: 'command-text' },
              `curl -fsSL https://raw.githubusercontent.com/${releaseRepo}/refs/heads/main/install-agent.sh | bash -s -- --server-id ${serverId} --secret-key ${secretKey} --server ${dashboardUrl}`
            ),
            h('div', {
              class: 'copy-btn',
              onClick: () => copyToClipboard(
                `curl -fsSL https://raw.githubusercontent.com/${releaseRepo}/refs/heads/main/install-agent.sh | bash -s -- --server-id ${serverId} --secret-key ${secretKey} --server ${dashboardUrl}`,
                '一键安装'
              )
            }, [h(CopyOutlined)])
          ])
        ]),

        // 区块3：手动安装
        h('div', { class: 'deploy-block' }, [
          h('div', { class: 'block-title' }, '方案二：手动安装'),
          h('div', { class: 'step-list' }, [
            h('div', { class: 'step-item' }, [
              h('span', { class: 'step-num' }, '1'),
              h('span', { class: 'step-text' }, [
                '下载对应系统的二进制文件：',
                h('a', { href: `https://github.com/${releaseRepo}/releases/latest`, target: '_blank' }, '前往下载')
              ])
            ]),
            h('div', { class: 'step-item' }, [
              h('span', { class: 'step-num' }, '2'),
              h('span', { class: 'step-text' }, '赋予执行权限：chmod +x better-monitor-agent')
            ]),
            h('div', { class: 'step-item' }, [
              h('span', { class: 'step-num' }, '3'),
              h('span', { class: 'step-text' }, '使用以下命令启动：')
            ])
          ]),
          h('div', { class: 'command-box' }, [
            h('pre', { class: 'command-text' },
              `./better-monitor-agent --server ${dashboardUrl} --server-id ${serverId} --secret-key ${secretKey}`
            ),
            h('div', {
              class: 'copy-btn',
              onClick: () => copyToClipboard(
                `./better-monitor-agent --server ${dashboardUrl} --server-id ${serverId} --secret-key ${secretKey}`,
                '启动'
              )
            }, [h(CopyOutlined)])
          ])
        ])
      ]),
      okText: '完成',
      width: 720,
      class: 'deploy-modal glass-modal',
      maskClosable: true
    });
  }).catch(err => {
    console.error('获取设置失败:', err);
    message.warning('无法获取发布仓库信息');
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

.dark .token-modal .ant-modal-content {
  background: #252526;
  color: #e0e0e0;
}


:deep(.deploy-modal .ant-modal-content) {
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.95), rgba(246, 250, 255, 0.55));
  backdrop-filter: blur(32px) saturate(190%);
  -webkit-backdrop-filter: blur(32px) saturate(190%);
  border-radius: 28px;
  border: 1px solid rgba(255, 255, 255, 0.55);
  box-shadow: 0 35px 80px rgba(15, 23, 42, 0.35), 0 0 0 1px rgba(255, 255, 255, 0.4) inset;
  padding: 0;
  overflow: hidden;
  position: relative;
}

:deep(.deploy-modal .ant-modal-content)::before,
:deep(.deploy-modal .ant-modal-content)::after {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 0;
}

:deep(.deploy-modal .ant-modal-content)::before {
  background: radial-gradient(circle at 20% 15%, rgba(255, 255, 255, 0.7), transparent 60%);
  opacity: 0.9;
}

:deep(.deploy-modal .ant-modal-content)::after {
  background: radial-gradient(circle at 80% -10%, rgba(59, 130, 246, 0.4), transparent 55%);
  filter: blur(18px);
}

:deep(.deploy-modal .ant-modal-header),
:deep(.deploy-modal .ant-modal-body),
:deep(.deploy-modal .ant-modal-footer) {
  background: transparent;
  position: relative;
  z-index: 1;
}

:deep(.deploy-modal .ant-modal-header) {
  border-bottom: 1px solid rgba(255, 255, 255, 0.4);
  padding: 22px 32px 12px;
  margin-bottom: 0;
}

:deep(.deploy-modal .ant-modal-body) {
  padding: 28px 32px 32px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

:deep(.deploy-modal .ant-modal-footer) {
  border-top: 1px solid rgba(255, 255, 255, 0.3);
  padding: 16px 32px 24px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.25), rgba(255, 255, 255, 0));
}

:deep(.deploy-modal .ant-modal-title) {
  font-size: 20px;
  font-weight: 700;
  display: flex;
  align-items: center;
  gap: 10px;
  background: linear-gradient(120deg, #2563eb, #7c3aed);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

:deep(.deploy-modal .ant-modal-footer .ant-btn-primary) {
  border: none;
  background: linear-gradient(135deg, #2563eb, #0ea5e9);
  box-shadow: 0 15px 40px rgba(37, 99, 235, 0.35);
  border-radius: 999px;
  padding: 0 32px;
  height: 40px;
}

:deep(.deploy-desc) {
  color: rgba(15, 23, 42, 0.65);
  margin-bottom: 12px;
  font-size: 15px;
  line-height: 1.6;
}

:deep(.deploy-block) {
  position: relative;
  background: rgba(255, 255, 255, 0.65);
  border: 1px solid rgba(255, 255, 255, 0.6);
  border-radius: 20px;
  padding: 20px;
  margin-bottom: 4px;
  box-shadow: 0 20px 45px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(20px) saturate(160%);
  -webkit-backdrop-filter: blur(20px) saturate(160%);
  overflow: hidden;
}

:deep(.deploy-block::after) {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  border: 1px solid rgba(255, 255, 255, 0.35);
  pointer-events: none;
  mix-blend-mode: soft-light;
  opacity: 0.6;
}

:deep(.deploy-block.recommended-block) {
  border-color: rgba(59, 130, 246, 0.4);
  background: linear-gradient(150deg, rgba(59, 130, 246, 0.16), rgba(59, 130, 246, 0.06));
  box-shadow: 0 25px 60px rgba(59, 130, 246, 0.25);
}

:deep(.block-header) {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

:deep(.block-title) {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 12px;
}

:deep(.block-header .block-title) {
  margin-bottom: 0;
}

:deep(.recommend-badge) {
  font-size: 10px;
  padding: 2px 10px;
  border-radius: 999px;
  font-weight: 600;
  background: rgba(59, 130, 246, 0.2);
  color: #2563eb;
  letter-spacing: 0.05em;
}

:deep(.block-desc) {
  font-size: 13px;
  color: rgba(15, 23, 42, 0.6);
  margin-bottom: 12px;
}

:deep(.token-grid) {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 18px;
}

:deep(.token-item) {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

:deep(.token-label) {
  font-size: 12px;
  color: rgba(15, 23, 42, 0.5);
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

:deep(.token-value-box) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: rgba(255, 255, 255, 0.85);
  padding: 8px 12px;
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.25);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.4), 0 12px 30px rgba(15, 23, 42, 0.08);
}

:deep(.token-value) {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
  color: var(--text-primary);
  font-weight: 600;
  word-break: break-all;
}

:deep(.token-copy-icon) {
  color: #94a3b8;
  cursor: pointer;
  transition: all 0.25s ease;
  padding: 4px;
  border-radius: 50%;
  background: rgba(148, 163, 184, 0.15);
  border: 1px solid rgba(148, 163, 184, 0.25);
}

:deep(.token-copy-icon:hover) {
  color: #fff;
  background: linear-gradient(135deg, #2563eb, #7c3aed);
  border-color: transparent;
  box-shadow: 0 10px 30px rgba(37, 99, 235, 0.35);
}

:deep(.command-box) {
  position: relative;
  background: rgba(15, 23, 42, 0.92);
  border-radius: 16px;
  padding: 16px 48px 16px 20px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.05), 0 25px 50px rgba(15, 23, 42, 0.45);
}

:deep(.command-text) {
  margin: 0;
  color: #f1f5f9;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

:deep(.copy-btn) {
  position: absolute;
  top: 10px;
  right: 10px;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #cbd5f5;
  cursor: pointer;
  transition: all 0.2s ease;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(148, 163, 184, 0.35);
  box-shadow: 0 15px 30px rgba(37, 99, 235, 0.3);
}

:deep(.copy-btn:hover) {
  background: linear-gradient(135deg, #2563eb, #0ea5e9);
  color: #fff;
  transform: translateY(-2px);
}

:deep(.step-list) {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 16px;
}

:deep(.step-item) {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  font-size: 13px;
  color: rgba(15, 23, 42, 0.65);
}

:deep(.step-num) {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.15), rgba(37, 99, 235, 0.35));
  font-size: 12px;
  font-weight: 600;
  color: #1d4ed8;
  flex-shrink: 0;
  box-shadow: 0 6px 15px rgba(37, 99, 235, 0.25);
}

/* Dark Mode Adaptations */
.dark :deep(.deploy-modal .ant-modal-content) {
  background: linear-gradient(135deg, rgba(17, 24, 39, 0.92), rgba(12, 18, 30, 0.9));
  border: 1px solid rgba(148, 163, 184, 0.25);
  box-shadow: 0 35px 90px rgba(0, 0, 0, 0.75);
}

.dark :deep(.deploy-modal .ant-modal-content)::before {
  background: radial-gradient(circle at 15% 10%, rgba(255, 255, 255, 0.25), transparent 60%);
  opacity: 0.5;
}

.dark :deep(.deploy-modal .ant-modal-content)::after {
  background: radial-gradient(circle at 80% -10%, rgba(99, 102, 241, 0.45), transparent 55%);
}

.dark :deep(.deploy-modal .ant-modal-header) {
  border-bottom-color: rgba(148, 163, 184, 0.25);
}

.dark :deep(.deploy-modal .ant-modal-footer) {
  border-top-color: rgba(148, 163, 184, 0.2);
}

.dark :deep(.deploy-desc) {
  color: rgba(226, 232, 240, 0.7);
}

.dark :deep(.block-title),
.dark :deep(.block-desc),
.dark :deep(.token-label),
.dark :deep(.step-item) {
  color: rgba(226, 232, 240, 0.7);
}

.dark :deep(.deploy-block) {
  background: rgba(15, 23, 42, 0.75);
  border-color: rgba(148, 163, 184, 0.2);
  box-shadow: 0 25px 70px rgba(0, 0, 0, 0.65);
}

.dark :deep(.deploy-block.recommended-block) {
  background: linear-gradient(150deg, rgba(59, 130, 246, 0.22), rgba(79, 70, 229, 0.15));
  border-color: rgba(79, 70, 229, 0.4);
}

.dark :deep(.recommend-badge) {
  background: rgba(59, 130, 246, 0.25);
  color: #c4d6ff;
}

.dark :deep(.token-value-box) {
  background: rgba(30, 41, 59, 0.75);
  border-color: rgba(148, 163, 184, 0.3);
}

.dark :deep(.token-copy-icon) {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(148, 163, 184, 0.2);
  color: rgba(226, 232, 240, 0.8);
}

.dark :deep(.token-copy-icon:hover) {
  color: #fff;
}

.dark :deep(.command-box) {
  background: rgba(2, 6, 23, 0.9);
  border-color: rgba(59, 130, 246, 0.25);
  box-shadow: inset 0 0 0 1px rgba(59, 130, 246, 0.15), 0 30px 60px rgba(0, 0, 0, 0.75);
}

.dark :deep(.copy-btn) {
  border-color: rgba(59, 130, 246, 0.3);
  color: rgba(226, 232, 240, 0.8);
}

.dark :deep(.copy-btn:hover) {
  box-shadow: 0 15px 35px rgba(59, 130, 246, 0.45);
}

.dark :deep(.step-num) {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.5), rgba(37, 99, 235, 0.6));
  color: #e0e7ff;
}
</style>
