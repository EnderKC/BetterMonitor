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

/* ============================================
   CSS variables are defined in:
   src/styles/variables.css
   ============================================ */

/* Global Reset & Typography */
html,
body {
  margin: 0;
  padding: 0;
  height: 100%;
  font-family: var(--font-family-sans);
  background-color: var(--body-bg);
  color: var(--text-primary);
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  transition: var(--transition-color);
}

#app {
  height: 100vh;
}

.modern-ui {
  min-height: 100vh;
  background-color: var(--body-bg);
  background-image:
    radial-gradient(circle at 10% 20%, var(--bg-radial-primary) 0%, transparent 40%),
    radial-gradient(circle at 90% 80%, var(--bg-radial-success) 0%, transparent 40%),
    radial-gradient(circle at 50% 50%, var(--bg-radial-accent) 0%, transparent 60%);
  background-attachment: fixed;
  transition: background-image 0.3s, background-color 0.3s;
}

/* Glassmorphism Utility */
.glass-card {
  background: var(--card-bg);
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  border: 1px solid var(--alpha-white-40);
  transition: var(--transition);
}

:root.dark .glass-card {
  border: 1px solid var(--alpha-white-08);
}

.glass-card:hover {
  box-shadow: var(--shadow-md);
  transform: translateY(-2px);
  border-color: var(--alpha-white-80);
}

:root.dark .glass-card:hover {
  border-color: var(--alpha-white-15);
}

/* Ant Design Overrides */

/* Buttons */
.ant-btn {
  border-radius: var(--radius-md);
  font-weight: var(--font-weight-medium);
  box-shadow: none;
  border: none;
  height: var(--btn-height);
  padding: var(--spacing-xs) var(--spacing-md);
}

.ant-btn-primary {
  background: var(--primary-color);
  box-shadow: var(--btn-primary-shadow);
}

:root.dark .ant-btn-primary {
  box-shadow: var(--btn-primary-shadow);
}

.ant-btn-primary:hover {
  background: var(--primary-hover);
  box-shadow: var(--btn-primary-hover-shadow);
}

:root.dark .ant-btn-primary:hover {
  box-shadow: var(--btn-primary-hover-shadow);
}

.ant-btn-default {
  background: var(--alpha-white-80);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
}

:root.dark .ant-btn-default {
  background: var(--alpha-white-05);
  border: 1px solid var(--border-default);
}

.ant-btn-default:hover {
  background: #fff;
  border-color: var(--border-default);
  color: var(--primary-color);
}

:root.dark .ant-btn-default:hover {
  background: var(--alpha-white-10);
  border-color: var(--alpha-white-20);
}

/* Inputs */
.ant-input,
.ant-input-number,
.ant-select-selector {
  border-radius: var(--radius-sm) !important;
  border-color: var(--input-border) !important;
  background: var(--input-bg) !important;
  backdrop-filter: blur(var(--blur-sm));
  transition: var(--transition) !important;
}

:root.dark .ant-input,
:root.dark .ant-input-number,
:root.dark .ant-select-selector {
  border-color: var(--input-border) !important;
  background: var(--input-bg) !important;
  color: var(--text-primary) !important;
}

.ant-input:focus,
.ant-input-number:focus,
.ant-select-selector:focus {
  background: var(--input-focus-bg) !important;
  box-shadow: var(--input-focus-shadow) !important;
  border-color: var(--primary-color) !important;
}

:root.dark .ant-input:focus,
:root.dark .ant-input-number:focus,
:root.dark .ant-select-selector:focus {
  background: var(--input-focus-bg) !important;
  box-shadow: var(--input-focus-shadow) !important;
}

/* Cards */
.ant-card {
  border-radius: var(--radius-lg) !important;
  border: none !important;
  background: rgba(255, 255, 255, 0.7) !important;
  backdrop-filter: blur(var(--blur-md));
  box-shadow: var(--shadow-sm) !important;
}

:root.dark .ant-card {
  background: var(--card-bg) !important;
}

.ant-card-head {
  border-bottom: 1px solid var(--border-subtle) !important;
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary) !important;
}

:root.dark .ant-card-head {
  border-bottom: 1px solid var(--alpha-white-08) !important;
}

/* Menu Sub */
.ant-menu-sub {
  background: var(--alpha-white-20) !important;
  border-radius: var(--radius-sm) !important;
}

:root.dark .ant-menu-sub {
  background: var(--alpha-black-20) !important;
}

/* Tables */
.ant-table {
  background: transparent !important;
  color: var(--text-primary) !important;
}

.ant-table-thead>tr>th {
  background: var(--table-header-bg) !important;
  backdrop-filter: blur(var(--blur-sm));
  color: var(--text-secondary) !important;
  font-weight: var(--font-weight-semibold) !important;
  border-bottom: 1px solid var(--table-border) !important;
}

:root.dark .ant-table-thead>tr>th {
  background: var(--table-header-bg) !important;
  border-bottom: 1px solid var(--table-border) !important;
}

.ant-table-tbody>tr>td {
  background: transparent !important;
  border-bottom: 1px solid var(--alpha-black-03) !important;
  transition: var(--transition-fast);
}

:root.dark .ant-table-tbody>tr>td {
  border-bottom: 1px solid var(--alpha-white-05) !important;
}

.ant-table-tbody>tr:hover>td {
  background: var(--table-row-hover) !important;
}

:root.dark .ant-table-tbody>tr:hover>td {
  background: var(--table-row-hover) !important;
}

/* Modals */
.ant-modal-content {
  border-radius: var(--modal-radius) !important;
  background: var(--modal-bg) !important;
  backdrop-filter: blur(var(--blur-lg)) !important;
  -webkit-backdrop-filter: blur(var(--blur-lg)) !important;
  box-shadow: var(--modal-shadow) !important;
  border: 1px solid var(--modal-border);
}

:root.dark .ant-modal-content {
  background: var(--modal-bg) !important;
  border: 1px solid var(--modal-border);
}

.ant-modal-header {
  background: transparent !important;
  border-bottom: 1px solid var(--border-subtle) !important;
}

:root.dark .ant-modal-header {
  border-bottom: 1px solid var(--alpha-white-08) !important;
}

.ant-modal-title {
  font-weight: var(--font-weight-bold);
  font-size: var(--font-size-xl);
  color: var(--text-primary) !important;
}

/* Tags */
.ant-tag {
  border-radius: var(--radius-xs);
  border: none;
  font-weight: var(--font-weight-medium);
  padding: var(--spacing-2xs) var(--spacing-sm);
}

.ant-tag-success {
  background: var(--success-bg);
  color: var(--success-color);
}

.ant-tag-error {
  background: var(--error-bg);
  color: var(--error-color);
}

.ant-tag-warning {
  background: var(--warning-bg);
  color: var(--warning-color);
}

.ant-tag-processing {
  background: var(--info-bg);
  color: var(--primary-color);
}

/* Scrollbar */
::-webkit-scrollbar {
  width: var(--scrollbar-size);
  height: var(--scrollbar-size);
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: var(--scrollbar-thumb);
  border-radius: var(--scrollbar-radius);
}

:root.dark ::-webkit-scrollbar-thumb {
  background: var(--scrollbar-thumb);
}

::-webkit-scrollbar-thumb:hover {
  background: var(--scrollbar-thumb-hover);
}

:root.dark ::-webkit-scrollbar-thumb:hover {
  background: var(--scrollbar-thumb-hover);
}

/* Utilities */
.gradient-text {
  background: var(--gradient-brand);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

:root.dark .gradient-text {
  background: var(--gradient-brand);
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
  background: var(--primary-light) !important;
}

:root.dark .ant-dropdown-menu-item-divider {
  background-color: var(--border-default) !important;
}

/* Dark Mode - Select Dropdown */
:root.dark .ant-select-dropdown {
  background: var(--dropdown-bg) !important;
}

:root.dark .ant-select-item {
  color: var(--text-primary) !important;
}

:root.dark .ant-select-item-option-selected {
  background: var(--primary-light) !important;
}

:root.dark .ant-select-item-option-active {
  background: var(--primary-bg) !important;
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
  border-bottom: 1px solid var(--border-default) !important;
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
  background: var(--alpha-white-05) !important;
  border-color: var(--border-default) !important;
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
  background: var(--alpha-white-05) !important;
  border-color: var(--border-default) !important;
}

:root.dark .ant-alert-message,
:root.dark .ant-alert-description {
  color: var(--text-primary) !important;
}

/* Dark Mode - Collapse */
:root.dark .ant-collapse {
  background: transparent !important;
  border-color: var(--border-default) !important;
}

:root.dark .ant-collapse-item {
  border-color: var(--border-default) !important;
}

:root.dark .ant-collapse-header {
  color: var(--text-primary) !important;
  background: var(--alpha-white-05) !important;
}

:root.dark .ant-collapse-content {
  background: transparent !important;
  color: var(--text-primary) !important;
  border-color: var(--border-default) !important;
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
  border-color: var(--border-default) !important;
}

/* Dark Mode - List */
:root.dark .ant-list-item {
  border-color: var(--border-default) !important;
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
  background: var(--primary-bg) !important;
}

/* Dark Mode - Transfer */
:root.dark .ant-transfer-list {
  background: var(--card-bg) !important;
  border-color: var(--border-default) !important;
}

:root.dark .ant-transfer-list-header {
  background: var(--alpha-white-05) !important;
  border-color: var(--border-default) !important;
  color: var(--text-primary) !important;
}

:root.dark .ant-transfer-list-content-item {
  color: var(--text-primary) !important;
}

/* Dark Mode - Upload */
:root.dark .ant-upload {
  background: var(--alpha-white-05) !important;
  border-color: var(--border-default) !important;
}

:root.dark .ant-upload-list-item {
  background: var(--alpha-white-05) !important;
  border-color: var(--border-default) !important;
  color: var(--text-primary) !important;
}

/* Dark Mode - Layout Footer */
:root.dark .ant-layout-footer {
  color: var(--text-hint) !important;
}
</style>
