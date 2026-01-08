<script setup lang="ts">
defineOptions({
  name: 'ServerTerminal'
});
import { ref, onMounted, onUnmounted, computed, onBeforeUnmount, watch } from 'vue';
import { useVirtualList } from '@vueuse/core';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal, Drawer, Tree, Button, Input, Upload, Tooltip } from 'ant-design-vue';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { getToken } from '../../utils/auth';
import 'xterm/css/xterm.css';
import service from '../../utils/request';
import {
  FolderOutlined,
  FileOutlined,
  FileTextOutlined,
  FileImageOutlined,
  ArrowLeftOutlined,
  UploadOutlined,
  DownloadOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  FolderOpenOutlined,
  MenuOutlined,
  CloseOutlined,
  SaveOutlined,
  CopyOutlined,
  FileAddOutlined,
  FolderAddOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined
} from '@ant-design/icons-vue';

// CodeMirror 编辑器相关
import { Codemirror } from 'vue-codemirror';
import { basicSetup } from 'codemirror';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { javascript } from '@codemirror/lang-javascript';
import { python } from '@codemirror/lang-python';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { json } from '@codemirror/lang-json';
import { xml } from '@codemirror/lang-xml';
import { php } from '@codemirror/lang-php';
import { cpp } from '@codemirror/lang-cpp';
import { java } from '@codemirror/lang-java';
import { rust } from '@codemirror/lang-rust';
import { sql } from '@codemirror/lang-sql';
import { markdown } from '@codemirror/lang-markdown';
import { StreamLanguage } from '@codemirror/language';
import { shell } from '@codemirror/legacy-modes/mode/shell';
import { go } from '@codemirror/legacy-modes/mode/go';
import { yaml } from '@codemirror/legacy-modes/mode/yaml';

// 定义响应类型接口
interface ApiResponse<T = any> {
  success: boolean;
  message?: string;
  data?: T;
  [key: string]: any; // 允许额外的字段
}

// 服务器状态响应
interface ServerStatusResponse {
  success: boolean;
  online: boolean;
  status: string;
  name: string;
  message: string;
  last_heartbeat: string;
  data: any;
}

// 文件项目接口
interface FileItem {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  permission: string;
}

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// 终端状态
const terminalRef = ref<HTMLElement | null>(null);
const connected = ref(false);
const terminal = ref<Terminal | null>(null);
const fitAddon = ref<FitAddon | null>(null);
let ws: WebSocket | null = null;
let statusWs: WebSocket | null = null; // 状态监控WebSocket
type TerminalDisposable = { dispose: () => void } | null;
let terminalDataDisposable: TerminalDisposable = null;

// 终端会话
const sessions = ref<{ id: string; name: string }[]>([]);
const currentSession = ref<string>(''); // 当前会话ID
const sessionName = ref<string>(''); // 新会话名称
const sessionModalVisible = ref<boolean>(false);

// 添加状态变量
const checkingHeartbeat = ref(false);
const agentNotConnected = ref(false);

// WebSocket连接保护机制
const isFileOperationInProgress = ref(false);
const maxReconnectAttempts = 3;
let reconnectAttempts = 0;
let reconnectTimer: number | null = null;

// 文件管理器状态
const fileManagerVisible = ref(false);
const fileManagerWidth = ref(260);
const currentPath = ref('/');
const fileList = ref<FileItem[]>([]);
const selectedFile = ref<FileItem | null>(null);
const showHiddenFiles = ref(false);
const fileSearchKeyword = ref('');
const fileLoading = ref(false);

// 文件编辑相关
const fileEditVisible = ref(false);
const editingFile = ref<FileItem | null>(null);
const fileContent = ref('');
const fileEditLoading = ref(false);

// 多标签页编辑器
interface EditorTab {
  id: string;
  file: FileItem;
  content: string;
  originalContent: string; // 原始内容，用于检测变化
  isDirty: boolean;
  isLoading: boolean;
  language: string;
}

const editorVisible = ref(false);
const editorTabs = ref<EditorTab[]>([]);
const activeTabId = ref<string>('');
const editorHeight = ref(350);
const minEditorHeight = 200;
const maxEditorHeight = ref(800); // 使用响应式变量，动态计算最大高度

// 布局状态

// 系统状态侧边栏
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

// 右键菜单相关
const contextMenuVisible = ref(false);
const contextMenuPosition = ref({ x: 0, y: 0 });
const contextMenuTarget = ref<FileItem | null>(null);

// 新建文件/文件夹相关
const newFileModalVisible = ref(false);
const newFolderModalVisible = ref(false);
const newFileName = ref('');
const newFolderName = ref('');

// 拖拽上传相关
const isDragOver = ref(false);
const fileUploadInput = ref<HTMLInputElement | null>(null);

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    // 首先尝试获取公开的服务器状态信息
    const statusResponse: ServerStatusResponse = await service.get(`/servers/${serverId.value}/status`);
    console.log('服务器状态响应:', statusResponse);

    if (statusResponse && statusResponse.success) {
      // 从状态API更新服务器信息
      serverInfo.value = {
        ...serverInfo.value,
        id: serverId.value,
        name: statusResponse.name || `服务器 ${serverId.value}`,
        status: statusResponse.status,
        online: statusResponse.online
      };

      // 如果服务器在线，获取更详细的信息
      if (statusResponse.online) {
        try {
          const detailResponse: ApiResponse = await service.get(`/servers/${serverId.value}`);
          if (detailResponse && detailResponse.server) {
            serverInfo.value = {
              ...serverInfo.value,
              ...detailResponse.server,
              online: true
            };
          }
        } catch (error) {
          // 如果获取详情失败，仍然保留基本信息
          console.error('获取服务器详情失败:', error);
        }
      }

      console.log('更新的服务器信息:', serverInfo.value);

      // 获取初始状态后，建立状态WebSocket连接
      connectStatusWebSocket();
    } else {
      message.error('获取服务器状态失败');
    }
  } catch (error) {
    console.error('获取服务器信息失败:', error);
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
  }
};

// 文件管理器功能
// 获取文件列表
const fetchFileList = async (path: string = currentPath.value) => {
  if (!serverInfo.value.online) {
    message.warning('服务器未在线，无法访问文件');
    return;
  }

  fileLoading.value = true;
  try {
    console.log('正在获取文件列表，路径:', path);
    const response = await service.get(`/servers/${serverId.value}/files`, {
      params: { path }
    });

    console.log('文件列表API响应:', response);

    let files: FileItem[] = [];

    // 根据不同的响应格式处理数据
    if (Array.isArray(response)) {
      // 直接是数组
      files = response;
    } else if (response && typeof response === 'object') {
      // 尝试从对象中获取文件列表
      if (Array.isArray(response.files)) {
        files = response.files;
      } else if (Array.isArray(response.data)) {
        files = response.data;
      }
    } else {
      console.warn('未知的文件列表响应格式:', response);
      files = [];
    }

    // 为每个文件添加完整路径
    files = files.map(file => ({
      ...file,
      path: path === '/' ? `/${file.name}` : `${path}/${file.name}`
    }));

    // 过滤隐藏文件
    if (!showHiddenFiles.value) {
      files = files.filter(file => !file.name.startsWith('.'));
    }

    // 搜索过滤
    if (fileSearchKeyword.value) {
      files = files.filter(file =>
        file.name.toLowerCase().includes(fileSearchKeyword.value.toLowerCase())
      );
    }

    // 排序：文件夹在前，然后按名称排序
    files.sort((a, b) => {
      if (a.is_dir && !b.is_dir) return -1;
      if (!a.is_dir && b.is_dir) return 1;
      return a.name.localeCompare(b.name);
    });

    fileList.value = files;
    currentPath.value = path;
    console.log('处理后的文件列表:', files);
  } catch (error) {
    console.error('获取文件列表失败:', error);
    message.error(`获取文件列表失败: ${error.message || '网络错误'}`);
    fileList.value = [];
  } finally {
    fileLoading.value = false;
  }
};

// 虚拟列表配置
const { list, containerProps, wrapperProps } = useVirtualList(fileList, {
  itemHeight: 46, // 46px per item (approx)
});

// 进入目录
const enterDirectory = (file: FileItem) => {
  if (file.is_dir) {
    fetchFileList(file.path);
  }
};

// 返回上级目录
const goToParentDirectory = () => {
  const parentPath = currentPath.value.split('/').slice(0, -1).join('/') || '/';
  fetchFileList(parentPath);
};

// 切换隐藏文件显示
const toggleHiddenFiles = () => {
  showHiddenFiles.value = !showHiddenFiles.value;
  fetchFileList();
};

// 刷新文件列表
const refreshFileList = () => {
  fetchFileList();
};

// 获取文件图标
const getFileIcon = (file: FileItem) => {
  if (file.is_dir) {
    return FolderOutlined;
  }

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
    case 'css':
    case 'js':
    case 'ts':
    case 'sh':
    case 'py':
    case 'go':
    case 'java':
    case 'php':
      return FileTextOutlined;
    case 'jpg':
    case 'jpeg':
    case 'png':
    case 'gif':
    case 'svg':
      return FileImageOutlined;
    default:
      return FileOutlined;
  }
};

// 格式化文件大小
const formatFileSize = (size: number) => {
  if (size === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(size) / Math.log(k));
  return parseFloat((size / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
};

// 检查文件是否可编辑（只有文本文件可以编辑）
const isEditableFile = (file: FileItem) => {
  if (file.is_dir) return false;

  const ext = file.name.split('.').pop()?.toLowerCase();
  const editableExtensions = [
    // 文本文件
    'txt', 'log', 'md', 'readme',
    // 配置文件
    'conf', 'ini', 'cfg', 'config', 'env',
    // 数据格式
    'yaml', 'yml', 'json', 'xml', 'toml',
    // 网页文件
    'html', 'htm', 'css', 'scss', 'sass', 'less',
    // 脚本文件
    'js', 'ts', 'jsx', 'tsx', 'vue',
    'sh', 'bash', 'zsh', 'fish', 'ps1', 'bat', 'cmd',
    'py', 'rb', 'pl', 'php', 'go', 'rs', 'java', 'c', 'cpp', 'h', 'hpp',
    // 其他常见文本格式
    'sql', 'dockerfile', 'gitignore', 'gitattributes',
    'csv', 'tsv', 'properties', 'list'
  ];

  // 检查扩展名
  if (ext && editableExtensions.includes(ext)) {
    return true;
  }

  // 检查无扩展名的常见文本文件
  const textFileNames = [
    'readme', 'license', 'changelog', 'dockerfile', 'makefile',
    '.gitignore', '.gitattributes', '.env', '.editorconfig',
    '.htaccess', '.bashrc', '.zshrc', '.vimrc'
  ];

  const fileName = file.name.toLowerCase();
  return textFileNames.some(name => fileName === name || fileName.endsWith(name));
};

// 编辑文件 - 在多标签页编辑器中打开
const editFile = async (file: FileItem) => {
  if (file.is_dir) {
    message.warning('无法编辑文件夹');
    return;
  }

  // 检查文件是否可编辑
  if (!isEditableFile(file)) {
    message.warning('此文件类型不支持在线编辑，仅支持文本类型文件');
    return;
  }

  // 检查文件大小（限制5MB以下）
  if (file.size > 5 * 1024 * 1024) {
    message.warning('文件过大，无法在线编辑（限制5MB以下）');
    return;
  }

  // 检查文件是否已经在编辑器中打开
  const existingTab = editorTabs.value.find(tab => tab.file.path === file.path);
  if (existingTab) {
    activeTabId.value = existingTab.id;
    editorVisible.value = true;
    return;
  }

  // 创建新标签页
  const tabId = `tab_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  const language = detectLanguage(file.name);
  const newTab: EditorTab = {
    id: tabId,
    file: file,
    content: '',
    originalContent: '',
    isDirty: false,
    isLoading: true,
    language: language
  };

  editorTabs.value.push(newTab);
  activeTabId.value = tabId;

  // 自动显示编辑器
  editorVisible.value = true;

  // 加载文件内容
  try {
    const response = await service.get(`/servers/${serverId.value}/files/content`, {
      params: { path: file.path }
    });

    console.log('获取文件内容响应:', response);

    // 处理响应数据，支持多种格式
    let content = '';
    if (response === null || response === undefined) {
      console.error('响应为空');
      content = '';
    } else if (typeof response === 'string') {
      content = response;
      console.log('响应是字符串，长度:', response.length);
    } else if (typeof response === 'object') {
      if ('data' in response && response.data !== undefined) {
        content = typeof response.data === 'string' ? response.data : JSON.stringify(response.data);
      } else if ('content' in response && response.content !== undefined) {
        content = typeof response.content === 'string' ? response.content : JSON.stringify(response.content);
      } else {
        content = JSON.stringify(response);
      }
    } else {
      content = String(response);
    }

    const tab = editorTabs.value.find(t => t.id === tabId);
    if (tab) {
      tab.content = content;
      tab.originalContent = content; // 设置原始内容
      tab.isLoading = false;
      console.log('设置编辑器内容，长度:', content.length);
    }
  } catch (error) {
    console.error('读取文件内容失败:', error);
    message.error('读取文件内容失败');
    // 移除失败的标签页
    closeEditorTab(tabId);
  }
};

// 保存文件（旧的弹窗编辑器）
const saveFile = async () => {
  if (!editingFile.value) return;

  fileEditLoading.value = true;
  try {
    await service.post(`/servers/${serverId.value}/files/content`, {
      path: editingFile.value.path,
      content: fileContent.value
    });

    message.success('文件保存成功');
    fileEditVisible.value = false;
  } catch (error) {
    console.error('保存文件失败:', error);
    message.error('保存文件失败');
  } finally {
    fileEditLoading.value = false;
  }
};

// ========== 多标签页编辑器方法 ==========

// 关闭编辑器标签页
const closeEditorTab = (tabId: string) => {
  const tabIndex = editorTabs.value.findIndex(tab => tab.id === tabId);
  if (tabIndex === -1) return;

  const tab = editorTabs.value[tabIndex];

  // 如果文件有未保存的更改，询问用户
  if (tab.isDirty) {
    Modal.confirm({
      title: '未保存的更改',
      content: `文件 "${tab.file.name}" 有未保存的更改，确定要关闭吗？`,
      okText: '强制关闭',
      okType: 'danger',
      cancelText: '取消',
      onOk() {
        doCloseTab(tabId, tabIndex);
      }
    });
  } else {
    doCloseTab(tabId, tabIndex);
  }
};

// 执行关闭标签页
const doCloseTab = (tabId: string, tabIndex: number) => {
  editorTabs.value.splice(tabIndex, 1);

  // 如果关闭的是当前活跃的标签页，切换到其他标签页
  if (activeTabId.value === tabId) {
    if (editorTabs.value.length > 0) {
      // 优先选择右侧的标签页，如果没有则选择左侧的
      const newIndex = tabIndex < editorTabs.value.length ? tabIndex : tabIndex - 1;
      activeTabId.value = editorTabs.value[newIndex].id;
    } else {
      // 没有其他标签页了，关闭编辑器
      activeTabId.value = '';
      editorVisible.value = false;
    }
  }
};

// 切换到指定标签页
const switchToTab = (tabId: string) => {
  activeTabId.value = tabId;
};

// 获取当前活跃的标签页
const activeTab = computed(() => {
  return editorTabs.value.find(tab => tab.id === activeTabId.value);
});

// 保存当前标签页的文件
const saveActiveTab = async () => {
  const tab = activeTab.value;
  if (!tab || tab.isLoading) return;

  tab.isLoading = true;
  isFileOperationInProgress.value = true; // 标记文件操作开始

  try {
    console.log('保存文件内容，路径:', tab.file.path);
    console.log('保存文件内容长度:', tab.content.length);

    // 检查WebSocket连接状态
    const wasConnected = connected.value;

    await service.put(`/servers/${serverId.value}/files/content`, {
      path: tab.file.path,
      content: tab.content
    }, {
      timeout: 30000, // 增加超时时间到30秒
      headers: {
        'Connection': 'keep-alive' // 保持连接
      }
    });

    tab.isDirty = false;
    tab.originalContent = tab.content; // 更新原始内容
    message.success(`文件 "${tab.file.name}" 保存成功`);

    // 如果之前连接着，但现在断开了，尝试重新连接
    if (wasConnected && !connected.value && currentSession.value) {
      console.log('检测到文件保存后终端连接断开，尝试重新连接...');
      setTimeout(() => {
        reconnectTerminalSafely();
      }, 1000);
    }

  } catch (error) {
    console.error('保存文件失败:', error);
    message.error(`保存文件 "${tab.file.name}" 失败`);
  } finally {
    tab.isLoading = false;
    isFileOperationInProgress.value = false; // 标记文件操作结束
  }
};

// 保存所有标签页
const saveAllTabs = async () => {
  const dirtyTabs = editorTabs.value.filter(tab => tab.isDirty && !tab.isLoading);
  if (dirtyTabs.length === 0) {
    message.info('没有需要保存的文件');
    return;
  }

  message.loading('正在保存所有文件...', 0);
  isFileOperationInProgress.value = true; // 标记文件操作开始

  const wasConnected = connected.value; // 记录保存前的连接状态

  try {
    const savePromises = dirtyTabs.map(async (tab) => {
      tab.isLoading = true;
      try {
        await service.put(`/servers/${serverId.value}/files/content`, {
          path: tab.file.path,
          content: tab.content
        }, {
          timeout: 30000, // 增加超时时间到30秒
          headers: {
            'Connection': 'keep-alive' // 保持连接
          }
        });
        tab.isDirty = false;
        tab.originalContent = tab.content; // 更新原始内容
        return { success: true, file: tab.file.name };
      } catch (error) {
        console.error(`保存文件 ${tab.file.name} 失败:`, error);
        return { success: false, file: tab.file.name, error };
      } finally {
        tab.isLoading = false;
      }
    });

    const results = await Promise.all(savePromises);
    const successCount = results.filter(r => r.success).length;
    const failCount = results.length - successCount;

    message.destroy();

    if (failCount === 0) {
      message.success(`成功保存 ${successCount} 个文件`);
    } else {
      message.warning(`保存完成：${successCount} 个成功，${failCount} 个失败`);
    }

    // 如果之前连接着，但现在断开了，尝试重新连接
    if (wasConnected && !connected.value && currentSession.value) {
      console.log('检测到批量保存后终端连接断开，尝试重新连接...');
      setTimeout(() => {
        reconnectTerminalSafely();
      }, 1500);
    }

  } catch (error) {
    message.destroy();
    message.error('保存文件时发生错误');
  } finally {
    isFileOperationInProgress.value = false; // 标记文件操作结束
  }
};

// 关闭所有标签页
const closeAllTabs = () => {
  const dirtyTabs = editorTabs.value.filter(tab => tab.isDirty);

  if (dirtyTabs.length > 0) {
    Modal.confirm({
      title: '未保存的更改',
      content: `有 ${dirtyTabs.length} 个文件有未保存的更改，确定要全部关闭吗？`,
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

// 切换编辑器可见性
const toggleEditor = () => {
  editorVisible.value = !editorVisible.value;

  if (editorVisible.value && editorTabs.value.length === 0) {
    message.info('编辑器已打开，请从文件管理器中选择文件进行编辑');
  }
};

// 文件内容更改时的处理
const onEditorContentChange = (content: string) => {
  const tab = activeTab.value;
  if (!tab) return;

  // 更新内容并检查是否有更改
  tab.content = content;
  const hasChanges = content !== tab.originalContent;
  tab.isDirty = hasChanges;
};

// 获取文件语言类型（用于语法高亮）
const detectLanguage = (fileName: string): string => {
  const ext = fileName.split('.').pop()?.toLowerCase();
  const langMap: Record<string, string> = {
    'js': 'javascript',
    'ts': 'typescript',
    'jsx': 'javascript',
    'tsx': 'typescript',
    'vue': 'javascript',
    'html': 'html',
    'htm': 'html',
    'css': 'css',
    'scss': 'css',
    'sass': 'css',
    'less': 'css',
    'json': 'json',
    'xml': 'xml',
    'yaml': 'yaml',
    'yml': 'yaml',
    'md': 'markdown',
    'py': 'python',
    'go': 'go',
    'java': 'java',
    'php': 'php',
    'rb': 'ruby',
    'sh': 'shell',
    'bash': 'shell',
    'sql': 'sql',
    'dockerfile': 'dockerfile',
    'c': 'cpp',
    'cpp': 'cpp',
    'cc': 'cpp',
    'cxx': 'cpp',
    'h': 'cpp',
    'hpp': 'cpp',
    'rs': 'rust'
  };

  return langMap[ext || ''] || 'text';
};

// 获取 CodeMirror 语言扩展
const getLanguageExtension = (language: string) => {
  switch (language) {
    case 'javascript':
    case 'typescript':
      return javascript();
    case 'python':
      return python();
    case 'html':
      return html();
    case 'css':
      return css();
    case 'json':
      return json();
    case 'xml':
      return xml();
    case 'php':
      return php();
    case 'cpp':
      return cpp();
    case 'java':
      return java();
    case 'rust':
      return rust();
    case 'sql':
      return sql();
    case 'markdown':
      return markdown();
    case 'shell':
      return StreamLanguage.define(shell);
    case 'go':
      return StreamLanguage.define(go);
    case 'yaml':
      return StreamLanguage.define(yaml);
    default:
      return [];
  }
};

// ========== 编辑器高度调整功能 ==========

let isResizing = false;
let startY = 0;
let startHeight = 0;

// 计算合适的最大编辑器高度
const updateMaxEditorHeight = () => {
  const windowHeight = window.innerHeight;
  const baseMaxHeight = 800;
  const dynamicMaxHeight = Math.max(baseMaxHeight, windowHeight * 0.7);
  maxEditorHeight.value = Math.min(dynamicMaxHeight, windowHeight - 200); // 确保留出足够空间给终端
};

// 开始拖拽调整高度
const startResize = (e: MouseEvent) => {
  if (e.button !== 0) return; // 只响应左键

  isResizing = true;
  startY = e.clientY;
  startHeight = editorHeight.value;

  document.addEventListener('mousemove', handleEditorResize, { passive: false });
  document.addEventListener('mouseup', stopResize);
  document.body.style.cursor = 'ns-resize';
  document.body.style.userSelect = 'none';

  // 添加临时样式以改善视觉反馈
  // document.body.style.pointerEvents = 'none'; // Removed as it blocks mouse events
  const handle = e.target as HTMLElement;
  handle.style.background = 'rgba(24, 144, 255, 0.2)';

  e.preventDefault();
  e.stopPropagation();
};

// 处理拖拽过程
const handleEditorResize = (e: MouseEvent) => {
  if (!isResizing) return;

  const deltaY = e.clientY - startY;
  const newHeight = startHeight + deltaY;

  // 动态计算最大高度（窗口高度的80%）
  const windowHeight = window.innerHeight;
  const dynamicMaxHeight = Math.max(maxEditorHeight.value, windowHeight * 0.8);

  // 限制高度范围并提供平滑的边界效果
  let finalHeight = newHeight;
  if (newHeight < minEditorHeight) {
    finalHeight = minEditorHeight;
  } else if (newHeight > dynamicMaxHeight) {
    finalHeight = dynamicMaxHeight;
  }

  editorHeight.value = finalHeight;

  // 实时调整终端大小（防抖处理）
  clearTimeout((window as any).resizeTimeout);
  (window as any).resizeTimeout = setTimeout(() => {
    if (fitAddon.value && terminal.value) {
      fitAddon.value.fit();
    }
  }, 16); // 约60fps的更新频率

  e.preventDefault();
};

// 停止拖拽
const stopResize = () => {
  if (!isResizing) return;

  isResizing = false;
  document.removeEventListener('mousemove', handleEditorResize);
  document.removeEventListener('mouseup', stopResize);

  // 恢复样式
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  document.body.style.pointerEvents = '';

  // 清理拖拽条的临时样式
  const handles = document.querySelectorAll('.editor-resize-handle');
  handles.forEach(handle => {
    (handle as HTMLElement).style.background = '';
  });

  // 延迟调整终端大小，确保布局已完成
  setTimeout(() => {
    autoResize();
  }, 50);
};

// 获取终端当前工作目录
const getTerminalWorkingDirectory = async () => {
  if (!serverInfo.value.online || !currentSession.value) {
    console.warn('服务器离线或没有当前会话，无法获取工作目录');
    return '/';
  }

  try {
    console.log('获取终端工作目录，会话ID:', currentSession.value);
    const response = await service.get<{ success: boolean; working_dir: string }>(`/servers/${serverId.value}/terminal/sessions/${currentSession.value}/cwd`);
    console.log('工作目录API响应:', response);

    if (response && response.success && response.working_dir) {
      console.log('获取到终端工作目录:', response.working_dir);
      return response.working_dir;
    } else {
      console.warn('获取工作目录响应格式不正确:', response);
      return '/';
    }
  } catch (error) {
    console.error('获取终端工作目录失败:', error);
    // 如果获取失败，回退到根目录
    return '/';
  }
};

// 切换文件管理器显示
const toggleFileManager = async () => {
  fileManagerVisible.value = !fileManagerVisible.value;
  console.log('切换文件管理器显示:', fileManagerVisible.value, '服务器在线:', serverInfo.value.online);

  if (fileManagerVisible.value && serverInfo.value.online) {
    // 尝试获取终端当前工作目录
    let targetPath = '/';

    if (currentSession.value) {
      try {
        targetPath = await getTerminalWorkingDirectory();
        console.log('将文件管理器导航到终端当前目录:', targetPath);
        message.info(`文件管理器已跳转到终端当前目录: ${targetPath}`);
      } catch (error) {
        console.error('获取终端工作目录失败，使用根目录:', error);
        message.warning('无法获取终端当前目录，显示根目录');
      }
    } else {
      console.log('没有活跃的终端会话，显示根目录');
      message.info('显示根目录（没有活跃的终端会话）');
    }

    // 获取指定路径的文件列表
    await fetchFileList(targetPath);
  }
};


// 切换系统状态面板
const toggleSystemStatus = () => {
  systemStatusVisible.value = !systemStatusVisible.value;
};

// 格式化字节数
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

// 格式化百分比
const formatPercentage = (value: number): string => {
  return value.toFixed(1) + '%';
};

// 计算内存使用百分比
const getMemoryPercentage = (): number => {
  if (systemStatusData.value.memory_total === 0) return 0;
  return (systemStatusData.value.memory_used / systemStatusData.value.memory_total) * 100;
};

// 计算磁盘使用百分比
const getDiskPercentage = (): number => {
  if (systemStatusData.value.disk_total === 0) return 0;
  return (systemStatusData.value.disk_used / systemStatusData.value.disk_total) * 100;
};

// 获取进度条颜色
const getProgressColor = (percentage: number): string => {
  if (percentage >= 90) return '#ff4d4f';
  if (percentage >= 70) return '#faad14';
  return '#52c41a';
};

// 格式化网络速度
const formatNetworkSpeed = (bytesPerSecond: number): string => {
  if (bytesPerSecond === 0) return '0 B/s';
  const k = 1024;
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
  const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k));
  return parseFloat((bytesPerSecond / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

// 更新系统状态数据（参考探针页面的处理逻辑）
const updateSystemStatusData = (data: any) => {
  console.log('更新系统状态数据:', data);
  console.log('更新前系统状态数据:', JSON.stringify(systemStatusData.value));

  // 更新CPU使用率（处理小数格式的CPU使用率）
  if (data.cpu_usage !== undefined) {
    let cpuValue = Number(data.cpu_usage);
    console.log('CPU使用率原始值:', data.cpu_usage, '转换后值:', cpuValue);
    if (cpuValue < 1 && cpuValue > 0) {
      cpuValue = cpuValue * 100;
      console.log('CPU值小于1，放大100倍:', cpuValue);
    }
    systemStatusData.value.cpu_usage = Math.min(Math.max(cpuValue, 0), 100);
    console.log('最终CPU使用率:', systemStatusData.value.cpu_usage);
  }

  // 更新内存使用情况
  if (data.memory_used !== undefined) {
    // 如果memory_used是百分比（<=100）则需要计算实际字节数
    if (data.memory_used <= 100 && data.memory_total && data.memory_total > 0) {
      // 如果传入的是百分比，计算实际使用的字节数
      systemStatusData.value.memory_used = (data.memory_used / 100) * data.memory_total;
    } else {
      // 否则假设是字节数，直接使用
      systemStatusData.value.memory_used = Number(data.memory_used);
    }
  }

  if (data.memory_total !== undefined) {
    systemStatusData.value.memory_total = Number(data.memory_total);
  }

  // 更新磁盘使用情况
  if (data.disk_used !== undefined) {
    // 如果disk_used是百分比（<=100）则需要计算实际字节数
    if (data.disk_used <= 100 && data.disk_total && data.disk_total > 0) {
      // 如果传入的是百分比，计算实际使用的字节数
      systemStatusData.value.disk_used = (data.disk_used / 100) * data.disk_total;
    } else {
      // 否则假设是字节数，直接使用
      systemStatusData.value.disk_used = Number(data.disk_used);
    }
  }

  if (data.disk_total !== undefined) {
    systemStatusData.value.disk_total = Number(data.disk_total);
  }

  // 更新系统负载
  if (data.load_avg_1 !== undefined) {
    systemStatusData.value.load_avg_1 = Number(data.load_avg_1);
  }

  if (data.load_avg_5 !== undefined) {
    systemStatusData.value.load_avg_5 = Number(data.load_avg_5);
  }

  if (data.load_avg_15 !== undefined) {
    systemStatusData.value.load_avg_15 = Number(data.load_avg_15);
  }

  // 更新网络速度
  if (data.network_in !== undefined) {
    systemStatusData.value.network_in = Number(data.network_in);
  }

  if (data.network_out !== undefined) {
    systemStatusData.value.network_out = Number(data.network_out);
  }

  console.log('更新后系统状态数据:', JSON.stringify(systemStatusData.value));
};

// 测试数据更新函数
const testSystemStatusUpdate = () => {
  const testData = {
    cpu_usage: 25.5,
    memory_used: 4 * 1024 * 1024 * 1024, // 4GB
    memory_total: 16 * 1024 * 1024 * 1024, // 16GB
    disk_used: 20 * 1024 * 1024 * 1024, // 20GB
    disk_total: 100 * 1024 * 1024 * 1024, // 100GB
    load_avg_1: 1.5,
    load_avg_5: 1.2,
    load_avg_15: 0.8,
    network_in: 50000,
    network_out: 30000
  };
  console.log('测试系统状态数据更新:', testData);
  updateSystemStatusData(testData);
};

// 初始化时设置一些默认测试数据
const initializeSystemStatusData = () => {
  console.log('初始化系统状态数据...');
  testSystemStatusUpdate();
};

// 刷新系统状态数据（只获取监控数据，不影响终端连接）
const refreshSystemStatus = async () => {
  try {
    console.log('手动刷新系统状态数据...');
    const monitorResponse = await service.get(`/servers/${serverId.value}/monitor`);
    console.log('手动获取监控数据响应:', monitorResponse);

    if (monitorResponse && monitorResponse.data && monitorResponse.data.length > 0) {
      // 取最新的数据点
      const latestData = monitorResponse.data[monitorResponse.data.length - 1];
      updateSystemStatusData(latestData);
      message.success('系统状态已刷新');
    } else {
      console.warn('没有监控数据可用，使用测试数据');
      testSystemStatusUpdate();
      message.info('使用测试数据更新了系统状态');
    }
  } catch (error) {
    console.warn('获取监控数据失败:', error);
    // 如果获取失败，使用测试数据
    testSystemStatusUpdate();
    message.warning('获取监控数据失败，使用测试数据');
  }
};

// 监听文件管理器可见性变化
watch(fileManagerVisible, (visible) => {
  if (visible && serverInfo.value.online) {
    // 总是重新加载文件列表以确保数据最新
    fetchFileList('/');
  }

  // 调整终端大小
  setTimeout(() => {
    if (fitAddon.value && terminal.value) {
      fitAddon.value.fit();
    }
  }, 300);
});

// 监听服务器在线状态变化
watch(() => serverInfo.value.online, (online) => {
  if (online && fileManagerVisible.value) {
    // 服务器上线时重新加载文件列表
    fetchFileList('/');
  }
});

// 监听搜索关键词变化
watch(fileSearchKeyword, () => {
  fetchFileList();
});

// 计算布局样式
const layoutStyle = computed(() => ({
  minHeight: 'calc(100vh - 80px)',
  position: 'relative' as const,
  top: 'auto',
  left: 'auto',
  right: 'auto',
  bottom: 'auto',
  zIndex: 'auto',
  background: 'transparent'
}));

// 工作区容器样式
const workspaceContainerStyle = computed(() => {
  let width = '100%';

  // 计算需要减去的宽度
  const deductions = [];
  if (fileManagerVisible.value) {
    deductions.push(`${fileManagerWidth.value}px`);
  }
  if (systemStatusVisible.value) {
    deductions.push(`${systemStatusWidth.value}px`);
  }

  if (deductions.length > 0) {
    width = `calc(100% - ${deductions.join(' - ')})`;
  }

  return {
    width,
    height: 'fit-content',
    display: 'flex',
    flexDirection: 'column' as 'column'
  };
});

const terminalContainerStyle = computed(() => ({
  flex: 1,
  position: 'relative' as 'relative',
  height: editorVisible.value ? `calc(100% - ${editorHeight.value}px)` : '100%'
}));

// 路径分段（智能省略中间部分）
const pathSegments = computed(() => {
  if (currentPath.value === '/') {
    return [{ name: '根目录', isEllipsis: false, originalIndex: 0 }];
  }

  const segments = currentPath.value.split('/').filter(Boolean);
  const fullSegments = ['根目录', ...segments];

  // 如果路径不长，直接返回完整路径
  if (fullSegments.length <= 4) {
    return fullSegments.map((name, index) => ({
      name,
      isEllipsis: false,
      originalIndex: index
    }));
  }

  // 长路径：保留开始1个、结尾2个，中间用省略号
  const result = [];
  result.push({ name: fullSegments[0], isEllipsis: false, originalIndex: 0 });
  result.push({ name: '...', isEllipsis: true, originalIndex: -1 });
  result.push({
    name: fullSegments[fullSegments.length - 2],
    isEllipsis: false,
    originalIndex: fullSegments.length - 2
  });
  result.push({
    name: fullSegments[fullSegments.length - 1],
    isEllipsis: false,
    originalIndex: fullSegments.length - 1
  });

  return result;
});

// 根据分段索引获取路径
const getPathFromSegments = (originalIndex: number) => {
  if (originalIndex === 0) {
    return '/';
  }
  if (originalIndex === -1) {
    return currentPath.value; // 省略号点击显示完整路径
  }
  const segments = currentPath.value.split('/').filter(Boolean);
  return '/' + segments.slice(0, originalIndex).join('/');
};

// 导航到指定路径
const navigateToPath = (path: string) => {
  if (path !== currentPath.value) {
    fetchFileList(path);
  }
};

// 显示完整路径的状态
const showFullPath = ref(false);

// 切换显示完整路径
const toggleFullPath = () => {
  showFullPath.value = !showFullPath.value;
};

// 获取完整路径分段（用于下拉显示）
const fullPathSegments = computed(() => {
  if (currentPath.value === '/') {
    return [{ name: '根目录', path: '/' }];
  }

  const segments = currentPath.value.split('/').filter(Boolean);
  const result = [{ name: '根目录', path: '/' }];

  let buildPath = '';
  segments.forEach((segment, index) => {
    buildPath += '/' + segment;
    result.push({ name: segment, path: buildPath });
  });

  return result;
});

// 建立状态监控WebSocket连接
const connectStatusWebSocket = () => {
  if (statusWs) {
    statusWs.close();
  }

  const token = getToken();
  if (!token) {
    console.warn('没有token，无法连接状态WebSocket');
    return;
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const wsUrl = `${protocol}//${host}/api/servers/${serverId.value}/ws?token=${encodeURIComponent(token)}`;

  console.log('连接状态WebSocket:', wsUrl);

  statusWs = new WebSocket(wsUrl);

  statusWs.onopen = () => {
    console.log('状态WebSocket连接已建立');
    // 暴露WebSocket实例到全局供调试
    (window as any).statusWs = statusWs;
  };

  statusWs.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      console.log('收到状态WebSocket消息:', data);

      if (data.type === 'welcome') {
        console.log('收到状态WebSocket欢迎消息');
        agentNotConnected.value = false;

        // 直接更新系统信息字段，而不是重新调用updateServerInfo
        if (data.data) {
          // 更新服务器在线状态
          if (data.data.status !== undefined) {
            serverInfo.value.status = data.data.status;
            serverInfo.value.online = data.data.status === 'online';
          }

          // 更新系统信息字段，保留原有的last_heartbeat
          Object.keys(data.data).forEach(key => {
            if (key !== 'last_heartbeat' && serverInfo.value.hasOwnProperty(key)) {
              serverInfo.value[key] = data.data[key];
            }
          });

          // 检查欢迎消息中是否包含监控数据
          if (data.data.cpu_usage !== undefined || data.data.memory_used !== undefined) {
            console.log('欢迎消息中包含监控数据，更新系统状态');
            updateSystemStatusData(data.data);
          }
        }
      } else if (data.type === 'status') {
        console.log('收到服务器状态更新:', data.data);
        if (data.data && data.data.online !== undefined) {
          serverInfo.value.online = data.data.online;
          serverInfo.value.status = data.data.online ? 'online' : 'offline';
        }
      } else if (data.type === 'heartbeat') {
        console.log('收到心跳消息:', data);
        // 处理心跳消息中的监控数据
        if (data.data) {
          console.log('心跳消息中包含监控数据:', data.data);
          updateSystemStatusData(data.data);
        } else if (data.cpu_usage !== undefined || data.memory_used !== undefined) {
          console.log('心跳消息本身包含监控数据');
          updateSystemStatusData(data);
        }
      } else if (data.type === 'monitor') {
        console.log('收到监控数据:', data.data);
        // 更新系统状态数据
        if (data.data) {
          updateSystemStatusData(data.data);
        } else {
          updateSystemStatusData(data);
        }
      } else if (data.type === 'no_data') {
        console.log('收到无监控数据消息:', data.message);
        message.warning('服务器没有监控数据，请检查agent是否正常运行');
      } else {
        console.log('收到未知类型的WebSocket消息:', data);
      }
    } catch (error) {
      console.error('解析状态WebSocket消息失败:', error, event.data);
    }
  };

  statusWs.onerror = (error) => {
    console.error('状态WebSocket连接错误:', error);
  };

  statusWs.onclose = (event) => {
    console.log('状态WebSocket连接关闭:', event.code, event.reason);

    // 如果不是手动关闭，尝试重连
    if (event.code !== 1000) {
      console.log('状态WebSocket异常关闭，5秒后尝试重连...');
      setTimeout(() => {
        if (serverInfo.value.id) {
          connectStatusWebSocket();
        }
      }, 5000);
    }
  };
};

// 检查心跳
const checkHeartbeat = async () => {
  checkingHeartbeat.value = true;
  agentNotConnected.value = false;

  try {
    // 使用新的状态API
    const statusResponse: ServerStatusResponse = await service.get(`/servers/${serverId.value}/status`);
    console.log('服务器状态响应:', statusResponse);

    if (statusResponse && statusResponse.success) {
      // 更新服务器状态
      serverInfo.value.online = statusResponse.online;
      serverInfo.value.status = statusResponse.status;

      if (statusResponse.online) {
        message.success('服务器在线');

        // 刷新一次会话列表
        await fetchSessions();

        // 尝试获取最新的监控数据
        try {
          const monitorResponse = await service.get(`/servers/${serverId.value}/monitor`);
          console.log('手动获取监控数据响应:', monitorResponse);
          if (monitorResponse && monitorResponse.data) {
            updateSystemStatusData(monitorResponse.data);
            message.success('系统状态已更新');
          }
        } catch (error) {
          console.warn('获取监控数据失败:', error);
        }

        return;
      } else {
        message.warning('服务器离线');
      }
    }

    // 如果服务器离线，尝试检查心跳（只有管理员可以做这个操作）
    const token = getToken();
    if (!token) {
      message.warning('请先登录再尝试更新服务器状态');
      return;
    }

    // 尝试更新状态API
    message.info('正在尝试更新服务器状态...');
    try {
      const response: ServerStatusResponse = await service.put(`/servers/${serverId.value}/update`, {
        status: 'online'
      });

      console.log('更新状态响应:', response);

      if (response && response.message) {
        message.success('服务器状态已更新');

        // 重新获取服务器信息
        await fetchServerInfo();
      } else {
        message.warning('无法更新服务器状态');
      }
    } catch (error) {
      console.error('更新服务器状态出错:', error);
      message.error('更新服务器状态失败');
    }
  } catch (error: any) {
    console.error('检查服务器状态出错:', error);
    message.error('检查服务器状态失败');
  } finally {
    checkingHeartbeat.value = false;
  }
};

// 获取终端会话列表
const fetchSessions = async () => {
  try {
    const response: ServerStatusResponse = await service.get(`/servers/${serverId.value}/terminal/sessions`);
    console.log('会话列表响应:', response);

    // 确保处理为空数组而不是null
    if (response && response.success && response.data) {
      sessions.value = Array.isArray(response.data) ? response.data : [];
      console.log('处理后的会话列表:', sessions.value);

      // 如果有会话，但当前没有选择任何会话，则自动选择第一个
      if (sessions.value.length > 0 && !currentSession.value) {
        currentSession.value = sessions.value[0].id;
        console.log('自动选择第一个会话:', currentSession.value);
      }
    } else {
      console.warn('会话列表响应格式不正确:', response);
      sessions.value = [];
    }
  } catch (error) {
    console.error('获取终端会话列表失败:', error);
    sessions.value = []; // 失败时也设置为空数组
  }
};

// 初始化终端
const initTerminal = () => {
  if (!terminalRef.value) return;

  try {
    // 如果已存在终端实例，先销毁它
    if (terminal.value) {
      terminal.value.dispose();
      terminal.value = null;
    }

    // 销毁已有的FitAddon
    if (fitAddon.value) {
      fitAddon.value = null;
    }

    // 创建终端实例
    terminal.value = new Terminal({
      cursorBlink: true,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 14,
      lineHeight: 1.2,
      theme: {
        background: '#1e1e1e',
        foreground: '#ffffff',
        cursor: '#ffffff',
        cursorAccent: '#000000',
        black: '#000000',
        red: '#ee5d43',
        green: '#00d700',
        yellow: '#ffd700',
        blue: '#0087ff',
        magenta: '#ff00ff',
        cyan: '#00ffff',
        white: '#ffffff',
        brightBlack: '#808080',
        brightRed: '#ff5555',
        brightGreen: '#55ff55',
        brightYellow: '#ffff55',
        brightBlue: '#5555ff',
        brightMagenta: '#ff55ff',
        brightCyan: '#55ffff',
        brightWhite: '#ffffff'
      },
      // echo: true, // 启用本地回显，用户输入立即显示在终端
      allowTransparency: true, // 支持透明背景
      convertEol: true, // 将换行符转换为回车换行
      disableStdin: false, // 确保标准输入已启用
      // rendererType: 'canvas', // 使用canvas渲染器以获得更好的性能
    });

    // 创建自适应插件
    fitAddon.value = new FitAddon();

    // 安全加载插件
    if (terminal.value && fitAddon.value) {
      terminal.value.loadAddon(fitAddon.value);
    }

    // 打开终端
    terminal.value.open(terminalRef.value);

    // 调整终端大小
    if (fitAddon.value) {
      fitAddon.value.fit();
    }

    // 添加窗口大小变化监听
    window.addEventListener('resize', handleResize);
  } catch (error) {
    console.error('初始化终端失败:', error);
    message.error('初始化终端失败');
  }
};

// 处理窗口大小变化
const handleResize = () => {
  try {
    // 更新最大编辑器高度
    updateMaxEditorHeight();

    if (fitAddon.value && terminal.value) {
      fitAddon.value.fit();
      if (ws && connected.value && terminal.value.cols && terminal.value.rows) {
        // 发送调整大小命令
        const dimensions = {
          cols: terminal.value.cols,
          rows: terminal.value.rows
        };
        try {
          ws.send(JSON.stringify({
            type: 'shell_command',
            payload: {
              type: 'resize',
              data: JSON.stringify(dimensions),
              session: currentSession.value
            }
          }));
          console.log(`调整终端大小: ${dimensions.cols}x${dimensions.rows}`);
        } catch (error) {
          console.error('发送调整大小命令到WebSocket失败:', error);
        }
      }
    }
  } catch (error) {
    console.warn('调整终端大小失败:', error);
  }
};

// 自动调整大小（在合适的时机调用）
const autoResize = () => {
  // 延迟调整大小，确保DOM已完全渲染
  setTimeout(() => {
    handleResize();
  }, 100);
};

// 创建新会话
const createSession = async () => {
  if (!sessionName.value.trim()) {
    message.warning('请输入会话名称');
    return;
  }

  // 检查服务器是否在线
  if (!serverInfo.value.online) {
    message.warning('服务器离线，无法创建会话');
    return;
  }

  try {
    const response: ServerStatusResponse = await service.post(`/servers/${serverId.value}/terminal/sessions`, {
      name: sessionName.value.trim()
    });

    console.log('创建会话响应:', response);

    if (response && response.success) {
      message.success('会话创建成功');
      sessionModalVisible.value = false;

      // 保存新会话ID，用于后续连接
      const newSessionId = response.data?.id;

      if (newSessionId) {
        console.log('新创建的会话ID:', newSessionId);

        // 刷新会话列表
        await fetchSessions();

        // 先断开当前连接（如果有）
        if (connected.value) {
          disconnectTerminal();
        }

        // 明确设置当前会话ID为新创建的会话ID
        currentSession.value = newSessionId;
        console.log('设置当前会话ID:', currentSession.value);

        // 连接到新会话
        setTimeout(() => {
          console.log('尝试连接到新会话:', currentSession.value);
          connectToSession(newSessionId);
        }, 500); // 短暂延迟，确保UI更新
      } else {
        console.error('创建会话响应中没有会话ID');
        message.warning('创建会话成功，但无法自动连接，请手动选择会话');
        await fetchSessions();
      }
    } else {
      message.error(response?.message || '创建会话失败');
    }
  } catch (error: any) {
    console.error('创建会话出错:', error);
    message.error('创建会话失败');
  }
};

// 连接到会话
const connectToSession = (sessionId: string) => {
  if (!sessionId) {
    message.warning('请先选择一个会话');
    return;
  }

  // 获取会话名称，用于展示
  const session = sessions.value.find(s => s.id === sessionId);
  const sessionName = session ? session.name : sessionId;

  console.log(`正在连接到会话: ${sessionName} (ID: ${sessionId})`);

  // 获取token
  const token = getToken();
  if (!token) {
    message.error('未登录，无法连接终端');
    return;
  }

  // 关闭之前的连接
  if (ws) {
    try {
      console.log('关闭现有WebSocket连接');
      ws.onclose = null; // 防止触发重连
      ws.close();
    } catch (e) {
      console.error('关闭WebSocket连接失败:', e);
    }
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  // 修正WebSocket URL，与后端路由匹配
  const wsUrl = `${protocol}//${window.location.host}/api/servers/${serverId.value}/ws?token=${token}&session=${sessionId}`;

  console.log('连接终端WebSocket:', wsUrl);

  // 显示连接中提示
  terminal.value?.write(`正在连接到会话 "${sessionName}"...\r\n`);

  // 创建新连接
  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    connected.value = true;
    agentNotConnected.value = false;
    console.log('WebSocket连接成功');

    // 确保更新当前会话ID
    currentSession.value = sessionId;

    terminal.value?.write(`连接成功！会话名称: ${sessionName}\r\n`);

    // 连接成功，服务器必定在线，更新服务器状态
    if (!serverInfo.value.online) {
      console.log('终端WebSocket连接成功，更新服务器状态为在线');
      serverInfo.value.online = true;
      serverInfo.value.status = 'online';
    }

    // 自动调整终端大小
    autoResize();

    // 监听终端输入
    if (terminalDataDisposable) {
      try {
        terminalDataDisposable.dispose();
      } catch (e) {
        console.warn('清理旧的终端输入监听失败:', e);
      }
      terminalDataDisposable = null;
    }
    terminalDataDisposable = terminal.value?.onData((data) => {
      if (ws && ws.readyState === 1) {
        ws.send(JSON.stringify({
          type: 'shell_command',
          payload: {
            type: 'input',
            data: data,
            session: sessionId
          }
        }));
      }
    });
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);

      if (data.type === 'shell_response') {
        // 接收终端输出
        // 处理两种可能的消息格式
        let sessionId, outputData;

        if (data.payload) {
          // 新格式：{type, payload: {session, data}}
          sessionId = data.payload.session;
          outputData = data.payload.data;
        } else {
          // 旧格式：{type, session, data}
          sessionId = data.session;
          outputData = data.data;
        }

        // 只有当会话ID匹配时才写入终端
        if (sessionId === currentSession.value) {
          // 确保ANSI转义序列可以正常工作
          terminal.value?.write(outputData);
        } else {
          console.warn(`收到非当前会话的输出: ${sessionId}，当前会话: ${currentSession.value}`);
        }
      } else if (data.type === 'shell_error') {
        // 处理错误消息 - 使用ANSI红色显示错误
        terminal.value?.write(`\r\n\x1b[31m错误: ${data.error}\x1b[0m\r\n`);
      } else if (data.type === 'shell_close') {
        // 处理会话关闭 - 使用ANSI黄色显示会话关闭信息
        terminal.value?.write(`\r\n\x1b[33m会话已关闭: ${data.message}\x1b[0m\r\n`);
        connected.value = false;
      } else if (data.type === 'welcome') {
        // 欢迎消息，可以不处理
        console.log('收到欢迎消息:', data.message);
      } else if (data.type === 'error') {
        // 普通错误
        terminal.value?.write(`\r\n\x1b[31m${data.message}\x1b[0m\r\n`);

        // 处理特定错误
        handleWebSocketError(data.message);
      } else {
        // 其他未知类型
        console.log('收到未知类型消息:', data);
      }
    } catch (e) {
      // 如果不是JSON格式，直接显示
      console.error('解析WebSocket消息失败:', e);
      terminal.value?.write(`\r\n\x1b[31m解析消息失败\x1b[0m\r\n`);
    }
  };

  ws.onerror = (error) => {
    console.error('终端WebSocket错误:', error);
    terminal.value?.write('\r\n\x1b[31m连接发生错误\x1b[0m\r\n');
    connected.value = false;
  };

  ws.onclose = (event) => {
    console.log('WebSocket连接关闭，代码:', event.code, '原因:', event.reason);
    terminal.value?.write('\r\n\x1b[31m连接已关闭\x1b[0m\r\n');
    connected.value = false;

    // 如果是在文件操作期间断开，且不是手动关闭，尝试重连
    if (currentSession.value && event.code !== 1000 && event.code !== 1001) {
      if (isFileOperationInProgress.value) {
        console.log('检测到文件操作期间连接断开，将在操作完成后重连');
        // 文件操作期间断开，等待操作完成后重连
        const checkAndReconnect = () => {
          if (!isFileOperationInProgress.value) {
            console.log('文件操作已完成，开始重连');
            setTimeout(() => {
              reconnectTerminalSafely();
            }, 2000);
          } else {
            // 继续等待
            setTimeout(checkAndReconnect, 1000);
          }
        };
        setTimeout(checkAndReconnect, 1000);
      } else {
        // 非文件操作期间的断开，立即尝试重连
        console.log('检测到意外断开，尝试重连');
        setTimeout(() => {
          reconnectTerminalSafely();
        }, 2000);
      }
    }
  };
};

// 安全重连终端
const reconnectTerminalSafely = async () => {
  if (reconnectAttempts >= maxReconnectAttempts) {
    console.log('已达到最大重连次数，停止重连');
    message.warning('终端连接已断开，请手动重新连接');
    return;
  }

  if (connected.value) {
    console.log('终端已连接，无需重连');
    return;
  }

  if (!currentSession.value) {
    console.log('没有活跃会话，无法重连');
    return;
  }

  console.log(`尝试第 ${reconnectAttempts + 1} 次重连终端...`);
  reconnectAttempts++;

  try {
    // 等待一秒以确保之前的连接完全关闭
    await new Promise(resolve => setTimeout(resolve, 1000));

    // 重新连接
    await connectToSession(currentSession.value);

    if (connected.value) {
      console.log('终端重连成功');
      reconnectAttempts = 0; // 重置重连计数
      message.success('终端连接已恢复');
    } else {
      // 重连失败，等待后再次尝试
      if (reconnectAttempts < maxReconnectAttempts) {
        const delay = Math.min(5000 * reconnectAttempts, 15000); // 递增延迟，最多15秒
        console.log(`重连失败，${delay / 1000}秒后再次尝试...`);
        reconnectTimer = setTimeout(() => {
          reconnectTerminalSafely();
        }, delay);
      }
    }
  } catch (error) {
    console.error('重连过程中发生错误:', error);
    if (reconnectAttempts < maxReconnectAttempts) {
      const delay = Math.min(5000 * reconnectAttempts, 15000);
      console.log(`重连出错，${delay / 1000}秒后再次尝试...`);
      reconnectTimer = setTimeout(() => {
        reconnectTerminalSafely();
      }, delay);
    }
  }
};

// 断开终端连接
const disconnectTerminal = () => {
  try {
    // 清除重连定时器
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }

    if (ws) {
      try {
        ws.onclose = null; // 防止触发重连
        ws.close();
        ws = null;
      } catch (e) {
        console.error('关闭WebSocket连接失败:', e);
      }
    }
    connected.value = false;
    currentSession.value = '';

    if (terminal.value) {
      try {
        terminal.value.write('\r\n\x1b[31m已断开连接\x1b[0m\r\n');
      } catch (e) {
        console.warn('写入终端失败:', e);
      }
    }
  } catch (error) {
    console.warn('断开终端连接失败:', error);
  }
};

// 删除会话
const deleteSession = (sessionId: string) => {
  Modal.confirm({
    title: '确认删除',
    content: '确定要删除这个终端会话吗？该操作不可恢复。',
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await service.delete(`/servers/${serverId.value}/terminal/sessions/${sessionId}`);
        message.success('会话已删除');

        // 如果当前连接的是被删除的会话，则断开连接
        if (currentSession.value === sessionId) {
          disconnectTerminal();
        }

        // 刷新会话列表
        fetchSessions();
      } catch (error) {
        console.error('删除会话失败:', error);
      }
    },
  });
};

// 获取当前会话名称
const currentSessionName = computed(() => {
  const session = sessions.value.find(s => s.id === currentSession.value);
  return session ? session.name : '';
});

// 处理WebSocket错误
const handleWebSocketError = (errorMessage: string) => {
  if (errorMessage.includes('Agent未连接')) {
    agentNotConnected.value = true;
    message.error('服务器Agent未连接，请检查Agent状态或联系管理员');
  }
};

// 页面挂载时初始化
onMounted(async () => {
  // 暴露测试函数到全局供调试使用
  (window as any).testSystemStatusUpdate = testSystemStatusUpdate;
  (window as any).updateSystemStatusData = updateSystemStatusData;

  // 初始化系统状态数据（设置默认测试数据）
  initializeSystemStatusData();

  // 初始化编辑器最大高度
  updateMaxEditorHeight();

  // 获取服务器信息
  await fetchServerInfo();

  // 初始化终端
  initTerminal();

  // 自动调整终端大小
  autoResize();

  // 获取终端会话列表
  await fetchSessions();
  console.log('初始会话列表:', sessions.value);

  // 如果有会话列表，但服务器状态显示为离线，强制设置为在线
  if (sessions.value.length > 0 && !serverInfo.value.online) {
    console.log('检测到会话列表存在，但服务器显示为离线，强制更新状态为在线');
    serverInfo.value.online = true;
    serverInfo.value.status = 'online';
    message.info('检测到会话列表存在，已更新服务器状态');

    // 自动连接第一个会话
    if (!currentSession.value && sessions.value.length > 0) {
      const firstSession = sessions.value[0];
      console.log(`自动选择第一个会话: ${firstSession.name} (ID: ${firstSession.id})`);
      currentSession.value = firstSession.id;

      // 延迟连接，确保界面已经渲染
      setTimeout(() => {
        if (!connected.value) {
          console.log('尝试自动连接到第一个会话');
          connectToSession(firstSession.id);
        }
      }, 1000);
    }
  }

  // 尝试检查心跳以确认服务器状态
  setTimeout(() => {
    if (!serverInfo.value.online) {
      console.log('尝试通过心跳检查服务器状态');
      checkHeartbeat();
    }
  }, 1000);

  // 监听页面卸载事件，确保关闭连接
  window.addEventListener('beforeunload', disconnectTerminal);
});

// 页面卸载时清理
onUnmounted(() => {
  // 清理重连定时器
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }

  // 断开终端连接
  disconnectTerminal();

  // 移除事件监听器
  window.removeEventListener('resize', handleResize);
  window.removeEventListener('beforeunload', disconnectTerminal);

  // 清理拖拽事件监听器
  if (isResizing) {
    document.removeEventListener('mousemove', handleEditorResize);
    document.removeEventListener('mouseup', stopResize);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  }

  // 安全销毁终端和插件
  if (fitAddon.value) {
    try {
      // 销毁fitAddon前确保它已被加载
      fitAddon.value = null;
    } catch (error) {
      console.warn('销毁FitAddon失败:', error);
    }
  }

  // 安全销毁终端
  if (terminal.value) {
    try {
      terminal.value.dispose();
      terminal.value = null;
    } catch (error) {
      console.warn('销毁Terminal失败:', error);
    }
  }
  if (terminalDataDisposable) {
    try {
      terminalDataDisposable.dispose();
    } catch (error) {
      console.warn('销毁输入监听失败:', error);
    }
    terminalDataDisposable = null;
  }

  // 关闭状态WebSocket连接
  if (statusWs) {
    try {
      statusWs.onclose = null; // 防止触发重连
      statusWs.close();
      statusWs = null;
    } catch (error) {
      console.warn('关闭状态WebSocket失败:', error);
    }
  }
});

// 返回服务器详情页
const goBack = () => {
  try {
    // 清理重连定时器
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }

    // 先安全断开所有连接
    disconnectTerminal();

    // 确保可能的WebSocket连接已关闭
    if (ws) {
      try {
        ws.onclose = null;
        ws.close();
        ws = null;
      } catch (e) { }
    }

    if (statusWs) {
      try {
        statusWs.onclose = null;
        statusWs.close();
        statusWs = null;
      } catch (e) { }
    }

    // 确保FitAddon和Terminal不再被引用
    if (fitAddon.value) {
      fitAddon.value = null;
    }

    // 移除窗口事件监听器
    window.removeEventListener('resize', handleResize);
    window.removeEventListener('beforeunload', disconnectTerminal);

    // 最后进行导航
    router.push(`/admin/servers/${serverId.value}`);
  } catch (error) {
    console.warn('导航返回时发生错误:', error);
    // 即使出错也尝试导航
    router.push(`/admin/servers/${serverId.value}`);
  }
};

// 显示创建会话对话框
const showCreateSessionModal = () => {
  sessionName.value = '';
  sessionModalVisible.value = true;
};

// 连接终端
const connectTerminal = () => {
  if (!currentSession.value) {
    message.warning('请先选择一个会话');
    return;
  }

  // 查找当前选择的会话，获取名称
  const session = sessions.value.find(s => s.id === currentSession.value);
  if (!session) {
    console.error('找不到选择的会话:', currentSession.value);
    message.warning('选择的会话不存在或已失效，请重新选择');
    return;
  }

  console.log(`正在连接到选定会话: ${session.name} (ID: ${currentSession.value})`);
  connectToSession(currentSession.value);
};

// 发送数据到WebSocket
const sendData = (data: string) => {
  if (ws && ws.readyState === 1) {
    try {
      ws.send(data);
    } catch (error) {
      console.error('发送数据到WebSocket失败:', error);
    }
  }
};

// 右键菜单相关方法
const handleFileContextMenu = (event: MouseEvent, file: FileItem) => {
  event.preventDefault();
  contextMenuPosition.value = calculateMenuPosition(event.clientX, event.clientY);
  contextMenuTarget.value = file;
  contextMenuVisible.value = true;
};

const handleFileListContextMenu = (event: MouseEvent) => {
  event.preventDefault();
  // 检查是否点击在文件项上
  const target = event.target as HTMLElement;
  if (target && target.closest('.file-item')) {
    return; // 如果点击在文件项上，不显示空白区域菜单
  }

  contextMenuPosition.value = calculateMenuPosition(event.clientX, event.clientY);
  contextMenuTarget.value = null;
  contextMenuVisible.value = true;
};

// 计算菜单位置，确保不超出屏幕边界
const calculateMenuPosition = (x: number, y: number) => {
  const menuWidth = 180; // 预估菜单宽度
  const menuHeight = 200; // 预估菜单高度

  let finalX = x;
  let finalY = y;

  // 检查右边界
  if (x + menuWidth > window.innerWidth) {
    finalX = x - menuWidth;
  }

  // 检查下边界
  if (y + menuHeight > window.innerHeight) {
    finalY = y - menuHeight;
  }

  // 确保不超出左边界和上边界
  finalX = Math.max(10, finalX);
  finalY = Math.max(10, finalY);

  return { x: finalX, y: finalY };
};

const handleContextMenuClick = async ({ key }: { key: string }) => {
  contextMenuVisible.value = false;

  switch (key) {
    case 'copyPath':
      if (contextMenuTarget.value) {
        try {
          await navigator.clipboard.writeText(contextMenuTarget.value.path);
          message.success('路径已复制到剪贴板');
        } catch (error) {
          message.error('复制路径失败');
        }
      }
      break;

    case 'download':
      if (contextMenuTarget.value && !contextMenuTarget.value.is_dir) {
        handleDownloadFile(contextMenuTarget.value);
      }
      break;

    case 'delete':
      if (contextMenuTarget.value) {
        handleDeleteFile(contextMenuTarget.value);
      }
      break;

    case 'newFile':
      newFileName.value = '';
      newFileModalVisible.value = true;
      break;

    case 'newFolder':
      newFolderName.value = '';
      newFolderModalVisible.value = true;
      break;

    case 'upload':
      fileUploadInput.value?.click();
      break;

    case 'refresh':
      refreshFileList();
      break;
  }
};

// 下载文件
const handleDownloadFile = async (file: FileItem) => {
  try {
    // 获取token
    const token = getToken();
    if (!token) {
      message.error('请先登录');
      return;
    }

    const response = await service.get(`/servers/${serverId.value}/files/download`, {
      params: {
        path: file.path,
        token: token
      },
      responseType: 'blob'
    });

    // response是AxiosResponse，我们需要response.data来获取实际的blob数据
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
    console.error('下载文件失败:', error);
    message.error('下载文件失败');
  }
};

// 删除文件/文件夹
const handleDeleteFile = async (file: FileItem) => {
  Modal.confirm({
    title: `确定要删除${file.is_dir ? '文件夹' : '文件'} "${file.name}" 吗？`,
    content: file.is_dir ? '此操作将删除文件夹及其所有内容，且无法恢复。' : '此操作无法恢复。',
    okText: '确定',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await service.post(`/servers/${serverId.value}/files/delete`, {
          paths: [file.path]
        });
        message.success('删除成功');
        refreshFileList();
      } catch (error) {
        console.error('删除失败:', error);
        message.error('删除失败');
      }
    }
  });
};

// 新建文件
const handleNewFile = async () => {
  if (!newFileName.value.trim()) {
    message.error('请输入文件名');
    return;
  }

  const filePath = currentPath.value === '/'
    ? `/${newFileName.value.trim()}`
    : `${currentPath.value}/${newFileName.value.trim()}`;

  try {
    await service.post(`/servers/${serverId.value}/files/create`, {
      path: filePath,
      content: ''
    });

    message.success('文件创建成功');
    newFileModalVisible.value = false;
    refreshFileList();
  } catch (error) {
    console.error('创建文件失败:', error);
    message.error('创建文件失败');
  }
};

// 新建文件夹
const handleNewFolder = async () => {
  if (!newFolderName.value.trim()) {
    message.error('请输入文件夹名');
    return;
  }

  const folderPath = currentPath.value === '/'
    ? `/${newFolderName.value.trim()}`
    : `${currentPath.value}/${newFolderName.value.trim()}`;

  try {
    await service.post(`/servers/${serverId.value}/files/mkdir`, {
      path: folderPath
    });

    message.success('文件夹创建成功');
    newFolderModalVisible.value = false;
    refreshFileList();
  } catch (error) {
    console.error('创建文件夹失败:', error);
    message.error('创建文件夹失败');
  }
};

// 拖拽上传相关方法
const handleDragOver = (event: DragEvent) => {
  event.preventDefault();
};

const handleDragEnter = (event: DragEvent) => {
  event.preventDefault();
  isDragOver.value = true;
};

const handleDragLeave = (event: DragEvent) => {
  event.preventDefault();
  // 只有当离开整个文件列表区域时才设置为false
  const target = event.target as HTMLElement;
  const fileList = target.closest('.file-list');
  if (!fileList || !fileList.contains(event.relatedTarget as Node)) {
    isDragOver.value = false;
  }
};

const handleFileDrop = (event: DragEvent) => {
  event.preventDefault();
  isDragOver.value = false;

  const files = event.dataTransfer?.files;
  if (files && files.length > 0) {
    uploadFiles(files);
  }
};

const handleFileUpload = (event: Event) => {
  const target = event.target as HTMLInputElement;
  const files = target.files;
  if (files && files.length > 0) {
    uploadFiles(files);
  }
  // 清空input的值，以便可以重复选择同一个文件
  target.value = '';
};

const uploadFiles = async (files: FileList) => {
  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    await uploadSingleFile(file);
  }
  refreshFileList();
};

const uploadSingleFile = async (file: File) => {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('path', currentPath.value);

  try {
    await service.post(`/servers/${serverId.value}/files/upload`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data'
      },
      timeout: 60000 // 60秒超时
    });

    message.success(`文件 "${file.name}" 上传成功`);
  } catch (error) {
    console.error(`上传文件 "${file.name}" 失败:`, error);
    message.error(`上传文件 "${file.name}" 失败`);
  }
};
</script>

<template>
  <div class="terminal-container" :style="layoutStyle">
    <!-- 页面头部 -->
    <a-page-header title="终端" :sub-title="serverInfo.name" @back="goBack" :class="{ 'header-hidden': false }">
      <template #tags>
        <a-tag :color="serverInfo.online ? 'success' : 'error'">
          {{ serverInfo.online ? '在线' : '离线' }}
        </a-tag>
        <a-tag v-if="connected" color="success">已连接</a-tag>
        <a-tag v-else color="default">未连接</a-tag>
        <a-tag v-if="isFileOperationInProgress" color="orange">
          <template #icon>
            <SaveOutlined />
          </template>
          文件操作中
        </a-tag>
        <a-tag v-if="reconnectAttempts > 0" color="warning">
          重连中 ({{ reconnectAttempts }}/{{ maxReconnectAttempts }})
        </a-tag>
      </template>

      <template #extra>
        <a-space>
          <a-tooltip title="切换文件管理器">
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
        <!-- 文件管理器头部 -->
        <div class="file-manager-header">
          <div class="header-top">
            <div class="header-title">
              <FolderOpenOutlined />
              <span>文件浏览</span>
            </div>

            <div class="header-actions">
              <a-tooltip title="刷新">
                <a-button size="small" type="text" @click="refreshFileList" :loading="fileLoading">
                  <template #icon>
                    <ReloadOutlined />
                  </template>
                </a-button>
              </a-tooltip>
              <a-tooltip :title="showHiddenFiles ? '隐藏隐藏文件' : '显示隐藏文件'">
                <a-button size="small" type="text" @click="toggleHiddenFiles"
                  :class="{ 'active-btn': showHiddenFiles }">
                  <template #icon>
                    <MenuOutlined />
                  </template>
                </a-button>
              </a-tooltip>
            </div>
          </div>

          <div class="path-section">
            <div class="path-bar">
              <a-tooltip title="返回上级">
                <a-button size="small" type="text" @click="goToParentDirectory"
                  :disabled="currentPath === '/' || fileLoading" class="back-btn">
                  <template #icon>
                    <ArrowLeftOutlined />
                  </template>
                </a-button>
              </a-tooltip>
              <div class="path-breadcrumb">
                <template v-if="showFullPath">
                  <span v-for="(segment, index) in fullPathSegments" :key="`full-${index}`" class="path-segment" :class="{
                    'path-segment-active': index === fullPathSegments.length - 1
                  }" @click="navigateToPath(segment.path)" :title="segment.name">
                    {{ segment.name }}
                    <span v-if="index < fullPathSegments.length - 1" class="path-separator">/</span>
                  </span>
                  <a-tooltip title="收起完整路径">
                    <a-button size="small" type="text" @click="toggleFullPath" class="collapse-btn">
                      <template #icon>
                        <ArrowLeftOutlined style="transform: rotate(90deg);" />
                      </template>
                    </a-button>
                  </a-tooltip>
                </template>
                <template v-else>
                  <span v-for="(segment, index) in pathSegments" :key="index" class="path-segment" :class="{
                    'path-segment-active': index === pathSegments.length - 1 && !segment.isEllipsis,
                    'path-ellipsis': segment.isEllipsis
                  }"
                    @click="segment.isEllipsis ? toggleFullPath() : navigateToPath(getPathFromSegments(segment.originalIndex))"
                    :title="segment.isEllipsis ? '点击展开完整路径' : segment.name">
                    {{ segment.name }}
                    <span v-if="index < pathSegments.length - 1" class="path-separator">/</span>
                  </span>
                </template>
              </div>
            </div>
          </div>

          <div class="file-stats" v-if="fileList.length > 0">
            <span class="stats-text">
              {{fileList.filter(f => f.is_dir).length}}个文件夹 · {{fileList.filter(f => !f.is_dir).length}}个文件
            </span>
          </div>
        </div>

        <!-- 搜索框 -->
        <div class="file-search">
          <a-input v-model:value="fileSearchKeyword" placeholder="搜索文件..." size="small" allowClear>
            <template #prefix>
              <SearchOutlined />
            </template>
          </a-input>
        </div>

        <!-- 文件列表 -->
        <div class="file-list" @contextmenu="handleFileListContextMenu" @drop="handleFileDrop"
          @dragover="handleDragOver" @dragenter="handleDragEnter" @dragleave="handleDragLeave"
          :class="{ 'drag-over': isDragOver }" v-bind="containerProps">
          <a-spin :spinning="fileLoading" size="small">
            <div v-if="fileList.length === 0 && !fileLoading" class="empty-placeholder">
              <div class="empty-icon">📁</div>
              <div class="empty-text">此目录为空</div>
              <div class="empty-hint">右键空白区域可以新建文件或文件夹</div>
              <div class="empty-hint">拖拽文件到此处可以上传</div>
            </div>
            <div v-bind="wrapperProps">
              <div v-for="item in list" :key="item.data.path" class="file-item" :class="{
                'file-item-selected': selectedFile?.path === item.data.path,
                'file-item-directory': item.data.is_dir
              }" @click="selectedFile = item.data"
                @dblclick="item.data.is_dir ? enterDirectory(item.data) : editFile(item.data)"
                @contextmenu="(event) => handleFileContextMenu(event, item.data)" :style="{ height: '46px' }">
                <div class="file-icon" :class="{ 'directory-icon': item.data.is_dir }">
                  <component :is="getFileIcon(item.data)" />
                </div>
                <div class="file-info">
                  <div class="file-name" :title="item.data.name">{{ item.data.name }}</div>
                  <div class="file-meta">
                    <span v-if="!item.data.is_dir" class="file-size">{{ formatFileSize(item.data.size || 0) }}</span>
                    <span v-if="item.data.mod_time" class="file-time">{{ new
                      Date(item.data.mod_time).toLocaleDateString()
                      }}</span>
                    <span v-if="item.data.permission" class="file-permission">{{ item.data.permission }}</span>
                  </div>
                </div>
                <div class="file-actions">
                  <a-tooltip title="编辑文件" v-if="!item.data.is_dir && isEditableFile(item.data)">
                    <a-button size="small" type="text" @click.stop="editFile(item.data)" class="edit-btn">
                      <template #icon>
                        <EditOutlined />
                      </template>
                    </a-button>
                  </a-tooltip>
                </div>
              </div>
            </div>
          </a-spin>
        </div>
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
                <Codemirror v-model="activeTab.content" :style="{ height: `${editorHeight - 80}px` }"
                  :extensions="[basicSetup, getLanguageExtension(activeTab.language), vscodeDark]" :autofocus="true"
                  :indent-with-tab="true" :tab-size="2" :placeholder="`编辑 ${activeTab.file.name}...`"
                  @change="(content) => onEditorContentChange(content)" />
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
                <div ref="terminalRef" class="terminal"></div>
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
      </div> <!-- 工作区容器结束 -->

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

    <!-- 文件编辑模态框 -->
    <a-modal v-model:visible="fileEditVisible" :title="`编辑文件 - ${editingFile?.name}`" width="80%" @ok="saveFile"
      @cancel="fileEditVisible = false" :confirmLoading="fileEditLoading">
      <template #footer>
        <a-button @click="fileEditVisible = false">取消</a-button>
        <a-button type="primary" @click="saveFile" :loading="fileEditLoading">保存</a-button>
      </template>

      <div class="file-editor">
        <a-textarea v-model:value="fileContent" :rows="20" placeholder="文件内容..."
          style="font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;" />
      </div>
    </a-modal>

    <!-- 创建会话对话框 -->
    <a-modal v-model:visible="sessionModalVisible" title="创建新终端会话" @ok="createSession" :maskClosable="false">
      <a-form layout="vertical">
        <a-form-item label="会话名称" required>
          <a-input v-model:value="sessionName" placeholder="请输入会话名称" @pressEnter="createSession" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 文件上传 -->
    <input ref="fileUploadInput" type="file" multiple style="display: none" @change="handleFileUpload" />
  </div>

  <!-- 右键菜单 - 作为全局浮层 -->
  <teleport to="body">
    <div v-if="contextMenuVisible" class="context-menu-overlay" @click="contextMenuVisible = false"
      @contextmenu.prevent>
      <div class="context-menu" :style="{
        position: 'fixed',
        left: contextMenuPosition.x + 'px',
        top: contextMenuPosition.y + 'px',
        zIndex: 10000
      }" @click.stop>
        <a-menu @click="handleContextMenuClick" mode="vertical">
          <!-- 文件/文件夹菜单 -->
          <template v-if="contextMenuTarget">
            <a-menu-item key="copyPath">
              <template #icon>
                <CopyOutlined />
              </template>
              复制路径
            </a-menu-item>
            <a-menu-item v-if="!contextMenuTarget.is_dir" key="download">
              <template #icon>
                <DownloadOutlined />
              </template>
              下载文件
            </a-menu-item>
            <a-menu-divider />
            <a-menu-item key="delete" danger>
              <template #icon>
                <DeleteOutlined />
              </template>
              删除
            </a-menu-item>
          </template>
          <!-- 空白区域菜单 -->
          <template v-else>
            <a-menu-item key="newFile">
              <template #icon>
                <FileAddOutlined />
              </template>
              新建文件
            </a-menu-item>
            <a-menu-item key="newFolder">
              <template #icon>
                <FolderAddOutlined />
              </template>
              新建文件夹
            </a-menu-item>
            <a-menu-divider />
            <a-menu-item key="upload">
              <template #icon>
                <UploadOutlined />
              </template>
              上传文件
            </a-menu-item>
            <a-menu-item key="refresh">
              <template #icon>
                <ReloadOutlined />
              </template>
              刷新
            </a-menu-item>
          </template>
        </a-menu>
      </div>
    </div>
  </teleport>

  <!-- 新建文件对话框 -->
  <a-modal v-model:visible="newFileModalVisible" title="新建文件" @ok="handleNewFile" @cancel="newFileModalVisible = false">
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
</template>

<style scoped>
:root {
  --glass-bg: rgba(255, 255, 255, 0.7);
  --glass-border: rgba(0, 0, 0, 0.05);
  --glass-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
  --radius-lg: 16px;
}

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

.terminal-container {
  padding: 0;
  background: transparent;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.header-hidden {
  display: none;
}

.main-content {
  display: flex;
  flex: 1;
  margin-top: 16px;
  gap: 16px;
  height: calc(100vh - 140px);
  overflow: hidden;
}

/* File Manager */
.file-manager-sidebar {
  width: 280px;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
  transition: all 0.3s ease;
}

.file-manager-header {
  padding: 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.5);
}

.header-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  color: #1d1d1f;
  font-size: 15px;
}

.path-bar {
  display: flex;
  align-items: center;
  background: rgba(0, 0, 0, 0.03);
  padding: 4px;
  border-radius: 8px;
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.path-breadcrumb {
  flex: 1;
  display: flex;
  align-items: center;
  overflow-x: auto;
  padding: 4px 8px;
  font-size: 13px;
  color: #1d1d1f;
}

.file-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.file-actions {
  opacity: 0;
  transition: all 0.2s ease;
  display: flex;
  gap: 4px;
  align-items: center;
  margin-left: auto;
  /* Push to far right */
  padding-left: 8px;
}

.file-item {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 8px;
  margin-bottom: 2px;
  cursor: pointer;
  transition: all 0.2s ease;
  border: 1px solid transparent;
}

.file-item:hover {
  background: rgba(0, 0, 0, 0.05);
}

.file-item-directory {
  background: rgba(0, 122, 255, 0.05);
}

.file-item-directory:hover {
  background: rgba(0, 122, 255, 0.1);
}

.file-item-selected {
  background: rgba(0, 122, 255, 0.1) !important;
  color: #007aff;
}

.file-icon {
  margin-right: 12px;
  font-size: 18px;
  color: #8e8e93;
}

.file-name {
  font-size: 13px;
  font-weight: 500;
  color: #1d1d1f;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-meta {
  font-size: 11px;
  color: #8e8e93;
  margin-top: 2px;
  display: flex;
  gap: 8px;
}

/* Terminal & Workspace */
.workspace-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;
  overflow: hidden;
}

.terminal-section {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px;
  padding: 16px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
  min-height: 400px;
}

.terminal-wrapper {
  flex: 1;
  background: #1e1e1e;
  border-radius: 12px;
  overflow: hidden;
  padding: 12px;
  box-shadow: inset 0 0 20px rgba(0, 0, 0, 0.5);
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

.session-select-compact {
  display: flex;
  align-items: center;
  gap: 12px;
}

.connection-status {
  display: flex;
  align-items: center;
  gap: 8px;
}

.session-actions-compact {
  display: flex;
  align-items: center;
}

/* System Status Sidebar */
.system-status-sidebar {
  width: 280px;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
}

.system-status-header {
  padding: 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.5);
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
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.02);
}

.status-title {
  font-weight: 600;
  color: #1d1d1f;
  font-size: 13px;
  margin-bottom: 8px;
}

.status-value {
  font-weight: 700;
  color: #007aff;
  font-size: 14px;
  font-family: "SF Mono", Menlo, monospace;
}

/* Editor */
.editor-container {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.05);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.05);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.editor-header {
  background: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  padding: 0 12px;
  height: 42px;
  display: flex;
  align-items: flex-end;
}

.editor-tabs {
  flex: 1;
  display: flex;
  align-items: flex-end;
  overflow-x: auto;
  overflow-y: hidden;
  height: 100%;
  scrollbar-width: none;
  /* Hide scrollbar for cleaner look */
  margin-right: 8px;
}

.editor-tabs::-webkit-scrollbar {
  display: none;
}

.editor-actions {
  display: flex;
  align-items: center;
  padding-bottom: 6px;
  gap: 4px;
  flex-shrink: 0;
}

.editor-tab {
  background: rgba(0, 0, 0, 0.03);
  border-radius: 8px 8px 0 0;
  padding: 8px 12px;
  margin-right: 2px;
  font-size: 12px;
  color: #8e8e93;
  cursor: pointer;
  min-width: 120px;
  max-width: 300px;
  position: relative;
  flex-shrink: 0;
  /* Prevent shrinking */
  border: 1px solid transparent;
  /* Prepare for active state border */
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.tab-content {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  overflow: hidden;
}

.tab-icon {
  display: flex;
  align-items: center;
  font-size: 14px;
}

.tab-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.tab-language {
  font-size: 10px;
  opacity: 0.6;
  margin-left: 4px;
}

.tab-dirty-indicator {
  font-size: 10px;
  color: #faad14;
  margin-left: 4px;
}

.tab-close {
  opacity: 0;
  transition: opacity 0.2s;
  margin-left: 4px;
  flex-shrink: 0;
}

.editor-tab:hover .tab-close {
  opacity: 1;
}

.editor-tab-active .tab-close {
  opacity: 1;
}

/* 编辑器高度调整拖拽条 */
.editor-resize-handle {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 16px;
  /* Increased height for easier grabbing */
  cursor: row-resize;
  /* Better cursor */
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  /* Ensure it's on top */
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(4px);
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  transition: background 0.2s;
}

.editor-resize-handle:hover {
  background: rgba(0, 122, 255, 0.1);
}

.resize-line {
  width: 40px;
  height: 4px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 2px;
}

.dark .resize-line {
  background: rgba(255, 255, 255, 0.2);
}

.editor-tab-active {
  background: rgba(255, 255, 255, 0.95);
  color: #1d1d1f;
  box-shadow: 0 -2px 10px rgba(0, 122, 255, 0.1);
}

/* Responsive */
@media (max-width: 1200px) {
  .main-content {
    flex-direction: column;
    height: auto;
    overflow: visible;
  }

  .file-manager-sidebar,
  .system-status-sidebar {
    width: 100%;
    height: 300px;
  }
}
</style>

<style>
/* Global Dark Mode Styles */
.dark .file-manager-sidebar,
.dark .terminal-section,
.dark .system-status-sidebar,
.dark .editor-container {
  background: rgba(30, 30, 30, 0.6) !important;
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
}

.dark .session-controller {
  background: rgba(40, 40, 40, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.dark .file-manager-header,
.dark .system-status-header,
.dark .editor-header {
  background: rgba(40, 40, 40, 0.6);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.dark .header-title,
.dark .status-title,
.dark .file-name {
  color: #f5f5f7;
}

.dark .path-bar {
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.dark .path-breadcrumb {
  color: #f5f5f7;
}

.dark .file-item:hover {
  background: rgba(255, 255, 255, 0.1);
}

.dark .file-item-directory {
  background: rgba(0, 122, 255, 0.05);
  /* Light blue tint for folders */
}


.file-item-directory:hover {
  background: rgba(0, 122, 255, 0.1);
}

.dark .file-item-selected {
  background: rgba(0, 122, 255, 0.2) !important;
  border-color: rgba(0, 122, 255, 0.3);
}

.dark .status-item {
  background: rgba(40, 40, 40, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.dark .editor-tab {
  background: rgba(255, 255, 255, 0.05);
  color: #8e8e93;
}

.dark .editor-tab-active {
  background: rgba(40, 40, 40, 0.95);
  color: #f5f5f7;
  border-top: 2px solid #007aff;
}
</style>