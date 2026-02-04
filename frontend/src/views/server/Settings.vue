<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { message } from 'ant-design-vue';
import { useRouter } from 'vue-router';
import service from '../../utils/request';
import {
  SettingOutlined,
  ReloadOutlined,
  SaveOutlined,
  InfoCircleOutlined,
  CloudServerOutlined,
  DesktopOutlined,
  DatabaseOutlined,
  CloudSyncOutlined
} from '@ant-design/icons-vue';
import { useUserStore } from '../../stores/userStore';
import { useSettingsStore } from '../../stores/settingsStore';
import { useUIStore } from '../../stores/uiStore';
import VersionInfo from '../../components/VersionInfo.vue';

const userStore = useUserStore();
const router = useRouter();
const settingsStore = useSettingsStore();
const uiStore = useUIStore();

const ensureAdminAccess = async () => {
  console.log('[Settings] 开始检查管理员权限');
  console.log('[Settings] 当前管理员状态:', userStore.isAdmin);
  console.log('[Settings] 当前用户信息:', userStore.userInfo);
  console.log('[Settings] 当前token:', userStore.token ? '已设置' : '未设置');

  // 如果当前状态不是管理员，尝试刷新一次用户信息
  if (!userStore.isAdmin) {
    console.log('[Settings] 不是管理员，尝试刷新用户信息');
    const success = await userStore.getUserInfo(true);
    console.log('[Settings] 刷新用户信息结果:', success);
    console.log('[Settings] 刷新后的管理员状态:', userStore.isAdmin);
    console.log('[Settings] 刷新后的用户信息:', userStore.userInfo);
  }

  if (!userStore.isAdmin) {
    console.error('[Settings] 验证失败：不是管理员');
    message.error('只有管理员才能访问此页面');
    router.push('/');
    return false;
  }

  console.log('[Settings] 管理员权限验证通过');
  return true;
};

// 设置表单
const form = reactive({
  monitor_interval: '30s',
  ui_refresh_interval: '10s',
  chart_history_hours: 24,
  data_retention_days: 7,
  life_data_retention_days: 7,
  allow_public_life_probe_access: true,
  agent_release_repo: '',
  agent_release_channel: 'stable',
  agent_release_mirror: ''
});

// 页面状态
const loading = ref(false);
const saving = ref(false);
const activeTab = ref('agent');

// 持续时间选项
const durationOptions = [
  { value: '1s', label: '1秒' },
  { value: '5s', label: '5秒' },
  { value: '10s', label: '10秒' },
  { value: '15s', label: '15秒' },
  { value: '30s', label: '30秒' },
  { value: '1m', label: '1分钟' },
  { value: '2m', label: '2分钟' },
  { value: '5m', label: '5分钟' },
  { value: '10m', label: '10分钟' },
  { value: '15m', label: '15分钟' },
  { value: '30m', label: '30分钟' },
  { value: '1h', label: '1小时' }
];

// 历史数据小时数选项
const historyHoursOptions = [
  { value: 1, label: '1小时' },
  { value: 3, label: '3小时' },
  { value: 6, label: '6小时' },
  { value: 12, label: '12小时' },
  { value: 24, label: '1天(24小时)' },
  { value: 48, label: '2天(48小时)' },
  { value: 72, label: '3天(72小时)' },
  { value: 168, label: '1周(7天)' }
];

const releaseChannelOptions = [
  { value: 'stable', label: '稳定版' },
  { value: 'prerelease', label: '预发布' },
  { value: 'nightly', label: '开发版' }
];

// 获取设置
const loadSettings = async () => {
  loading.value = true;
  try {
    const settings = await service.get<{
      monitor_interval?: string;
      ui_refresh_interval?: string;
      chart_history_hours?: number;
      data_retention_days?: number;
      life_data_retention_days?: number;
      allow_public_life_probe_access?: boolean;
      agent_release_repo?: string;
      agent_release_channel?: string;
      agent_release_mirror?: string;
    }>('admin/settings');

    // 设置表单值
    if (settings.monitor_interval !== undefined) {
      form.monitor_interval = settings.monitor_interval;
    }

    if (settings.ui_refresh_interval !== undefined) {
      form.ui_refresh_interval = settings.ui_refresh_interval;
    }

    if (settings.chart_history_hours !== undefined) {
      form.chart_history_hours = settings.chart_history_hours;
    }

    if (settings.data_retention_days !== undefined) {
      form.data_retention_days = settings.data_retention_days;
    }

    if (settings.life_data_retention_days !== undefined) {
      form.life_data_retention_days = settings.life_data_retention_days;
    }

    if (settings.allow_public_life_probe_access !== undefined) {
      form.allow_public_life_probe_access = settings.allow_public_life_probe_access;
    }

    if (settings.agent_release_repo !== undefined) {
      form.agent_release_repo = settings.agent_release_repo;
    }

    if (settings.agent_release_channel !== undefined) {
      form.agent_release_channel = settings.agent_release_channel;
    }

    if (settings.agent_release_mirror !== undefined) {
      form.agent_release_mirror = settings.agent_release_mirror;
    }

    message.success('加载系统设置成功');
  } catch (error) {
    console.error('加载系统设置失败:', error);
    message.error('加载系统设置失败');
  } finally {
    loading.value = false;
  }
};

// 表单验证
const validateForm = () => {
  // 检查监控间隔
  if (!form.monitor_interval) {
    message.error('请选择监控数据上报间隔');
    return false;
  }

  // 检查UI刷新间隔
  if (!form.ui_refresh_interval) {
    message.error('请选择探针页面刷新间隔');
    return false;
  }

  // 检查数据保留天数
  if (form.data_retention_days === undefined || form.data_retention_days < 1) {
    message.error('数据保留天数必须大于等于1');
    return false;
  }

  if (form.life_data_retention_days === undefined || form.life_data_retention_days < 1) {
    message.error('生命探针数据保留天数必须大于等于1');
    return false;
  }

  if (!form.agent_release_repo) {
    message.error('请配置Agent发布仓库');
    return false;
  }

  return true;
};

// 保存设置
const saveSettings = async () => {
  // 验证表单
  if (!validateForm()) {
    return;
  }

  saving.value = true;
  try {
    console.log('正在保存设置:', form);

    // 调整API路径
    const response = await service.put('admin/settings', form);
    console.log('保存设置响应:', response);

    // 直接检查响应是否存在 - axios拦截器已经返回了response.data
    if (response) {
      message.success('设置已保存');
      // 重新获取设置以确认更改已应用
      await loadSettings();
      // 更新settingsStore
      settingsStore.updateSettings(form);
    } else {
      message.error('保存设置失败: 服务器响应为空');
    }
  } catch (error) {
    console.error('保存设置出错:', error);
    message.error(`保存设置失败: ${error instanceof Error ? error.message : '未知错误'}`);
  } finally {
    saving.value = false;
  }
};

// 页面初始化
onMounted(async () => {
  const hasAccess = await ensureAdminAccess();
  if (!hasAccess) {
    return;
  }

  // 先加载settingsStore的值（如果已有）
  if (settingsStore.loaded) {
    form.monitor_interval = settingsStore.monitorInterval;
    form.ui_refresh_interval = settingsStore.uiRefreshInterval;
    form.chart_history_hours = settingsStore.chartHistoryHours;
    form.data_retention_days = settingsStore.dataRetentionDays;
    form.life_data_retention_days = settingsStore.lifeDataRetentionDays;
    form.agent_release_repo = settingsStore.agentReleaseRepo || form.agent_release_repo;
    form.agent_release_channel = settingsStore.agentReleaseChannel || form.agent_release_channel;
    form.agent_release_mirror = settingsStore.agentReleaseMirror || form.agent_release_mirror;
  }

  // 然后获取最新设置
  try {
    await loadSettings();
  } finally {
    uiStore.stopLoading();
  }
});
</script>

<template>
  <div class="settings-container">
    <div class="page-header">
      <h1 class="page-title">系统设置</h1>
      <p class="page-subtitle">配置系统各项参数，调整心跳、监控和数据保留策略</p>
    </div>

    <div class="settings-layout">
      <!-- 左侧导航 -->
      <div class="settings-sidebar">
        <div class="ios-card sidebar-card">
          <div class="sidebar-item" :class="{ active: activeTab === 'agent' }" @click="activeTab = 'agent'">
            <div class="sidebar-icon"><cloud-server-outlined /></div>
            <span>Agent 设置</span>
          </div>
          <div class="sidebar-item" :class="{ active: activeTab === 'ui' }" @click="activeTab = 'ui'">
            <div class="sidebar-icon"><desktop-outlined /></div>
            <span>前端设置</span>
          </div>
          <div class="sidebar-item" :class="{ active: activeTab === 'data' }" @click="activeTab = 'data'">
            <div class="sidebar-icon"><database-outlined /></div>
            <span>数据策略</span>
          </div>
          <div class="sidebar-item" :class="{ active: activeTab === 'release' }" @click="activeTab = 'release'">
            <div class="sidebar-icon"><cloud-sync-outlined /></div>
            <span>Agent 发布</span>
          </div>
          <div class="sidebar-item" :class="{ active: activeTab === 'version' }" @click="activeTab = 'version'">
            <div class="sidebar-icon"><info-circle-outlined /></div>
            <span>版本信息</span>
          </div>
        </div>
      </div>

      <!-- 右侧内容 -->
      <div class="settings-content">
        <a-spin :spinning="loading">
          <!-- Agent 设置 -->
          <div v-if="activeTab === 'agent'" class="ios-card content-card">
            <div class="card-header">
              <h3 class="card-title">Agent 设置</h3>
              <p class="card-desc">配置 Agent 与服务器的通信频率</p>
            </div>
            <div class="card-body">
              <a-form layout="vertical" class="ios-form">
                <div class="form-section">
                  <a-form-item label="监控数据上报间隔">
                    <a-select v-model:value="form.monitor_interval" :options="durationOptions" class="ios-select" />
                    <div class="form-help">Agent向服务器上报监控数据（CPU、内存、磁盘等）的时间间隔</div>
                  </a-form-item>
                </div>

                <div class="form-actions">
                  <a-button type="primary" class="ios-btn ios-btn-primary" :loading="saving" @click="saveSettings">
                    <template #icon><save-outlined /></template>
                    保存更改
                  </a-button>
                </div>
              </a-form>
            </div>
          </div>

          <!-- 前端设置 -->
          <div v-if="activeTab === 'ui'" class="ios-card content-card">
            <div class="card-header">
              <h3 class="card-title">前端设置</h3>
              <p class="card-desc">配置界面的刷新频率</p>
            </div>
            <div class="card-body">
              <a-form layout="vertical" class="ios-form">
                <div class="form-section">
                  <a-form-item label="探针页面刷新间隔">
                    <a-select v-model:value="form.ui_refresh_interval" :options="durationOptions" class="ios-select" />
                    <div class="form-help">前端监控图表和探针页面数据自动刷新的时间间隔</div>
                  </a-form-item>
                </div>

                <div class="form-actions">
                  <a-button type="primary" class="ios-btn ios-btn-primary" :loading="saving" @click="saveSettings">
                    <template #icon><save-outlined /></template>
                    保存更改
                  </a-button>
                </div>
              </a-form>
            </div>
          </div>

          <!-- 数据策略 -->
          <div v-if="activeTab === 'data'" class="ios-card content-card">
            <div class="card-header">
              <h3 class="card-title">数据策略</h3>
              <p class="card-desc">管理历史数据的保留和清理策略</p>
            </div>
            <div class="card-body">
              <a-form layout="vertical" class="ios-form">
                <div class="form-section">
                  <a-form-item label="历史数据显示时间">
                    <a-select v-model:value="form.chart_history_hours" :options="historyHoursOptions"
                      class="ios-select" />
                    <div class="form-help">历史监控数据的保留时间，超出将被自动清理</div>
                  </a-form-item>

                  <a-form-item label="数据保留天数">
                    <a-input-number v-model:value="form.data_retention_days" :min="1" :max="90"
                      class="ios-input-number" />
                    <div class="form-help">历史监控数据的保留天数，超出将被自动清理</div>
                  </a-form-item>

                  <a-form-item label="生命探针数据保留天数">
                    <a-input-number v-model:value="form.life_data_retention_days" :min="1" :max="180"
                      class="ios-input-number" />
                    <div class="form-help">生命探针上报的健康数据保留天数</div>
                  </a-form-item>

                  <a-form-item label="允许公开访问生命探针">
                    <a-switch v-model:checked="form.allow_public_life_probe_access" checked-children="开启"
                      un-checked-children="关闭" />
                    <div class="form-help">是否允许未登录用户通过公开链接访问生命探针详情页面（需要探针本身也开启公开）</div>
                  </a-form-item>
                </div>

                <div class="form-actions">
                  <a-button type="primary" class="ios-btn ios-btn-primary" :loading="saving" @click="saveSettings">
                    <template #icon><save-outlined /></template>
                    保存更改
                  </a-button>
                </div>
              </a-form>
            </div>
          </div>

          <!-- Agent 发布配置 -->
          <div v-if="activeTab === 'release'" class="ios-card content-card">
            <div class="card-header">
              <h3 class="card-title">Agent 发布配置</h3>
              <p class="card-desc">配置Agent升级所使用的GitHub仓库与通道</p>
            </div>
            <div class="card-body">
              <a-form layout="vertical" class="ios-form">
                <div class="form-section">
                  <a-form-item label="Release仓库">
                    <a-input v-model:value="form.agent_release_repo" placeholder="例如: your-org/better-monitor-agent"
                      class="ios-input" />
                    <div class="form-help">用于读取Agent发布版本的GitHub仓库</div>
                  </a-form-item>

                  <a-form-item label="发布通道">
                    <a-select v-model:value="form.agent_release_channel" :options="releaseChannelOptions"
                      class="ios-select" />
                    <div class="form-help">选择默认使用的发布通道（稳定版/预发布/开发版）</div>
                  </a-form-item>

                  <a-form-item label="下载镜像（可选）">
                    <a-input v-model:value="form.agent_release_mirror" placeholder="例如: https://download.fastgit.org"
                      class="ios-input" />
                    <div class="form-help">可选，替换GitHub下载域名以提升下载速度</div>
                  </a-form-item>
                </div>

                <div class="form-actions">
                  <a-button type="primary" class="ios-btn ios-btn-primary" :loading="saving" @click="saveSettings">
                    <template #icon><save-outlined /></template>
                    保存更改
                  </a-button>
                </div>
              </a-form>
            </div>
          </div>

          <!-- 版本信息 -->
          <div v-if="activeTab === 'version'" class="ios-card content-card">
            <div class="card-header">
              <h3 class="card-title">版本信息</h3>
              <p class="card-desc">查看当前系统版本</p>
            </div>
            <div class="card-body">
              <VersionInfo />
            </div>
          </div>
        </a-spin>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-container {
  padding: 24px;
  min-height: 100%;
}

.page-header {
  margin-bottom: 32px;
}

.page-title {
  font-size: 28px;
  font-weight: 700;
  color: rgba(0, 0, 0, 0.85);
  margin-bottom: 8px;
  letter-spacing: -0.5px;
}

.page-subtitle {
  font-size: 14px;
  color: rgba(0, 0, 0, 0.45);
}

.settings-layout {
  display: flex;
  gap: 24px;
  align-items: flex-start;
}

/* Sidebar */
.settings-sidebar {
  width: 240px;
  flex-shrink: 0;
}

.sidebar-card {
  padding: 12px;
}

.sidebar-item {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  margin-bottom: 4px;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
  color: rgba(0, 0, 0, 0.65);
  font-weight: 500;
}

.sidebar-item:hover {
  background: rgba(0, 0, 0, 0.04);
  color: rgba(0, 0, 0, 0.85);
}

.sidebar-item.active {
  background: #e6f7ff;
  color: #1890ff;
}

.sidebar-icon {
  margin-right: 12px;
  font-size: 18px;
  display: flex;
  align-items: center;
}

/* Content */
.settings-content {
  flex: 1;
  min-width: 0;
}

/* iOS Card Style */
.ios-card {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: 20px;
  border: 1px solid rgba(255, 255, 255, 0.3);
  box-shadow:
    0 4px 24px -1px rgba(0, 0, 0, 0.05),
    0 0 1px 0 rgba(0, 0, 0, 0.1);
  overflow: hidden;
}

.content-card {
  min-height: 400px;
}

.card-header {
  padding: 24px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
}

.card-title {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 4px 0;
  color: rgba(0, 0, 0, 0.85);
}

.card-desc {
  font-size: 14px;
  color: rgba(0, 0, 0, 0.45);
  margin: 0;
}

.card-body {
  padding: 24px;
}

/* Form Styles */
.form-section {
  max-width: 600px;
}

.ios-form :deep(.ant-form-item-label > label) {
  font-size: 14px;
  color: rgba(0, 0, 0, 0.65);
  font-weight: 500;
}

.ios-input,
.ios-select,
.ios-input-number {
  width: 100%;
}

.ios-input {
  border-radius: 10px;
  border-color: rgba(0, 0, 0, 0.15);
  padding: 8px 12px;
  background: rgba(255, 255, 255, 0.5);
  transition: all 0.3s;
}

.ios-input:hover,
.ios-input:focus {
  background: #fff;
  border-color: #1890ff;
  box-shadow: 0 0 0 2px rgba(24, 144, 255, 0.1);
}

:deep(.ant-select-selector) {
  border-radius: 10px !important;
  border-color: rgba(0, 0, 0, 0.15) !important;
  background: rgba(255, 255, 255, 0.5) !important;
}

:deep(.ant-input-number) {
  border-radius: 10px;
  border-color: rgba(0, 0, 0, 0.15);
  background: rgba(255, 255, 255, 0.5);
  width: 100%;
}

.form-help {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.45);
  margin-top: 4px;
}

.form-actions {
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid rgba(0, 0, 0, 0.06);
  display: flex;
  align-items: center;
}

/* iOS Buttons */
.ios-btn {
  height: 40px;
  border-radius: 20px;
  font-weight: 500;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  border: none;
  transition: all 0.3s;
  padding: 0 24px;
}

.ios-btn-primary {
  background: linear-gradient(135deg, #1890ff 0%, #096dd9 100%);
}

.ios-btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(24, 144, 255, 0.3);
}

.ios-btn-secondary {
  background: rgba(0, 0, 0, 0.04);
  color: rgba(0, 0, 0, 0.65);
  box-shadow: none;
}

.ios-btn-secondary:hover {
  background: rgba(0, 0, 0, 0.08);
  color: rgba(0, 0, 0, 0.85);
}

.ml-16 {
  margin-left: 16px;
}

/* Responsive */
@media (max-width: 768px) {
  .settings-layout {
    flex-direction: column;
  }

  .settings-sidebar {
    width: 100%;
  }

  .sidebar-card {
    display: flex;
    overflow-x: auto;
    padding: 8px;
  }

  .sidebar-item {
    flex-shrink: 0;
    margin-bottom: 0;
    margin-right: 8px;
  }
}
</style>

<style>
.dark .page-title {
  color: #e0e0e0;
}

.dark .page-subtitle {
  color: #8c8c8c;
}

.dark .sidebar-item {
  color: #ccc;
}

.dark .sidebar-item:hover {
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
}

.dark .sidebar-item.active {
  background: rgba(24, 144, 255, 0.2);
  color: #177ddc;
}

.dark .ios-card {
  background: rgba(30, 30, 30, 0.7);
  border: 1px solid rgba(255, 255, 255, 0.05);
  box-shadow: 0 4px 24px -1px rgba(0, 0, 0, 0.2);
}

.dark .card-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.dark .card-title {
  color: #e0e0e0;
}

.dark .card-desc {
  color: #8c8c8c;
}

.dark .ios-form .ant-form-item-label>label {
  color: #ccc;
}

.dark .ios-input {
  background: rgba(0, 0, 0, 0.2);
  border-color: rgba(255, 255, 255, 0.1);
  color: #e0e0e0;
}

.dark .ios-input:hover,
.dark .ios-input:focus {
  background: rgba(0, 0, 0, 0.4);
  border-color: #177ddc;
}

.dark .ant-select-selector {
  background: rgba(0, 0, 0, 0.2) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
  color: #e0e0e0 !important;
}

.dark .ant-input-number {
  background: rgba(0, 0, 0, 0.2);
  border-color: rgba(255, 255, 255, 0.1);
  color: #e0e0e0;
}

.dark .ant-input-number-input {
  color: #e0e0e0;
}

.dark .form-help {
  color: #8c8c8c;
}

.dark .form-actions {
  border-top: 1px solid rgba(255, 255, 255, 0.05);
}

.dark .ios-btn-secondary {
  background: rgba(255, 255, 255, 0.1);
  color: #ccc;
}

.dark .ios-btn-secondary:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}
</style>
