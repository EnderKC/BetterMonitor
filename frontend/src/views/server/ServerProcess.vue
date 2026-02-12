<script setup lang="ts">
import { ref, onMounted, reactive, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal, Input, Tag } from 'ant-design-vue';
import {
  ReloadOutlined,
  SearchOutlined,
  StopOutlined,
  InfoCircleOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
// 导入服务器状态store
import { useServerStore } from '../../stores/serverStore';
import { useUIStore } from '../../stores/uiStore';

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));
// 获取服务器状态store
const serverStore = useServerStore();
const uiStore = useUIStore();

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// 进程列表
const processList = ref<any[]>([]);
const processLoading = ref(false);

// 过滤
const filters = reactive({
  search: '',
  port: '',
  hideSystem: true
});

// 计算服务器是否在线 (使用全局状态)
const isServerOnline = computed(() => {
  return serverStore.isServerOnline(serverId.value);
});

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    // 使用any类型避免TypeScript错误
    const response: any = await request.get(`/servers/${serverId.value}`);
    console.log('服务器详情响应:', response);

    // 从响应中提取服务器数据
    if (response && response.server) {
      serverInfo.value = response.server;

      // 更新全局状态
      const status = response.server.status || 'offline';
      const isOnline = response.server.online === true;

      console.log('更新服务器状态:', serverId.value, status, isOnline);
      serverStore.updateServerStatus(serverId.value, status);
    } else {
      console.error('响应中没有找到服务器数据');
      message.error('获取服务器数据失败');
    }
  } catch (error) {
    console.error('获取服务器信息失败:', error);
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
    uiStore.stopLoading();
  }
};

// 获取进程列表
const fetchProcessList = async () => {
  // 检查服务器是否在线
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法获取进程列表');
    return;
  }

  processLoading.value = true;
  try {
    const response = await request.get(`/servers/${serverId.value}/processes`);
    console.log('获取进程列表响应:', response);

    // 确定响应数据的位置（可能在response或response.data中）
    const responseData = response.data || response;

    // 处理返回的进程列表数据
    if (responseData && responseData.processes) {
      processList.value = responseData.processes || [];
      console.log(`加载了 ${processList.value.length} 个进程，总数: ${responseData.count || 0}`);
    } else {
      console.error('响应中没有找到进程列表数据');
      processList.value = [];
    }
  } catch (error) {
    console.error('获取进程列表失败:', error);
    message.error('获取进程列表失败');
    processList.value = [];
  } finally {
    processLoading.value = false;
  }
};

// 终止进程
const killProcess = (pid: number) => {
  // 检查服务器是否在线
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法终止进程');
    return;
  }

  Modal.confirm({
    title: '确认终止进程',
    content: `确定要终止进程 PID: ${pid} 吗？该操作不可恢复，可能导致程序异常。`,
    okText: '确认终止',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await request.delete(`/servers/${serverId.value}/processes/${pid}`);
        message.success('进程已终止');
        // 刷新进程列表
        fetchProcessList();
      } catch (error) {
        console.error('终止进程失败:', error);
        message.error('终止进程失败');
      }
    },
  });
};

// 过滤进程列表
const filteredProcessList = computed(() => {
  return processList.value.filter(process => {
    // 系统进程过滤
    if (filters.hideSystem && process.is_system) {
      return false;
    }

    // 搜索过滤
    if (filters.search &&
      !process.name.toLowerCase().includes(filters.search.toLowerCase()) &&
      !process.cmd.toLowerCase().includes(filters.search.toLowerCase()) &&
      !String(process.pid).includes(filters.search)) {
      return false;
    }

    // 端口过滤
    if (filters.port && process.ports) {
      // 检查端口是否在列表中
      if (!Array.isArray(process.ports) || !process.ports.includes(filters.port)) {
        return false;
      }
    } else if (filters.port) {
      // 如果指定了端口过滤，但进程没有端口，则过滤掉
      return false;
    }

    return true;
  });
});

// 进程详情
const processDetailVisible = ref(false);
const currentProcess = ref<any>(null);

// 显示进程详情
const showProcessDetail = (process: any) => {
  currentProcess.value = process;
  processDetailVisible.value = true;
};

// 关闭进程详情
const closeProcessDetail = () => {
  processDetailVisible.value = false;
  currentProcess.value = null;
};

// 格式化内存大小
const formatMemorySize = (bytes: number): string => {
  if (!bytes) return '0 B';

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = bytes;

  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }

  return `${size.toFixed(2)} ${units[i]}`;
};

// 页面挂载时初始化
onMounted(async () => {
  console.log('进程管理页面挂载，服务器ID:', serverId.value);

  // 获取服务器信息
  await fetchServerInfo();

  // 验证服务器是否在线
  if (isServerOnline.value) {
    await fetchProcessList();
  } else {
    console.warn('服务器离线，无法获取进程列表');
    message.warning('服务器离线，无法使用进程管理功能');
  }
});

// 返回服务器详情页
const goBack = () => {
  router.push(`/admin/servers/${serverId.value}`);
};

// 刷新进程列表
const refreshProcessList = () => {
  fetchProcessList();
};

// 清除过滤条件
const clearFilters = () => {
  filters.search = '';
  filters.port = '';
};

// 定义进程类型接口，解决TypeScript警告
interface ProcessInfo {
  pid: number;
  name: string;
  username: string;
  cpu_percent: number;
  memory_rss: number;
  memory_vms: number;
  ports: string[] | null;
  status: string;
  cmd: string;
  create_time: number;
  is_system: boolean;
  ppid: number;
}

// 表格排序状态
const sortState = ref({
  columnKey: '',
  order: null as 'ascend' | 'descend' | null
});

// 表格变化处理函数
const handleTableChange = (pagination: any, filters: any, sorter: any) => {
  // 更新排序状态
  sortState.value.columnKey = sorter.columnKey;
  sortState.value.order = sorter.order;
};
</script>

<template>
  <div class="process-container">
    <a-page-header title="进程管理" :sub-title="serverInfo.name" @back="goBack">
      <template #tags>
        <a-tag :color="isServerOnline ? 'success' : 'error'">
          {{ isServerOnline ? '在线' : '离线' }}
        </a-tag>
      </template>

      <template #extra>
        <a-button type="primary" @click="refreshProcessList" :loading="processLoading">
          <ReloadOutlined />
          刷新
        </a-button>
      </template>
    </a-page-header>

    <div class="process-content">
      <a-spin :spinning="loading">
        <a-alert v-if="!isServerOnline" type="warning" show-icon message="服务器当前离线，无法获取进程信息"
          style="margin-bottom: 24px" />

        <div v-else>
          <!-- 过滤器 -->
          <div class="filter-bar">
            <a-card :bordered="false">
              <a-row :gutter="24">
                <a-col :span="8">
                  <a-input v-model:value="filters.search" placeholder="搜索进程名称、PID或命令行" allowClear>
                    <template #prefix>
                      <SearchOutlined />
                    </template>
                  </a-input>
                </a-col>
                <a-col :span="6">
                  <a-input v-model:value="filters.port" placeholder="按端口号筛选" allowClear />
                </a-col>
                <a-col :span="8">
                  <a-checkbox v-model:checked="filters.hideSystem">
                    隐藏系统进程
                  </a-checkbox>
                </a-col>
                <a-col :span="2">
                  <a-button type="link" @click="clearFilters">
                    清除筛选
                  </a-button>
                </a-col>
              </a-row>
            </a-card>
          </div>

          <!-- 进程列表 -->
          <div class="process-list">
            <a-table :dataSource="filteredProcessList" :loading="processLoading"
              :pagination="{ pageSize: 10, showSizeChanger: true, showQuickJumper: true }" rowKey="pid"
              @change="handleTableChange">
              <a-table-column title="PID" dataIndex="pid" key="pid"
                :sorter="{ compare: (a: ProcessInfo, b: ProcessInfo) => a.pid - b.pid }"
                :sortDirections="['ascend', 'descend']" />
              <a-table-column title="名称" dataIndex="name" key="name"
                :sorter="{ compare: (a: ProcessInfo, b: ProcessInfo) => a.name.localeCompare(b.name) }"
                :sortDirections="['ascend', 'descend']">
                <template #customRender="{ text, record }">
                  <a @click="showProcessDetail(record)" class="process-name">
                    {{ text }}
                  </a>
                </template>
              </a-table-column>
              <a-table-column title="用户" dataIndex="username" key="username"
                :sorter="{ compare: (a: ProcessInfo, b: ProcessInfo) => a.username.localeCompare(b.username) }"
                :sortDirections="['ascend', 'descend']" />
              <a-table-column title="CPU" dataIndex="cpu_percent" key="cpu_percent"
                :sorter="{ compare: (a: ProcessInfo, b: ProcessInfo) => a.cpu_percent - b.cpu_percent }"
                :sortDirections="['ascend', 'descend']">
                <template #customRender="{ text }">
                  {{ text }}%
                </template>
              </a-table-column>
              <a-table-column title="内存" dataIndex="memory_rss" key="memory_rss"
                :sorter="{ compare: (a: ProcessInfo, b: ProcessInfo) => a.memory_rss - b.memory_rss }"
                :sortDirections="['ascend', 'descend']">
                <template #customRender="{ text }">
                  {{ formatMemorySize(text) }}
                </template>
              </a-table-column>
              <a-table-column title="端口" dataIndex="ports" key="ports">
                <template #customRender="{ record }">
                  <template v-if="record.ports && Array.isArray(record.ports) && record.ports.length">
                    <a-tag v-for="port in record.ports" :key="port" color="blue">
                      {{ port }}
                    </a-tag>
                  </template>
                  <span v-else>-</span>
                </template>
              </a-table-column>
              <a-table-column title="状态" dataIndex="status" key="status"
                :sorter="{ compare: (a: ProcessInfo, b: ProcessInfo) => a.status.localeCompare(b.status) }"
                :sortDirections="['ascend', 'descend']">
                <template #customRender="{ text }">
                  <a-tag :color="text === 'running' ? 'success' : 'default'">
                    {{ text === 'running' ? '运行中' : text }}
                  </a-tag>
                </template>
              </a-table-column>
              <a-table-column title="操作">
                <template #customRender="{ record }">
                  <a-space>
                    <a-button type="primary" size="small" @click="showProcessDetail(record)">
                      <InfoCircleOutlined />
                      详情
                    </a-button>
                    <a-button type="primary" danger size="small" @click="killProcess(record.pid)">
                      <StopOutlined />
                      终止
                    </a-button>
                  </a-space>
                </template>
              </a-table-column>
            </a-table>
          </div>
        </div>
      </a-spin>
    </div>
  </div>

  <!-- 进程详情对话框 -->
  <a-modal v-model:open="processDetailVisible" :title="`进程详情 (PID: ${currentProcess?.pid || '-'})`" width="700px"
    @cancel="closeProcessDetail">
    <template #footer>
      <a-button type="primary" @click="closeProcessDetail">关闭</a-button>
    </template>
    <div v-if="currentProcess" class="process-detail-grid">
      <div class="detail-item full-width">
        <div class="detail-label">名称</div>
        <div class="detail-value highlight">{{ currentProcess.name || '-' }}</div>
      </div>

      <div class="detail-item">
        <div class="detail-label">PID</div>
        <div class="detail-value mono">{{ currentProcess.pid || '-' }}</div>
      </div>

      <div class="detail-item">
        <div class="detail-label">用户</div>
        <div class="detail-value">{{ currentProcess.username || '-' }}</div>
      </div>

      <div class="detail-item">
        <div class="detail-label">CPU使用率</div>
        <div class="detail-value">{{ currentProcess.cpu_percent ? `${currentProcess.cpu_percent.toFixed(2)}%` : '-' }}
        </div>
      </div>

      <div class="detail-item">
        <div class="detail-label">内存使用</div>
        <div class="detail-value">{{ currentProcess.memory_rss ? formatMemorySize(currentProcess.memory_rss) : '-' }}
        </div>
      </div>

      <div class="detail-item">
        <div class="detail-label">状态</div>
        <div class="detail-value">
          <a-tag :color="currentProcess.status === 'running' ? 'success' : 'default'">
            {{ currentProcess.status === 'running' ? '运行中' : currentProcess.status }}
          </a-tag>
        </div>
      </div>

      <div class="detail-item">
        <div class="detail-label">启动时间</div>
        <div class="detail-value">{{ currentProcess.create_time ? new Date(currentProcess.create_time *
          1000).toLocaleString() : '-' }}</div>
      </div>

      <div class="detail-item full-width">
        <div class="detail-label">监听端口</div>
        <div class="detail-value">
          <template v-if="currentProcess.ports && Array.isArray(currentProcess.ports) && currentProcess.ports.length">
            <a-tag v-for="port in currentProcess.ports" :key="port" color="blue" style="margin-right: 5px">
              {{ port }}
            </a-tag>
          </template>
          <template v-else>-</template>
        </div>
      </div>

      <div class="detail-item full-width">
        <div class="detail-label">命令行</div>
        <div class="detail-value cmd-text">{{ currentProcess.cmd || '-' }}</div>
      </div>
    </div>
  </a-modal>
</template>

<style scoped>
.process-container {
  padding: 0;
  background: transparent;
}

.process-content {
  margin-top: 16px;
}

.filter-bar {
  margin-bottom: 16px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border: 1px solid var(--alpha-black-05);
  border-radius: var(--radius-lg);
  box-shadow: 0 8px 32px var(--alpha-black-05);
}

.process-list {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border: 1px solid var(--alpha-black-05);
  border-radius: var(--radius-lg);
  box-shadow: 0 8px 32px var(--alpha-black-05);
  overflow: hidden;
}

.process-name {
  color: var(--primary-color);
  cursor: pointer;
  font-weight: var(--font-weight-medium);
  transition: all 0.2s ease;
}

.process-name:hover {
  text-decoration: underline;
  color: #0051d5;
}

/* 详情样式 */
/* 详情样式优化 */
.process-detail-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
  padding: 8px;
}

.detail-item {
  background: var(--alpha-black-02);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.detail-item.full-width {
  grid-column: span 2;
}

.detail-label {
  font-size: var(--font-size-xs);
  color: #8e8e93;
  font-weight: var(--font-weight-medium);
}

.detail-value {
  font-size: var(--font-size-md);
  color: #1d1d1f;
  font-weight: var(--font-weight-medium);
  word-break: break-all;
}

.detail-value.highlight {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--primary-color);
}

.detail-value.mono {
  font-family: "SF Mono", Menlo, monospace;
}

.detail-value.cmd-text {
  font-family: "SF Mono", Menlo, monospace;
  font-size: var(--font-size-xs);
  background: var(--alpha-black-03);
  padding: 8px;
  border-radius: var(--radius-sm);
  margin-top: 4px;
}

/* 表格样式优化 */
:deep(.ant-table) {
  background: transparent;
}

:deep(.ant-table-thead > tr > th) {
  background: var(--alpha-white-60);
  border-bottom: 1px solid var(--alpha-black-05);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
  font-size: var(--font-size-sm);
}

:deep(.ant-table-tbody > tr > td) {
  background: transparent;
  border-bottom: 1px solid var(--alpha-black-03);
  font-size: var(--font-size-sm);
}

:deep(.ant-table-tbody > tr:hover > td) {
  background: var(--primary-bg);
}

/* 模态框样式 */
:deep(.ant-modal-content) {
  background: var(--card-bg);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  border: 1px solid var(--alpha-white-40);
  box-shadow: var(--shadow-lg);
}

:deep(.ant-modal-header) {
  background: var(--alpha-white-50);
  border-bottom: 1px solid var(--alpha-black-05);
  border-radius: var(--radius-lg) 16px 0 0;
}

:deep(.ant-modal-title) {
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
}

:deep(.ant-modal-body) {
  background: transparent;
}

:deep(.ant-modal-footer) {
  background: var(--alpha-white-30);
  border-top: 1px solid var(--alpha-black-05);
  border-radius: 0 0 16px 16px;
}

/* 标签样式优化 */
:deep(.ant-tag) {
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  padding: 2px 10px;
  font-weight: var(--font-weight-medium);
}

/* 输入框样式 */
:deep(.ant-input) {
  border-radius: 10px;
  border: 1px solid var(--alpha-black-10);
}

:deep(.ant-input:focus) {
  border-color: var(--primary-color);
  box-shadow: 0 0 0 2px var(--primary-light);
}

/* 复选框样式 */
:deep(.ant-checkbox-wrapper) {
  font-size: var(--font-size-sm);
  color: var(--text-primary);
}
</style>

<style>
/* Global Dark Mode Styles for ServerProcess */
.dark .filter-bar,
.dark .process-list {
  background: rgba(30, 30, 30, 0.6) !important;
  border: 1px solid var(--alpha-white-10);
  box-shadow: 0 8px 32px var(--alpha-black-20);
}

.dark .ant-table-thead>tr>th {
  background: rgba(40, 40, 40, 0.6) !important;
  border-bottom: 1px solid var(--alpha-white-10) !important;
  color: var(--body-bg) !important;
}

.dark .ant-table-tbody>tr>td {
  border-bottom: 1px solid var(--alpha-white-05) !important;
  color: var(--body-bg) !important;
}

.dark .ant-table-tbody>tr:hover>td {
  background: var(--alpha-white-05) !important;
}

.dark .process-name {
  color: #0a84ff !important;
}

.dark .process-name:hover {
  color: #409cff !important;
}

/* Modal Dark Mode */
.dark .ant-modal-content {
  background: rgba(40, 40, 40, 0.8) !important;
  backdrop-filter: blur(var(--blur-md)) !important;
  border: 1px solid var(--alpha-white-10) !important;
  box-shadow: 0 8px 32px var(--alpha-black-40) !important;
}

.dark .ant-modal-header {
  background: rgba(50, 50, 50, 0.6) !important;
  border-bottom: 1px solid var(--alpha-white-10) !important;
}

.dark .ant-modal-title {
  color: var(--body-bg) !important;
}

.dark .ant-modal-footer {
  background: rgba(50, 50, 50, 0.3) !important;
  border-top: 1px solid var(--alpha-white-10) !important;
}

.dark .ant-modal-close {
  color: var(--body-bg) !important;
}

.dark .detail-item {
  background: var(--alpha-white-05);
}

.dark .detail-label {
  color: #8e8e93;
}

.dark .detail-value {
  color: var(--body-bg);
}

.dark .detail-value.highlight {
  color: #0a84ff;
}

.dark .detail-value.cmd-text {
  background: var(--alpha-black-20);
  color: var(--body-bg);
}

/* Input Dark Mode */
.dark .ant-input {
  background: var(--alpha-black-20) !important;
  border: 1px solid var(--alpha-white-10) !important;
  color: var(--body-bg) !important;
}

.dark .ant-input:focus {
  border-color: #0a84ff !important;
  box-shadow: 0 0 0 2px rgba(10, 132, 255, 0.2) !important;
}

.dark .ant-checkbox-wrapper {
  color: var(--body-bg) !important;
}
</style>