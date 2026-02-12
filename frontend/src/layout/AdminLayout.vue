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
  BellOutlined,
  HeartOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { clearLoginInfo, getUser } from '../utils/auth';
import ThemeSwitch from '@/components/ThemeSwitch.vue';
import { getDashboardVersion } from '@/utils/version';

import { useThemeStore } from '@/stores/theme';
import { storeToRefs } from 'pinia';
import GlobalSkeleton from '@/components/GlobalSkeleton.vue';
import { useUIStore } from '@/stores/uiStore';

const router = useRouter();
const route = useRoute();
const collapsed = ref(false);
const user = computed(() => getUser() || {});
const themeStore = useThemeStore();
const { isDark } = storeToRefs(themeStore);
const uiStore = useUIStore();

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
const goToLifeProbes = () => router.push('/admin/life-probes');
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

      <a-menu v-model:selectedKeys="selectedKeys" v-model:openKeys="openKeys" mode="inline"
        :theme="isDark ? 'dark' : 'light'" class="modern-menu">
        <a-menu-item key="/admin/servers" @click="goToServers">
          <template #icon>
            <DesktopOutlined />
          </template>
          <span>服务器管理</span>
        </a-menu-item>
        <a-menu-item key="/admin/life-probes" @click="goToLifeProbes">
          <template #icon>
            <HeartOutlined />
          </template>
          <span>生命探针</span>
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
      <a-layout-header class="modern-header">
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
          <!-- 全局骨架屏 -->
          <transition name="fade">
            <GlobalSkeleton v-if="uiStore.isPageLoading"
              style="position: absolute; z-index: 10; padding: 24px; top: 0; left: 0; width: 100%; height: 100%;" />
          </transition>

          <router-view v-slot="{ Component, route }">
            <transition name="slide-fade" mode="out-in">
              <keep-alive>
                <component :is="Component" :key="route.fullPath" v-if="route.meta.keepAlive"
                  v-show="!uiStore.isPageLoading" />
              </keep-alive>
            </transition>
            <transition name="slide-fade" mode="out-in">
              <component :is="Component" :key="route.fullPath" v-if="!route.meta.keepAlive"
                v-show="!uiStore.isPageLoading" />
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
        <span class="footer-right">&copy; {{ currentYear }}</span>
        <span class="author-text">Designed by EnderKC</span>
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
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-right: 1px solid var(--border-subtle);
  z-index: 100;
}

.modern-sider :deep(.ant-layout-sider-children) {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: transparent;
}

.logo {
  height: var(--header-height);
  padding: 0 20px;
  display: flex;
  align-items: center;
  background: transparent;
}

.logo h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-bold);
  letter-spacing: -0.5px;
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.modern-menu {
  flex: 1;
  border-right: none;
  background: transparent;
  padding: 10px var(--spacing-sm);
}

.modern-menu :deep(.ant-menu-item),
.modern-menu :deep(.ant-menu-submenu-title) {
  margin: var(--spacing-xs) 0;
  width: 100%;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-weight: var(--font-weight-medium);
  height: var(--menu-item-height);
  line-height: var(--menu-item-height);
  transition: var(--transition-fast);
}

.modern-menu :deep(.ant-menu-item:hover),
.modern-menu :deep(.ant-menu-submenu-title:hover) {
  color: var(--text-primary);
  background: var(--alpha-black-03);
}

.modern-menu :deep(.ant-menu-item-selected) {
  background-color: var(--primary-light);
  color: var(--primary-color);
  font-weight: var(--font-weight-semibold);
}

.modern-menu :deep(.ant-menu-item-selected::after) {
  border-right: none;
}

.collapse-trigger {
  height: var(--sidebar-trigger-height);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  cursor: pointer;
  transition: color 0.3s;
  border-top: 1px solid var(--border-subtle);
}

.collapse-trigger:hover {
  color: var(--primary-color);
}

/* Header Style */
.modern-header {
  padding: 0;
  height: var(--header-height);
  line-height: var(--header-height);
  background: var(--header-bg);
  z-index: 99;
  border-radius: 0 0 var(--radius-lg) 0;
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 100%;
  padding: 0 var(--spacing-lg);
}

.header-title {
  display: flex;
  align-items: center;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
  letter-spacing: -0.5px;
}

.header-title span {
  margin-left: var(--spacing-sm);
}

.header-actions {
  display: flex;
  align-items: center;
}

.user-avatar {
  display: flex;
  align-items: center;
  cursor: pointer;
  padding: 0 var(--spacing-sm);
  border-radius: var(--radius-md);
  transition: var(--transition);
  background: var(--header-bg) !important;
  border: 1px solid var(--border-subtle);
}

.user-avatar:hover {
  background: var(--alpha-white-80);
  box-shadow: 0 2px 8px var(--alpha-black-05);
}

.user-avatar-inner {
  background: var(--gradient-brand);
  color: white;
  margin-right: var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.username {
  font-size: var(--font-size-md);
  font-weight: var(--font-weight-medium);
  margin-right: var(--spacing-xs);
  color: var(--text-primary);
}

/* Content Area */
.modern-content {
  padding: var(--spacing-lg);
  background: transparent;
  overflow-y: auto;
}

.content-wrapper {
  min-height: calc(100vh - 130px);
  position: relative;
}

/* Footer */
.modern-footer {
  line-height: 1.1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
  text-align: center;
  padding: var(--spacing-md) var(--spacing-lg);
  color: var(--text-hint);
  font-size: var(--font-size-xs);
  background: var(--footer-bg) !important;
  border-top: 1px solid var(--alpha-black-04);
  transition: background 0.3s ease, border-color 0.3s ease;
}

.footer-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.version-badge {
  font-size: var(--font-size-xs);
  padding: var(--spacing-2xs) 10px;
  border-radius: var(--radius-pill);
  background: var(--primary-light);
  color: var(--text-secondary);
  font-weight: var(--font-weight-semibold);
  letter-spacing: 0.5px;
}


/* User Dropdown */
:deep(.user-dropdown .ant-dropdown-menu) {
  padding: var(--spacing-xs);
  border-radius: var(--radius-md);
  background: var(--dropdown-bg);
  backdrop-filter: blur(var(--blur-md));
  box-shadow: 0 10px 30px var(--alpha-black-10);
  border: 1px solid var(--border-subtle);
}

:deep(.user-dropdown .ant-dropdown-menu-item) {
  border-radius: var(--radius-sm);
  padding: var(--spacing-sm) var(--spacing-sm);
  font-weight: var(--font-weight-medium);
}

:deep(.user-dropdown .ant-dropdown-menu-item:hover) {
  background: var(--primary-light);
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
    padding: 0 var(--spacing-md);
  }

  .modern-content {
    padding: var(--spacing-md);
  }
}

/* Transitions */
.slide-fade-enter-active,
.slide-fade-leave-active {
  transition: all 0.3s ease;
}

.slide-fade-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.slide-fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}
</style>

<style>
.dark .modern-footer {
  background: var(--footer-bg) !important;
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
  background: var(--alpha-white-08);
}

.dark .modern-menu .ant-menu-item-selected {
  background-color: var(--primary-light);
  color: var(--primary-color);
}

.dark .collapse-trigger {
  border-top: 1px solid var(--alpha-white-08);
  color: var(--text-secondary);
}

.dark .collapse-trigger:hover {
  color: var(--primary-color);
  background: var(--alpha-white-05);
}

.dark .user-avatar {
  background: var(--header-bg) !important;
  color: var(--text-primary);
}

.dark .user-avatar-inner {
  background: var(--primary-color);
}

.dark .user-avatar:hover {
  background: var(--alpha-white-15);
  box-shadow: 0 2px 8px var(--alpha-white-10);
}

.dark .username {
  color: var(--text-primary);
}

.dark .user-dropdown .ant-dropdown-menu {
  border: 1px solid var(--border-default);
  box-shadow: 0 4px 12px var(--alpha-black-50);
}

.dark .user-dropdown .ant-dropdown-menu-item {
  color: var(--text-primary);
}

.dark .user-dropdown .ant-dropdown-menu-item:hover {
  background: var(--primary-light);
  color: var(--primary-color);
}

.dark .ant-dropdown-menu-item-divider {
  background-color: var(--border-default);
}

.dark .footer-right {
  color: var(--text-primary);
}

.dark .version-badge {
  background: var(--alpha-white-15);
  color: var(--text-primary);
}

.author-text {
  color: rgba(154, 181, 202, 0.41);
}
</style>
