<template>
  <div class="file-manager">
    <!-- Toolbar -->
    <div class="file-toolbar">
      <div class="left-tools">
        <a-button-group>
          <a-button @click="goUp" :disabled="currentPath === '/' || currentPath === ''">
            <template #icon>
              <ArrowUpOutlined />
            </template>
          </a-button>
          <a-button @click="refresh">
            <template #icon>
              <ReloadOutlined />
            </template>
          </a-button>
          <a-button @click="goHome">
            <template #icon>
              <HomeOutlined />
            </template>
          </a-button>
        </a-button-group>

        <a-breadcrumb class="path-breadcrumb">
          <a-breadcrumb-item>
            <a @click="navigateTo('/')">根目录</a>
          </a-breadcrumb-item>
          <a-breadcrumb-item v-for="(part, index) in pathParts" :key="index">
            <a @click="navigateTo(getPathUpTo(index))">{{ part }}</a>
          </a-breadcrumb-item>
        </a-breadcrumb>
      </div>

      <div class="right-tools">
        <a-input-search v-model:value="searchQuery" placeholder="搜索文件..." style="width: 200px" allow-clear
          class="mac-search" />
        <a-dropdown>
          <template #overlay>
            <a-menu @click="handleMenuClick">
              <a-menu-item key="new-file">
                <FileAddOutlined /> 新建文件
              </a-menu-item>
              <a-menu-item key="new-folder">
                <FolderAddOutlined /> 新建文件夹
              </a-menu-item>
              <a-menu-item key="upload">
                <CloudUploadOutlined /> 上传文件
              </a-menu-item>
            </a-menu>
          </template>
          <a-button type="primary" class="mac-btn">
            <PlusOutlined /> 新建
            <DownOutlined />
          </a-button>
        </a-dropdown>
      </div>
    </div>

    <!-- File List -->
    <div class="file-list-container">
      <a-table :data-source="filteredFiles" :columns="columns" :pagination="false"
        :scroll="{ y: 'calc(100vh - 300px)' }" row-key="name" size="small" :custom-row="customRow" :loading="loading"
        class="mac-table">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <div class="file-name-cell">
              <component :is="getFileIcon(record)" :class="['file-icon', record.is_dir ? 'is-dir' : '']" />
              <a v-if="record.is_dir" class="file-name" @click.stop="handleRowClick(record)">{{ record.name }}</a>
              <span v-else class="file-name">{{ record.name }}</span>
            </div>
          </template>
          <template v-else-if="column.key === 'size'">
            {{ record.is_dir ? '-' : formatFileSize(record.size) }}
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space>
              <a-tooltip title="编辑" v-if="!record.is_dir">
                <a-button type="text" size="small" @click.stop="emit('edit', record)">
                  <EditOutlined />
                </a-button>
              </a-tooltip>
              <a-tooltip title="下载" v-if="!record.is_dir">
                <a-button type="text" size="small" @click.stop="emit('download', record)">
                  <DownloadOutlined />
                </a-button>
              </a-tooltip>
              <a-popconfirm title="确定要删除吗？" @confirm.stop="emit('delete', record)">
                <a-button type="text" danger size="small" @click.stop>
                  <DeleteOutlined />
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import {
  ArrowUpOutlined,
  ReloadOutlined,
  HomeOutlined,
  PlusOutlined,
  DownOutlined,
  FileAddOutlined,
  FolderAddOutlined,
  CloudUploadOutlined,
  FolderOutlined,
  FileOutlined,
  FileTextOutlined,
  FileImageOutlined,
  EditOutlined,
  DownloadOutlined,
  DeleteOutlined
} from '@ant-design/icons-vue';

interface FileItem {
  name: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  path: string;
}

const props = defineProps<{
  files: FileItem[];
  currentPath: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  (e: 'navigate', path: string): void;
  (e: 'refresh'): void;
  (e: 'edit', file: FileItem): void;
  (e: 'download', file: FileItem): void;
  (e: 'delete', file: FileItem): void;
  (e: 'create-file'): void;
  (e: 'create-folder'): void;
  (e: 'upload'): void;
}>();

const searchQuery = ref('');

const columns = [
  { title: '名称', key: 'name', width: '50%' },
  { title: '大小', key: 'size', width: '15%' },
  { title: '修改时间', dataIndex: 'mod_time', key: 'mod_time', width: '25%' },
  { title: '操作', key: 'actions', width: '10%' }
];

const pathParts = computed(() => {
  return props.currentPath.split('/').filter(Boolean);
});

const filteredFiles = computed(() => {
  if (!searchQuery.value) return props.files;
  return props.files.filter(f => f.name.toLowerCase().includes(searchQuery.value.toLowerCase()));
});

const getPathUpTo = (index: number) => {
  return '/' + pathParts.value.slice(0, index + 1).join('/');
};

const navigateTo = (path: string) => {
  emit('navigate', path);
};

const goUp = () => {
  const parts = props.currentPath.split('/').filter(Boolean);
  parts.pop();
  emit('navigate', '/' + parts.join('/'));
};

const goHome = () => {
  emit('navigate', '/');
};

const refresh = () => {
  emit('refresh');
};

const handleRowClick = (record: FileItem) => {
  if (record.is_dir) {
    const newPath = props.currentPath === '/'
      ? `/${record.name}`
      : `${props.currentPath}/${record.name}`;
    emit('navigate', newPath);
  }
};

const customRow = (record: FileItem) => {
  return {
    onClick: () => handleRowClick(record)
  };
};

const handleMenuClick = (e: any) => {
  if (e.key === 'new-file') emit('create-file');
  else if (e.key === 'new-folder') emit('create-folder');
  else if (e.key === 'upload') emit('upload');
};

const formatFileSize = (size: number) => {
  if (size === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(size) / Math.log(k));
  return parseFloat((size / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
};

const getFileIcon = (file: FileItem) => {
  if (file.is_dir) return FolderOutlined;
  const ext = file.name.split('.').pop()?.toLowerCase();
  if (['jpg', 'png', 'gif', 'svg'].includes(ext || '')) return FileImageOutlined;
  if (['txt', 'md', 'json', 'js', 'ts', 'html', 'css', 'py', 'go'].includes(ext || '')) return FileTextOutlined;
  return FileOutlined;
};
</script>

<style scoped>
.file-manager {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: transparent;
}

.file-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  margin-bottom: 8px;
}

.left-tools {
  display: flex;
  align-items: center;
  gap: 16px;
}

.path-breadcrumb {
  font-size: var(--font-size-md);
}

.file-list-container {
  flex: 1;
  overflow: hidden;
  background: var(--alpha-white-50);
  backdrop-filter: blur(var(--blur-sm));
  border-radius: var(--radius-sm);
  border: 1px solid var(--alpha-black-05);
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.file-icon {
  font-size: var(--font-size-xl);
  color: #8c8c8c;
}

.file-icon.is-dir {
  color: var(--primary-color);
}

.file-name {
  font-weight: var(--font-weight-medium);
}

.mac-search :deep(.ant-input) {
  border-radius: var(--radius-xs);
  background: var(--alpha-white-60);
  backdrop-filter: blur(5px);
}

.mac-btn {
  border-radius: var(--radius-xs);
}

.mac-table :deep(.ant-table) {
  background: transparent;
}

.mac-table :deep(.ant-table-thead > tr > th) {
  background: var(--alpha-black-02);
  font-weight: var(--font-weight-semibold);
}

.mac-table :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid var(--alpha-black-03);
}

.mac-table :deep(.ant-table-tbody > tr:hover > td) {
  background: var(--alpha-black-02);
}
</style>

<style>
/* Dark mode support */
.dark .file-list-container {
  background: rgba(30, 30, 30, 0.5);
  border-color: var(--alpha-white-10);
}

.dark .mac-search .ant-input {
  background: var(--alpha-black-20);
  border-color: var(--alpha-white-10);
  color: #fff;
}

.dark .mac-table .ant-table-thead>tr>th {
  background: var(--alpha-white-05);
  color: #e6e6e6;
  border-bottom-color: var(--alpha-white-05);
}

.dark .mac-table .ant-table-tbody>tr>td {
  border-bottom-color: var(--alpha-white-05);
  color: #e6e6e6;
}

.dark .mac-table .ant-table-tbody>tr:hover>td {
  background: var(--alpha-white-05);
}
</style>
