<script setup lang="ts">
import { computed } from 'vue';
import { ConfigProvider, theme } from 'ant-design-vue';
import { useThemeStore } from '@/stores/theme';
import { storeToRefs } from 'pinia';
import zhCN from 'ant-design-vue/es/locale/zh_CN';

const themeStore = useThemeStore();
const { isDark } = storeToRefs(themeStore);

const themeAlgorithm = computed(() => {
  return isDark.value ? theme.darkAlgorithm : theme.defaultAlgorithm;
});

const themeConfig = computed(() => ({
  algorithm: themeAlgorithm.value,
  token: {
    colorPrimary: '#007AFF',
    borderRadius: 8,
    wireframe: false,
  },
}));
</script>

<template>
  <ConfigProvider :locale="zhCN" :theme="themeConfig">
    <div class="modern-ui">
      <router-view v-slot="{ Component }">
        <transition name="fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </div>
  </ConfigProvider>
</template>

<style>
/* Fade Transition */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

:root {
  /* macOS Color Palette */
  --primary-color: #007AFF;
  /* macOS Blue */
  --primary-hover: #0077ED;
  --primary-light: rgba(0, 122, 255, 0.1);
  --primary-bg: rgba(0, 122, 255, 0.05);

  --success-color: #34C759;
  /* macOS Green */
  --warning-color: #FF9500;
  /* macOS Orange */
  --error-color: #FF3B30;
  /* macOS Red */
  --info-color: #5856D6;
  /* macOS Purple */

  /* Neutral Colors */
  --body-bg: #F5F5F7;
  /* macOS System Gray 6 */
  --card-bg: rgba(255, 255, 255, 0.6);
  --card-border: rgba(255, 255, 255, 0.4);

  /* Text Colors */
  --text-primary: #1D1D1F;
  --text-secondary: #8e8e9d;
  --text-hint: #A1A1A6;

  /* Shadows */
  --shadow-sm: 0 2px 8px rgba(0, 0, 0, 0.04);
  --shadow-md: 0 8px 24px rgba(0, 0, 0, 0.08);
  --shadow-lg: 0 16px 48px rgba(0, 0, 0, 0.16);
  --shadow-glow: 0 0 20px rgba(0, 122, 255, 0.3);

  /* Radius */
  --radius-sm: 8px;
  --radius-md: 12px;
  --radius-lg: 16px;
  --radius-xl: 24px;

  /* Spacing */
  --spacing-xs: 4px;
  --spacing-sm: 8px;
  --spacing-md: 16px;
  --spacing-lg: 24px;
  --spacing-xl: 32px;

  /* Transition */
  --transition: all 0.3s cubic-bezier(0.25, 0.1, 0.25, 1);

  /* Component Backgrounds */
  --sidebar-bg: rgba(255, 255, 255, 0.5);
  --dropdown-bg: rgba(255, 255, 255, 0.9);
  --footer-bg: rgba(255, 255, 255, 0.5);
  --header-bg: rgba(255, 255, 255, 0.5);
}

/* Dark Mode (One Dark Style) */
:root.dark {
  --primary-color: #61afef;
  --primary-hover: #4d8ec4;
  --primary-light: rgba(97, 175, 239, 0.15);
  --primary-bg: rgba(97, 175, 239, 0.05);

  --success-color: #98c379;
  --warning-color: #e5c07b;
  --error-color: #e06c75;
  --info-color: #c678dd;

  /* One Dark Backgrounds */
  --body-bg: #282c34;
  --card-bg: rgba(44, 49, 60, 0.7);
  /* #2c313c with opacity */
  --card-border: rgba(255, 255, 255, 0.1);

  /* Text Colors */
  --text-primary: #c3cad8;
  --text-secondary: #8a93a4;
  --text-hint: #4b5263;

  /* Shadows */
  --shadow-sm: 0 2px 8px rgba(0, 0, 0, 0.2);
  --shadow-md: 0 8px 24px rgba(0, 0, 0, 0.3);
  --shadow-lg: 0 16px 48px rgba(0, 0, 0, 0.4);
  --shadow-glow: 0 0 20px rgba(97, 175, 239, 0.2);

  /* Component Backgrounds */
  --sidebar-bg: rgba(33, 37, 43, 0.5);
  --dropdown-bg: #2c313c;
  --footer-bg: rgba(33, 37, 43, 0.5);
  --header-bg: rgba(33, 37, 43, 0.5);
}

/* Global Reset & Typography */
html,
body {
  margin: 0;
  padding: 0;
  height: 100%;
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", Helvetica, Arial, sans-serif;
  background-color: var(--body-bg);
  color: var(--text-primary);
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  transition: background-color 0.3s, color 0.3s;
}

#app {
  height: 100vh;
}

.modern-ui {
  min-height: 100vh;
  background-color: var(--body-bg);
  background-image:
    radial-gradient(circle at 10% 20%, rgba(0, 122, 255, 0.15) 0%, transparent 40%),
    radial-gradient(circle at 90% 80%, rgba(52, 199, 89, 0.15) 0%, transparent 40%),
    radial-gradient(circle at 50% 50%, rgba(88, 86, 214, 0.1) 0%, transparent 60%);
  background-attachment: fixed;
  transition: background-image 0.3s, background-color 0.3s;
}

:root.dark .modern-ui {
  background-image:
    radial-gradient(circle at 10% 20%, rgba(97, 175, 239, 0.15) 0%, transparent 40%),
    radial-gradient(circle at 90% 80%, rgba(152, 195, 121, 0.15) 0%, transparent 40%),
    radial-gradient(circle at 50% 50%, rgba(198, 120, 221, 0.1) 0%, transparent 60%);
}

/* Glassmorphism Utility */
.glass-card {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  border: 1px solid rgba(255, 255, 255, 0.4);
  transition: var(--transition);
}

:root.dark .glass-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.glass-card:hover {
  box-shadow: var(--shadow-md);
  transform: translateY(-2px);
  border-color: rgba(255, 255, 255, 0.8);
}

:root.dark .glass-card:hover {
  border-color: rgba(255, 255, 255, 0.15);
}

/* Ant Design Overrides */

/* Buttons */
.ant-btn {
  border-radius: var(--radius-md);
  font-weight: 500;
  box-shadow: none;
  border: none;
  height: 36px;
  padding: 4px 16px;
}

.ant-btn-primary {
  background: var(--primary-color);
  box-shadow: 0 4px 12px rgba(0, 122, 255, 0.3);
}

:root.dark .ant-btn-primary {
  box-shadow: 0 4px 12px rgba(97, 175, 239, 0.2);
}

.ant-btn-primary:hover {
  background: var(--primary-hover);
  box-shadow: 0 6px 16px rgba(0, 122, 255, 0.4);
}

:root.dark .ant-btn-primary:hover {
  box-shadow: 0 6px 16px rgba(97, 175, 239, 0.3);
}

.ant-btn-default {
  background: rgba(255, 255, 255, 0.8);
  border: 1px solid rgba(0, 0, 0, 0.05);
  color: var(--text-primary);
}

:root.dark .ant-btn-default {
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.ant-btn-default:hover {
  background: #fff;
  border-color: rgba(0, 0, 0, 0.1);
  color: var(--primary-color);
}

:root.dark .ant-btn-default:hover {
  background: rgba(255, 255, 255, 0.1);
  border-color: rgba(255, 255, 255, 0.2);
}

/* Inputs */
.ant-input,
.ant-input-number,
.ant-select-selector {
  border-radius: var(--radius-sm) !important;
  border-color: rgba(0, 0, 0, 0.1) !important;
  background: rgba(255, 255, 255, 0.6) !important;
  backdrop-filter: blur(10px);
  transition: var(--transition) !important;
}

:root.dark .ant-input,
:root.dark .ant-input-number,
:root.dark .ant-select-selector {
  border-color: rgba(255, 255, 255, 0.1) !important;
  background: rgba(0, 0, 0, 0.2) !important;
  color: var(--text-primary) !important;
}

.ant-input:focus,
.ant-input-number:focus,
.ant-select-selector:focus {
  background: #fff !important;
  box-shadow: 0 0 0 4px rgba(0, 122, 255, 0.1) !important;
  border-color: var(--primary-color) !important;
}

:root.dark .ant-input:focus,
:root.dark .ant-input-number:focus,
:root.dark .ant-select-selector:focus {
  background: rgba(0, 0, 0, 0.4) !important;
  box-shadow: 0 0 0 4px rgba(97, 175, 239, 0.1) !important;
}

/* Cards */
.ant-card {
  border-radius: var(--radius-lg) !important;
  border: none !important;
  background: rgba(255, 255, 255, 0.7) !important;
  backdrop-filter: blur(20px);
  box-shadow: var(--shadow-sm) !important;
}

:root.dark .ant-card {
  background: var(--card-bg) !important;
}

.ant-card-head {
  border-bottom: 1px solid rgba(0, 0, 0, 0.05) !important;
  font-weight: 600;
  color: var(--text-primary) !important;
}

:root.dark .ant-card-head {
  border-bottom: 1px solid rgba(255, 255, 255, 0.08) !important;
}

/* 菜单内部 */
.ant-menu-sub {
  background: rgba(255, 255, 255, 0.2) !important;
  border-radius: var(--radius-sm) !important;
}

:root.dark .ant-menu-sub {
  background: rgba(0, 0, 0, 0.2) !important;
}

/* Tables */
.ant-table {
  background: transparent !important;
  color: var(--text-primary) !important;
}

.ant-table-thead>tr>th {
  background: rgba(245, 245, 247, 0.5) !important;
  backdrop-filter: blur(10px);
  color: var(--text-secondary) !important;
  font-weight: 600 !important;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05) !important;
}

:root.dark .ant-table-thead>tr>th {
  background: rgba(40, 44, 52, 0.5) !important;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08) !important;
}

.ant-table-tbody>tr>td {
  background: transparent !important;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03) !important;
  transition: background 0.2s;
}

:root.dark .ant-table-tbody>tr>td {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05) !important;
}

.ant-table-tbody>tr:hover>td {
  background: rgba(0, 122, 255, 0.03) !important;
}

:root.dark .ant-table-tbody>tr:hover>td {
  background: rgba(97, 175, 239, 0.05) !important;
}

/* Modals */
.ant-modal-content {
  border-radius: 20px !important;
  background: rgba(255, 255, 255, 0.85) !important;
  backdrop-filter: blur(25px) !important;
  -webkit-backdrop-filter: blur(25px) !important;
  box-shadow: 0 24px 48px rgba(0, 0, 0, 0.2) !important;
  border: 1px solid rgba(255, 255, 255, 0.5);
}

:root.dark .ant-modal-content {
  background: rgba(40, 44, 52, 0.9) !important;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.ant-modal-header {
  background: transparent !important;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05) !important;
}

:root.dark .ant-modal-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.08) !important;
}

.ant-modal-title {
  font-weight: 700;
  font-size: 18px;
  color: var(--text-primary) !important;
}

/* Tags */
.ant-tag {
  border-radius: 6px;
  border: none;
  font-weight: 500;
  padding: 2px 8px;
}

.ant-tag-success {
  background: rgba(52, 199, 89, 0.15);
  color: var(--success-color);
}

.ant-tag-error {
  background: rgba(255, 59, 48, 0.15);
  color: var(--error-color);
}

.ant-tag-warning {
  background: rgba(255, 149, 0, 0.15);
  color: var(--warning-color);
}

.ant-tag-processing {
  background: rgba(0, 122, 255, 0.15);
  color: var(--primary-color);
}

/* Scrollbar */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.2);
  border-radius: 4px;
}

:root.dark ::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(0, 0, 0, 0.3);
}

:root.dark ::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}

/* Utilities */
.gradient-text {
  background: linear-gradient(135deg, #007AFF, #5856D6);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

:root.dark .gradient-text {
  background: linear-gradient(135deg, #61afef, #c678dd);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

/* Dark Mode - Dropdown & Popup Components */
:root.dark .ant-dropdown,
:root.dark .ant-dropdown-menu {
  background: var(--dropdown-bg) !important;
  color: var(--text-primary) !important;
}

:root.dark .ant-dropdown-menu-item,
:root.dark .ant-dropdown-menu-submenu-title {
  color: var(--text-primary) !important;
}

:root.dark .ant-dropdown-menu-item:hover,
:root.dark .ant-dropdown-menu-submenu-title:hover {
  background: rgba(97, 175, 239, 0.15) !important;
}

:root.dark .ant-dropdown-menu-item-divider {
  background-color: rgba(255, 255, 255, 0.1) !important;
}

/* Dark Mode - Select Dropdown */
:root.dark .ant-select-dropdown {
  background: var(--dropdown-bg) !important;
}

:root.dark .ant-select-item {
  color: var(--text-primary) !important;
}

:root.dark .ant-select-item-option-selected {
  background: rgba(97, 175, 239, 0.15) !important;
}

:root.dark .ant-select-item-option-active {
  background: rgba(97, 175, 239, 0.1) !important;
}

/* Dark Mode - Popover & Tooltip */
:root.dark .ant-popover-inner,
:root.dark .ant-tooltip-inner {
  background: var(--dropdown-bg) !important;
  color: var(--text-primary) !important;
}

:root.dark .ant-popover-arrow-content,
:root.dark .ant-tooltip-arrow-content {
  background: var(--dropdown-bg) !important;
}

/* Dark Mode - Message & Notification */
:root.dark .ant-message-notice-content,
:root.dark .ant-notification-notice {
  background: var(--card-bg) !important;
  color: var(--text-primary) !important;
  box-shadow: var(--shadow-lg) !important;
}

/* Dark Mode - Drawer */
:root.dark .ant-drawer-content {
  background: var(--body-bg) !important;
  color: var(--text-primary) !important;
}

:root.dark .ant-drawer-header {
  background: var(--card-bg) !important;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1) !important;
  color: var(--text-primary) !important;
}

:root.dark .ant-drawer-body {
  background: var(--body-bg) !important;
}

/* Dark Mode - Tabs */
:root.dark .ant-tabs-nav {
  background: transparent !important;
}

:root.dark .ant-tabs-tab {
  color: var(--text-secondary) !important;
}

:root.dark .ant-tabs-tab-active {
  color: var(--primary-color) !important;
}

:root.dark .ant-tabs-tab:hover {
  color: var(--text-primary) !important;
}

/* Dark Mode - Form Items */
:root.dark .ant-form-item-label>label {
  color: var(--text-primary) !important;
}

:root.dark .ant-checkbox-wrapper {
  color: var(--text-primary) !important;
}

:root.dark .ant-radio-wrapper {
  color: var(--text-primary) !important;
}

/* Dark Mode - Pagination */
:root.dark .ant-pagination-item {
  background: rgba(255, 255, 255, 0.05) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

:root.dark .ant-pagination-item a {
  color: var(--text-primary) !important;
}

:root.dark .ant-pagination-item-active {
  background: var(--primary-color) !important;
  border-color: var(--primary-color) !important;
}

:root.dark .ant-pagination-item-active a {
  color: #fff !important;
}

/* Dark Mode - Empty State */
:root.dark .ant-empty-description {
  color: var(--text-secondary) !important;
}

/* Dark Mode - Spin */
:root.dark .ant-spin-text {
  color: var(--text-primary) !important;
}

/* Dark Mode - Badge */
:root.dark .ant-badge-count {
  background: var(--error-color) !important;
}

/* Dark Mode - Alert */
:root.dark .ant-alert {
  background: rgba(255, 255, 255, 0.05) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

:root.dark .ant-alert-message,
:root.dark .ant-alert-description {
  color: var(--text-primary) !important;
}

/* Dark Mode - Collapse */
:root.dark .ant-collapse {
  background: transparent !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

:root.dark .ant-collapse-item {
  border-color: rgba(255, 255, 255, 0.1) !important;
}

:root.dark .ant-collapse-header {
  color: var(--text-primary) !important;
  background: rgba(255, 255, 255, 0.03) !important;
}

:root.dark .ant-collapse-content {
  background: transparent !important;
  color: var(--text-primary) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

/* Dark Mode - Descriptions */
:root.dark .ant-descriptions-item-label {
  color: var(--text-secondary) !important;
}

:root.dark .ant-descriptions-item-content {
  color: var(--text-primary) !important;
}

/* Dark Mode - Divider */
:root.dark .ant-divider {
  border-color: rgba(255, 255, 255, 0.1) !important;
}

/* Dark Mode - List */
:root.dark .ant-list-item {
  border-color: rgba(255, 255, 255, 0.1) !important;
  color: var(--text-primary) !important;
}

/* Dark Mode - Steps */
:root.dark .ant-steps-item-title {
  color: var(--text-primary) !important;
}

:root.dark .ant-steps-item-description {
  color: var(--text-secondary) !important;
}

/* Dark Mode - Timeline */
:root.dark .ant-timeline-item-content {
  color: var(--text-primary) !important;
}

/* Dark Mode - Tree */
:root.dark .ant-tree {
  background: transparent !important;
  color: var(--text-primary) !important;
}

:root.dark .ant-tree-node-content-wrapper {
  color: var(--text-primary) !important;
}

:root.dark .ant-tree-node-content-wrapper:hover {
  background: rgba(97, 175, 239, 0.1) !important;
}

/* Dark Mode - Transfer */
:root.dark .ant-transfer-list {
  background: var(--card-bg) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

:root.dark .ant-transfer-list-header {
  background: rgba(255, 255, 255, 0.05) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
  color: var(--text-primary) !important;
}

:root.dark .ant-transfer-list-content-item {
  color: var(--text-primary) !important;
}

/* Dark Mode - Upload */
:root.dark .ant-upload {
  background: rgba(255, 255, 255, 0.05) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
}

:root.dark .ant-upload-list-item {
  background: rgba(255, 255, 255, 0.03) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
  color: var(--text-primary) !important;
}

/* Dark Mode - Layout Footer */
:root.dark .ant-layout-footer {
  color: var(--text-hint) !important;
}
</style>
