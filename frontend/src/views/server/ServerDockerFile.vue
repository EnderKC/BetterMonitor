<script setup lang="ts">
import { ref, reactive, onMounted, computed, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal } from 'ant-design-vue';
import {
  ArrowLeftOutlined,
  CloudUploadOutlined,
  FileAddOutlined,
  FolderAddOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
import { getToken } from '../../utils/auth';
import { useServerStore } from '../../stores/serverStore';
import FileManager from '../../components/server/FileManager.vue';
import CodeEditor from '../../components/server/CodeEditor.vue';

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

const basePrefix = computed(() => `/servers/${serverId.value}/docker/containers/${containerId.value}`);
const isServerOnline = computed(() => serverStore.isServerOnline(serverId.value));

const buildUrl = (suffix: string) => `${basePrefix.value}${suffix}`;

const fetchServerInfo = async () => {
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
  }
};

const fetchFileList = async (path: string = '/') => {
  loading.value = true;
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
      path: `${path}/${file.name}`.replace(/\/+/g, '/')
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

const handleNavigate = (path: string) => {
  fetchFileList(path);
};

const handleRefresh = () => {
  fetchFileList(currentPath.value);
};

const openFile = async (record: any) => {
  try {
    editLoading.value = true;
    const response: ApiResponse = await request.get(buildUrl('/files/content'), { params: { path: record.path || record.key } });
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
    await request.put(buildUrl('/files/content'), { path: editingFile.value.path || editingFile.value.key, content: fileContent.value });
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
  const token = getToken();
  const filePath = record.path || record.key;
  const downloadUrl = `${window.location.origin}/api/servers/${serverId.value}/docker/containers/${containerId.value}/files/download?path=${encodeURIComponent(
    filePath
  )}&token=${token}`;
  window.open(downloadUrl, '_blank');
};

const deleteFiles = async (record: any) => {
  try {
    await request.post(buildUrl('/files/delete'), { paths: [record.path || record.key] });
    message.success('删除成功');
    fetchFileList(currentPath.value);
  } catch (error) {
    console.error(error);
    message.error('删除失败');
  }
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

const handleCreateFile = () => {
  createFormState.type = 'file';
  createModalVisible.value = true;
};

const handleCreateFolder = () => {
  createFormState.type = 'directory';
  createModalVisible.value = true;
};

onMounted(() => {
  fetchServerInfo();
  fetchFileList('/');
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
    </div>

    <!-- 主内容区 -->
    <div class="content-wrapper">
      <div class="glass-panel main-panel">
        <FileManager
          :files="fileList"
          :current-path="currentPath"
          :loading="loading"
          @navigate="handleNavigate"
          @refresh="handleRefresh"
          @edit="openFile"
          @download="downloadFile"
          @delete="deleteFiles"
          @create-file="handleCreateFile"
          @create-folder="handleCreateFolder"
          @upload="uploadModalVisible = true"
        />
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
      <div style="height: 70vh;">
        <CodeEditor
          v-if="editModalVisible"
          v-model:value="fileContent"
          :filename="editingFile?.name"
        />
      </div>
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

.content-wrapper {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.main-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 16px;
  overflow: hidden;
}

.upload-area {
  padding: 20px;
}
</style>

<style>
/* Dark Mode Global Overrides */
.dark .server-file-container {
  background-color: #1e1e1e;
}

.dark .glass-panel {
  background: rgba(30, 30, 30, 0.7);
  border-color: rgba(255, 255, 255, 0.05);
}

.dark .title-text {
  color: #e0e0e0;
}

.dark .subtitle-text,
.dark .back-btn {
  color: #aaa;
}
</style>
