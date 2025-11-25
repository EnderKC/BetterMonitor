<script setup lang="ts">
import { ref, reactive, onMounted, computed, defineComponent, nextTick, watch } from 'vue';
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
  EnterOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
import { getToken } from '../../utils/auth';
// 导入服务器状态store
import { useServerStore } from '../../stores/serverStore';
// 导入CodeMirror相关组件
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
  router.push(`/admin/servers/${serverId.value}`);
};

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
</script>

<template>
  <div class="file-container">
    <a-page-header title="文件管理" :sub-title="serverInfo.name" @back="goBack">
      <template #tags>
        <a-tag :color="isServerOnline ? 'success' : 'error'">
          {{ isServerOnline ? '在线' : '离线' }}
        </a-tag>
      </template>
    </a-page-header>

    <div class="file-content">
      <a-spin :spinning="loading">
        <a-alert v-if="!isServerOnline" type="warning" show-icon message="服务器当前离线，无法使用文件管理功能"
          style="margin-bottom: 24px" />

        <div v-else>
          <div class="file-explorer">
            <!-- 左侧目录树 -->
            <div class="file-sidebar">
              <div class="sidebar-header">
                <h4>目录结构</h4>
              </div>
              <div class="sidebar-content">
                <a-tree v-model:expandedKeys="expandedKeys" :tree-data="treeData" @select="handleTreeSelect"
                  @expand="handleTreeExpand" :default-expanded-keys="['/']" :load-data="loadTreeNodeChildren">
                  <template #icon="{ isLeaf }">
                    <FolderOutlined v-if="!isLeaf" />
                    <FileOutlined v-else />
                  </template>
                </a-tree>
              </div>
            </div>

            <!-- 右侧文件列表 -->
            <div class="file-main">
              <div class="file-toolbar">
                <div class="file-breadcrumb">
                  <a-breadcrumb>
                    <a-breadcrumb-item v-for="(item, index) in breadcrumbItems" :key="index">
                      <a v-if="item.path !== currentPath" @click="fetchFileList(item.path)">{{ item.title }}</a>
                      <span v-else>{{ item.title }}</span>
                    </a-breadcrumb-item>
                  </a-breadcrumb>
                </div>

                <div class="file-search">
                  <a-input-search v-model:value="searchKeyword" placeholder="搜索文件..." style="width: 200px;"
                    @search="(value: string) => searchKeyword = value"
                    @change="(e: Event) => searchKeyword = (e.target as HTMLInputElement).value">
                    <template #prefix>
                      <SearchOutlined />
                    </template>
                  </a-input-search>
                </div>

                <div class="file-actions">
                  <a-space>
                    <a-button @click="pathJumpModalVisible = true">
                      <template #icon>
                        <EnterOutlined />
                      </template>
                      跳转
                    </a-button>
                    <a-button type="primary" @click="uploadModalVisible = true">
                      <template #icon>
                        <UploadOutlined />
                      </template>
                      上传
                    </a-button>
                    <a-button @click="createModalVisible = true">
                      <template #icon>
                        <PlusOutlined />
                      </template>
                      新建
                    </a-button>
                    <a-button danger :disabled="selectedFiles.length === 0" @click="deleteFiles">
                      <template #icon>
                        <DeleteOutlined />
                      </template>
                      删除
                    </a-button>
                    <a-button @click="fetchFileList(currentPath)">
                      <template #icon>
                        <ReloadOutlined />
                      </template>
                      刷新
                    </a-button>
                    <a-button @click="navigateToParent" :disabled="currentPath === '/'">
                      <template #icon>
                        <ArrowLeftOutlined />
                      </template>
                      上级目录
                    </a-button>
                  </a-space>
                </div>
              </div>

              <div class="file-list">
                <a-table :dataSource="filteredFileList" :columns="columns" :pagination="false" :row-selection="{
                  selectedRowKeys,
                  onChange: onSelectChange
                }" :row-key="(record: any) => record.key" @row-dblclick="handleRowDoubleClick">
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'name'">
                      <span :class="{ 'folder-name': record.is_dir }"
                        @click="record.is_dir ? handleFolderClick(record) : null"
                        :title="record.is_dir ? '点击或双击打开文件夹' : ''">
                        <component :is="getFileIcon(record)"
                          :style="{ marginRight: '8px', color: record.is_dir ? '#1890ff' : '#666' }" />
                        {{ record.name }}
                      </span>
                    </template>

                    <template v-else-if="column.key === 'action'">
                      <a-space>
                        <a v-if="record.is_dir" @click="handleFolderClick(record)" title="打开文件夹">
                          <FolderOutlined style="color: #1890ff;" />
                        </a>
                        <a v-if="!record.is_dir" @click="downloadFile(record)" title="下载">
                          <DownloadOutlined />
                        </a>
                        <a v-if="!record.is_dir && isTextFile(record)" @click="openFileEditor(record)" title="编辑">
                          <EditOutlined />
                        </a>
                        <a class="text-danger" @click="selectedFiles = [record]; deleteFiles()" title="删除">
                          <DeleteOutlined />
                        </a>
                      </a-space>
                    </template>
                  </template>
                </a-table>
              </div>
            </div>
          </div>
        </div>
      </a-spin>
    </div>

    <!-- 上传文件对话框 -->
    <a-modal v-model:open="uploadModalVisible" title="上传文件" @ok="handleFileUpload" :confirm-loading="uploading"
      :maskClosable="false" :width="520">
      <div class="upload-container">
        <p>当前目录: <span class="current-path">{{ currentPath }}</span></p>
        <a-upload-dragger :beforeUpload="() => false" @change="handleFileChange"
          :fileList="fileToUpload ? [{ uid: '1', name: fileToUpload.name }] : []" :multiple="false"
          class="upload-dragger">
          <p class="ant-upload-drag-icon">
            <UploadOutlined />
          </p>
          <p class="ant-upload-text">点击或拖拽文件到此区域上传</p>
          <p class="ant-upload-hint">
            支持单个文件上传，上传后文件将保存到当前目录
          </p>
        </a-upload-dragger>

        <div class="selected-file" v-if="fileToUpload">
          <div class="file-info">
            <component :is="getFileIcon({ name: fileToUpload.name })" :style="{ marginRight: '8px', color: '#666' }" />
            <span class="file-name">{{ fileToUpload.name }}</span>
            <span class="file-size">{{ formatFileSize(fileToUpload.size) }}</span>
          </div>
        </div>
      </div>
    </a-modal>

    <!-- 编辑文件对话框 -->
    <a-modal v-model:open="editModalVisible" title="编辑文件" width="80%" @ok="saveFileContent"
      :confirm-loading="editLoading" :maskClosable="false" :footer="null" :destroyOnClose="true" style="top: 20px;">
      <div class="file-editor">
        <div class="editor-header">
          <div class="file-info">
            <span class="file-name">{{ editingFile?.name }}</span>
            <span class="file-lang">{{ fileLanguage }}</span>
          </div>
          <div class="editor-actions">
            <a-button type="primary" @click="saveFileContent" :loading="editLoading">
              保存
            </a-button>
            <a-button @click="closeEditor" style="margin-left: 8px;">
              关闭
            </a-button>
          </div>
        </div>

        <!-- 改进的编辑器结构 -->
        <div v-if="editModalVisible && !editLoading" style="height: 70vh;">
          <Codemirror v-model="fileContent" :style="{ height: '100%' }"
            :extensions="[basicSetup, getLanguageExtension(fileLanguage), vscodeDark]" :autofocus="true"
            :indent-with-tab="true" :tab-size="2" placeholder="文件内容" />
        </div>

        <!-- 加载中状态 -->
        <div v-else style="height: 70vh; display: flex; align-items: center; justify-content: center;">
          <a-spin size="large" />
        </div>
      </div>
    </a-modal>

    <!-- 新建文件/目录对话框 -->
    <a-modal v-model:open="createModalVisible" title="新建文件/目录" @ok="createFileOrDirectory" :maskClosable="false">
      <div>
        <p>当前目录: {{ currentPath }}</p>
        <a-form :model="createFormState" layout="vertical">
          <a-form-item label="类型">
            <a-radio-group v-model:value="createFormState.type">
              <a-radio value="file">文件</a-radio>
              <a-radio value="directory">目录</a-radio>
            </a-radio-group>
          </a-form-item>

          <a-form-item :label="createFormState.type === 'file' ? '文件名' : '目录名'" required>
            <a-input v-model:value="createFormState.name"
              :placeholder="createFormState.type === 'file' ? '输入文件名' : '输入目录名'" />
          </a-form-item>

          <a-form-item v-if="createFormState.type === 'file'" label="内容">
            <a-textarea v-model:value="createFormState.content" :rows="10" placeholder="输入文件内容" />
          </a-form-item>
        </a-form>
      </div>
    </a-modal>

    <!-- 路径跳转对话框 -->
    <a-modal v-model:open="pathJumpModalVisible" title="路径跳转" @ok="handlePathJump" @cancel="resetPathJump"
      :maskClosable="false">
      <div>
        <p>当前目录: {{ currentPath }}</p>
        <p class="path-jump-tip">输入要跳转的路径，以 / 开头表示绝对路径，否则表示相对于当前目录的路径</p>
        <a-form layout="vertical">
          <a-form-item label="目标路径" required>
            <a-input v-model:value="jumpToPath" placeholder="输入路径，如 /home/user 或 logs" @pressEnter="handlePathJump">
              <template #prefix>
                <span style="color: #999;">/</span>
              </template>
            </a-input>
          </a-form-item>
        </a-form>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.file-container {
  padding: 0;
  background: transparent;
}

.file-content {
  margin-top: 16px;
}

.file-explorer {
  display: flex;
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.4);
  border-radius: 16px;
  overflow: hidden;
  box-shadow: var(--shadow-md);
}

.file-sidebar {
  width: 260px;
  border-right: 1px solid rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.3);
  overflow: auto;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.5);
}

.sidebar-header h4 {
  margin-bottom: 0;
  font-weight: 600;
  color: var(--text-primary);
  font-size: 15px;
}

.sidebar-content {
  padding: 12px;
  max-height: 600px;
  overflow: auto;
}

.file-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.5);
}

.file-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(10px);
}

.file-breadcrumb {
  flex: 1;
}

.file-search {
  margin: 0 16px;
}

.path-jump-tip {
  color: var(--text-secondary);
  font-size: 12px;
  margin-bottom: 16px;
  padding: 8px 12px;
  background: rgba(0, 122, 255, 0.05);
  border-radius: 8px;
  border-left: 3px solid var(--primary-color);
}

/* 上传相关样式 */
.upload-container {
  padding: 0 20px;
}

.current-path {
  font-weight: 600;
  color: var(--primary-color);
  font-family: "SF Mono", Menlo, monospace;
}

.upload-dragger {
  margin: 20px 0;
}

.selected-file {
  margin-top: 16px;
  padding: 12px 16px;
  border-radius: 12px;
  background: rgba(0, 122, 255, 0.05);
  border: 1px solid rgba(0, 122, 255, 0.1);
}

.file-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-name {
  flex: 1;
  font-weight: 500;
  color: var(--text-primary);
}

.file-size {
  color: var(--text-secondary);
  font-size: 12px;
  font-family: "SF Mono", Menlo, monospace;
}

.file-list {
  flex: 1;
  overflow: auto;
  max-height: 600px;
  padding: 8px;
}
.file-manager-container {
  height: calc(100vh - 84px);
  display: flex;
  flex-direction: column;
  background-color: #f5f5f5;
}

.file-manager-header {
  padding: 12px 16px;
  background-color: #fff;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.path-navigator {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.file-manager-body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.file-tree-sidebar {
  width: 260px;
  background-color: #fff;
  border-right: 1px solid #e8e8e8;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.file-tree-sidebar {
  width: 260px;
  background-color: #fff;
  border-right: 1px solid #e8e8e8;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sidebar-header {
  padding: 12px;
  border-bottom: 1px solid #e8e8e8;
  font-weight: 600;
  color: #333;
}

.sidebar-content {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}

.file-list-content {
  flex: 1;
  background-color: #fff;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* Ant Design 覆盖样式 */
:deep(.ant-table-wrapper) {
  height: 100%;
}

:deep(.ant-spin-nested-loading) {
  height: 100%;
}

:deep(.ant-spin-container) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

:deep(.ant-table) {
  flex: 1;
  overflow: hidden;
}

:deep(.ant-table-container) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

:deep(.ant-table-body) {
  flex: 1;
  overflow-y: auto !important;
}

.file-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.file-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-name {
  font-weight: 600;
  font-size: 16px;
  color: var(--text-primary);
}

.file-lang {
  font-size: 11px;
  color: var(--primary-color);
  padding: 4px 8px;
  background: rgba(0, 122, 255, 0.1);
  border-radius: 6px;
  font-family: "SF Mono", Menlo, monospace;
  font-weight: 500;
}

.editor-actions {
  display: flex;
  gap: 8px;
}

.text-danger {
  color: #ff3b30;
}

.text-danger:hover {
  color: #d70015;
}

.folder-name {
  cursor: pointer;
  transition: all 0.2s ease;
}

.folder-name:hover {
  color: var(--primary-color);
  text-decoration: underline;
}

:deep(.cm-editor) {
  height: 100%;
  border-radius: 12px;
  overflow: hidden;
}

:deep(.cm-scroller) {
  overflow: auto;
  font-family: "SF Mono", Menlo, monospace;
}

:deep(.cm-content) {
  font-family: "SF Mono", Menlo, monospace;
}

/* 自定义上传组件样式 */
:deep(.ant-upload-drag) {
  border: 2px dashed rgba(0, 122, 255, 0.2);
  transition: all 0.3s ease;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.5);
}

:deep(.ant-upload-drag:hover) {
  border-color: var(--primary-color);
  background: rgba(0, 122, 255, 0.05);
}

:deep(.ant-upload-drag-icon) {
  margin-bottom: 16px;
  color: var(--primary-color);
  font-size: 48px;
}

:deep(.ant-upload-text) {
  margin: 0 0 8px;
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 500;
}

:deep(.ant-upload-hint) {
  color: var(--text-secondary);
  font-size: 14px;
}

/* 表格样式优化 */
:deep(.ant-table) {
  background: transparent;
}

:deep(.ant-table-thead > tr > th) {
  background: rgba(255, 255, 255, 0.6);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  font-weight: 600;
  color: var(--text-primary);
}

:deep(.ant-table-tbody > tr > td) {
  background: transparent;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03);
}

:deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(0, 122, 255, 0.05);
}

:deep(.ant-table-tbody > tr.ant-table-row-selected > td) {
  background: rgba(0, 122, 255, 0.1);
}

/* 面包屑样式 */
:deep(.ant-breadcrumb-link) {
  color: var(--text-secondary);
  transition: all 0.2s ease;
}

:deep(.ant-breadcrumb-link:hover) {
  color: var(--primary-color);
}

:deep(.ant-breadcrumb-separator) {
  color: var(--text-hint);
}

/* 树形组件样式 */
:deep(.ant-tree) {
  background: transparent;
}

:deep(.ant-tree-node-content-wrapper) {
  border-radius: 8px;
  transition: all 0.2s ease;
}

:deep(.ant-tree-node-content-wrapper:hover) {
  background: rgba(0, 122, 255, 0.08);
}

:deep(.ant-tree-node-selected .ant-tree-node-content-wrapper) {
  background: rgba(0, 122, 255, 0.15);
}

:deep(.ant-tree-title) {
  color: var(--text-primary);
  font-size: 13px;
}

/* 模态框样式 */
:deep(.ant-modal-content) {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.4);
  box-shadow: var(--shadow-lg);
}

:deep(.ant-modal-header) {
  background: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px 16px 0 0;
}

:deep(.ant-modal-title) {
  font-weight: 600;
  color: var(--text-primary);
}

:deep(.ant-modal-body) {
  background: transparent;
}

:deep(.ant-modal-footer) {
  background: rgba(255, 255, 255, 0.3);
  border-top: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 0 0 16px 16px;
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

:root.dark .ant-table-thead > tr > th {
  background: #2d2d2d;
  color: #ccc;
  border-bottom: 1px solid #333;
}

:root.dark .ant-table-tbody > tr > td {
  border-bottom: 1px solid #333;
  color: #ccc;
}

:root.dark .ant-table-tbody > tr:hover > td {
  background: #2a2d2e !important;
}

:root.dark .ant-table-row-selected > td {
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
</style>
