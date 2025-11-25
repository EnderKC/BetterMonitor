<script setup lang="ts">
import { ref, reactive, onMounted, computed, defineComponent, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal, Tree, Table } from 'ant-design-vue';
import {
  FolderOutlined,
  FileOutlined,
  FileTextOutlined,
  FileImageOutlined,
  FileZipOutlined,
  FileExcelOutlined,
  FileWordOutlined,
  FilePdfOutlined,
  ArrowLeftOutlined,
  UploadOutlined,
  DownloadOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  EnterOutlined,
  CloudUploadOutlined,
  FileAddOutlined,
  FolderAddOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
import { getToken } from '../../utils/auth';
import { useServerStore } from '../../stores/serverStore';
import { Codemirror } from 'vue-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { json } from '@codemirror/lang-json';
import { markdown } from '@codemirror/lang-markdown';
import { python } from '@codemirror/lang-python';
import { php } from '@codemirror/lang-php';
import { xml } from '@codemirror/lang-xml';
import { rust } from '@codemirror/lang-rust';
import { sql } from '@codemirror/lang-sql';
import { java } from '@codemirror/lang-java';
import { cpp } from '@codemirror/lang-cpp';
import { StreamLanguage } from '@codemirror/language';
import { shell } from '@codemirror/legacy-modes/mode/shell';
import { nginx } from '@codemirror/legacy-modes/mode/nginx';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { basicSetup } from 'codemirror';

defineComponent({
  components: { Codemirror }
});

interface ApiResponse {
  [key: string]: any;
  data?: string | any;
  content?: string | any;
}

interface ServerResponse {
  server: any;
}

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));
const containerId = ref<string>(route.params.containerId as string);
const containerName = (route.query.name as string) || containerId.value;
const serverStore = useServerStore();

const serverInfo = ref<any>({});
const loading = ref(true);

const currentPath = ref('/');
const fileList = ref<any[]>([]);
const selectedRowKeys = ref<string[]>([]);
const selectedFiles = ref<any[]>([]);
const searchKeyword = ref('');

const treeData = ref<any[]>([]);
const expandedKeys = ref<string[]>([]);
const autoExpandParent = ref<boolean>(true);

const uploadModalVisible = ref(false);
const fileToUpload = ref<File | null>(null);
const uploading = ref(false);

const editModalVisible = ref(false);
const fileContent = ref('');
const editingFile = ref<any>(null);
const editLoading = ref(false);

const createModalVisible = ref(false);
const createFormState = reactive({
  type: 'file',
  name: '',
  content: ''
});

const pathJumpModalVisible = ref(false);
const jumpToPath = ref('');

const basePrefix = computed(() => `/servers/${serverId.value}/docker/containers/${containerId.value}`);
const isServerOnline = computed(() => serverStore.isServerOnline(serverId.value));

const getFileIcon = (file: any) => {
  if (file.is_dir) return FolderOutlined;
  const ext = file.name.split('.').pop()?.toLowerCase();
  switch (ext) {
    case 'txt':
    case 'log':
    case 'md':
    case 'conf':
    case 'ini':
    case 'yaml':
    case 'yml':
    case 'json':
    case 'xml':
    case 'html':
    case 'htm':
    case 'css':
    case 'js':
    case 'ts':
    case 'sh':
    case 'bash':
    case 'py':
    case 'go':
    case 'java':
    case 'c':
    case 'cpp':
    case 'h':
    case 'php':
      return FileTextOutlined;
    case 'jpg':
    case 'jpeg':
    case 'png':
    case 'gif':
    case 'bmp':
    case 'svg':
    case 'webp':
      return FileImageOutlined;
    case 'zip':
    case 'rar':
    case 'tar':
    case 'gz':
    case '7z':
      return FileZipOutlined;
    case 'xls':
    case 'xlsx':
    case 'csv':
      return FileExcelOutlined;
    case 'doc':
    case 'docx':
      return FileWordOutlined;
    case 'pdf':
      return FilePdfOutlined;
    default:
      return FileOutlined;
  }
};

const languageExtensions = {
  javascript,
  html,
  css,
  json,
  markdown,
  python,
  php,
  xml,
  rust,
  sql,
  java,
  cpp
};

const detectLanguage = (fileName: string) => {
  const ext = fileName.split('.').pop()?.toLowerCase() || '';
  switch (ext) {
    case 'js':
    case 'jsx':
    case 'ts':
    case 'tsx':
      return languageExtensions.javascript();
    case 'html':
    case 'htm':
      return languageExtensions.html();
    case 'css':
      return languageExtensions.css();
    case 'json':
      return languageExtensions.json();
    case 'md':
    case 'markdown':
      return languageExtensions.markdown();
    case 'py':
      return languageExtensions.python();
    case 'php':
      return languageExtensions.php();
    case 'xml':
      return languageExtensions.xml();
    case 'rs':
      return languageExtensions.rust();
    case 'sql':
      return languageExtensions.sql();
    case 'java':
      return languageExtensions.java();
    case 'c':
    case 'cpp':
    case 'h':
    case 'hpp':
      return languageExtensions.cpp();
    case 'sh':
    case 'bash':
      return StreamLanguage.define(shell);
    case 'conf':
    case 'nginx':
      return StreamLanguage.define(nginx);
    default:
      return null;
  }
};

const codemirrorExtensions = computed(() => {
  const ext = detectLanguage(editingFile.value?.name || '');
  const result = [basicSetup, vscodeDark];
  if (ext) result.push(ext);
  return result;
});

const fetchServerInfo = async () => {
  loading.value = true;
  try {
    const response = await request.get<ServerResponse>(`/servers/${serverId.value}`);
    if (response && (response as any).server) {
      serverInfo.value = (response as any).server;
      const status = (response as any).server.status || 'offline';
      const online = (response as any).server.online === true;
      serverStore.updateServerStatus(serverId.value, status);
      if (!online) message.warning('服务器离线，容器文件管理不可用');
    }
  } catch (err) {
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
  }
};

const fetchFileList = async (path: string = '/') => {
  loading.value = true;
  selectedRowKeys.value = [];
  selectedFiles.value = [];
  searchKeyword.value = '';

  try {
    const response: any = await request.get(buildUrl('/files'), { params: { path } });
    let items: any[] = [];
    if (Array.isArray(response)) {
      items = response;
    } else if (response && Array.isArray(response.data)) {
      items = response.data;
    }
    fileList.value = items.map((file: any) => ({
      ...file,
      key: `${path}/${file.name}`.replace(/\/+/g, '/')
    }));
    currentPath.value = path;
  } catch (error) {
    console.error(error);
    message.error('获取文件列表失败');
    fileList.value = [];
  } finally {
    loading.value = false;
  }
};

const treeLoading = ref(false);

const fetchDirectoryTree = async () => {
  treeLoading.value = true;
  try {
    const response: any = await request.get(buildUrl('/files/children'), { params: { path: '/' } });
    let items: any[] = [];
    if (Array.isArray(response)) items = response;
    else if (response && Array.isArray(response.data)) items = response.data;

    // 修复重复问题：确保 key 唯一，并且只在初始化时设置根节点
    treeData.value = items.filter((i: any) => i.is_dir).map((item: any) => ({
      title: item.name,
      key: `/${item.name}`,
      isLeaf: false,
      children: undefined,
      hasLoadedChildren: false
    }));
    expandedKeys.value = ['/'];
  } catch (error) {
    console.error(error);
    treeData.value = [];
  } finally {
    treeLoading.value = false;
  }
};

const loadTreeNodeChildren = async (node: any) => {
  if (node.hasLoadedChildren) return;
  try {
    const response: any = await request.get(buildUrl('/files/children'), { params: { path: node.key } });
    let items: any[] = [];
    if (Array.isArray(response)) items = response;
    else if (response && Array.isArray(response.data)) items = response.data;

    const directories = items.filter((item) => item.is_dir);
    const children = directories.map((item) => {
      const path = `${node.key}/${item.name}`.replace(/\/+/g, '/');
      return { title: item.name, key: path, isLeaf: false, children: undefined, hasLoadedChildren: false };
    });

    // 递归更新树数据，确保不产生副本
    const updateNodeInTree = (nodes: any[]): any[] =>
      nodes.map((n) => {
        if (n.key === node.key) {
          return { ...n, children, hasLoadedChildren: true };
        } else if (n.children) {
          return { ...n, children: updateNodeInTree(n.children) };
        }
        return n;
      });

    treeData.value = updateNodeInTree(treeData.value);
  } catch (error) {
    console.error(error);
  }
};

const onTreeSelect = (selectedKeys: string[], { node }: any) => {
  if (selectedKeys.length === 0) return;
  const path = selectedKeys[0];
  fetchFileList(path);
  // 自动展开
  if (!expandedKeys.value.includes(path)) {
    expandedKeys.value = [...expandedKeys.value, path];
  }
  if (node && node.isLeaf === false && !node.children) {
    loadTreeNodeChildren(node);
  }
};

const formatFileSize = (size: number) => {
  if (size === null || size === undefined) return '未知';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(2)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GB`;
};

const openPath = (path: string) => {
  fetchFileList(path);
};

const goUp = () => {
  if (currentPath.value === '/' || currentPath.value === '') return;
  const parts = currentPath.value.split('/').filter(Boolean);
  parts.pop();
  const parentPath = '/' + parts.join('/');
  fetchFileList(parentPath === '//' ? '/' : parentPath);
};

const openFile = async (record: any) => {
  if (record.is_dir) {
    fetchFileList(record.key);
    return;
  }
  try {
    editLoading.value = true;
    const response: ApiResponse = await request.get(buildUrl('/files/content'), { params: { path: record.key } });
    let content = '';
    if (response && typeof response === 'string') content = response;
    else if (response && typeof (response as any).content === 'string') content = (response as any).content;
    fileContent.value = content;
    editingFile.value = record;
    editModalVisible.value = true;
    await nextTick();
  } catch (error) {
    console.error(error);
    message.error('获取文件内容失败');
  } finally {
    editLoading.value = false;
  }
};

const saveFile = async () => {
  if (!editingFile.value) return;
  editLoading.value = true;
  try {
    await request.put(buildUrl('/files/content'), { path: editingFile.value.key, content: fileContent.value });
    editModalVisible.value = false;
    message.success('保存成功');
  } catch (error) {
    console.error(error);
    message.error('保存失败');
  } finally {
    editLoading.value = false;
  }
};

const handleUpload = async () => {
  if (!fileToUpload.value) {
    message.warning('请选择文件');
    return;
  }
  uploading.value = true;
  try {
    const formData = new FormData();
    formData.append('file', fileToUpload.value);
    formData.append('path', currentPath.value);
    await request.post(buildUrl('/files/upload'), formData, { headers: { 'Content-Type': 'multipart/form-data' } });
    uploadModalVisible.value = false;
    fileToUpload.value = null;
    fetchFileList(currentPath.value);
    message.success('上传成功');
  } catch (error) {
    console.error(error);
    message.error('上传失败');
  } finally {
    uploading.value = false;
  }
};

const downloadFile = (record: any) => {
  if (record.is_dir) {
    message.warning('暂不支持下载目录');
    return;
  }
  const token = getToken();
  const filePath = record.key;
  const downloadUrl = `${window.location.origin}/api/servers/${serverId.value}/docker/containers/${containerId.value}/files/download?path=${encodeURIComponent(
    filePath
  )}&token=${token}`;
  window.open(downloadUrl, '_blank');
};

const deleteFiles = async (records?: any[]) => {
  const targets = records || selectedFiles.value;
  if (targets.length === 0) {
    message.warning('请选择要删除的文件或目录');
    return;
  }
  Modal.confirm({
    title: '确认删除',
    content: `确定删除选中的 ${targets.length} 个项目吗？`,
    onOk: async () => {
      try {
        await request.post(buildUrl('/files/delete'), { paths: targets.map((f) => f.key) });
        message.success('删除成功');
        fetchFileList(currentPath.value);
      } catch (error) {
        console.error(error);
        message.error('删除失败');
      }
    }
  });
};

const createFileOrDirectory = async () => {
  if (!createFormState.name) {
    message.warning('请输入名称');
    return;
  }
  try {
    if (createFormState.type === 'file') {
      await request.post(buildUrl('/files/create'), {
        path: `${currentPath.value}/${createFormState.name}`.replace(/\/+/g, '/'),
        content: createFormState.content
      });
    } else {
      await request.post(buildUrl('/files/mkdir'), {
        path: `${currentPath.value}/${createFormState.name}`.replace(/\/+/g, '/')
      });
    }
    message.success('创建成功');
    createModalVisible.value = false;
    createFormState.name = '';
    createFormState.content = '';
    fetchFileList(currentPath.value);
  } catch (error) {
    console.error(error);
    message.error('创建失败');
  }
};

const jumpToPathConfirm = () => {
  if (!jumpToPath.value) {
    message.warning('请输入路径');
    return;
  }
  fetchFileList(jumpToPath.value);
  pathJumpModalVisible.value = false;
};

const onSelectionChange = (keys: string[], rows: any[]) => {
  selectedRowKeys.value = keys as string[];
  selectedFiles.value = rows;
};

const filterdFileList = computed(() => {
  if (!searchKeyword.value) return fileList.value;
  return fileList.value.filter((file) => file.name.toLowerCase().includes(searchKeyword.value.toLowerCase()));
});

const breadcrumbItems = computed(() => {
  const parts = currentPath.value.split('/').filter(Boolean);
  const items = [{ title: '根目录', path: '/' }];
  let acc = '';
  parts.forEach((p) => {
    acc += `/${p}`;
    items.push({ title: p, path: acc.replace(/\/+/g, '/') });
  });
  return items;
});

const buildUrl = (suffix: string) => `${basePrefix.value}${suffix}`;

onMounted(() => {
  fetchServerInfo();
  fetchFileList('/');
  fetchDirectoryTree();
});
</script>

<template>
  <div class="server-file-container">
    <!-- 顶部导航栏 -->
    <div class="glass-panel header-panel">
      <div class="header-left">
        <a-button type="text" class="back-btn" @click="router.push(`/admin/servers/${serverId}/docker`)">
          <template #icon>
            <ArrowLeftOutlined />
          </template>
          返回容器列表
        </a-button>
        <div class="header-title">
          <span class="title-text">容器文件管理</span>
          <span class="subtitle-text">{{ containerName }}</span>
        </div>
      </div>
      <div class="header-right">
        <a-space>
          <a-button type="primary" class="action-btn" @click="createModalVisible = true">
            <template #icon>
              <PlusOutlined />
            </template>
            新建
          </a-button>
          <a-button class="action-btn" @click="uploadModalVisible = true">
            <template #icon>
              <CloudUploadOutlined />
            </template>
            上传
          </a-button>
          <a-button class="action-btn" danger :disabled="selectedFiles.length === 0" @click="deleteFiles()">
            <template #icon>
              <DeleteOutlined />
            </template>
            删除
          </a-button>
          <a-button class="action-btn" @click="() => fetchFileList(currentPath)">
            <template #icon>
              <ReloadOutlined />
            </template>
            刷新
          </a-button>
          <a-button class="action-btn" @click="() => (pathJumpModalVisible = true)">
            <template #icon>
              <EnterOutlined />
            </template>
            跳转
          </a-button>
        </a-space>
      </div>
    </div>

    <!-- 主内容区 -->
    <div class="content-wrapper">
      <!-- 左侧目录树 -->
      <div class="glass-panel sidebar-panel">
        <div class="sidebar-header">
          <span class="sidebar-title">目录结构</span>
        </div>
        <div class="tree-container">
          <div v-if="treeLoading" class="loading-tree">
            <a-spin size="small" />
          </div>
          <a-tree v-else-if="treeData.length > 0" :tree-data="treeData" :expanded-keys="expandedKeys"
            :auto-expand-parent="autoExpandParent" block-node show-line
            @expand="(keys: string[]) => (expandedKeys = keys)" @select="onTreeSelect"
            :load-data="loadTreeNodeChildren">
            <template #icon="{ data }">
              <FolderOutlined />
            </template>
          </a-tree>
          <div v-else class="empty-tree">
            <span class="text-secondary">无子目录</span>
          </div>
        </div>
      </div>

      <!-- 右侧文件列表 -->
      <div class="glass-panel main-panel">
        <!-- 工具栏 -->
        <div class="toolbar-section">
          <div class="breadcrumb-nav">
            <a-button type="text" size="small" @click="goUp" :disabled="currentPath === '/'">
              <ArrowLeftOutlined />
            </a-button>
            <a-breadcrumb separator="/">
              <a-breadcrumb-item v-for="item in breadcrumbItems" :key="item.path">
                <a v-if="item.path !== currentPath" @click="fetchFileList(item.path)">{{ item.title }}</a>
                <span v-else>{{ item.title }}</span>
              </a-breadcrumb-item>
            </a-breadcrumb>
          </div>
          <div class="search-box">
            <a-input v-model:value="searchKeyword" placeholder="搜索当前目录..." allow-clear class="search-input">
              <template #prefix>
                <SearchOutlined />
              </template>
            </a-input>
          </div>
        </div>

        <!-- 文件表格 -->
        <div class="table-container">
          <a-table :data-source="filterdFileList" :loading="loading" row-key="key" size="middle" :pagination="false"
            :row-selection="{
              selectedRowKeys,
              onChange: onSelectionChange
            }" :scroll="{ y: 'calc(100vh - 300px)' }">
            <a-table-column title="名称" data-index="name" width="40%">
              <template #default="{ record }">
                <div class="file-name-cell">
                  <component :is="getFileIcon(record)" class="file-icon" :class="{ 'is-dir': record.is_dir }" />
                  <a v-if="record.is_dir" class="file-link" @click="openPath(record.key)">{{ record.name }}</a>
                  <a v-else class="file-link" @click="openFile(record)">{{ record.name }}</a>
                </div>
              </template>
            </a-table-column>
            <a-table-column title="大小" data-index="size" width="15%">
              <template #default="{ record }">
                <span v-if="record.is_dir" class="text-secondary">-</span>
                <span v-else class="text-secondary">{{ formatFileSize(record.size) }}</span>
              </template>
            </a-table-column>
            <a-table-column title="修改时间" data-index="mod_time" width="25%">
              <template #default="{ record }">
                <span class="text-secondary">{{ record.mod_time || record.modTime }}</span>
              </template>
            </a-table-column>
            <a-table-column title="权限" data-index="mode" width="10%">
              <template #default="{ record }">
                <span class="text-secondary">{{ record.mode }}</span>
              </template>
            </a-table-column>
            <a-table-column title="操作" width="10%">
              <template #default="{ record }">
                <a-space size="small">
                  <a-tooltip title="编辑" v-if="!record.is_dir">
                    <a-button type="text" size="small" @click="openFile(record)">
                      <EditOutlined />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="下载" v-if="!record.is_dir">
                    <a-button type="text" size="small" @click="downloadFile(record)">
                      <DownloadOutlined />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="删除">
                    <a-button type="text" danger size="small" @click="deleteFiles([record])">
                      <DeleteOutlined />
                    </a-button>
                  </a-tooltip>
                </a-space>
              </template>
            </a-table-column>
          </a-table>
        </div>
      </div>
    </div>

    <!-- 上传文件弹窗 -->
    <a-modal v-model:open="uploadModalVisible" title="上传文件" @ok="handleUpload" :confirm-loading="uploading"
      class="glass-modal">
      <div class="upload-area">
        <a-upload-dragger :before-upload="() => false" :show-upload-list="false"
          @change="(info: any) => (fileToUpload = info.file)">
          <p class="ant-upload-drag-icon">
            <CloudUploadOutlined />
          </p>
          <p class="ant-upload-text">点击或拖拽文件到此区域上传</p>
          <p class="ant-upload-hint" v-if="fileToUpload">
            已选择: {{ fileToUpload.name }}
          </p>
        </a-upload-dragger>
      </div>
    </a-modal>

    <!-- 编辑文件 -->
    <a-modal v-model:open="editModalVisible" :title="editingFile?.name || '编辑文件'" :confirm-loading="editLoading"
      width="80%" @ok="saveFile" class="glass-modal editor-modal" :bodyStyle="{ padding: 0 }">
      <Codemirror v-model="fileContent" :extensions="codemirrorExtensions" :style="{ height: '70vh' }"
        :placeholder="editingFile?.name || ''" />
    </a-modal>

    <!-- 新建文件或目录 -->
    <a-modal v-model:open="createModalVisible" title="新建" @ok="createFileOrDirectory" class="glass-modal">
      <a-form layout="vertical">
        <a-form-item label="类型">
          <a-radio-group v-model:value="createFormState.type" button-style="solid">
            <a-radio-button value="file">
              <FileAddOutlined /> 文件
            </a-radio-button>
            <a-radio-button value="directory">
              <FolderAddOutlined /> 目录
            </a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-form-item label="名称">
          <a-input v-model:value="createFormState.name" placeholder="请输入名称" />
        </a-form-item>
        <a-form-item v-if="createFormState.type === 'file'" label="初始内容">
          <a-textarea v-model:value="createFormState.content" rows="4" placeholder="可选" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 跳转路径 -->
    <a-modal v-model:open="pathJumpModalVisible" title="跳转到路径" @ok="jumpToPathConfirm" class="glass-modal">
      <a-input v-model:value="jumpToPath" placeholder="/etc" prefix="/" />
    </a-modal>
  </div>
</template>

<style scoped>
.server-file-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 16px;
  padding: 16px;
  background-color: #f5f5f7;
  /* macOS background */
}

/* Glass Panel Styles */
.glass-panel {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.3);
  border-radius: 12px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.05);
}

.header-panel {
  padding: 16px 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.back-btn {
  color: #666;
}

.header-title {
  display: flex;
  flex-direction: column;
}

.title-text {
  font-size: 18px;
  font-weight: 600;
  color: #1d1d1f;
}

.subtitle-text {
  font-size: 12px;
  color: #86868b;
}

.action-btn {
  border-radius: 8px;
}

.content-wrapper {
  display: flex;
  gap: 16px;
  flex: 1;
  min-height: 0;
  /* Important for nested scrolling */
}

.sidebar-panel {
  width: 280px;
  display: flex;
  flex-direction: column;
  padding: 16px;
}

.sidebar-header {
  padding-bottom: 12px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  margin-bottom: 12px;
}

.sidebar-title {
  font-weight: 600;
  color: #1d1d1f;
}

.tree-container {
  flex: 1;
  overflow-y: auto;
}

.empty-tree {
  display: flex;
  justify-content: center;
  padding: 20px;
}

.main-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}

.toolbar-section {
  padding: 12px 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.breadcrumb-nav {
  display: flex;
  align-items: center;
  gap: 8px;
}

.search-box {
  width: 240px;
}

.search-input {
  border-radius: 8px;
}

.table-container {
  flex: 1;
  overflow: hidden;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-icon {
  font-size: 18px;
  color: #86868b;
}

.file-icon.is-dir {
  color: #007aff;
  /* macOS Blue */
}

.file-link {
  color: #1d1d1f;
  text-decoration: none;
}

.file-link:hover {
  color: #007aff;
}

.text-secondary {
  color: #86868b;
}

/* Custom Scrollbar */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(0, 0, 0, 0.2);
}

/* Ant Design Overrides for Glassmorphism */
:deep(.ant-table-wrapper) {
  height: 100%;
}

:deep(.ant-table) {
  background: transparent;
}

:deep(.ant-table-thead > tr > th) {
  background: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

:deep(.ant-tree) {
  background: transparent;
}

:deep(.ant-tree .ant-tree-node-content-wrapper:hover) {
  background: rgba(0, 122, 255, 0.1);
}

:deep(.ant-tree .ant-tree-node-selected) {
  background: rgba(0, 122, 255, 0.15) !important;
}
</style>
<style>
.dark .server-file-container {
  background-color: #1e1e1e;
}

.dark .glass-panel {
  background: rgba(30, 30, 30, 0.7);
  border: 1px solid rgba(255, 255, 255, 0.05);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2);
}

.dark .back-btn {
  color: #aaa;
}

.dark .title-text {
  color: #e0e0e0;
}

.dark .subtitle-text {
  color: #888;
}

.dark .sidebar-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.dark .sidebar-title {
  color: #e0e0e0;
}

.dark .toolbar-section {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.dark .file-link {
  color: #e0e0e0;
}

.dark .text-secondary {
  color: #888;
}

.dark .ant-table-thead > tr > th {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  color: #ccc;
  background: rgba(30, 30, 30, 0.5);
}

.dark .ant-table-tbody > tr > td {
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.dark .ant-table-tbody > tr:hover > td {
  background: rgba(0, 122, 255, 0.05) !important;
}
</style>
