<script setup lang="ts">
import { ref, reactive, onMounted, computed, defineComponent, nextTick, watch, onBeforeUnmount } from 'vue';
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
  FileUnknownOutlined,
  ArrowLeftOutlined,
  UploadOutlined,
  DownloadOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  EnterOutlined,
  CodeOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
import { getToken } from '../../utils/auth';
// 导入服务器状态store
import { useServerStore } from '../../stores/serverStore';
import { useUIStore } from '../../stores/uiStore';
// 导入CodeMirror相关组件
import { Codemirror } from 'vue-codemirror';
// 导入 xterm
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';
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

// 定义API响应类型，使typescript正确处理
interface ApiResponse {
  [key: string]: any;
  data?: string | any;
  content?: string | any;
}

// 定义components以使用Codemirror组件
defineComponent({
  components: {
    Codemirror
  }
});

// 定义服务器响应数据类型
interface ServerResponse {
  server: {
    ID: number;
    name: string;
    status: string;
    online: boolean;
    secret_key: string;
    ip: string;
    last_heartbeat: string;
    system_info: string;
    // 其他属性省略
  }
}

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));

const normalizePath = (value?: string) => {
  if (!value) {
    return '/';
  }
  const trimmed = value.trim();
  if (!trimmed) {
    return '/';
  }
  return trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
};

const getQueryPath = (): string => {
  const raw = route.query.path;
  if (Array.isArray(raw)) {
    return raw[0] || '';
  }
  return typeof raw === 'string' ? raw : '';
};

const initialPath = ref<string>(normalizePath(getQueryPath()));
// 获取服务器状态store
const serverStore = useServerStore();
const uiStore = useUIStore();

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// 文件管理状态
const currentPath = ref('/');
const fileList = ref<any[]>([]);
const selectedRowKeys = ref<string[]>([]);
const selectedFiles = ref<any[]>([]);
// 搜索关键词
const searchKeyword = ref('');

// 目录树
const treeData = ref<any[]>([]);
const expandedKeys = ref<string[]>([]);

// 文件上传
const uploadModalVisible = ref(false);
const fileToUpload = ref<File | null>(null);
const uploading = ref(false);

// 文件编辑
const editModalVisible = ref(false);
const fileContent = ref('');
const editingFile = ref<any>(null);
const editLoading = ref(false);

// 新建文件/文件夹
const createModalVisible = ref(false);
const createFormState = reactive({
  type: 'file', // 'file' 或 'directory'
  name: '',
  content: ''
});

// 路径跳转
const pathJumpModalVisible = ref(false);
const jumpToPath = ref('');

// 终端相关
const terminalModalVisible = ref(false);
const terminalRef = ref<HTMLElement | null>(null);
const terminal = ref<Terminal | null>(null);
const fitAddon = ref<FitAddon | null>(null);
let terminalWs: WebSocket | null = null;
let terminalDataDisposable: { dispose: () => void } | null = null;
const terminalWorkingDir = ref<string>('/');
const terminalSessionId = ref<string>(''); // 存储终端会话ID

// 计算服务器是否在线 (使用全局状态)
const isServerOnline = computed(() => {
  return serverStore.isServerOnline(serverId.value);
});

// 文件类型图标映射
const getFileIcon = (file: any) => {
  if (file.is_dir) {
    return FolderOutlined;
  }

  // 获取文件扩展名
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

// 检测文件语言类型
const detectLanguage = (fileName: string) => {
  // 获取文件扩展名
  const ext = fileName.split('.').pop()?.toLowerCase() || '';

  // 根据扩展名返回对应的语言模式
  switch (ext) {
    case 'js':
    case 'jsx':
    case 'ts':
    case 'tsx':
      return 'javascript';
    case 'html':
    case 'htm':
      return 'html';
    case 'css':
      return 'css';
    case 'json':
      return 'json';
    case 'md':
      return 'markdown';
    case 'py':
      return 'python';
    case 'php':
      return 'php';
    case 'xml':
      return 'xml';
    case 'rs':
      return 'rust';
    case 'sql':
      return 'sql';
    case 'java':
      return 'java';
    case 'c':
    case 'cpp':
    case 'h':
    case 'hpp':
      return 'cpp';
    case 'sh':
    case 'bash':
      return 'shell';
    case 'conf':
      return 'nginx';
    default:
      return 'text';
  }
};

// 编辑文件对话框
const fileLanguage = ref('');

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    // 使用any类型避免TypeScript错误
    const response: any = await request.get(`/servers/${serverId.value}`);
    console.log('服务器详情响应:', response);

    // 从响应中提取服务器数据
    if (response && response.server) {
      serverInfo.value = response.server;

      // 更新全局状态
      const status = response.server.status || 'offline';
      const isOnline = response.server.online === true;

      console.log('更新服务器状态:', serverId.value, status, isOnline);
      serverStore.updateServerStatus(serverId.value, status);
    } else {
      console.error('响应中没有找到服务器数据');
      message.error('获取服务器数据失败');
    }
  } catch (error) {
    console.error('获取服务器信息失败:', error);
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
    uiStore.stopLoading();
  }
};

// 获取文件列表
const fetchFileList = async (path: string = '/') => {
  loading.value = true;
  selectedRowKeys.value = [];
  selectedFiles.value = [];
  // 清空搜索关键词
  searchKeyword.value = '';

  const safePath = normalizePath(path);

  try {
    const response = await request.get(`/servers/${serverId.value}/files`, {
      params: { path: safePath }
    });

    console.log('获取文件列表响应:', response);

    // 检查响应格式
    if (Array.isArray(response)) {
      // 直接使用数组
      fileList.value = response.map((file: any) => ({
        ...file,
        key: `${safePath === '/' ? '' : safePath}/${file.name}`
      }));
    } else if (response && Array.isArray(response.data)) {
      // 使用response.data数组
      fileList.value = response.data.map((file: any) => ({
        ...file,
        key: `${safePath === '/' ? '' : safePath}/${file.name}`
      }));
    } else {
      console.error('文件列表响应格式错误:', response);
      fileList.value = [];
    }

    console.log('处理后的文件列表:', fileList.value);
    currentPath.value = safePath;
  } catch (error) {
    console.error('获取文件列表失败:', error);
    message.error('获取文件列表失败');
    fileList.value = [];
  } finally {
    loading.value = false;
  }
};

// 获取目录树（初始加载，只加载根目录的第一层）
const fetchDirectoryTree = async () => {
  try {
    console.log('开始获取根目录树');
    const response = await request.get(`/servers/${serverId.value}/files/children`, {
      params: { path: '/' }
    });

    console.log('获取根目录树响应:', response);

    const buildTreeData = (items: any[], parentPath: string = '/'): any[] => {
      return items.filter(item => item.is_dir).map(item => {
        const path = parentPath === '/' ? `/${item.name}` : `${parentPath}/${item.name}`;

        return {
          title: item.name,
          key: path,
          isLeaf: false, // 所有目录都不是叶子节点，需要动态加载
          children: undefined, // 初始不加载子节点
          hasLoadedChildren: false // 标记是否已加载子节点
        };
      });
    };

    // 检查响应格式
    let items = [];
    if (Array.isArray(response)) {
      items = response;
    } else if (response && Array.isArray(response.data)) {
      items = response.data;
    } else {
      console.error('目录树响应格式错误:', response);
      treeData.value = [];
      return;
    }

    treeData.value = buildTreeData(items, '/');
    console.log('处理后的目录树:', treeData.value);
    expandedKeys.value = ['/'];
  } catch (error) {
    console.error('获取目录树失败:', error);
    treeData.value = [];
  }
};

// 动态加载目录子节点
const loadTreeNodeChildren = async (node: any) => {
  console.log('动态加载目录子节点:', node.key);

  if (node.hasLoadedChildren) {
    console.log('子节点已加载，跳过');
    return;
  }

  try {
    const response = await request.get(`/servers/${serverId.value}/files/children`, {
      params: { path: node.key }
    });

    console.log('获取子目录响应:', response);

    // 检查响应格式
    let items = [];
    if (Array.isArray(response)) {
      items = response;
    } else if (response && Array.isArray(response.data)) {
      items = response.data;
    } else {
      console.error('子目录响应格式错误:', response);
      return;
    }

    // 只保留目录
    const directories = items.filter(item => item.is_dir);

    // 构建子节点
    const children = directories.map(item => {
      const path = `${node.key}/${item.name}`.replace(/\/+/g, '/');

      return {
        title: item.name,
        key: path,
        isLeaf: false,
        children: undefined,
        hasLoadedChildren: false
      };
    });

    // 更新树节点
    const updateNodeInTree = (nodes: any[]): any[] => {
      return nodes.map(n => {
        if (n.key === node.key) {
          return {
            ...n,
            children: children,
            hasLoadedChildren: true
          };
        } else if (n.children) {
          return {
            ...n,
            children: updateNodeInTree(n.children)
          };
        }
        return n;
      });
    };

    treeData.value = updateNodeInTree(treeData.value);
    console.log('子节点加载完成:', children.length, '个目录');

  } catch (error) {
    console.error('加载子目录失败:', error);
  }
};

// 导航到上级目录
const navigateToParent = () => {
  if (currentPath.value === '/') return;

  const parts = currentPath.value.split('/').filter(p => p);
  parts.pop();
  const parentPath = parts.length === 0 ? '/' : `/${parts.join('/')}`;

  fetchFileList(parentPath);
};

// 双击文件/文件夹
const handleRowDoubleClick = (record: any) => {
  if (record.is_dir) {
    // 如果是目录，进入该目录
    const newPath = `${currentPath.value === '/' ? '' : currentPath.value}/${record.name}`;
    fetchFileList(newPath);
  } else {
    // 如果是文件，根据文件类型处理
    if (isTextFile(record)) {
      // 如果是文本文件，打开编辑器
      openFileEditor(record);
    } else {
      // 如果是其他类型的文件，提示下载
      message.info('非文本文件，请使用下载功能');
    }
  }
};

// 判断是否是文本文件
const isTextFile = (file: any) => {
  const textExtensions = [
    'txt', 'log', 'md', 'conf', 'ini', 'yaml', 'yml', 'json', 'xml',
    'html', 'htm', 'css', 'js', 'ts', 'sh', 'bash', 'py', 'go',
    'java', 'c', 'cpp', 'h', 'php', 'jsx', 'tsx', 'vue'
  ];

  // 获取文件扩展名
  const ext = file.name.split('.').pop()?.toLowerCase();

  return textExtensions.includes(ext);
};

// 打开文件编辑器（修改版）
const openFileEditor = async (file: any) => {
  editingFile.value = file;
  editLoading.value = true;

  // 设置文件语言
  fileLanguage.value = detectLanguage(file.name);
  console.log('检测到文件语言:', fileLanguage.value);

  try {
    const filePath = `${currentPath.value === '/' ? '' : currentPath.value}/${file.name}`;
    console.log('请求文件内容，路径:', filePath);
    const response: string | ApiResponse = await request.get(`/servers/${serverId.value}/files/content`, {
      params: { path: filePath }
    });

    console.log('获取文件内容响应详情:', response);

    // 处理响应数据
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

    console.log('设置的文件内容长度:', content.length);

    // 先清空内容
    fileContent.value = '';

    // 先打开模态框
    editModalVisible.value = true;

    // 等待DOM更新和模态框完全打开
    await nextTick();

    // 延迟设置内容，确保编辑器已完全初始化
    setTimeout(() => {
      fileContent.value = content;
      console.log('设置编辑器内容，长度:', content.length);

      // 如果还是不显示，可以尝试手动触发更新
      nextTick(() => {
        if (fileContent.value !== content) {
          const tempContent = content;
          fileContent.value = '';
          setTimeout(() => {
            fileContent.value = tempContent;
          }, 50);
        }
      });
    }, 200);

  } catch (error) {
    console.error('获取文件内容失败:', error);
    message.error('获取文件内容失败');
    closeEditor();
  } finally {
    editLoading.value = false;
  }
};

// 保存文件内容
const saveFileContent = async () => {
  if (!editingFile.value) return;

  editLoading.value = true;

  try {
    const filePath = `${currentPath.value === '/' ? '' : currentPath.value}/${editingFile.value.name}`;
    console.log('保存文件内容，路径:', filePath);
    console.log('保存文件内容长度:', fileContent.value.length);

    await request.put(`/servers/${serverId.value}/files/content`, {
      path: filePath,
      content: fileContent.value
    });

    message.success('文件保存成功');
    closeEditor();
  } catch (error) {
    console.error('保存文件内容失败:', error);
    message.error('保存文件内容失败');
  } finally {
    editLoading.value = false;
  }
};

// 关闭编辑器时清理状态
const closeEditor = () => {
  editModalVisible.value = false;
  fileContent.value = '';
  editingFile.value = null;
  fileLanguage.value = '';
};

// 上传文件
const handleFileUpload = async () => {
  if (!fileToUpload.value) {
    message.error('请选择要上传的文件');
    return;
  }

  uploading.value = true;

  try {
    const formData = new FormData();
    formData.append('file', fileToUpload.value);
    formData.append('path', currentPath.value);

    await request.post(`/servers/${serverId.value}/files/upload`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data'
      }
    });

    message.success('文件上传成功');
    uploadModalVisible.value = false;
    fileToUpload.value = null;

    // 刷新文件列表
    fetchFileList(currentPath.value);
  } catch (error) {
    console.error('上传文件失败:', error);
    message.error('上传文件失败');
  } finally {
    uploading.value = false;
  }
};

// 选择上传文件
const handleFileChange = (info: any) => {
  // 处理文件变更事件，包括通过点击选择和拖拽添加的文件
  const file = info.file?.originFileObj || info.file;

  if (file) {
    console.log('选择或拖拽的文件:', file.name);
    fileToUpload.value = file;
  }
};

// 下载文件
const downloadFile = (file: any) => {
  if (file.is_dir) {
    message.error('不能直接下载文件夹');
    return;
  }

  const filePath = `${currentPath.value === '/' ? '' : currentPath.value}/${file.name}`;
  const token = getToken();

  if (!token) {
    message.error('未登录，无法下载文件');
    return;
  }

  // 创建下载链接 (注意，需要添加/api前缀，确保与request.ts中的baseURL一致)
  const downloadUrl = `${window.location.origin}/api/servers/${serverId.value}/files/download?path=${encodeURIComponent(filePath)}&token=${token}`;
  console.log('下载文件URL:', downloadUrl);

  // 创建一个临时的a标签，模拟点击下载
  const a = document.createElement('a');
  a.href = downloadUrl;
  a.download = file.name;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
};

// 删除文件或目录
const deleteFiles = () => {
  if (selectedFiles.value.length === 0) {
    message.warning('请选择要删除的文件或目录');
    return;
  }

  const fileNames = selectedFiles.value.map(file => file.name).join(', ');

  Modal.confirm({
    title: '确认删除',
    content: `确定要删除选中的 ${selectedFiles.value.length} 个文件或目录吗？\n${fileNames}`,
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        const paths = selectedFiles.value.map(file => {
          return `${currentPath.value === '/' ? '' : currentPath.value}/${file.name}`;
        });

        await request.post(`/servers/${serverId.value}/files/delete`, { paths });

        message.success('删除成功');

        // 刷新文件列表
        fetchFileList(currentPath.value);
        // 刷新目录树中对应的节点
        await refreshTreeNode(currentPath.value);
      } catch (error) {
        console.error('删除文件或目录失败:', error);
        message.error('删除失败');
      }
    },
  });
};

// 创建文件或目录
const createFileOrDirectory = async () => {
  if (!createFormState.name.trim()) {
    message.error(`请输入${createFormState.type === 'file' ? '文件' : '目录'}名称`);
    return;
  }

  try {
    const path = `${currentPath.value === '/' ? '' : currentPath.value}/${createFormState.name}`;

    if (createFormState.type === 'file') {
      await request.post(`/servers/${serverId.value}/files/create`, {
        path,
        content: createFormState.content
      });
      message.success('文件创建成功');
    } else {
      await request.post(`/servers/${serverId.value}/files/mkdir`, { path });
      message.success('目录创建成功');
    }

    createModalVisible.value = false;

    // 重置表单
    createFormState.name = '';
    createFormState.content = '';

    // 刷新文件列表
    fetchFileList(currentPath.value);
    // 刷新目录树中对应的节点
    await refreshTreeNode(currentPath.value);
  } catch (error) {
    console.error('创建失败:', error);
    message.error('创建失败');
  }
};

// 刷新目录树中的特定节点
const refreshTreeNode = async (path: string) => {
  if (path === '/') {
    // 如果是根目录，重新加载整个树
    await fetchDirectoryTree();
    return;
  }

  // 找到父目录路径
  const parentPath = path.substring(0, path.lastIndexOf('/')) || '/';

  // 标记父节点需要重新加载
  const markNodeForReload = (nodes: any[]): any[] => {
    return nodes.map(n => {
      if (n.key === parentPath) {
        return {
          ...n,
          hasLoadedChildren: false,
          children: undefined
        };
      } else if (n.children) {
        return {
          ...n,
          children: markNodeForReload(n.children)
        };
      }
      return n;
    });
  };

  treeData.value = markNodeForReload(treeData.value);

  // 如果父节点当前是展开状态，重新加载其子节点
  if (expandedKeys.value.includes(parentPath)) {
    const findNode = (nodes: any[], targetKey: string): any => {
      for (const node of nodes) {
        if (node.key === targetKey) {
          return node;
        }
        if (node.children) {
          const found = findNode(node.children, targetKey);
          if (found) return found;
        }
      }
      return null;
    };

    const parentNode = findNode(treeData.value, parentPath);
    if (parentNode) {
      await loadTreeNodeChildren(parentNode);
    }
  }
};

// 从目录树选择目录
const handleTreeSelect = (selectedKeys: string[]) => {
  if (selectedKeys.length > 0) {
    fetchFileList(selectedKeys[0]);
  }
};

// 处理树节点展开事件
const handleTreeExpand = async (expandedKeys: any, info: any) => {
  expandedKeys.value = expandedKeys;

  // 如果是展开操作，并且节点还没有加载子节点，则动态加载
  if (info.expanded && info.node) {
    await loadTreeNodeChildren(info.node);
  }
};

// 表格列定义
const columns = [
  {
    title: '名称',
    dataIndex: 'name',
    key: 'name',
    sorter: (a: any, b: any) => a.name.localeCompare(b.name)
  },
  {
    title: '大小',
    dataIndex: 'size',
    key: 'size',
    sorter: (a: any, b: any) => a.size - b.size,
    customRender: ({ text, record }: { text: number, record: any }) => {
      if (record.is_dir) {
        return '-';
      }
      return formatFileSize(text);
    }
  },
  {
    title: '修改时间',
    dataIndex: 'mod_time',
    key: 'mod_time',
    sorter: (a: any, b: any) => new Date(a.mod_time).getTime() - new Date(b.mod_time).getTime(),
    customRender: ({ text }: { text: string }) => {
      return new Date(text).toLocaleString();
    }
  },
  {
    title: '权限',
    dataIndex: 'mode',
    key: 'mode'
  },
  {
    title: '操作',
    key: 'action'
  }
];

// 格式化文件大小
const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B';

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = bytes;

  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }

  return `${size.toFixed(2)} ${units[i]}`;
};

// 面包屑导航
const breadcrumbItems = computed(() => {
  const parts = currentPath.value.split('/').filter(p => p);

  // 始终包含根目录
  const items = [{ title: '根目录', path: '/' }];

  // 添加路径中的每一部分
  let currentPathBuilder = '';
  for (const part of parts) {
    currentPathBuilder += `/${part}`;
    items.push({
      title: part,
      path: currentPathBuilder
    });
  }

  return items;
});

// 文件搜索过滤
const filteredFileList = computed(() => {
  if (!searchKeyword.value.trim()) {
    return fileList.value;
  }

  const keyword = searchKeyword.value.toLowerCase();
  return fileList.value.filter(file => {
    return file.name.toLowerCase().includes(keyword);
  });
});

// 表格行选择
const onSelectChange = (keys: string[], rows: any[]) => {
  selectedRowKeys.value = keys;
  selectedFiles.value = rows;
};

// 页面挂载时初始化
onMounted(async () => {
  console.log('文件管理页面挂载，服务器ID:', serverId.value);
  console.log('当前授权令牌:', getToken());

  // 先获取服务器信息
  await fetchServerInfo();

  // 验证服务器是否在线
  if (isServerOnline.value) {
    await fetchFileList(initialPath.value || '/');
    await fetchDirectoryTree();
  } else {
    console.warn('服务器离线，无法获取文件列表和目录树');
    message.warning('服务器离线，无法使用文件管理功能');
  }
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeTerminal);
  // 清理ResizeObserver
  if (terminal.value && (terminal.value as any)._resizeObserver) {
    (terminal.value as any)._resizeObserver.disconnect();
  }
});

watch(
  () => route.query.path,
  (newVal) => {
    const queryValue = Array.isArray(newVal) ? newVal[0] : (typeof newVal === 'string' ? newVal : '');
    const nextPath = normalizePath(queryValue);
    if (!nextPath || nextPath === currentPath.value || !isServerOnline.value) {
      return;
    }
    fetchFileList(nextPath);
  }
);

// 返回服务器详情页
const goBack = () => {
  const from = route.query.from;
  if (from === 'nginx') {
    router.push(`/admin/servers/${serverId.value}/nginx`);
    return;
  }
  router.push(`/admin/servers/${serverId.value}`);
};

// 打开终端弹窗
const openTerminal = async () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法打开终端');
    return;
  }

  try {
    // 使用固定的会话ID，避免创建多个临时会话
    const fixedSessionId = `file-manager-temp-${serverId.value}`;

    // 先尝试删除旧会话（如果存在）
    try {
      await request.delete(`/servers/${serverId.value}/terminal/sessions/${fixedSessionId}`);
      console.log('已删除旧的文件管理器终端会话');
    } catch (error) {
      // 如果会话不存在，删除会失败，这是正常的，忽略错误
    }

    // 创建新的临时终端会话
    const response = await request.post(`/servers/${serverId.value}/terminal/sessions`, {
      id: fixedSessionId,
      name: `文件管理器临时终端`,
      cwd: currentPath.value
    });

    if (response.data && response.data.id) {
      terminalSessionId.value = response.data.id;
      terminalWorkingDir.value = currentPath.value;
      terminalModalVisible.value = true;

      nextTick(() => {
        initTerminal();
      });
    } else {
      message.error('创建终端会话失败');
    }
  } catch (error: any) {
    console.error('创建终端会话失败:', error);
    message.error(error.response?.data?.error || '创建终端会话失败');
  }
};

// 初始化终端
const initTerminal = () => {
  if (!terminalRef.value) return;

  // 创建终端实例
  terminal.value = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Monaco, Menlo, "DejaVu Sans Mono", "Lucida Console", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#d4d4d4',
    },
    rows: 24,
    cols: 100
  });

  fitAddon.value = new FitAddon();
  terminal.value.loadAddon(fitAddon.value);
  terminal.value.open(terminalRef.value);

  // 延时执行适应大小，确保DOM已经渲染完成
  setTimeout(() => {
    resizeTerminal();
  }, 100);

  // 连接WebSocket
  connectTerminalWs();

  // 添加resize监听
  window.addEventListener('resize', resizeTerminal);
};

// 调整终端大小
const resizeTerminal = () => {
  if (!fitAddon.value || !terminal.value) return;

  try {
    fitAddon.value.fit();

    // 如果连接已建立，通知后端调整大小
    if (terminalWs && terminalWs.readyState === WebSocket.OPEN && terminalSessionId.value) {
      const resizeCommand = {
        type: 'shell_command',
        payload: {
          type: 'resize',
          data: JSON.stringify({
            cols: terminal.value.cols,
            rows: terminal.value.rows
          }),
          session: terminalSessionId.value
        }
      };
      terminalWs.send(JSON.stringify(resizeCommand));
    }
  } catch (e) {
    console.error('Resize terminal error:', e);
  }
};

// 连接终端WebSocket
const connectTerminalWs = () => {
  if (!terminal.value) return;

  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsHost = window.location.host;
  const token = getToken();

  // 使用已创建的会话ID
  if (!terminalSessionId.value) {
    message.error('终端会话ID不存在');
    return;
  }

  // WebSocket URL 需要包含 session 参数
  terminalWs = new WebSocket(
    `${wsProtocol}//${wsHost}/api/servers/${serverId.value}/ws?token=${token}&session=${terminalSessionId.value}`
  );

  terminalWs.onopen = () => {
    console.log('Terminal WebSocket connected');
    terminal.value?.write(`正在连接到文件管理器终端...\r\n`);

    // 先发送 create 命令在 Agent 端创建终端会话
    const createCommand = {
      type: 'shell_command',
      payload: {
        type: 'create',
        data: '',
        session: terminalSessionId.value
      }
    };
    terminalWs?.send(JSON.stringify(createCommand));

    // 发送 cd 命令切换到当前目录
    setTimeout(() => {
      // 先发送resize确保后端知道当前终端大小
      resizeTerminal();

      // 延迟发送 cd 命令，确保 resize 已被处理
      setTimeout(() => {
        const cdCommand = {
          type: 'shell_command',
          payload: {
            type: 'input',
            data: `cd ${terminalWorkingDir.value}\n`,
            session: terminalSessionId.value
          }
        };
        terminalWs?.send(JSON.stringify(cdCommand));
      }, 100);
    }, 200);
  };

  terminalWs.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);

      if (data.type === 'shell_response') {
        // 接收终端输出
        let outputData;
        if (data.payload) {
          outputData = data.payload.data;
        } else {
          outputData = data.data;
        }
        terminal.value?.write(outputData);
      } else if (data.type === 'shell_error') {
        terminal.value?.write(`\r\n\x1b[31m错误: ${data.error}\x1b[0m\r\n`);
      } else if (data.type === 'welcome') {
        terminal.value?.write(`连接成功！\r\n`);
      }
    } catch (e) {
      // 如果不是JSON，直接写入终端
      terminal.value?.write(event.data);
    }
  };

  terminalWs.onerror = (error) => {
    console.error('Terminal WebSocket error:', error);
    message.error('终端连接失败');
  };

  terminalWs.onclose = () => {
    console.log('Terminal WebSocket closed');
  };

  // 监听终端输入
  terminalDataDisposable = terminal.value.onData((data: string) => {
    if (terminalWs?.readyState === WebSocket.OPEN) {
      terminalWs.send(JSON.stringify({
        type: 'shell_command',
        payload: {
          type: 'input',
          data: data,
          session: terminalSessionId.value
        }
      }));
    }
  });
};

// 关闭终端
const closeTerminal = async () => {
  // 清理终端
  terminalDataDisposable?.dispose();
  terminal.value?.dispose();
  terminal.value = null;
  fitAddon.value = null;

  // 关闭WebSocket
  if (terminalWs) {
    terminalWs.close();
    terminalWs = null;
  }

  // 删除临时会话
  if (terminalSessionId.value) {
    try {
      await request.delete(`/servers/${serverId.value}/terminal/sessions/${terminalSessionId.value}`);
      console.log('临时终端会话已删除');
    } catch (error) {
      console.error('删除终端会话失败:', error);
    }
    terminalSessionId.value = '';
  }

  terminalModalVisible.value = false;
};

// 组件卸载时清理
onBeforeUnmount(() => {
  closeTerminal();
});

// 获取对应语言的扩展
const getLanguageExtension = (lang: string) => {
  switch (lang) {
    case 'javascript':
      return javascript();
    case 'html':
      return html();
    case 'css':
      return css();
    case 'json':
      return json();
    case 'markdown':
      return markdown();
    case 'python':
      return python();
    case 'php':
      return php();
    case 'xml':
      return xml();
    case 'rust':
      return rust();
    case 'sql':
      return sql();
    case 'java':
      return java();
    case 'cpp':
      return cpp();
    case 'shell':
      return StreamLanguage.define(shell);
    case 'nginx':
      return StreamLanguage.define(nginx);
    default:
      return javascript(); // 默认使用JavaScript
  }
};

// 新增的文件夹双击打开功能
const handleFolderClick = (record: any) => {
  if (record.is_dir) {
    const newPath = `${currentPath.value === '/' ? '' : currentPath.value}/${record.name}`;
    fetchFileList(newPath);
  }
};

// 路径跳转功能
const handlePathJump = () => {
  if (!jumpToPath.value.trim()) {
    message.warning('请输入有效的路径');
    return;
  }

  // 格式化路径，确保以 / 开头
  let formattedPath = jumpToPath.value.trim();

  // 如果不是以 / 开头，则视为相对路径，加上当前路径
  if (!formattedPath.startsWith('/')) {
    formattedPath = `${currentPath.value === '/' ? '' : currentPath.value}/${formattedPath}`;
  }

  // 格式化路径，去除多余的 /
  formattedPath = formattedPath.replace(/\/+/g, '/');

  // 如果路径为空，默认跳转到根目录
  if (!formattedPath) {
    formattedPath = '/';
  }

  console.log('跳转到路径:', formattedPath);

  // 尝试跳转到指定路径
  fetchFileList(formattedPath);

  // 关闭模态框并重置路径
  pathJumpModalVisible.value = false;
  jumpToPath.value = '';
};

// 重置路径跳转状态
const resetPathJump = () => {
  pathJumpModalVisible.value = false;
  jumpToPath.value = '';
};

const customRow = (record: any) => {
  return {
    onClick: () => {
      // 单击行选中逻辑可以放在这里，如果需要的话
      selectedRowKeys.value = [record.key];
      selectedFiles.value = [record];
    },
    onDblclick: () => {
      handleRowDoubleClick(record);
    }
  };
};
</script>

<template>
  <div class="macos-container">
    <div class="macos-window">
      <!-- 顶部工具栏 -->
      <div class="window-toolbar">
        <div class="toolbar-left">
          <div class="window-controls">
            <div class="control-dot red"></div>
            <div class="control-dot yellow"></div>
            <div class="control-dot green"></div>
          </div>
          <div class="nav-controls">
            <a-button type="text" class="nav-btn" @click="navigateToParent" :disabled="currentPath === '/'">
              <ArrowLeftOutlined />
            </a-button>
            <span class="window-title">{{ serverInfo.name || '文件管理' }}</span>
          </div>
        </div>

        <div class="toolbar-center">
          <div class="path-breadcrumb">
            <a-breadcrumb separator=">">
              <a-breadcrumb-item v-for="(item, index) in breadcrumbItems" :key="index">
                <a v-if="item.path !== currentPath" @click="fetchFileList(item.path)">{{ item.title }}</a>
                <span v-else>{{ item.title }}</span>
              </a-breadcrumb-item>
            </a-breadcrumb>
          </div>
        </div>

        <div class="toolbar-right">
          <a-input-search v-model:value="searchKeyword" placeholder="搜索" class="search-input"
            @search="(value: string) => searchKeyword = value"
            @change="(e: Event) => searchKeyword = (e.target as HTMLInputElement).value">
            <template #prefix>
              <SearchOutlined />
            </template>
          </a-input-search>

          <a-dropdown trigger="click">
            <a-button class="action-btn">
              <PlusOutlined />
            </a-button>
            <template #overlay>
              <a-menu>
                <a-menu-item key="upload" @click="uploadModalVisible = true">
                  <UploadOutlined /> 上传文件
                </a-menu-item>
                <a-menu-item key="new-file" @click="createModalVisible = true; createFormState.type = 'file'">
                  <FileOutlined /> 新建文件
                </a-menu-item>
                <a-menu-item key="new-folder" @click="createModalVisible = true; createFormState.type = 'directory'">
                  <FolderOutlined /> 新建文件夹
                </a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>

          <a-button class="action-btn" @click="fetchFileList(currentPath)">
            <ReloadOutlined />
          </a-button>

          <a-button class="action-btn" @click="openTerminal" title="在当前目录打开终端">
            <CodeOutlined />
          </a-button>

          <a-button class="action-btn" @click="goBack">
            退出
          </a-button>
        </div>
      </div>

      <!-- 主内容区域 -->
      <div class="window-content">
        <a-spin :spinning="loading" wrapperClassName="full-height-spin">
          <div class="split-view">
            <!-- 左侧边栏 -->
            <div class="sidebar">
              <div class="sidebar-section">
                <div class="section-title">位置</div>
                <div class="sidebar-tree">
                  <a-tree v-model:expandedKeys="expandedKeys" :tree-data="treeData" @select="handleTreeSelect"
                    @expand="handleTreeExpand" :default-expanded-keys="['/']" :load-data="loadTreeNodeChildren"
                    block-node>
                    <template #icon="{ isLeaf }">
                      <FolderOutlined v-if="!isLeaf" style="color: #1890ff" />
                      <FolderOutlined v-else style="color: #1890ff" />
                    </template>
                  </a-tree>
                </div>
              </div>
            </div>

            <!-- 右侧文件列表 -->
            <div class="main-view">
              <div class="file-list-container">
                <a-table :dataSource="filteredFileList" :columns="columns" :pagination="false" :row-selection="{
                  selectedRowKeys,
                  onChange: onSelectChange
                }" :row-key="(record: any) => record.key" @row-dblclick="handleRowDoubleClick"
                  :scroll="{ y: 'calc(100vh - 180px)' }" size="small" :customRow="customRow">

                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'name'">
                      <div class="name-cell">
                        <component :is="getFileIcon(record)"
                          :style="{ fontSize: '16px', marginRight: '8px', color: record.is_dir ? '#1890ff' : '#8c8c8c' }" />
                        <span class="name-text">{{ record.name }}</span>
                      </div>
                    </template>

                    <template v-else-if="column.key === 'action'">
                      <div class="action-cell">
                        <a-tooltip title="下载" v-if="!record.is_dir">
                          <a-button type="text" size="small" @click.stop="downloadFile(record)">
                            <DownloadOutlined />
                          </a-button>
                        </a-tooltip>
                        <a-tooltip title="编辑" v-if="!record.is_dir && isTextFile(record)">
                          <a-button type="text" size="small" @click.stop="openFileEditor(record)">
                            <EditOutlined />
                          </a-button>
                        </a-tooltip>
                        <a-tooltip title="删除">
                          <a-button type="text" danger size="small"
                            @click.stop="selectedFiles = [record]; deleteFiles()">
                            <DeleteOutlined />
                          </a-button>
                        </a-tooltip>
                      </div>
                    </template>
                  </template>
                </a-table>
              </div>

              <!-- 底部状态栏 -->
              <div class="status-bar">
                <span>{{ filteredFileList.length }} 项</span>
                <span v-if="selectedFiles.length > 0">已选择 {{ selectedFiles.length }} 项</span>
              </div>
            </div>
          </div>
        </a-spin>
      </div>
    </div>

    <!-- 上传文件对话框 -->
    <a-modal v-model:open="uploadModalVisible" title="上传文件" @ok="handleFileUpload" :confirm-loading="uploading"
      :maskClosable="false" :width="520" class="macos-modal">
      <div class="upload-container">
        <p>当前目录: <span class="current-path">{{ currentPath }}</span></p>
        <a-upload-dragger :beforeUpload="() => false" @change="handleFileChange"
          :fileList="fileToUpload ? [{ uid: '1', name: fileToUpload.name }] : []" :multiple="false"
          class="upload-dragger">
          <p class="ant-upload-drag-icon">
            <UploadOutlined />
          </p>
          <p class="ant-upload-text">点击或拖拽文件到此区域上传</p>
        </a-upload-dragger>
      </div>
    </a-modal>

    <!-- 编辑文件对话框 -->
    <a-modal v-model:open="editModalVisible" width="80%" @ok="saveFileContent" :confirm-loading="editLoading"
      :maskClosable="false" :footer="null" :destroyOnClose="true" style="top: 20px;" class="macos-modal editor-modal"
      :title="null">
      <div class="file-editor">
        <div class="editor-header">
          <div class="file-info">
            <span class="file-name" :title="editingFile?.name">{{ editingFile?.name }}</span>
            <span class="file-lang">{{ fileLanguage }}</span>
          </div>
          <div class="editor-actions">
            <a-button type="primary" size="small" @click="saveFileContent" :loading="editLoading">保存</a-button>
            <a-button size="small" @click="closeEditor" style="margin-left: 8px;">关闭</a-button>
          </div>
        </div>
        <div v-if="editModalVisible && !editLoading" style="height: 70vh;">
          <Codemirror v-model="fileContent" :style="{ height: '100%' }"
            :extensions="[basicSetup, getLanguageExtension(fileLanguage), vscodeDark]" :autofocus="true"
            :indent-with-tab="true" :tab-size="2" placeholder="文件内容" />
        </div>
        <div v-else style="height: 70vh; display: flex; align-items: center; justify-content: center;">
          <a-spin size="large" />
        </div>
      </div>
    </a-modal>

    <!-- 新建文件/目录对话框 -->
    <a-modal v-model:open="createModalVisible" title="新建" @ok="createFileOrDirectory" :maskClosable="false"
      class="macos-modal">
      <a-form :model="createFormState" layout="vertical">
        <a-form-item label="名称" required>
          <a-input v-model:value="createFormState.name" placeholder="请输入名称" />
        </a-form-item>
        <a-form-item v-if="createFormState.type === 'file'" label="内容">
          <a-textarea v-model:value="createFormState.content" :rows="5" placeholder="可选" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 终端对话框 -->
    <a-modal v-model:open="terminalModalVisible" :title="`终端 - ${terminalWorkingDir}`" @cancel="closeTerminal"
      :footer="null" :width="900" :maskClosable="false" class="macos-modal terminal-modal">
      <div class="terminal-container">
        <div ref="terminalRef" class="terminal-wrapper"></div>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.macos-container {
  height: calc(100vh - 64px);
  /* 减去顶部导航栏高度 */
  padding: 16px;
  background: transparent;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.macos-window {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: 12px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.15);
  border: 1px solid rgba(255, 255, 255, 0.4);
  overflow: hidden;
}



/* Toolbar */
.window-toolbar {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(255, 255, 255, 0.5);
  -webkit-app-region: drag;
  /* 模拟可拖动区域 */
}



.toolbar-left {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 200px;
}

.window-controls {
  display: flex;
  gap: 8px;
}

.control-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.control-dot.red {
  background: #ff5f57;
  border: 1px solid #e0443e;
}

.control-dot.yellow {
  background: #febc2e;
  border: 1px solid #dba522;
}

.control-dot.green {
  background: #28c840;
  border: 1px solid #1aab29;
}

.nav-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.nav-btn {
  color: var(--text-secondary);
}

.window-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--text-primary);
}

.toolbar-center {
  flex: 1;
  display: flex;
  justify-content: center;
}

.path-breadcrumb {
  background: rgba(0, 0, 0, 0.05);
  padding: 4px 12px;
  border-radius: 6px;
  max-width: 400px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}



.toolbar-right {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 300px;
  justify-content: flex-end;
}

.search-input {
  width: 160px;
  border-radius: 6px;
}

.action-btn {
  border-radius: 6px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-secondary);
}

.action-btn:hover {
  background: rgba(0, 0, 0, 0.05);
  color: var(--text-primary);
}



/* Content */
.window-content {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.full-height-spin {
  height: 100%;
  width: 100%;
}

:deep(.ant-spin-container) {
  height: 100%;
}

.split-view {
  display: flex;
  height: 100%;
}

/* Sidebar */
.sidebar {
  width: 220px;
  background: rgba(245, 245, 245, 0.6);
  backdrop-filter: blur(10px);
  border-right: 1px solid rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
}



.sidebar-section {
  padding: 12px;
  flex: 1;
  overflow-y: auto;
}

.section-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 8px;
  padding-left: 8px;
  text-transform: uppercase;
}

.sidebar-tree :deep(.ant-tree) {
  background: transparent;
}

.sidebar-tree :deep(.ant-tree-node-content-wrapper) {
  border-radius: 6px;
  padding: 4px 0;
}

.sidebar-tree :deep(.ant-tree-node-selected .ant-tree-node-content-wrapper) {
  background: rgba(0, 122, 255, 0.15) !important;
  color: var(--primary-color);
}

/* Main View */
.main-view {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: rgba(255, 255, 255, 0.4);
}



.file-list-container {
  flex: 1;
  overflow: hidden;
  /* 关键：防止外层滚动 */
  padding: 0;
}

/* Table Styling */
:deep(.ant-table-thead > tr > th) {
  background: transparent;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  font-size: 12px;
  color: var(--text-secondary);
  padding: 8px 16px;
}



:deep(.ant-table-tbody > tr > td) {
  padding: 8px 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03);
}




:deep(.ant-upload-hint) {
  color: var(--text-secondary);
  font-size: 14px;
}

/* Zebra Striping */
:deep(.ant-table-tbody > tr:nth-child(even)) {
  background-color: rgba(0, 0, 0, 0.01);
}



.name-cell {
  display: flex;
  align-items: center;
}

.name-text {
  font-weight: 500;
  color: var(--text-primary);
}

.action-cell {
  opacity: 0;
  transition: opacity 0.2s;
}

:deep(.ant-table-row:hover) .action-cell {
  opacity: 1;
}

/* Status Bar */
.status-bar {
  height: 28px;
  border-top: 1px solid rgba(0, 0, 0, 0.06);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  font-size: 11px;
  color: var(--text-secondary);
  background: rgba(255, 255, 255, 0.3);
}



.file-info {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
  /* Ensure text truncation works */
  margin-right: 16px;
}

.file-name {
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 14px;
}

.file-size {
  color: var(--text-secondary);
  font-size: 12px;
  font-family: "SF Mono", Menlo, monospace;
}

/* 编辑器样式 */
.file-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding-top: 8px;
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  padding: 0 4px 12px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  flex-shrink: 0;
}

.file-lang {
  font-size: 10px;
  color: #fff;
  padding: 2px 8px;
  background: var(--primary-color);
  border-radius: 10px;
  font-family: "SF Mono", Menlo, monospace;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  flex-shrink: 0;
}

.editor-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

/* Modal Styling */
:global(.macos-modal .ant-modal-content) {
  border-radius: 12px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(20px);
}



:global(.macos-modal .ant-modal-header) {
  background: transparent;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
}
</style>

<style>
/* Dark Mode Styles - Non-scoped to work with :root.dark */
:root.dark .file-manager-container {
  background-color: #1e1e1e;
}

:root.dark .file-manager-header {
  background-color: #252526;
  border-bottom: 1px solid #333;
}

:root.dark .file-tree-sidebar {
  background-color: #252526;
  border-right: 1px solid #333;
}

:root.dark .sidebar-header {
  border-bottom: 1px solid #333;
  color: #ccc;
}

:root.dark .file-list-content {
  background-color: #1e1e1e;
}

/* Dark Mode Table Overrides */
:root.dark .ant-table {
  background: transparent;
  color: #ccc;
}

:root.dark .ant-table-thead>tr>th {
  background: #2d2d2d;
  color: #ccc;
  border-bottom: 1px solid #333;
}

:root.dark .ant-table-tbody>tr>td {
  border-bottom: 1px solid #333;
  color: #ccc;
}

:root.dark .ant-table-tbody>tr:hover>td {
  background: #2a2d2e !important;
}

:root.dark .ant-table-row-selected>td {
  background: #37373d !important;
}

/* Tree Dark Mode */
:root.dark .ant-tree {
  background: transparent;
  color: #ccc;
}

:root.dark .ant-tree .ant-tree-node-content-wrapper:hover {
  background-color: #2a2d2e;
}

:root.dark .ant-tree .ant-tree-node-selected .ant-tree-node-content-wrapper {
  background-color: #37373d;
}

/* Input Dark Mode */
:root.dark .ant-input-search .ant-input {
  background-color: #3c3c3c;
  border-color: #3c3c3c;
  color: #ccc;
}

:root.dark .ant-input-search .ant-input:focus {
  border-color: #007acc;
}

:root.dark .ant-input-search .ant-btn {
  border-color: #3c3c3c;
}

/* Editor Dark Mode */
:root.dark .editor-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

:root.dark .file-name {
  color: #ccc;
}

:root.dark .file-lang {
  color: #61afef;
  background: rgba(97, 175, 239, 0.15);
}

:root.dark .file-info {
  color: #ccc;
}

:root.dark .file-explorer {
  border: 1px solid rgba(255, 255, 255, 0.1);
}

:root.dark .file-sidebar {
  background: rgba(33, 37, 43, 0.9);
  border-right: 1px solid rgba(255, 255, 255, 0.1);
}

:root.dark .sidebar-header h4 {
  color: #ccc;
}

:root.dark .file-main {
  background: rgba(40, 44, 52, 0.5);
}

:root.dark .file-toolbar {
  background: rgba(33, 37, 43, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

:root.dark .current-path {
  color: #61afef;
}

:root.dark .selected-file {
  background: rgba(97, 175, 239, 0.1);
  border: 1px solid rgba(97, 175, 239, 0.2);
}

:root.dark .path-jump-tip {
  background: rgba(97, 175, 239, 0.1);
  border-left: 3px solid #61afef;
  color: #abb2bf;
}

:root.dark .folder-name:hover {
  color: #61afef;
}

:root.dark .text-danger {
  color: #e06c75;
}

:root.dark .text-danger:hover {
  color: #be5046;
}

/* Migrated Dark Mode Styles */
.dark .macos-window {
  background: rgba(30, 30, 30, 0.85);
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.4);
}

.dark .window-toolbar {
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.05);
}

.dark .path-breadcrumb {
  background: rgba(255, 255, 255, 0.1);
}

.dark .action-btn:hover {
  background: rgba(255, 255, 255, 0.1);
}

.dark .sidebar {
  background: rgba(0, 0, 0, 0.2);
  border-right: 1px solid rgba(255, 255, 255, 0.08);
}

.dark .main-view {
  background: transparent;
}

.dark .ant-table-thead>tr>th {
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.dark .ant-table-tbody>tr>td {
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
}

.dark .ant-table-tbody>tr:nth-child(even) {
  background-color: rgba(255, 255, 255, 0.02);
}

.dark .status-bar {
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.05);
}

.dark .macos-modal .ant-modal-content {
  background: rgba(40, 40, 40, 0.9);
}

.dark .macos-modal .ant-modal-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.dark .file-lang {
  background: #61afef;
  color: #1e1e1e;
}

/* Hide default modal header for editor since we have a custom one */
.editor-modal .ant-modal-header {
  display: none;
}

.editor-modal .ant-modal-body {
  padding: 20px 24px;
}

/* Terminal Modal Styles */
.terminal-container {
  height: 500px;
  background-color: #1e1e1e;
  border-radius: 4px;
  overflow: hidden;
}

.terminal-wrapper {
  height: 100%;
  width: 100%;
  padding: 8px;
}

:global(.terminal-modal .ant-modal-body) {
  padding: 0;
}

:global(.terminal-modal .ant-modal-content) {
  background: #1e1e1e;
}

:global(.terminal-modal .ant-modal-header) {
  background: #2d2d2d;
  border-bottom: 1px solid #333;
}

:global(.terminal-modal .ant-modal-title) {
  color: #ccc;
  font-family: 'SF Mono', Menlo, monospace;
  font-size: 13px;
}
</style>
