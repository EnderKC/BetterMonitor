<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import {
  DashboardOutlined,
  UserOutlined,
  LogoutOutlined,
  MenuUnfoldOutlined,
  MenuFoldOutlined,
  DesktopOutlined,
  DownOutlined,
  AppstoreOutlined,
  SettingOutlined,
  BellOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { clearLoginInfo, getUser } from '../utils/auth';
import ThemeSwitch from '@/components/ThemeSwitch.vue';
import { getDashboardVersion } from '@/utils/version';

import { useThemeStore } from '@/stores/theme';
import { storeToRefs } from 'pinia';

const router = useRouter();
const route = useRoute();
const collapsed = ref(false);
const user = computed(() => getUser() || {});
const themeStore = useThemeStore();
const { isDark } = storeToRefs(themeStore);

// 切换菜单收起/展开
const toggleCollapsed = () => {
  collapsed.value = !collapsed.value;
};

// 退出登录
const handleLogout = () => {
  clearLoginInfo();
  message.success('已退出登录');
  router.push('/login');
};

// 获取当前选中的菜单项
const selectedKeys = computed(() => {
  const path = route.path;
  if (path.startsWith('/admin/servers') && path !== '/admin/servers') {
    return ['/admin/servers'];
  }
  if (path.startsWith('/admin/alerts/')) {
    return [path];
  }
  return [path];
});

// 获取当前打开的子菜单
const openKeys = ref(['servers', 'alerts']);

const goToServers = () => router.push('/admin/servers');
const goToDashboard = () => router.push('/dashboard');
const goToProfile = () => router.push('/admin/profile');
const goToSettings = () => router.push('/admin/settings');
const goToAlertSettings = () => router.push('/admin/alerts/settings');
const goToNotificationChannels = () => router.push('/admin/alerts/channels');
const goToAlertRecords = () => router.push('/admin/alerts/records');

const dashboardVersion = ref('');
const currentYear = new Date().getFullYear();

const loadDashboardVersion = async () => {
  try {
    const versionInfo = await getDashboardVersion();
    dashboardVersion.value = versionInfo?.version || '';
  } catch (error) {
    console.error('获取面板版本失败:', error);
  }
};

onMounted(() => {
  loadDashboardVersion();
});
</script>

<template>
  <a-layout class="modern-layout">
    <!-- 侧边栏 -->
    <a-layout-sider v-model:collapsed="collapsed" collapsible class="modern-sider" :trigger="null" :width="220"
      :collapsedWidth="64">
      <div class="logo">
        <h2 class="gradient-text">{{ collapsed ? 'BM' : 'Better Monitor' }}</h2>
      </div>

      <a-menu v-model:selectedKeys="selectedKeys" v-model:openKeys="openKeys" mode="inline" :theme="isDark ? 'dark' : 'light'"
        class="modern-menu">
        <a-menu-item key="/admin/servers" @click="goToServers">
          <template #icon>
            <DesktopOutlined />
          </template>
          <span>服务器管理</span>
        </a-menu-item>

        <a-sub-menu key="alerts">
          <template #icon>
            <BellOutlined />
          </template>
          <template #title>预警管理</template>
          <a-menu-item key="/admin/alerts/settings" @click="goToAlertSettings">
            预警设置
          </a-menu-item>
          <a-menu-item key="/admin/alerts/channels" @click="goToNotificationChannels">
            通知渠道
          </a-menu-item>
          <a-menu-item key="/admin/alerts/records" @click="goToAlertRecords">
            预警记录
          </a-menu-item>
        </a-sub-menu>

        <a-menu-item key="/dashboard" @click="goToDashboard">
          <template #icon>
            <DashboardOutlined />
          </template>
          <span>探针页面</span>
        </a-menu-item>

        <a-menu-item key="/admin/profile" @click="goToProfile">
          <template #icon>
            <UserOutlined />
          </template>
          <span>个人资料</span>
        </a-menu-item>

        <a-menu-item key="/admin/settings" @click="goToSettings" v-if="user.role === 'admin'">
          <template #icon>
            <SettingOutlined />
          </template>
          <span>系统设置</span>
        </a-menu-item>

        <a-menu-item key="logout" @click="handleLogout">
          <template #icon>
            <LogoutOutlined />
          </template>
          <span>退出登录</span>
        </a-menu-item>
      </a-menu>

      <div class="collapse-trigger" @click="toggleCollapsed">
        <MenuUnfoldOutlined v-if="collapsed" />
        <MenuFoldOutlined v-else />
      </div>
    </a-layout-sider>

    <!-- 内容区 -->
    <a-layout>
      <!-- 头部 -->
      <a-layout-header class="modern-header glass-card">
        <div class="header-content">
          <div class="header-title">
            <AppstoreOutlined />
            <span>{{ route.meta.title || '服务器管理系统' }}</span>
          </div>

          <div class="header-actions">
            <ThemeSwitch style="margin-right: 16px" />
            <a-dropdown :trigger="['click']" overlay-class-name="user-dropdown">
              <div class="user-avatar">
                <a-avatar :size="28" class="user-avatar-inner">{{ user.username ? user.username.charAt(0).toUpperCase()
                  :
                  'U' }}</a-avatar>
                <span class="username">{{ user.username || '用户' }}</span>
                <DownOutlined />
              </div>
              <template #overlay>
                <a-menu class="modern-dropdown-menu glass-card">
                  <a-menu-item key="profile" @click="goToProfile">
                    <UserOutlined /> 个人资料
                  </a-menu-item>
                  <a-menu-divider />
                  <a-menu-item key="logout" @click="handleLogout">
                    <LogoutOutlined /> 退出登录
                  </a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
          </div>
        </div>
      </a-layout-header>

      <!-- 内容 -->
      <a-layout-content class="modern-content">
        <div class="content-wrapper">
          <router-view v-slot="{ Component }">
            <transition name="fade">
              <keep-alive include="ServerList,Dashboard">
                <component :is="Component" :key="route.fullPath" />
              </keep-alive>
            </transition>
          </router-view>
        </div>
      </a-layout-content>

      <!-- 页脚 -->
      <a-layout-footer class="modern-footer">
        <div class="footer-left">
          <span class="gradient-text">Better-Monitor</span>
          <span class="version-badge">
            面板 {{ dashboardVersion ? 'v' + dashboardVersion : '版本未知' }}
          </span>
        </div>
        <span>&copy; {{ currentYear }}</span>
      </a-layout-footer>
    </a-layout>
  </a-layout>
</template>

<style scoped>
.modern-layout {
  min-height: 100vh;
  background: transparent;
}

/* Sidebar Style - macOS Sidebar */
.modern-sider {
  background: var(--sidebar-bg) !important;
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-right: 1px solid rgba(0, 0, 0, 0.05);
  z-index: 100;
}

.modern-sider :deep(.ant-layout-sider-children) {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: transparent;
}

.logo {
  height: 60px;
  padding: 0 20px;
  display: flex;
  align-items: center;
  background: transparent;
}

.logo h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.5px;
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.modern-menu {
  flex: 1;
  border-right: none;
  background: transparent;
  padding: 10px 12px;
}

.modern-menu :deep(.ant-menu-item),
.modern-menu :deep(.ant-menu-submenu-title) {
  margin: 4px 0;
  width: 100%;
  border-radius: 8px;
  color: var(--text-secondary);
  font-weight: 500;
  height: 40px;
  line-height: 40px;
  transition: all 0.2s;
}

.modern-menu :deep(.ant-menu-item:hover),
.modern-menu :deep(.ant-menu-submenu-title:hover) {
  color: var(--text-primary);
  background: rgba(0, 0, 0, 0.03);
}

.modern-menu :deep(.ant-menu-item-selected) {
  background-color: rgba(0, 122, 255, 0.1);
  color: var(--primary-color);
  font-weight: 600;
}

.modern-menu :deep(.ant-menu-item-selected::after) {
  border-right: none;
}

.collapse-trigger {
  height: 50px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  cursor: pointer;
  transition: color 0.3s;
  border-top: 1px solid rgba(0, 0, 0, 0.05);
}

.collapse-trigger:hover {
  color: var(--primary-color);
}

/* Header Style */
.modern-header {
  padding: 0;
  height: 60px;
  line-height: 60px;
  background: var(--header-bg);
  z-index: 99;
  /* 覆盖一下 毛玻璃卡片的圆角 */
  border-radius: 0 0 16px 0;
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 100%;
  padding: 0 24px;
}

.header-title {
  display: flex;
  align-items: center;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: -0.5px;
}

.header-title span {
  margin-left: 12px;
}

.header-actions {
  display: flex;
  align-items: center;
}

.user-avatar {
  display: flex;
  align-items: center;
  cursor: pointer;
  padding: 0 12px;
  border-radius: 12px;
  transition: all 0.3s;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.user-avatar:hover {
  background: rgba(255, 255, 255, 0.861);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
}

.user-avatar-inner {
  background: linear-gradient(135deg, #007AFF, #5856D6);
  color: white;
  margin-right: 8px;
  font-size: 12px;
}

.username {
  font-size: 14px;
  font-weight: 500;
  margin-right: 6px;
  color: var(--text-primary);
}

/* Content Area */
.modern-content {
  padding: 24px;
  background: transparent;
  overflow-y: auto;
}

.content-wrapper {
  min-height: calc(100vh - 130px);
}

/* Footer */
.modern-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  text-align: center;
  padding: 16px 24px;
  color: var(--text-hint);
  font-size: 12px;
  background: var(--body-bg);
  border-top: 1px solid rgba(0, 0, 0, 0.04);
  box-shadow: 0 -12px 30px rgba(0, 0, 0, 0.06);
  transition: background 0.3s ease, border-color 0.3s ease;
}

.footer-left {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.version-badge {
  font-size: 12px;
  padding: 2px 10px;
  border-radius: 999px;
  background: rgba(0, 122, 255, 0.1);
  color: var(--text-secondary);
  font-weight: 600;
  letter-spacing: 0.5px;
}


/* User Dropdown */
:deep(.user-dropdown .ant-dropdown-menu) {
  padding: 6px;
  border-radius: 12px;
  background: var(--dropdown-bg);
  backdrop-filter: blur(20px);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.1);
  border: 1px solid rgba(0, 0, 0, 0.05);
}

:deep(.user-dropdown .ant-dropdown-menu-item) {
  border-radius: 8px;
  padding: 8px 12px;
  font-weight: 500;
}

:deep(.user-dropdown .ant-dropdown-menu-item:hover) {
  background: rgba(0, 122, 255, 0.1);
  color: var(--primary-color);
}

/* Responsive */
@media (max-width: 768px) {
  .username {
    display: none;
  }

  .modern-header {
    padding: 0;
  }

  .header-content {
    padding: 0 16px;
  }

  .modern-content {
    padding: 16px;
  }
}

/* Transitions */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

</style>

<style>
.dark .modern-footer {
  background: linear-gradient(180deg, rgba(24, 27, 37, 0.98), rgba(12, 14, 20, 0.98));
  border-top: 1px solid rgba(255, 255, 255, 0.04);
  box-shadow: 0 -16px 40px rgba(0, 0, 0, 0.5);
  color: var(--text-hint);
}

.dark .modern-menu {
  background: transparent;
}

.dark .modern-menu .ant-menu-item,
.dark .modern-menu .ant-menu-submenu-title {
  color: var(--text-secondary);
}

.dark .modern-menu .ant-menu-item:hover,
.dark .modern-menu .ant-menu-submenu-title:hover {
  color: var(--text-primary);
  background: rgba(255, 255, 255, 0.08);
}

.dark .modern-menu .ant-menu-item-selected {
  background-color: rgba(97, 175, 239, 0.2);
  color: var(--primary-color);
}

.dark .collapse-trigger {
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  color: var(--text-secondary);
}

.dark .collapse-trigger:hover {
  color: var(--primary-color);
  background: rgba(255, 255, 255, 0.05);
}

.dark .user-avatar {
  background: rgba(33, 37, 43, 0.8);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: var(--text-primary);
}

.dark .user-avatar:hover {
  background: rgba(255, 255, 255, 0.15);
}

.dark .username {
  color: var(--text-primary);
}

.dark .user-dropdown .ant-dropdown-menu {
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.5);
}

.dark .user-dropdown .ant-dropdown-menu-item {
  color: var(--text-primary);
}

.dark .user-dropdown .ant-dropdown-menu-item:hover {
  background: rgba(97, 175, 239, 0.15);
  color: var(--primary-color);
}

.dark .ant-dropdown-menu-item-divider {
  background-color: rgba(255, 255, 255, 0.1);
}

.dark .version-badge {
  background: rgba(255, 255, 255, 0.15);
  color: var(--text-primary);
}
</style>
