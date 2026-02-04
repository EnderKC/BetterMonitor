<template>
  <div class="server-terminal-page">
    <!-- 顶部导航栏 -->
    <a-page-header class="terminal-header" :title="serverInfo.name || `服务器 ${serverId}`" :sub-title="serverInfo.ip"
      @back="goBack">
      <template #tags>
        <a-tag :color="serverInfo.online ? 'success' : 'error'">
          {{ serverInfo.online ? '在线' : '离线' }}
        </a-tag>
        <a-tag v-if="currentSessionName" color="processing">{{ currentSessionName }}</a-tag>
      </template>
      <template #extra>
        <a-space>
          <a-tooltip title="文件管理器">
            <a-button @click="toggleFileManager" :type="fileManagerVisible ? 'primary' : 'default'"
              :disabled="!serverInfo.online">
              <template #icon>
                <FolderOpenOutlined />
              </template>
              文件
            </a-button>
          </a-tooltip>

          <a-tooltip title="切换编辑器">
            <a-button @click="toggleEditor" :type="editorVisible ? 'primary' : 'default'">
              <template #icon>
                <EditOutlined />
              </template>
              编辑器
              <a-badge v-if="editorTabs.length > 0" :count="editorTabs.length" :offset="[10, -5]"
                style="margin-left: 4px;" />
            </a-button>
          </a-tooltip>

          <a-tooltip title="系统状态">
            <a-button @click="toggleSystemStatus" :type="systemStatusVisible ? 'primary' : 'default'"
              :disabled="!serverInfo.online">
              <template #icon>
                <MenuOutlined />
              </template>
              状态
            </a-button>
          </a-tooltip>

          <a-button @click="checkHeartbeat" :loading="checkingHeartbeat">
            检查状态
          </a-button>
          <a-button @click="showCreateSessionModal" :disabled="!serverInfo.online">
            创建会话
          </a-button>
          <a-button type="primary" danger @click="disconnectTerminal" :disabled="!connected">
            断开连接
          </a-button>
        </a-space>
      </template>
    </a-page-header>

    <!-- 主要内容区域 -->
    <div class="main-content">
      <!-- 文件管理器侧边栏 -->
      <div v-if="fileManagerVisible" class="file-manager-sidebar" :style="{ width: fileManagerWidth + 'px' }">
        <FileManager :files="fileList" :current-path="currentPath" :loading="fileLoading" @navigate="handleNavigate"
          @refresh="refreshFileList" @edit="editFile" @download="handleDownloadFile" @delete="handleDeleteFile"
          @create-file="showCreateFileModal" @create-folder="showCreateFolderModal" @upload="triggerUpload" />
      </div>

      <!-- 主要工作区域 (编辑器 + 终端) -->
      <div class="workspace-container" :style="workspaceContainerStyle">
        <!-- 多标签页编辑器 -->
        <div v-if="editorVisible" class="editor-container" :style="{ height: editorHeight + 'px' }">
          <!-- 编辑器头部 -->
          <div class="editor-header">
            <div class="editor-tabs">
              <div v-for="tab in editorTabs" :key="tab.id" class="editor-tab"
                :class="{ 'editor-tab-active': tab.id === activeTabId }" @click="switchToTab(tab.id)">
                <div class="tab-content">
                  <div class="tab-icon">
                    <component :is="getFileIcon(tab.file)" />
                  </div>
                  <span class="tab-name" :title="tab.file.path">{{ tab.file.name }}</span>
                  <span class="tab-language">{{ tab.language }}</span>
                  <div v-if="tab.isDirty" class="tab-dirty-indicator">●</div>
                  <a-spin v-if="tab.isLoading" size="small" />
                </div>
                <a-button size="small" type="text" @click.stop="closeEditorTab(tab.id)" class="tab-close">
                  <template #icon>
                    <CloseOutlined />
                  </template>
                </a-button>
              </div>
            </div>

            <div class="editor-actions">
              <a-tooltip title="保存当前文件">
                <a-button size="small" type="text" @click="saveActiveTab"
                  :disabled="!activeTab || !activeTab.isDirty || activeTab.isLoading">
                  <template #icon>
                    <SaveOutlined />
                  </template>
                </a-button>
              </a-tooltip>
              <a-tooltip title="保存所有文件">
                <a-button size="small" type="text" @click="saveAllTabs" :disabled="!editorTabs.some(t => t.isDirty)">
                  <template #icon>
                    <SaveOutlined style="color: #52c41a;" />
                  </template>
                </a-button>
              </a-tooltip>
              <a-tooltip title="关闭所有标签页">
                <a-button size="small" type="text" @click="closeAllTabs" danger>
                  <template #icon>
                    <CloseOutlined />
                  </template>
                </a-button>
              </a-tooltip>
            </div>
          </div>

          <!-- 编辑器内容 -->
          <div class="editor-content">
            <div v-if="!activeTab" class="editor-empty">
              <div class="empty-content">
                <EditOutlined style="font-size: 48px; color: #d9d9d9; margin-bottom: 16px;" />
                <p style="color: #999;">没有打开的文件</p>
                <p style="color: #ccc; font-size: 12px;">点击文件管理器中的可编辑文件来开始编辑</p>
              </div>
            </div>
            <div v-else class="editor-main">
              <div v-if="activeTab.isLoading" class="editor-loading">
                <a-spin size="large" tip="正在加载文件内容..." />
              </div>
              <div v-else class="code-editor-wrapper">
                <CodeEditor v-model:value="activeTab.content" :filename="activeTab.file.name"
                  :language="activeTab.language" @change="(content) => onEditorContentChange(content)"
                  @save="saveActiveTab" />
              </div>
            </div>
          </div>

          <!-- 拖拽调整高度的控制条 -->
          <div class="editor-resize-handle" @mousedown="startResize">
            <div class="resize-line"></div>
          </div>
        </div>

        <!-- 终端区域 -->
        <div class="terminal-section" :style="terminalContainerStyle">
          <a-spin :spinning="loading">
            <a-alert v-if="!serverInfo.online && !connected" type="warning" show-icon message="服务器当前离线，无法使用终端功能"
              style="margin-bottom: 12px">
              <template #action>
                <a-button type="primary" size="small" @click="checkHeartbeat">
                  检查状态
                </a-button>
              </template>
            </a-alert>

            <a-alert v-if="serverInfo.online && agentNotConnected" type="warning" show-icon
              message="服务器Agent未连接，终端功能可能无法正常使用" style="margin-bottom: 12px">
              <template #action>
                <a-button type="primary" size="small" @click="checkHeartbeat">
                  刷新
                </a-button>
              </template>
            </a-alert>

            <div v-else class="terminal-container-wrapper">
              <!-- 终端主体 -->
              <div class="terminal-wrapper">
              <TerminalView ref="terminalViewRef" :socket-url="terminalSocketUrl" :session="currentSession"
                  @connected="onTerminalConnected" @disconnected="onTerminalDisconnected" @error="onTerminalError" />
              </div>

              <!-- 会话管理控制器 (右下角) -->
              <div class="session-controller">
                <div class="session-select-compact">
                  <span v-if="connected" class="connection-status">
                    <a-tag color="processing" size="small">{{ currentSessionName }}</a-tag>
                    <a-tag color="success" size="small">已连接</a-tag>
                  </span>
                  <span v-else class="connection-status">
                    <a-tag color="default" size="small">未连接</a-tag>
                  </span>
                  <a-select v-model:value="currentSession" placeholder="选择会话" size="small" style="width: 120px"
                    :disabled="connected">
                    <a-select-option v-for="session in sessions" :key="session.id" :value="session.id">
                      {{ session.name }}
                    </a-select-option>
                  </a-select>
                </div>

                <div class="session-actions-compact">
                  <a-space size="small">
                    <a-button size="small" type="primary" @click="connectTerminal"
                      :disabled="!currentSession || connected">
                      连接
                    </a-button>
                    <a-popconfirm title="确定要删除此会话吗？" ok-text="确定" cancel-text="取消"
                      @confirm="deleteSession(currentSession)">
                      <a-button size="small" danger :disabled="!currentSession || connected">
                        删除
                      </a-button>
                    </a-popconfirm>
                  </a-space>
                </div>
              </div>
            </div>
          </a-spin>
        </div>
      </div>

      <!-- 系统状态侧边栏 -->
      <div v-if="systemStatusVisible" class="system-status-sidebar" :style="{ width: systemStatusWidth + 'px' }">
        <div class="system-status-header">
          <div class="header-title">
            <MenuOutlined />
            <span>系统状态</span>
          </div>
          <a-tooltip title="刷新">
            <a-button size="small" type="text" @click="refreshSystemStatus">
              <template #icon>
                <ReloadOutlined />
              </template>
            </a-button>
          </a-tooltip>
        </div>

        <div class="system-status-content">
          <!-- CPU 使用率 -->
          <div class="status-item">
            <div class="status-header">
              <span class="status-title">CPU 使用率</span>
              <span class="status-value">{{ formatPercentage(systemStatusData.cpu_usage) }}</span>
            </div>
            <a-progress :percent="systemStatusData.cpu_usage"
              :stroke-color="getProgressColor(systemStatusData.cpu_usage)" :showInfo="false" size="small" />
          </div>

          <!-- 内存使用 -->
          <div class="status-item">
            <div class="status-header">
              <span class="status-title">内存使用</span>
              <span class="status-value">{{ formatPercentage(getMemoryPercentage()) }}</span>
            </div>
            <a-progress :percent="getMemoryPercentage()" :stroke-color="getProgressColor(getMemoryPercentage())"
              :showInfo="false" size="small" />
            <div class="status-detail">
              {{ formatBytes(systemStatusData.memory_used) }} / {{ formatBytes(systemStatusData.memory_total) }}
            </div>
          </div>

          <!-- 磁盘使用 -->
          <div class="status-item">
            <div class="status-header">
              <span class="status-title">磁盘使用</span>
              <span class="status-value">{{ formatPercentage(getDiskPercentage()) }}</span>
            </div>
            <a-progress :percent="getDiskPercentage()" :stroke-color="getProgressColor(getDiskPercentage())"
              :showInfo="false" size="small" />
            <div class="status-detail">
              {{ formatBytes(systemStatusData.disk_used) }} / {{ formatBytes(systemStatusData.disk_total) }}
            </div>
          </div>

          <!-- 系统负载 -->
          <div class="status-item">
            <div class="status-header">
              <span class="status-title">系统负载</span>
            </div>
            <div class="load-metrics">
              <div class="load-item">
                <span class="load-label">1分钟</span>
                <span class="load-value">{{ systemStatusData.load_avg_1.toFixed(2) }}</span>
              </div>
              <div class="load-item">
                <span class="load-label">5分钟</span>
                <span class="load-value">{{ systemStatusData.load_avg_5.toFixed(2) }}</span>
              </div>
              <div class="load-item">
                <span class="load-label">15分钟</span>
                <span class="load-value">{{ systemStatusData.load_avg_15.toFixed(2) }}</span>
              </div>
            </div>
          </div>

          <!-- 网络速度 -->
          <div class="status-item">
            <div class="status-header">
              <span class="status-title">网络速度</span>
            </div>
            <div class="network-metrics">
              <div class="network-item">
                <span class="network-label">↓ 入站</span>
                <span class="network-value">{{ formatNetworkSpeed(systemStatusData.network_in) }}</span>
              </div>
              <div class="network-item">
                <span class="network-label">↑ 出站</span>
                <span class="network-value">{{ formatNetworkSpeed(systemStatusData.network_out) }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 创建会话对话框 -->
    <a-modal v-model:visible="sessionModalVisible" title="创建新终端会话" @ok="createSession" :maskClosable="false">
      <a-form layout="vertical">
        <a-form-item label="会话名称" required>
          <a-input v-model:value="sessionName" placeholder="请输入会话名称" @pressEnter="createSession" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 新建文件对话框 -->
    <a-modal v-model:visible="newFileModalVisible" title="新建文件" @ok="handleNewFile"
      @cancel="newFileModalVisible = false">
      <a-form layout="vertical">
        <a-form-item label="文件名" required>
          <a-input v-model:value="newFileName" placeholder="请输入文件名（如：example.txt）" @pressEnter="handleNewFile" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 新建文件夹对话框 -->
    <a-modal v-model:visible="newFolderModalVisible" title="新建文件夹" @ok="handleNewFolder"
      @cancel="newFolderModalVisible = false">
      <a-form layout="vertical">
        <a-form-item label="文件夹名" required>
          <a-input v-model:value="newFolderName" placeholder="请输入文件夹名" @pressEnter="handleNewFolder" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 文件上传 -->
    <input ref="fileUploadInput" type="file" multiple style="display: none" @change="handleFileUpload" />
  </div>
</template>

<script setup lang="ts">
defineOptions({
  name: 'ServerTerminal'
});
import { ref, onMounted, onUnmounted, computed, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal } from 'ant-design-vue';
import {
  MenuOutlined,
  ReloadOutlined,
  EditOutlined,
  FolderOpenOutlined,
  SaveOutlined,
  CloseOutlined,
  FileTextOutlined,
  FileImageOutlined,
  FileOutlined,
  FolderOutlined
} from '@ant-design/icons-vue';
import service from '../../utils/request';
import { getToken } from '../../utils/auth';
import { useUIStore } from '../../stores/uiStore';

// Import reusable components
import TerminalView from '../../components/server/TerminalView.vue';
import FileManager from '../../components/server/FileManager.vue';
import CodeEditor from '../../components/server/CodeEditor.vue';

// 定义响应类型接口
interface ApiResponse<T = any> {
  success: boolean;
  message?: string;
  data?: T;
  [key: string]: any;
}

interface ServerStatusResponse {
  success: boolean;
  online: boolean;
  status: string;
  name: string;
  message: string;
  last_heartbeat: string;
  data: any;
}

interface FileItem {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  permission: string;
}

interface EditorTab {
  id: string;
  file: FileItem;
  content: string;
  originalContent: string;
  isDirty: boolean;
  isLoading: boolean;
  language: string;
}

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// 终端状态
const terminalViewRef = ref<InstanceType<typeof TerminalView> | null>(null);
const connected = ref(false);
const sessions = ref<{ id: string; name: string }[]>([]);
const currentSession = ref<string>('');
const sessionName = ref<string>('');
const sessionModalVisible = ref<boolean>(false);
const checkingHeartbeat = ref(false);
const agentNotConnected = ref(false);
let statusWs: WebSocket | null = null;
const uiStore = useUIStore();

// 文件管理器状态
const fileManagerVisible = ref(false);
const fileManagerWidth = ref(280);
const currentPath = ref('/');
const fileList = ref<FileItem[]>([]);
const fileLoading = ref(false);
const showHiddenFiles = ref(false);
const fileUploadInput = ref<HTMLInputElement | null>(null);
const newFileModalVisible = ref(false);
const newFolderModalVisible = ref(false);
const newFileName = ref('');
const newFolderName = ref('');

// 编辑器状态
const editorVisible = ref(false);
const editorTabs = ref<EditorTab[]>([]);
const activeTabId = ref<string>('');
const editorHeight = ref(350);
const minEditorHeight = 200;
const maxEditorHeight = ref(800);
const isFileOperationInProgress = ref(false);

// 系统状态
const systemStatusVisible = ref(false);
const systemStatusWidth = ref(280);
const systemStatusData = ref({
  cpu_usage: 0,
  memory_used: 0,
  memory_total: 0,
  disk_used: 0,
  disk_total: 0,
  load_avg_1: 0,
  load_avg_5: 0,
  load_avg_15: 0,
  network_in: 0,
  network_out: 0
});

// 计算终端Socket URL
const terminalSocketUrl = computed(() => {
  if (!serverInfo.value.online || !currentSession.value) return '';
  const token = getToken();
  if (!token) return '';
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  return `${protocol}//${host}/api/servers/${serverId.value}/ws?token=${encodeURIComponent(token)}&session=${currentSession.value}`;
});

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    const statusResponse: ServerStatusResponse = await service.get(`/servers/${serverId.value}/status`);
    if (statusResponse && statusResponse.success) {
      serverInfo.value = {
        ...serverInfo.value,
        id: serverId.value,
        name: statusResponse.name || `服务器 ${serverId.value}`,
        status: statusResponse.status,
        online: statusResponse.online
      };

      if (statusResponse.online) {
        try {
          const detailResponse: ApiResponse = await service.get(`/servers/${serverId.value}`);
          if (detailResponse && detailResponse.server) {
            serverInfo.value = { ...serverInfo.value, ...detailResponse.server, online: true };
          }
        } catch (error) {
          console.error('获取服务器详情失败:', error);
        }
      }
      connectStatusWebSocket();
    } else {
      message.error('获取服务器状态失败');
    }
  } catch (error) {
    console.error('获取服务器信息失败:', error);
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
    uiStore.stopLoading();
  }
};

// 文件管理器操作
const fetchFileList = async (path: string = currentPath.value) => {
  if (!serverInfo.value.online) {
    message.warning('服务器未在线，无法访问文件');
    return;
  }

  fileLoading.value = true;
  try {
    const response = await service.get(`/servers/${serverId.value}/files`, { params: { path } });
    let files: FileItem[] = [];
    if (Array.isArray(response)) {
      files = response;
    } else if (response && typeof response === 'object') {
      if (Array.isArray(response.files)) files = response.files;
      else if (Array.isArray(response.data)) files = response.data;
    }

    files = files.map(file => ({
      ...file,
      path: path === '/' ? `/${file.name}` : `${path}/${file.name}`
    }));

    if (!showHiddenFiles.value) {
      files = files.filter(file => !file.name.startsWith('.'));
    }

    files.sort((a, b) => {
      if (a.is_dir && !b.is_dir) return -1;
      if (!a.is_dir && b.is_dir) return 1;
      return a.name.localeCompare(b.name);
    });

    fileList.value = files;
    currentPath.value = path;
  } catch (error: any) {
    console.error('获取文件列表失败:', error);
    message.error(`获取文件列表失败: ${error.message || '网络错误'}`);
    fileList.value = [];
  } finally {
    fileLoading.value = false;
  }
};

const handleNavigate = (path: string) => {
  fetchFileList(path);
};

const refreshFileList = () => {
  fetchFileList();
};

const triggerUpload = () => {
  fileUploadInput.value?.click();
};

const handleFileUpload = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  if (!input.files || input.files.length === 0) return;

  const files = Array.from(input.files);
  const formData = new FormData();
  files.forEach(file => {
    formData.append('files', file);
  });
  formData.append('path', currentPath.value);

  const hide = message.loading('正在上传文件...', 0);
  try {
    await service.post(`/servers/${serverId.value}/files/upload`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    });
    message.success('文件上传成功');
    refreshFileList();
  } catch (error) {
    console.error('上传失败:', error);
    message.error('文件上传失败');
  } finally {
    hide();
    input.value = '';
  }
};

const showCreateFileModal = () => {
  newFileName.value = '';
  newFileModalVisible.value = true;
};

const showCreateFolderModal = () => {
  newFolderName.value = '';
  newFolderModalVisible.value = true;
};

const handleNewFile = async () => {
  if (!newFileName.value.trim()) return message.error('请输入文件名');
  const filePath = currentPath.value === '/' ? `/${newFileName.value.trim()}` : `${currentPath.value}/${newFileName.value.trim()}`;
  try {
    await service.post(`/servers/${serverId.value}/files/create`, { path: filePath, content: '' });
    message.success('文件创建成功');
    newFileModalVisible.value = false;
    refreshFileList();
  } catch (error) {
    message.error('创建文件失败');
  }
};

const handleNewFolder = async () => {
  if (!newFolderName.value.trim()) return message.error('请输入文件夹名');
  const folderPath = currentPath.value === '/' ? `/${newFolderName.value.trim()}` : `${currentPath.value}/${newFolderName.value.trim()}`;
  try {
    await service.post(`/servers/${serverId.value}/files/mkdir`, { path: folderPath });
    message.success('文件夹创建成功');
    newFolderModalVisible.value = false;
    refreshFileList();
  } catch (error) {
    message.error('创建文件夹失败');
  }
};

const handleDeleteFile = async (file: FileItem) => {
  try {
    await service.post(`/servers/${serverId.value}/files/delete`, { paths: [file.path] });
    message.success('删除成功');
    refreshFileList();
  } catch (error) {
    message.error('删除失败');
  }
};

const handleDownloadFile = async (file: FileItem) => {
  try {
    const token = getToken();
    if (!token) return message.error('请先登录');
    const response = await service.get(`/servers/${serverId.value}/files/download`, {
      params: { path: file.path, token },
      responseType: 'blob'
    });
    const blob = response.data || response;
    const url = window.URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = file.name;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    window.URL.revokeObjectURL(url);
    message.success('文件下载成功');
  } catch (error) {
    message.error('下载文件失败');
  }
};

// 编辑器操作
const isEditableFile = (file: FileItem) => {
  if (file.is_dir) return false;
  const ext = file.name.split('.').pop()?.toLowerCase();
  const editableExtensions = ['txt', 'log', 'md', 'conf', 'ini', 'yaml', 'yml', 'json', 'xml', 'html', 'css', 'js', 'ts', 'vue', 'sh', 'py', 'go', 'java', 'php', 'sql', 'dockerfile'];
  return ext && editableExtensions.includes(ext);
};

const editFile = async (file: FileItem) => {
  if (file.is_dir) return message.warning('无法编辑文件夹');
  if (!isEditableFile(file)) return message.warning('此文件类型不支持在线编辑');
  if (file.size > 5 * 1024 * 1024) return message.warning('文件过大，无法在线编辑');

  const existingTab = editorTabs.value.find(tab => tab.file.path === file.path);
  if (existingTab) {
    activeTabId.value = existingTab.id;
    editorVisible.value = true;
    return;
  }

  const tabId = `tab_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  const newTab: EditorTab = {
    id: tabId,
    file: file,
    content: '',
    originalContent: '',
    isDirty: false,
    isLoading: true,
    language: detectLanguage(file.name)
  };

  editorTabs.value.push(newTab);
  activeTabId.value = tabId;
  editorVisible.value = true;

  try {
    const response = await service.get(`/servers/${serverId.value}/files/content`, { params: { path: file.path } });
    let content = '';
    if (typeof response === 'string') content = response;
    else if (response && typeof response === 'object') {
      content = response.data || response.content || JSON.stringify(response);
    }
    const tab = editorTabs.value.find(t => t.id === tabId);
    if (tab) {
      tab.content = content;
      tab.originalContent = content;
      tab.isLoading = false;
    }
  } catch (error) {
    message.error('读取文件内容失败');
    closeEditorTab(tabId);
  }
};

const detectLanguage = (fileName: string): string => {
  const ext = fileName.split('.').pop()?.toLowerCase();
  const langMap: Record<string, string> = {
    'js': 'javascript', 'ts': 'typescript', 'vue': 'html', 'html': 'html', 'css': 'css',
    'json': 'json', 'md': 'markdown', 'py': 'python', 'go': 'go', 'java': 'java',
    'sh': 'shell', 'sql': 'sql', 'yaml': 'yaml', 'yml': 'yaml'
  };
  return langMap[ext || ''] || 'text';
};

const closeEditorTab = (tabId: string) => {
  const tabIndex = editorTabs.value.findIndex(tab => tab.id === tabId);
  if (tabIndex === -1) return;
  const tab = editorTabs.value[tabIndex];
  if (tab.isDirty) {
    Modal.confirm({
      title: '未保存的更改',
      content: `文件 "${tab.file.name}" 有未保存的更改，确定要关闭吗？`,
      okText: '强制关闭',
      okType: 'danger',
      cancelText: '取消',
      onOk() { doCloseTab(tabId, tabIndex); }
    });
  } else {
    doCloseTab(tabId, tabIndex);
  }
};

const doCloseTab = (tabId: string, tabIndex: number) => {
  editorTabs.value.splice(tabIndex, 1);
  if (activeTabId.value === tabId) {
    if (editorTabs.value.length > 0) {
      const newIndex = tabIndex < editorTabs.value.length ? tabIndex : tabIndex - 1;
      activeTabId.value = editorTabs.value[newIndex].id;
    } else {
      activeTabId.value = '';
      editorVisible.value = false;
    }
  }
};

const activeTab = computed(() => editorTabs.value.find(tab => tab.id === activeTabId.value));
const switchToTab = (tabId: string) => { activeTabId.value = tabId; };

const onEditorContentChange = (content: string) => {
  const tab = activeTab.value;
  if (!tab) return;
  tab.content = content;
  tab.isDirty = content !== tab.originalContent;
};

const saveActiveTab = async () => {
  const tab = activeTab.value;
  if (!tab || tab.isLoading) return;
  tab.isLoading = true;
  isFileOperationInProgress.value = true;
  try {
    await service.put(`/servers/${serverId.value}/files/content`, {
      path: tab.file.path,
      content: tab.content
    });
    tab.isDirty = false;
    tab.originalContent = tab.content;
    message.success(`文件 "${tab.file.name}" 保存成功`);
  } catch (error) {
    message.error(`保存文件 "${tab.file.name}" 失败`);
  } finally {
    tab.isLoading = false;
    isFileOperationInProgress.value = false;
  }
};

const saveAllTabs = async () => {
  const dirtyTabs = editorTabs.value.filter(tab => tab.isDirty && !tab.isLoading);
  if (dirtyTabs.length === 0) return message.info('没有需要保存的文件');
  message.loading('正在保存所有文件...', 0);
  isFileOperationInProgress.value = true;
  try {
    await Promise.all(dirtyTabs.map(async (tab) => {
      tab.isLoading = true;
      try {
        await service.put(`/servers/${serverId.value}/files/content`, {
          path: tab.file.path,
          content: tab.content
        });
        tab.isDirty = false;
        tab.originalContent = tab.content;
      } finally {
        tab.isLoading = false;
      }
    }));
    message.destroy();
    message.success('所有文件保存成功');
  } catch (error) {
    message.destroy();
    message.error('保存文件时发生错误');
  } finally {
    isFileOperationInProgress.value = false;
  }
};

const closeAllTabs = () => {
  if (editorTabs.value.some(tab => tab.isDirty)) {
    Modal.confirm({
      title: '未保存的更改',
      content: '有文件有未保存的更改，确定要全部关闭吗？',
      okText: '强制关闭全部',
      okType: 'danger',
      cancelText: '取消',
      onOk() {
        editorTabs.value = [];
        activeTabId.value = '';
        editorVisible.value = false;
      }
    });
  } else {
    editorTabs.value = [];
    activeTabId.value = '';
    editorVisible.value = false;
  }
};

// 终端操作
const fetchSessions = async () => {
  try {
    const response: ServerStatusResponse = await service.get(`/servers/${serverId.value}/terminal/sessions`);
    if (response && response.success && response.data) {
      sessions.value = Array.isArray(response.data) ? response.data : [];
      if (sessions.value.length > 0 && !currentSession.value) {
        currentSession.value = sessions.value[0].id;
      }
    } else {
      sessions.value = [];
    }
  } catch (error) {
    sessions.value = [];
  }
};

const showCreateSessionModal = () => {
  sessionName.value = '';
  sessionModalVisible.value = true;
};

const createSession = async () => {
  if (!sessionName.value.trim()) return message.warning('请输入会话名称');
  try {
    const response: ServerStatusResponse = await service.post(`/servers/${serverId.value}/terminal/sessions`, {
      name: sessionName.value.trim()
    });
    if (response && response.success) {
      message.success('会话创建成功');
      sessionModalVisible.value = false;
      await fetchSessions();
      if (response.data?.id) {
        currentSession.value = response.data.id;
        // TerminalView will auto-connect due to computed socketUrl change
      }
    } else {
      message.error(response?.message || '创建会话失败');
    }
  } catch (error) {
    message.error('创建会话失败');
  }
};

const deleteSession = async (sessionId: string) => {
  try {
    await service.delete(`/servers/${serverId.value}/terminal/sessions/${sessionId}`);
    message.success('会话已删除');
    if (currentSession.value === sessionId) {
      currentSession.value = '';
    }
    fetchSessions();
  } catch (error) {
    message.error('删除会话失败');
  }
};

const connectTerminal = () => {
  if (!currentSession.value) return message.warning('请先选择一个会话');
  terminalViewRef.value?.connect();
};

const disconnectTerminal = () => {
  terminalViewRef.value?.disconnect();
};

const onTerminalConnected = () => {
  connected.value = true;
  agentNotConnected.value = false;
};

const onTerminalDisconnected = () => {
  connected.value = false;
};

const onTerminalError = (msg: string) => {
  if (msg.includes('Agent')) agentNotConnected.value = true;
};

const currentSessionName = computed(() => {
  const session = sessions.value.find(s => s.id === currentSession.value);
  return session ? session.name : '';
});

// 系统状态
const connectStatusWebSocket = () => {
  if (statusWs) statusWs.close();
  const token = getToken();
  if (!token) return;
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const wsUrl = `${protocol}//${host}/api/servers/${serverId.value}/ws?token=${encodeURIComponent(token)}`;

  statusWs = new WebSocket(wsUrl);
  statusWs.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'welcome' || data.type === 'status' || data.type === 'heartbeat' || data.type === 'monitor') {
        if (data.data) updateSystemStatusData(data.data);
        if (data.data?.status) serverInfo.value.online = data.data.status === 'online';
      }
    } catch (e) { }
  };
};

const updateSystemStatusData = (data: any) => {
  if (data.cpu_usage !== undefined) systemStatusData.value.cpu_usage = Number(data.cpu_usage);
  if (data.memory_used !== undefined) systemStatusData.value.memory_used = Number(data.memory_used);
  if (data.memory_total !== undefined) systemStatusData.value.memory_total = Number(data.memory_total);
  if (data.disk_used !== undefined) systemStatusData.value.disk_used = Number(data.disk_used);
  if (data.disk_total !== undefined) systemStatusData.value.disk_total = Number(data.disk_total);
  if (data.network_in !== undefined) systemStatusData.value.network_in = Number(data.network_in);
  if (data.network_out !== undefined) systemStatusData.value.network_out = Number(data.network_out);
  if (data.load_avg_1 !== undefined) systemStatusData.value.load_avg_1 = Number(data.load_avg_1);
  if (data.load_avg_5 !== undefined) systemStatusData.value.load_avg_5 = Number(data.load_avg_5);
  if (data.load_avg_15 !== undefined) systemStatusData.value.load_avg_15 = Number(data.load_avg_15);
};

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const formatPercentage = (value: number) => value.toFixed(1) + '%';
const getMemoryPercentage = () => systemStatusData.value.memory_total ? (systemStatusData.value.memory_used / systemStatusData.value.memory_total) * 100 : 0;
const getDiskPercentage = () => systemStatusData.value.disk_total ? (systemStatusData.value.disk_used / systemStatusData.value.disk_total) * 100 : 0;
const getProgressColor = (percentage: number) => percentage >= 90 ? '#ff4d4f' : percentage >= 70 ? '#faad14' : '#52c41a';
const formatNetworkSpeed = (bytes: number) => formatBytes(bytes) + '/s';

const refreshSystemStatus = async () => {
  try {
    const monitorResponse = await service.get(`/servers/${serverId.value}/monitor`);
    if (monitorResponse && monitorResponse.data && monitorResponse.data.length > 0) {
      updateSystemStatusData(monitorResponse.data[monitorResponse.data.length - 1]);
      message.success('系统状态已刷新');
    }
  } catch (error) {
    message.warning('获取监控数据失败');
  }
};

const checkHeartbeat = async () => {
  checkingHeartbeat.value = true;
  try {
    const statusResponse: ServerStatusResponse = await service.get(`/servers/${serverId.value}/status`);
    if (statusResponse && statusResponse.success) {
      serverInfo.value.online = statusResponse.online;
      serverInfo.value.status = statusResponse.status;
      if (statusResponse.online) {
        message.success('服务器在线');
        await fetchSessions();
        refreshSystemStatus();
      } else {
        message.warning('服务器离线');
      }
    }
  } catch (error) {
    message.error('检查状态失败');
  } finally {
    checkingHeartbeat.value = false;
  }
};

const toggleFileManager = () => { fileManagerVisible.value = !fileManagerVisible.value; };
const toggleEditor = () => { editorVisible.value = !editorVisible.value; };
const toggleSystemStatus = () => { systemStatusVisible.value = !systemStatusVisible.value; };
const goBack = () => { router.push(`/admin/servers/${serverId.value}`); };

const getFileIcon = (file: FileItem) => {
  if (file.is_dir) return FolderOutlined;
  const ext = file.name.split('.').pop()?.toLowerCase();
  if (['jpg', 'png', 'gif', 'svg'].includes(ext || '')) return FileImageOutlined;
  if (['txt', 'md', 'json', 'js', 'ts', 'html', 'css', 'py', 'go'].includes(ext || '')) return FileTextOutlined;
  return FileOutlined;
};

// 拖拽调整高度
let isResizing = false;
let startY = 0;
let startHeight = 0;

const startResize = (e: MouseEvent) => {
  isResizing = true;
  startY = e.clientY;
  startHeight = editorHeight.value;
  document.addEventListener('mousemove', handleEditorResize);
  document.addEventListener('mouseup', stopResize);
  document.body.style.cursor = 'ns-resize';
  document.body.style.userSelect = 'none';
};

const handleEditorResize = (e: MouseEvent) => {
  if (!isResizing) return;
  const deltaY = e.clientY - startY;
  const newHeight = Math.min(Math.max(startHeight + deltaY, minEditorHeight), maxEditorHeight.value);
  editorHeight.value = newHeight;
  terminalViewRef.value?.resize();
};

const stopResize = () => {
  isResizing = false;
  document.removeEventListener('mousemove', handleEditorResize);
  document.removeEventListener('mouseup', stopResize);
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  terminalViewRef.value?.resize();
};

const updateMaxEditorHeight = () => {
  maxEditorHeight.value = Math.max(800, window.innerHeight * 0.7);
};

// 布局样式
const workspaceContainerStyle = computed(() => {
  let width = '100%';
  const deductions = [];
  if (fileManagerVisible.value) deductions.push(`${fileManagerWidth.value}px`);
  if (systemStatusVisible.value) deductions.push(`${systemStatusWidth.value}px`);
  if (deductions.length > 0) width = `calc(100% - ${deductions.join(' - ')})`;
  return { width, height: 'fit-content', display: 'flex', flexDirection: 'column' as 'column' };
});

const terminalContainerStyle = computed(() => ({
  flex: 1,
  position: 'relative' as 'relative',
  height: editorVisible.value ? `calc(100% - ${editorHeight.value}px)` : '100%'
}));

onMounted(async () => {
  updateMaxEditorHeight();
  window.addEventListener('resize', updateMaxEditorHeight);
  await fetchServerInfo();
  await fetchSessions();
  if (sessions.value.length > 0 && !currentSession.value) {
    currentSession.value = sessions.value[0].id;
  }
});

onUnmounted(() => {
  window.removeEventListener('resize', updateMaxEditorHeight);
  if (statusWs) statusWs.close();
});
</script>

<style scoped>
.server-terminal-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  flex: 1;
  overflow: hidden;
  gap: 16px;
  padding: 0;
}

.terminal-header {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.main-content {
  display: flex;
  flex: 1;
  margin-top: 0;
  gap: 16px;
  height: calc(100vh - 140px);
  overflow: hidden;
  padding: 0 16px 16px 16px;
}

.file-manager-sidebar,
.system-status-sidebar {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
}

.workspace-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;
  overflow: hidden;
}

.editor-container {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid rgba(0, 0, 0, 0.05);
  position: relative;
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 8px;
  background: rgba(0, 0, 0, 0.03);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  height: 40px;
}

.editor-tabs {
  display: flex;
  overflow-x: auto;
  flex: 1;
}

.editor-tab {
  display: flex;
  align-items: center;
  padding: 0 12px;
  height: 40px;
  cursor: pointer;
  border-right: 1px solid rgba(0, 0, 0, 0.05);
  background: transparent;
  transition: all 0.2s;
  min-width: 120px;
  max-width: 200px;
}

.editor-tab:hover {
  background: rgba(0, 0, 0, 0.02);
}

.editor-tab-active {
  background: white;
  border-top: 2px solid #1890ff;
}

.tab-content {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  overflow: hidden;
}

.tab-name {
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tab-language {
  font-size: 10px;
  color: #999;
  background: rgba(0, 0, 0, 0.05);
  padding: 1px 4px;
  border-radius: 4px;
}

.tab-dirty-indicator {
  font-size: 12px;
  color: #faad14;
}

.editor-content {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.editor-main {
  height: 100%;
}

.code-editor-wrapper {
  height: 100%;
}

.editor-empty {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.02);
}

.empty-content {
  text-align: center;
}

.editor-resize-handle {
  height: 6px;
  background: transparent;
  cursor: ns-resize;
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
}

.editor-resize-handle:hover {
  background: rgba(24, 144, 255, 0.1);
}

.resize-line {
  width: 40px;
  height: 3px;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 2px;
}

.terminal-section {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px;
  padding: 16px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
  overflow: hidden;
}

.terminal-container-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.terminal-wrapper {
  flex: 1;
  background: #1e1e1e;
  border-radius: 12px;
  overflow: hidden;
  padding: 12px;
  box-shadow: inset 0 0 20px rgba(0, 0, 0, 0.5);
  position: relative;
}

.session-controller {
  margin-top: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(255, 255, 255, 0.5);
  padding: 8px 12px;
  border-radius: 12px;
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.session-select-compact,
.session-actions-compact {
  display: flex;
  align-items: center;
  gap: 12px;
}

.system-status-header {
  padding: 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.5);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.system-status-content {
  flex: 1;
  padding: 16px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.status-item {
  background: rgba(255, 255, 255, 0.5);
  border-radius: 12px;
  padding: 16px;
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.status-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.status-title {
  font-size: 13px;
  font-weight: 600;
}

.status-value {
  font-family: monospace;
  font-weight: 700;
  color: #1890ff;
}

.status-detail {
  font-size: 11px;
  color: #999;
  margin-top: 4px;
  text-align: right;
}

.load-metrics,
.network-metrics {
  display: flex;
  justify-content: space-between;
}

.load-item,
.network-item {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.load-label,
.network-label {
  font-size: 11px;
  color: #999;
}

.load-value,
.network-value {
  font-size: 13px;
  font-weight: 600;
}
</style>

<style>
/* Dark Mode Global Overrides */
.dark .terminal-header,
.dark .file-manager-sidebar,
.dark .system-status-sidebar,
.dark .editor-container,
.dark .terminal-section {
  background: rgba(30, 30, 30, 0.7);
  border-color: rgba(255, 255, 255, 0.1);
}

.dark .editor-header,
.dark .session-controller,
.dark .system-status-header,
.dark .status-item {
  background: rgba(0, 0, 0, 0.2);
  border-color: rgba(255, 255, 255, 0.05);
}

.dark .editor-tab:hover {
  background: rgba(255, 255, 255, 0.05);
}

.dark .editor-tab-active {
  background: rgba(40, 40, 40, 1);
  border-top-color: #177ddc;
}

.dark .tab-language {
  background: rgba(255, 255, 255, 0.1);
  color: #bbb;
}

.dark .editor-empty {
  background: rgba(0, 0, 0, 0.1);
}

.dark .status-title {
  color: #e6e6e6;
}

.dark .status-detail,
.dark .load-label,
.dark .network-label {
  color: #888;
}

.dark .load-value,
.dark .network-value {
  color: #e6e6e6;
}
</style>