<script setup lang="ts">
import { ref, onMounted, h } from 'vue';
import { Card, Descriptions, Tag, Button, Space, Table, Spin, message, Modal, Select } from 'ant-design-vue';
import { InfoCircleOutlined, SyncOutlined, DownloadOutlined, ExclamationCircleOutlined } from '@ant-design/icons-vue';
import {
  getDashboardVersion,
  getSystemInfo,
  getServersVersions,
  getLatestAgentRelease,
  forceAgentUpgrade,
  type VersionInfo,
  type SystemInfo,
  type ServerVersion,
  type AgentReleaseInfo
} from '../utils/version';
import { useUserStore } from '../stores/userStore';
import moment from 'moment';

// 组件数据
const loading = ref(false);
const dashboardVersion = ref<VersionInfo | null>(null);
const systemInfo = ref<SystemInfo | null>(null);
const serversVersions = ref<ServerVersion[]>([]);
const releaseInfo = ref<AgentReleaseInfo | null>(null);
const userStore = useUserStore();

// OTA更新相关
const updating = ref(false);
const updateModalVisible = ref(false);
const selectedServerId = ref<number | null>(null);
const updatingAll = ref(false);
const updateAllModalVisible = ref(false);
const serversToUpdate = ref<ServerVersion[]>([]);

// 表格列定义
const columns = [
  {
    title: '服务器名称',
    dataIndex: 'name',
    key: 'name',
  },
  {
    title: '主机地址',
    dataIndex: 'host',
    key: 'host',
    customRender: ({ text }: { text: string }) => {
      if (!text) return '-';
      const tags = [];
      const ips = text.split(/[,\s]+/).filter((ip: string) => ip.trim() !== '');
      ips.forEach((ip: string) => {
        tags.push(h(Tag, { color: 'blue' }, () => ip));
      });
      return h('div', { style: 'display: flex; flex-direction: column; gap: 4px; align-items: flex-start;' }, tags);
    }
  },
  {
    title: 'Agent版本',
    dataIndex: 'agentVersion',
    key: 'agentVersion',
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
  },
  {
    title: '最后心跳',
    dataIndex: 'lastHeartbeat',
    key: 'lastHeartbeat',
  },
  {
    title: '操作',
    key: 'action',
  },
];

// 获取状态标签
const getStatusTag = (status: number) => {
  switch (status) {
    case 1:
      return { color: 'green', text: '在线' };
    case 0:
      return { color: 'red', text: '离线' };
    default:
      return { color: 'gray', text: '未知' };
  }
};

// 格式化时间
const formatTime = (timeStr?: string) => {
  if (!timeStr) return '-';
  return moment(timeStr).format('YYYY-MM-DD HH:mm:ss');
};

// 格式化运行时间
const formatUptime = (uptimeStr?: string) => {
  if (!uptimeStr) return '-';
  return uptimeStr;
};

// 获取版本颜色
const getVersionColor = (version?: string) => {
  if (!version) return 'gray';
  if (version === 'unknown' || version === 'dev') return 'orange';
  return 'blue';
};

// 比较版本号（改进版本）
const compareVersions = (v1?: string, v2?: string) => {
  if (!v1 || !v2) return 0;
  if (v1 === v2) return 0;

  // 特殊版本处理
  if (v1 === 'dev' && v2 !== 'dev') return -1; // dev版本总是需要更新
  if (v1 !== 'dev' && v2 === 'dev') return 1;
  if (v1 === 'dev' && v2 === 'dev') return 0;

  // 未知版本处理
  if (v1 === 'unknown' && v2 !== 'unknown') return -1;
  if (v1 !== 'unknown' && v2 === 'unknown') return 1;
  if (v1 === 'unknown' && v2 === 'unknown') return 0;

  const parts1 = v1.split('.').map(Number);
  const parts2 = v2.split('.').map(Number);

  for (let i = 0; i < Math.max(parts1.length, parts2.length); i++) {
    const part1 = parts1[i] || 0;
    const part2 = parts2[i] || 0;

    if (part1 < part2) return -1;
    if (part1 > part2) return 1;
  }

  return 0;
};

// 最新Agent版本信息
const latestAgentVersion = ref<string>('');

// 检查是否需要更新（改进版本）
const needsUpdate = (serverVersion?: string) => {
  if (!serverVersion) return false;

  // 如果有最新Agent版本，使用它作为目标版本
  const targetVersion = latestAgentVersion.value || dashboardVersion.value?.version;
  if (!targetVersion) return false;

  return compareVersions(serverVersion, targetVersion) < 0;
};

// 获取所有版本信息
const fetchVersions = async () => {
  loading.value = true;
  try {
    // 并行获取所有版本信息
    const [dashVersion, sysInfo, serversVer, release] = await Promise.all([
      getDashboardVersion(),
      getSystemInfo(),
      getServersVersions(),
      getLatestAgentRelease().catch(() => null)
    ]);

    dashboardVersion.value = dashVersion;
    systemInfo.value = sysInfo;
    serversVersions.value = serversVer;
    releaseInfo.value = release;
    if (release && release.version) {
      latestAgentVersion.value = release.version;
    }

    message.success('版本信息已更新');
  } catch (error) {
    console.error('获取版本信息失败:', error);
    message.error('获取版本信息失败');
  } finally {
    loading.value = false;
  }
};

// 显示OTA更新确认对话框
const showUpdateModal = (serverId: number) => {
  selectedServerId.value = serverId;
  updateModalVisible.value = true;
};

// 执行升级
const performUpgrade = async () => {
  if (!selectedServerId.value) return;

  updating.value = true;
  try {
    const result = await forceAgentUpgrade({
      serverIds: [selectedServerId.value],
      targetVersion: latestAgentVersion.value || dashboardVersion.value?.version || undefined
    });

    if (result.success) {
      message.success(result.message || '升级请求已发送');
      updateModalVisible.value = false;
      // 延迟刷新版本信息，给agent时间更新
      setTimeout(() => {
        fetchVersions();
      }, 3000);
    } else {
      message.error(`升级失败: ${result.message}`);
    }
  } catch (error) {
    console.error('升级失败:', error);
    message.error('升级失败');
  } finally {
    updating.value = false;
  }
};

// 取消更新
const cancelUpdate = () => {
  updateModalVisible.value = false;
  selectedServerId.value = null;
};

// 批量升级相关
const showUpdateAllModal = () => {
  // 筛选出在线且需要更新的服务器
  const targets = serversVersions.value.filter(s => s.status === 1 && needsUpdate(s.agentVersion));

  if (targets.length === 0) {
    message.info('没有需要更新的在线服务器');
    return;
  }

  serversToUpdate.value = targets;
  updateAllModalVisible.value = true;
};

const performUpdateAll = async () => {
  if (serversToUpdate.value.length === 0) return;

  updatingAll.value = true;
  try {
    const serverIds = serversToUpdate.value.map(s => s.id);
    const result = await forceAgentUpgrade({
      serverIds: serverIds,
      targetVersion: latestAgentVersion.value || dashboardVersion.value?.version || undefined
    });

    if (result.success) {
      message.success(result.message || `已向 ${result.result.success.length} 台服务器发送升级指令`);
      updateAllModalVisible.value = false;
      // 延迟刷新版本信息
      setTimeout(() => {
        fetchVersions();
      }, 3000);
    } else {
      message.error(`批量升级失败: ${result.message}`);
    }
  } catch (error) {
    console.error('批量升级失败:', error);
    message.error('批量升级失败');
  } finally {
    updatingAll.value = false;
  }
};

const cancelUpdateAll = () => {
  updateAllModalVisible.value = false;
  serversToUpdate.value = [];
};

// 获取选中的服务器信息
const getSelectedServer = () => {
  return serversVersions.value.find(s => s.id === selectedServerId.value);
};

// 组件挂载时获取版本信息
onMounted(() => {
  fetchVersions();
});
</script>

<template>
  <div class="version-info">
    <Space direction="vertical" size="large" style="width: 100%">
      <!-- Dashboard版本信息 -->
      <Card title="Dashboard版本信息" size="small">
        <Spin :spinning="loading">
          <Descriptions :column="2" bordered size="small">
            <Descriptions.Item label="版本">
              <Tag :color="getVersionColor(dashboardVersion?.version)">
                {{ dashboardVersion?.version || '-' }}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="构建时间">
              {{ formatTime(dashboardVersion?.buildTime) }}
            </Descriptions.Item>
            <Descriptions.Item label="Go版本">
              {{ dashboardVersion?.goVersion || '-' }}
            </Descriptions.Item>
          </Descriptions>
        </Spin>
      </Card>

      <!-- 系统信息 -->
      <Card title="系统信息" size="small">
        <Spin :spinning="loading">
          <Descriptions :column="2" bordered size="small">
            <Descriptions.Item label="启动时间">
              {{ formatTime(systemInfo?.startTime) }}
            </Descriptions.Item>
            <Descriptions.Item label="运行时间">
              {{ formatUptime(systemInfo?.uptime) }}
            </Descriptions.Item>
            <Descriptions.Item label="操作系统">
              {{ systemInfo?.osInfo || '-' }}
            </Descriptions.Item>
            <Descriptions.Item label="系统架构">
              {{ systemInfo?.arch || '-' }}
            </Descriptions.Item>
            <Descriptions.Item label="CPU核心数">
              {{ systemInfo?.cpuCount || '-' }}
            </Descriptions.Item>
            <Descriptions.Item label="内存总量">
              {{ systemInfo?.memoryTotal || '-' }}
            </Descriptions.Item>
          </Descriptions>
        </Spin>
      </Card>

      <!-- Agent版本信息 -->
      <Card size="small">
        <template #title>
          <Space>
            <span>Agent版本信息</span>
            <Button type="primary" size="small" :loading="loading" @click="fetchVersions">
              <SyncOutlined /> 刷新
            </Button>
            <Button type="primary" size="small" :loading="updatingAll" @click="showUpdateAllModal">
              <DownloadOutlined /> 一键更新
            </Button>
          </Space>
        </template>

        <Spin :spinning="loading">
          <Table :dataSource="serversVersions" :columns="columns" :pagination="false" size="small"
            :rowKey="record => record.id">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'agentVersion'">
                <Space>
                  <Tag :color="getVersionColor(record.agentVersion)">
                    {{ record.agentVersion || '未知' }}
                  </Tag>
                  <Tag v-if="needsUpdate(record.agentVersion)" color="orange">
                    <ExclamationCircleOutlined /> 需要更新
                  </Tag>
                </Space>
              </template>
              <template v-else-if="column.key === 'status'">
                <Tag :color="getStatusTag(record.status).color">
                  {{ getStatusTag(record.status).text }}
                </Tag>
              </template>
              <template v-else-if="column.key === 'lastHeartbeat'">
                {{ formatTime(record.lastHeartbeat) }}
              </template>
              <template v-else-if="column.key === 'action'">
                <Space>
                  <Button v-if="record.status === 1 && needsUpdate(record.agentVersion)" type="primary" size="small"
                    @click="showUpdateModal(record.id)">
                    <DownloadOutlined /> 升级
                  </Button>
                  <span v-else-if="record.status !== 1" style="color: #999">
                    离线
                  </span>
                  <span v-else style="color: #52c41a">
                    {{ latestAgentVersion ? '已是最新版本' : '最新版本' }}
                  </span>
                </Space>
              </template>
            </template>
          </Table>
        </Spin>
      </Card>

      <!-- Agent发布信息 -->
      <Card title="Agent发布信息" size="small">
        <Spin :spinning="loading">
          <Descriptions :column="2" bordered size="small">
            <Descriptions.Item label="最新版本">
              <Tag :color="getVersionColor(releaseInfo?.version || '')">
                {{ releaseInfo && releaseInfo.version ? releaseInfo.version : '未知' }}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="发布时间">
              {{ releaseInfo && releaseInfo.publishedAt ? formatTime(releaseInfo.publishedAt) : '-' }}
            </Descriptions.Item>
            <Descriptions.Item label="仓库" :span="2">
              <template v-if="releaseInfo && releaseInfo.release_repo">
                <a :href="`https://github.com/${releaseInfo.release_repo}`" target="_blank">
                  {{ releaseInfo.release_repo }}
                </a>
              </template>
              <template v-else>-</template>
            </Descriptions.Item>
          </Descriptions>
          <div class="release-notes" v-if="releaseInfo && releaseInfo.notes">
            <p style="margin-top: 12px;">发布说明：</p>
            <pre class="notes">{{ releaseInfo.notes }}</pre>
          </div>
        </Spin>
      </Card>
    </Space>

    <!-- OTA更新确认对话框 -->
    <Modal v-model:open="updateModalVisible" title="确认升级" :confirmLoading="updating" @ok="performUpgrade"
      @cancel="cancelUpdate">
      <p>确定要更新服务器 <strong>{{ getSelectedServer()?.name }}</strong> 的Agent吗？</p>
      <p>当前版本：<Tag>{{ getSelectedServer()?.agentVersion || '未知' }}</Tag>
      </p>
      <p>目标版本：<Tag color="blue">{{ latestAgentVersion || dashboardVersion?.version || '最新' }}</Tag>
      </p>
      <p style="color: #faad14;">
        <ExclamationCircleOutlined /> 更新过程中Agent服务会短暂中断，请确保服务器状态正常。
      </p>
    </Modal>

    <!-- 批量更新确认对话框 -->
    <Modal v-model:open="updateAllModalVisible" title="确认批量升级" :confirmLoading="updatingAll" @ok="performUpdateAll"
      @cancel="cancelUpdateAll">
      <p>确定要更新以下 <strong>{{ serversToUpdate.length }}</strong> 台服务器的Agent吗？</p>
      <div
        style="max-height: 200px; overflow-y: auto; margin: 10px 0; background: #f5f5f5; padding: 10px; border-radius: 4px;">
        <div v-for="server in serversToUpdate" :key="server.id" style="margin-bottom: 4px;">
          <Space>
            <span>{{ server.name }}</span>
            <Tag>{{ server.agentVersion || '未知' }}</Tag>
          </Space>
        </div>
      </div>
      <p>目标版本：<Tag color="blue">{{ latestAgentVersion || dashboardVersion?.version || '最新' }}</Tag>
      </p>
      <p style="color: #faad14;">
        <ExclamationCircleOutlined /> 更新过程中Agent服务会短暂中断，请确保服务器状态正常。
      </p>
    </Modal>
  </div>
</template>

<style scoped>
.version-info {
  width: 100%;
}

code {
  background: #f5f5f5;
  padding: 2px 4px;
  border-radius: 3px;
  font-family: 'Courier New', monospace;
  font-size: 12px;
}

.release-notes .notes {
  background: #f7f7f8;
  border-radius: 6px;
  padding: 12px;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
}
</style>
