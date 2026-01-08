<script setup lang="ts">
import { ref, reactive, computed, onMounted, h } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal, Tabs } from 'ant-design-vue';
import {
  ReloadOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  DeleteOutlined,
  PlusOutlined,
  EyeOutlined,
  SearchOutlined,
  SyncOutlined,
  DownloadOutlined,
  FileTextOutlined,
  FolderOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  ClockCircleOutlined,
  QuestionCircleOutlined,
  InfoCircleOutlined,
  CodeOutlined,
  DownOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';
import { useServerStore } from '../../stores/serverStore';
import Convert from 'ansi-to-html';

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));

// 获取服务器状态store
const serverStore = useServerStore();

// 服务器详情
const serverInfo = ref<any>({});
const loading = ref(true);

// 标签页
const activeKey = ref('containers');

// 容器列表
const containers = ref<any[]>([]);
const containersLoading = ref(false);
const containerSearch = ref('');

// 镜像列表
const images = ref<any[]>([]);
const imagesLoading = ref(false);
const imageSearch = ref('');

// Compose列表
const composes = ref<any[]>([]);
const composesLoading = ref(false);

// 创建容器表单
const createContainerVisible = ref(false);
const containerForm = reactive({
  name: '',
  image: '',
  ports: [{ hostPort: '', containerPort: '' }],
  volumes: [{ hostPath: '', containerPath: '' }],
  envs: [{ key: '', value: '' }],
  command: '',
  restart: 'no',
  network: 'bridge'
});

// 拉取镜像表单
const pullImageVisible = ref(false);
const pullForm = ref('');
const pullLoading = ref(false);

// Compose表单
const composeFormVisible = ref(false);
const composeForm = reactive({
  name: '',
  content: ''
});

// 创建ANSI转HTML转换器实例
const ansiConverter = new Convert({
  newline: true,
  escapeXML: true,
  colors: {
    0: '#000',
    1: '#A00',
    2: '#0A0',
    3: '#A50',
    4: '#00A',
    5: '#A0A',
    6: '#0AA',
    7: '#AAA',
    8: '#555',
    9: '#F55',
    10: '#5F5',
    11: '#FF5',
    12: '#55F',
    13: '#F5F',
    14: '#5FF',
    15: '#FFF'
  }
});

// 计算服务器是否在线 (使用全局状态)
const isServerOnline = computed(() => {
  return serverStore.isServerOnline(serverId.value);
});

// 创建环境变量对象的类型
interface EnvVars {
  [key: string]: string;
}

// 解析Docker容器状态
const parseContainerStatus = (status: string): string => {
  if (!status) return 'unknown';

  const lowerStatus = status.toLowerCase();

  if (lowerStatus.startsWith('up')) return 'running';
  if (lowerStatus.startsWith('exited')) return 'exited';
  if (lowerStatus.startsWith('created')) return 'created';
  if (lowerStatus.startsWith('paused')) return 'paused';
  if (lowerStatus.startsWith('restarting')) return 'restarting';
  if (lowerStatus.startsWith('removing')) return 'removing';
  if (lowerStatus.startsWith('dead')) return 'dead';

  return 'unknown';
};

// 获取实际容器状态 (优先使用state字段，其次使用status解析结果)
const getContainerActualState = (container: any): string => {
  if (container.state && typeof container.state === 'string') {
    return container.state.toLowerCase();
  }
  return parseContainerStatus(container.status);
};

// 获取服务器详情
const fetchServerInfo = async () => {
  loading.value = true;
  try {
    // 使用any类型避免TypeScript错误
    const response: any = await request.get(`/servers/${serverId.value}`);

    // 从响应中提取服务器数据
    if (response && response.server) {
      serverInfo.value = response.server;

      // 更新全局状态
      const status = response.server.status || 'offline';
      const isOnline = response.server.online === true;

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

// 获取容器列表
const fetchContainers = async () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法获取容器列表');
    return;
  }

  containersLoading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/containers`);
    console.log('获取容器列表响应:', response);

    // 处理不同的返回格式
    if (Array.isArray(response)) {
      // 直接是数组
      containers.value = response;
    } else if (response && response.data && Array.isArray(response.data)) {
      // 嵌套在data字段中
      containers.value = response.data;
    } else if (response && response.containers) {
      if (Array.isArray(response.containers)) {
        // 嵌套在containers字段中且是数组
        containers.value = response.containers;
      } else if (response.containers === null) {
        // containers为null，表示没有容器
        containers.value = [];
        console.log('服务器上没有运行的容器');
      }
    } else {
      console.error('获取容器列表格式错误:', response);
      containers.value = [];
    }
  } catch (error) {
    console.error('获取容器列表失败:', error);
    message.error('获取容器列表失败');
    containers.value = [];
  } finally {
    containersLoading.value = false;
  }
};

const openContainerTerminal = (container: any) => {
  const id = container.id || container.ID;
  if (!id) {
    message.warning('无法识别容器ID');
    return;
  }
  const name = container.name || container.Names?.[0] || id;
  router.push({
    name: 'ServerDockerTerminal',
    params: { id: serverId.value, containerId: id },
    query: { name },
  });
};

const openContainerFile = (container: any) => {
  const id = container.id || container.ID;
  if (!id) {
    message.warning('无法识别容器ID');
    return;
  }
  const name = container.name || container.Names?.[0] || id;
  router.push({
    name: 'ServerDockerFile',
    params: { id: serverId.value, containerId: id },
    query: { name },
  });
};

// 获取镜像列表
const fetchImages = async () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法获取镜像列表');
    return;
  }

  imagesLoading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/images`);
    console.log('获取镜像列表响应:', response);

    // 处理不同的返回格式
    if (Array.isArray(response)) {
      // 直接是数组
      images.value = response;
    } else if (response && response.data && Array.isArray(response.data)) {
      // 嵌套在data字段中
      images.value = response.data;
    } else if (response && response.images) {
      if (Array.isArray(response.images)) {
        // 嵌套在images字段中且是数组
        images.value = response.images;
      } else if (response.images === null) {
        // images为null，表示没有镜像
        images.value = [];
        console.log('服务器上没有Docker镜像');
      }
    } else {
      console.error('获取镜像列表格式错误:', response);
      images.value = [];
    }
  } catch (error) {
    console.error('获取镜像列表失败:', error);
    message.error('获取镜像列表失败');
    images.value = [];
  } finally {
    imagesLoading.value = false;
  }
};

// 获取Compose列表
const fetchComposes = async () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法获取Compose列表');
    return;
  }

  composesLoading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/composes`);
    console.log('获取Compose列表响应:', response);

    // 处理不同的返回格式
    if (Array.isArray(response)) {
      // 直接是数组
      composes.value = response;
    } else if (response && response.data && Array.isArray(response.data)) {
      // 嵌套在data字段中
      composes.value = response.data;
    } else if (response && response.composes) {
      if (Array.isArray(response.composes)) {
        // 嵌套在composes字段中且是数组
        composes.value = response.composes;
      } else if (response.composes === null) {
        // composes为null，表示没有Compose项目
        composes.value = [];
        console.log('服务器上没有Compose项目');
      }
    } else {
      console.error('获取Compose列表格式错误:', response);
      composes.value = [];
    }
  } catch (error) {
    console.error('获取Compose列表失败:', error);
    message.error('获取Compose列表失败');
    composes.value = [];
  } finally {
    composesLoading.value = false;
  }
};

// 过滤容器列表
const filteredContainers = computed(() => {
  if (!containerSearch.value) return containers.value;

  return containers.value.filter(container =>
    container.name.toLowerCase().includes(containerSearch.value.toLowerCase()) ||
    container.image.toLowerCase().includes(containerSearch.value.toLowerCase()) ||
    container.id.includes(containerSearch.value) ||
    (container.state && container.state.toLowerCase().includes(containerSearch.value.toLowerCase())) ||
    (container.status && container.status.toLowerCase().includes(containerSearch.value.toLowerCase()))
  );
});

// 过滤镜像列表
const filteredImages = computed(() => {
  if (!imageSearch.value) return images.value;

  return images.value.filter(image =>
    (image.repository && image.repository.toLowerCase().includes(imageSearch.value.toLowerCase())) ||
    (image.tag && image.tag.toLowerCase().includes(imageSearch.value.toLowerCase())) ||
    (image.id && image.id.includes(imageSearch.value))
  );
});

// 启动容器
const startContainer = async (id: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  try {
    await request.post(`/servers/${serverId.value}/docker/containers/${id}/start`);
    message.success('容器已启动');
    fetchContainers();
  } catch (error) {
    console.error('启动容器失败:', error);
    message.error('启动容器失败');
  }
};

// 停止容器
const stopContainer = async (id: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  try {
    await request.post(`/servers/${serverId.value}/docker/containers/${id}/stop`);
    message.success('容器已停止');
    fetchContainers();
  } catch (error) {
    console.error('停止容器失败:', error);
    message.error('停止容器失败');
  }
};

// 重启容器
const restartContainer = async (id: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  try {
    await request.post(`/servers/${serverId.value}/docker/containers/${id}/restart`);
    message.success('容器已重启');
    fetchContainers();
  } catch (error) {
    console.error('重启容器失败:', error);
    message.error('重启容器失败');
  }
};

// 删除容器
const removeContainer = (id: string, name: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  // 查找容器信息
  const container = containers.value.find(c => c.id === id);
  const isRunning = container && (container.state === 'running' || container.status.toLowerCase().includes('up'));

  // 如果容器正在运行，显示特殊提示
  if (isRunning) {
    Modal.confirm({
      title: '容器正在运行',
      content: h('div', [
        h('p', `容器 ${name} 当前正在运行中。`),
        h('p', '您可以选择:'),
        h('ul', { style: { paddingLeft: '20px', marginTop: '10px' } }, [
          h('li', '强制删除(会立即停止并删除容器)'),
          h('li', '先停止容器再删除')
        ])
      ]),
      okText: '强制删除',
      cancelText: '先停止',
      okType: 'danger',
      onOk: async () => {
        // 强制删除
        try {
          await request.delete(`/servers/${serverId.value}/docker/containers/${id}?force=true`);
          message.success('容器已强制删除');
          fetchContainers();
        } catch (error: any) {
          console.error('强制删除容器失败:', error);
          const errorMsg = error.response?.data?.error || '强制删除容器失败';
          message.error(errorMsg);
        }
      },
      onCancel: async () => {
        // 先停止再删除
        try {
          await request.post(`/servers/${serverId.value}/docker/containers/${id}/stop`);
          message.info('容器已停止，正在删除...');
          await request.delete(`/servers/${serverId.value}/docker/containers/${id}`);
          message.success('容器已删除');
          fetchContainers();
        } catch (error: any) {
          console.error('停止或删除容器失败:', error);
          const errorMsg = error.response?.data?.error || '操作失败';
          message.error(errorMsg);
        }
      }
    });
  } else {
    // 容器未运行，正常删除
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除容器 ${name} 吗？此操作不可恢复。`,
      okText: '确认',
      cancelText: '取消',
      okType: 'danger',
      onOk: async () => {
        try {
          await request.delete(`/servers/${serverId.value}/docker/containers/${id}`);
          message.success('容器已删除');
          fetchContainers();
        } catch (error: any) {
          console.error('删除容器失败:', error);
          const errorMsg = error.response?.data?.error || '删除容器失败';
          message.error(errorMsg);
        }
      }
    });
  }
};

// 查看容器日志
const viewContainerLogs = async (id: string, name: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  try {
    await fetchContainerLogs(id, name, 100); // 默认获取100行日志
  } catch (error) {
    console.error('获取容器日志失败:', error);
    message.error('获取容器日志失败');
  }
};

// 添加日志获取与显示的相关变量
const currentLogContainerId = ref<string>('');
const currentLogContainerName = ref<string>('');
const containerLogModal = ref<any>(null);
const logsTail = ref<number>(100);

// 获取容器日志并显示
const fetchContainerLogs = async (id: string, name: string, tail: number = 100) => {
  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/containers/${id}/logs?tail=${tail}`);
    console.log('获取容器日志响应:', response);

    // 处理不同的返回格式
    let logs = '';
    if (typeof response === 'string') {
      logs = response;
    } else if (response && typeof response.data === 'string') {
      logs = response.data;
    } else if (response && response.logs && typeof response.logs === 'string') {
      logs = response.logs;
    } else {
      logs = '无日志数据或格式不正确';
      console.error('获取容器日志格式错误:', response);
    }

    // 将ANSI转义序列转换为HTML
    const htmlLogs = ansiConverter.toHtml(logs);

    // 保存当前查看的容器ID和名称，用于刷新
    currentLogContainerId.value = id;
    currentLogContainerName.value = name;

    // 创建日志内容元素
    const logContent = h('div', {
      style: {
        maxHeight: '500px',
        overflow: 'auto',
        background: '#000',
        padding: '16px',
        borderRadius: '4px',
        fontFamily: 'monospace',
        color: '#FFF'
      },
      innerHTML: htmlLogs
    });

    // 创建标题元素
    const modalTitle = h('div', {
      style: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      }
    }, [
      h('span', `容器日志 - ${name}`),
      h('div', {
        style: {
          display: 'flex',
          gap: '8px'
        }
      }, [
        h('a-input-number', {
          style: { width: '100px' },
          value: logsTail.value,
          min: 10,
          max: 10000,
          step: 100,
          addonAfter: '行',
          onChange: (value: number) => {
            logsTail.value = value;
          }
        }),
        h('a-button', {
          type: 'primary',
          size: 'small',
          onClick: () => refreshContainerLogs()
        }, [h(SyncOutlined), ' 刷新'])
      ])
    ]);

    // 判断是否已有模态框
    if (containerLogModal.value) {
      // 销毁旧的模态框
      containerLogModal.value.destroy();
    }

    // 创建新的模态框
    containerLogModal.value = Modal.info({
      title: modalTitle,
      width: 800,
      content: logContent,
      okText: '关闭',
      afterClose: () => {
        containerLogModal.value = null;
      }
    });
  } catch (error) {
    console.error('获取容器日志失败:', error);
    message.error('获取容器日志失败');
  }
};

// 刷新容器日志
const refreshContainerLogs = async () => {
  if (!currentLogContainerId.value || !containerLogModal.value) return;

  try {
    await fetchContainerLogs(currentLogContainerId.value, currentLogContainerName.value, logsTail.value);
  } catch (error) {
    console.error('刷新容器日志失败:', error);
    message.error('刷新容器日志失败');
  }
};

// 删除镜像
const removeImage = (id: string, name: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  Modal.confirm({
    title: '确认删除',
    content: `确定要删除镜像 ${name} 吗？此操作不可恢复。`,
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await request.delete(`/servers/${serverId.value}/docker/images/${id}`);
        message.success('镜像已删除');
        fetchImages();
      } catch (error) {
        console.error('删除镜像失败:', error);
        message.error('删除镜像失败');
      }
    }
  });
};

// 拉取镜像
const pullImage = async () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  if (!pullForm.value) {
    message.error('请输入要拉取的镜像名称');
    return;
  }

  pullLoading.value = true;

  try {
    await request.post(`/servers/${serverId.value}/docker/images/pull`, {
      image: pullForm.value
    });

    message.success('镜像拉取任务已提交，请稍后刷新查看结果');
    pullImageVisible.value = false;
    pullForm.value = '';

    // 延迟3秒后刷新镜像列表
    setTimeout(() => {
      fetchImages();
    }, 3000);
  } catch (error) {
    console.error('拉取镜像失败:', error);
    message.error('拉取镜像失败');
  } finally {
    pullLoading.value = false;
  }
};

// Compose操作
const composeUp = async (name: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  try {
    await request.post(`/servers/${serverId.value}/docker/composes/${name}/up`);
    message.success('Compose服务已启动');
    fetchComposes();
  } catch (error) {
    console.error('启动Compose服务失败:', error);
    message.error('启动Compose服务失败');
  }
};

const composeDown = async (name: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  try {
    await request.post(`/servers/${serverId.value}/docker/composes/${name}/down`);
    message.success('Compose服务已停止');
    fetchComposes();
  } catch (error) {
    console.error('停止Compose服务失败:', error);
    message.error('停止Compose服务失败');
  }
};

const removeCompose = (name: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  Modal.confirm({
    title: '确认删除',
    content: `确定要删除Compose项目 ${name} 吗？此操作不可恢复。`,
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await request.delete(`/servers/${serverId.value}/docker/composes/${name}`);
        message.success('Compose项目已删除');
        fetchComposes();
      } catch (error) {
        console.error('删除Compose项目失败:', error);
        message.error('删除Compose项目失败');
      }
    }
  });
};

// 查看Compose配置
const viewComposeConfig = async (name: string) => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  try {
    const response: any = await request.get(`/servers/${serverId.value}/docker/composes/${name}/config`);
    console.log('获取Compose配置响应:', response);

    // 处理不同的返回格式
    let config = '';
    if (typeof response === 'string') {
      config = response;
    } else if (response && typeof response.data === 'string') {
      config = response.data;
    } else if (response && response.config && typeof response.config === 'string') {
      config = response.config;
    } else {
      config = '无配置数据或格式不正确';
      console.error('获取Compose配置格式错误:', response);
    }

    Modal.info({
      title: `Compose配置 - ${name}`,
      width: 800,
      content: h('div', [
        h('pre', {
          style: {
            maxHeight: '500px',
            overflow: 'auto',
            background: '#1e1e1e',
            padding: '16px',
            borderRadius: '4px',
            fontFamily: 'monospace',
            color: '#d4d4d4',
            fontSize: '14px',
            lineHeight: '1.5'
          }
        }, config)
      ]),
      okText: '关闭'
    });
  } catch (error) {
    console.error('获取Compose配置失败:', error);
    message.error('获取Compose配置失败');
  }
};

// 创建Compose项目
const createCompose = async () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  if (!composeForm.name || !composeForm.content) {
    message.error('请填写Compose项目名称和配置内容');
    return;
  }

  try {
    await request.post(`/servers/${serverId.value}/docker/composes`, composeForm);
    message.success('Compose项目已创建');
    composeFormVisible.value = false;
    composeForm.name = '';
    composeForm.content = '';
    fetchComposes();
  } catch (error) {
    console.error('创建Compose项目失败:', error);
    message.error('创建Compose项目失败');
  }
};

// 添加端口映射
const addPortMapping = () => {
  containerForm.ports.push({ hostPort: '', containerPort: '' });
};

// 删除端口映射
const removePortMapping = (index: number) => {
  containerForm.ports.splice(index, 1);
};

// 添加卷映射
const addVolumeMapping = () => {
  containerForm.volumes.push({ hostPath: '', containerPath: '' });
};

// 删除卷映射
const removeVolumeMapping = (index: number) => {
  containerForm.volumes.splice(index, 1);
};

// 添加环境变量
const addEnvVar = () => {
  containerForm.envs.push({ key: '', value: '' });
};

// 删除环境变量
const removeEnvVar = (index: number) => {
  containerForm.envs.splice(index, 1);
};

// 重置容器表单
const resetContainerForm = () => {
  containerForm.name = '';
  containerForm.image = '';
  containerForm.ports = [{ hostPort: '', containerPort: '' }];
  containerForm.volumes = [{ hostPath: '', containerPath: '' }];
  containerForm.envs = [{ key: '', value: '' }];
  containerForm.command = '';
  containerForm.restart = 'no';
  containerForm.network = 'bridge';
};

// 创建容器
const createContainer = async () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法执行此操作');
    return;
  }

  if (!containerForm.name) {
    message.error('请输入容器名称');
    return;
  }

  if (!containerForm.image) {
    message.error('请选择或输入镜像名称');
    return;
  }

  try {
    // 过滤空的端口映射
    const ports = containerForm.ports.filter(port => port.hostPort && port.containerPort)
      .map(port => `${port.hostPort}:${port.containerPort}`);

    // 过滤空的卷映射
    const volumes = containerForm.volumes.filter(volume => volume.hostPath && volume.containerPath)
      .map(volume => `${volume.hostPath}:${volume.containerPath}`);

    // 过滤空的环境变量并构建对象
    const env: Record<string, string> = {};
    containerForm.envs.forEach(item => {
      if (item.key) {
        env[item.key] = item.value || '';
      }
    });

    const response = await request.post(`/servers/${serverId.value}/docker/containers`, {
      name: containerForm.name,
      image: containerForm.image,
      ports: ports,
      volumes: volumes,
      env: env,
      command: containerForm.command,
      restart: containerForm.restart,
      network: containerForm.network
    });

    console.log('创建容器响应:', response);

    message.success('容器创建成功');
    createContainerVisible.value = false;
    resetContainerForm();
    fetchContainers();
  } catch (error) {
    console.error('创建容器失败:', error);
    message.error('创建容器失败');
  }
};

// 页面挂载时获取服务器信息和Docker数据
onMounted(() => {
  fetchServerInfo().then(() => {
    if (isServerOnline.value) {
      fetchContainers();
      fetchImages();
      fetchComposes();
    } else {
      message.warning('服务器离线，无法使用Docker管理功能');
    }
  });
});

// 返回服务器详情页
const goBack = () => {
  router.push(`/admin/servers/${serverId.value}`);
};

// 刷新数据
const refreshData = () => {
  if (!isServerOnline.value) {
    message.warning('服务器离线，无法刷新数据');
    return;
  }

  if (activeKey.value === 'containers') {
    fetchContainers();
  } else if (activeKey.value === 'images') {
    fetchImages();
  } else if (activeKey.value === 'composes') {
    fetchComposes();
  }
};

// 格式化创建时间
const formatTime = (timestamp: string) => {
  return new Date(timestamp).toLocaleString();
};

// 转换容器状态到中文
const containerStatusText = (status: string) => {
  const parsedStatus = parseContainerStatus(status);

  switch (parsedStatus) {
    case 'running':
      return '运行中';
    case 'exited':
      return '已停止';
    case 'created':
      return '已创建';
    case 'paused':
      return '已暂停';
    case 'restarting':
      return '重启中';
    case 'removing':
      return '移除中';
    case 'dead':
      return '已死亡';
    default:
      return status; // 返回原始状态
  }
};

// 获取容器状态颜色
const containerStatusColor = (status: string) => {
  const parsedStatus = parseContainerStatus(status);

  switch (parsedStatus) {
    case 'running':
      return 'green';
    case 'exited':
      return 'red';
    case 'created':
      return 'blue';
    case 'paused':
      return 'orange';
    case 'restarting':
      return 'gold';
    case 'removing':
      return 'purple';
    case 'dead':
      return 'black';
    default:
      return 'default';
  }
};

// 检查容器是否可操作
const isContainerActionable = (status: string) => {
  const parsedStatus = parseContainerStatus(status);

  // 如果容器处于正在移除或死亡状态，则不允许操作
  const nonActionableStates = ['removing', 'dead'];
  return !nonActionableStates.includes(parsedStatus);
};

// 修复$event隐式any类型的问题
const onTabChange = (key: string) => {
  activeKey.value = key;

  // 切换到对应标签页时刷新数据
  if (key === 'containers') {
    fetchContainers();
  } else if (key === 'images') {
    fetchImages();
  } else if (key === 'composes') {
    fetchComposes();
  }
};

// 获取容器状态图标
const getContainerStatusIcon = (status: string) => {
  const parsedStatus = parseContainerStatus(status);

  switch (parsedStatus) {
    case 'running':
      return h(CheckCircleOutlined);
    case 'exited':
      return h(CloseCircleOutlined);
    case 'created':
      return h(ClockCircleOutlined);
    case 'paused':
      return h(PauseCircleOutlined);
    case 'restarting':
      return h(SyncOutlined);
    case 'removing':
      return h(DeleteOutlined);
    case 'dead':
      return h(ExclamationCircleOutlined);
    default:
      return h(QuestionCircleOutlined);
  }
};
</script>

<template>
  <div class="docker-container">
    <a-page-header title="Docker管理" :sub-title="serverInfo.name" @back="goBack">
      <template #tags>
        <a-tag :color="isServerOnline ? 'success' : 'error'">
          {{ isServerOnline ? '在线' : '离线' }}
        </a-tag>
      </template>

      <template #extra>
        <a-space>
          <a-button type="primary" @click="refreshData">
            <template #icon>
              <ReloadOutlined />
            </template>
            刷新
          </a-button>
        </a-space>
      </template>
    </a-page-header>

    <div class="docker-content">
      <a-spin :spinning="loading">
        <a-alert v-if="!isServerOnline" type="warning" show-icon message="服务器当前离线，无法使用Docker管理功能"
          style="margin-bottom: 24px" />

        <div v-else>
          <a-tabs v-model:activeKey="activeKey" @change="onTabChange">
            <!-- 容器管理 -->
            <a-tab-pane key="containers" tab="容器管理">
              <div class="tab-header">
                <div class="search-box">
                  <a-input-search v-model:value="containerSearch" placeholder="搜索容器名称、镜像或ID" style="width: 300px"
                    :loading="containersLoading" />
                </div>
                <div class="action-box">
                  <a-space>
                    <a-button type="primary" @click="createContainerVisible = true">
                      <template #icon>
                        <PlusOutlined />
                      </template>
                      创建容器
                    </a-button>
                    <a-button @click="fetchContainers" :loading="containersLoading">
                      <template #icon>
                        <ReloadOutlined />
                      </template>
                      刷新
                    </a-button>
                  </a-space>
                </div>
              </div>

              <a-table :dataSource="filteredContainers" :loading="containersLoading" :pagination="{ pageSize: 10 }"
                rowKey="id">
                <template #emptyText>
                  <div style="text-align: center; padding: 16px;">
                    <p>暂无容器数据</p>
                    <p v-if="isServerOnline">服务器上没有运行的Docker容器</p>
                    <p v-else>服务器离线，无法获取容器数据</p>
                  </div>
                </template>

                <a-table-column title="容器ID" dataIndex="id" width="120">
                  <template #default="{ text }">
                    {{ text.substring(0, 12) }}
                  </template>
                </a-table-column>
                <a-table-column title="名称" dataIndex="name" />
                <a-table-column title="镜像" dataIndex="image" />
                <a-table-column title="状态" dataIndex="status">
                  <template #default="{ text, record }">
                    <a-tag :color="containerStatusColor(text)">
                      <span style="display: flex; align-items: center; gap: 4px;">
                        <component :is="getContainerStatusIcon(text)" />
                        <span>{{ containerStatusText(text) }}</span>
                        <a-tooltip>
                          <template #title>原始状态: {{ text }}</template>
                          <InfoCircleOutlined style="margin-left: 4px; opacity: 0.6;" />
                        </a-tooltip>
                      </span>
                    </a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="端口" dataIndex="ports">
                  <template #default="{ text }">
                    <div v-if="text && text.length > 0">
                      <a-tag v-for="port in text" :key="port" color="blue">
                        {{ port }}
                      </a-tag>
                    </div>
                    <span v-else>-</span>
                  </template>
                </a-table-column>
                <a-table-column title="创建时间" dataIndex="created">
                  <template #default="{ text }">
                    {{ formatTime(text) }}
                  </template>
                </a-table-column>
                <a-table-column title="操作" width="120">
                  <template #default="{ record }">
                    <a-dropdown>
                      <template #overlay>
                        <a-menu>
                          <a-menu-item v-if="parseContainerStatus(record.status) !== 'running'" key="start"
                            :disabled="!isContainerActionable(record.status)" @click="startContainer(record.id)">
                            <PlayCircleOutlined />
                            启动容器
                          </a-menu-item>
                          <a-menu-item v-if="parseContainerStatus(record.status) === 'running'" key="stop"
                            :disabled="!isContainerActionable(record.status)" @click="stopContainer(record.id)">
                            <PauseCircleOutlined />
                            停止容器
                          </a-menu-item>
                          <a-menu-item v-if="parseContainerStatus(record.status) === 'running'" key="restart"
                            :disabled="!isContainerActionable(record.status)" @click="restartContainer(record.id)">
                            <SyncOutlined />
                            重启容器
                          </a-menu-item>
                          <a-menu-divider />
                          <a-menu-item key="terminal" @click="openContainerTerminal(record)">
                            <CodeOutlined />
                            容器终端
                          </a-menu-item>
                          <a-menu-item key="file" @click="openContainerFile(record)">
                            <FolderOutlined />
                            容器文件
                          </a-menu-item>
                          <a-menu-item key="logs" @click="viewContainerLogs(record.id, record.name)">
                            <EyeOutlined />
                            查看日志
                          </a-menu-item>
                          <a-menu-divider />
                          <a-menu-item key="delete" danger
                            :disabled="parseContainerStatus(record.status) === 'removing'"
                            @click="removeContainer(record.id, record.name)">
                            <DeleteOutlined />
                            删除容器
                          </a-menu-item>
                        </a-menu>
                      </template>
                      <a-button type="primary" size="small">
                        操作
                        <DownOutlined />
                      </a-button>
                    </a-dropdown>
                  </template>
                </a-table-column>
              </a-table>
            </a-tab-pane>

            <!-- 镜像管理 -->
            <a-tab-pane key="images" tab="镜像管理">
              <div class="tab-header">
                <div class="search-box">
                  <a-input-search v-model:value="imageSearch" placeholder="搜索镜像名称、标签或ID" style="width: 300px"
                    :loading="imagesLoading" />
                </div>
                <div class="action-box">
                  <a-space>
                    <a-button type="primary" @click="pullImageVisible = true">
                      <template #icon>
                        <DownloadOutlined />
                      </template>
                      拉取镜像
                    </a-button>
                    <a-button @click="fetchImages" :loading="imagesLoading">
                      <template #icon>
                        <ReloadOutlined />
                      </template>
                      刷新
                    </a-button>
                  </a-space>
                </div>
              </div>

              <a-table :dataSource="filteredImages" :loading="imagesLoading" :pagination="{ pageSize: 10 }" rowKey="id">
                <template #emptyText>
                  <div style="text-align: center; padding: 16px;">
                    <p>暂无镜像数据</p>
                    <p v-if="isServerOnline">服务器上没有Docker镜像</p>
                    <p v-else>服务器离线，无法获取镜像数据</p>
                  </div>
                </template>

                <a-table-column title="镜像ID" dataIndex="id" width="120">
                  <template #default="{ text }">
                    {{ text.substring(0, 12) }}
                  </template>
                </a-table-column>
                <a-table-column title="仓库" dataIndex="repository" />
                <a-table-column title="标签" dataIndex="tag" />
                <a-table-column title="大小" dataIndex="size">
                  <template #default="{ text }">
                    {{ (text / (1024 * 1024)).toFixed(2) }} MB
                  </template>
                </a-table-column>
                <a-table-column title="创建时间" dataIndex="created">
                  <template #default="{ text }">
                    {{ formatTime(text) }}
                  </template>
                </a-table-column>
                <a-table-column title="操作">
                  <template #default="{ record }">
                    <a-space>
                      <a-button type="primary" danger size="small"
                        @click="removeImage(record.id, `${record.repository}:${record.tag}`)">
                        <template #icon>
                          <DeleteOutlined />
                        </template>
                        删除
                      </a-button>
                    </a-space>
                  </template>
                </a-table-column>
              </a-table>
            </a-tab-pane>

            <!-- Compose管理 -->
            <a-tab-pane key="composes" tab="Compose管理">
              <div class="tab-header">
                <div class="search-box">
                  <!-- Compose通常不会太多，所以不需要搜索框 -->
                </div>
                <div class="action-box">
                  <a-space>
                    <a-button type="primary" @click="composeFormVisible = true">
                      <template #icon>
                        <PlusOutlined />
                      </template>
                      创建Compose
                    </a-button>
                    <a-button @click="fetchComposes" :loading="composesLoading">
                      <template #icon>
                        <ReloadOutlined />
                      </template>
                      刷新
                    </a-button>
                  </a-space>
                </div>
              </div>

              <a-table :dataSource="composes" :loading="composesLoading" :pagination="{ pageSize: 10 }" rowKey="name">
                <template #emptyText>
                  <div style="text-align: center; padding: 16px;">
                    <p>暂无Compose项目数据</p>
                    <p v-if="isServerOnline">服务器上没有Docker Compose项目</p>
                    <p v-else>服务器离线，无法获取Compose项目数据</p>
                  </div>
                </template>

                <a-table-column title="名称" dataIndex="name" />
                <a-table-column title="状态" dataIndex="status">
                  <template #default="{ text }">
                    <a-tag :color="text === 'running' ? 'success' : 'error'">
                      {{ text === 'running' ? '运行中' : '已停止' }}
                    </a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="容器数量" dataIndex="container_count" />
                <a-table-column title="上次更新" dataIndex="updated_at">
                  <template #default="{ text }">
                    {{ formatTime(text) }}
                  </template>
                </a-table-column>
                <a-table-column title="操作">
                  <template #default="{ record }">
                    <a-space>
                      <a-button v-if="record.status !== 'running'" type="primary" size="small"
                        @click="composeUp(record.name)">
                        <template #icon>
                          <PlayCircleOutlined />
                        </template>
                        启动
                      </a-button>
                      <a-button v-if="record.status === 'running'" type="primary" danger size="small"
                        @click="composeDown(record.name)">
                        <template #icon>
                          <PauseCircleOutlined />
                        </template>
                        停止
                      </a-button>
                      <a-button type="primary" size="small" @click="viewComposeConfig(record.name)">
                        <template #icon>
                          <FileTextOutlined />
                        </template>
                        配置
                      </a-button>
                      <a-button type="primary" danger size="small" @click="removeCompose(record.name)">
                        <template #icon>
                          <DeleteOutlined />
                        </template>
                        删除
                      </a-button>
                    </a-space>
                  </template>
                </a-table-column>
              </a-table>
            </a-tab-pane>
          </a-tabs>
        </div>
      </a-spin>
    </div>

    <!-- 拉取镜像对话框 -->
    <a-modal v-model:visible="pullImageVisible" title="拉取镜像" @ok="pullImage" :confirmLoading="pullLoading"
      :maskClosable="false">
      <a-form layout="vertical">
        <a-form-item label="镜像名称" required>
          <a-input v-model:value="pullForm" placeholder="例如：nginx:latest、redis:6、ubuntu:20.04"
            @pressEnter="pullImage" />
          <div class="form-help">
            <p>格式：repository:tag</p>
            <p>如不指定tag，默认为latest</p>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- Compose表单对话框 -->
    <a-modal v-model:visible="composeFormVisible" title="创建Compose项目" width="700px" @ok="createCompose"
      :maskClosable="false">
      <a-form layout="vertical">
        <a-form-item label="项目名称" required>
          <a-input v-model:value="composeForm.name" placeholder="输入项目名称，例如：webapp" />
        </a-form-item>
        <a-form-item label="docker-compose.yml内容" required>
          <a-textarea v-model:value="composeForm.content" placeholder="输入docker-compose.yml内容" :rows="15"
            :autoSize="{ minRows: 15, maxRows: 25 }" />
          <div class="form-help">
            <p>YAML格式，请确保语法正确</p>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 创建容器对话框 -->
    <a-modal v-model:visible="createContainerVisible" title="创建容器" width="700px" @ok="createContainer"
      :maskClosable="false">
      <a-form layout="vertical">
        <a-form-item label="容器名称" required>
          <a-input v-model:value="containerForm.name" placeholder="输入容器名称，例如：my-nginx" />
        </a-form-item>
        <a-form-item label="镜像" required>
          <a-input-group compact>
            <a-select style="width: 75%" v-model:value="containerForm.image" placeholder="选择镜像或手动输入" show-search
              :filter-option="false" @search="(value: string) => containerForm.image = value">
              <a-select-option v-for="image in images" :key="image.id" :value="`${image.repository}:${image.tag}`">
                {{ image.repository }}:{{ image.tag }}
              </a-select-option>
            </a-select>
            <a-button style="width: 25%" @click="fetchImages" :loading="imagesLoading" title="刷新镜像列表">
              <template #icon>
                <ReloadOutlined />
              </template>
              刷新镜像
            </a-button>
          </a-input-group>
        </a-form-item>

        <a-divider orientation="left">端口映射</a-divider>
        <div v-for="(port, index) in containerForm.ports" :key="'port-' + index">
          <a-row :gutter="8">
            <a-col :span="10">
              <a-form-item :label="index === 0 ? '主机端口' : undefined">
                <a-input v-model:value="port.hostPort" placeholder="例如：8080" />
              </a-form-item>
            </a-col>
            <a-col :span="10">
              <a-form-item :label="index === 0 ? '容器端口' : undefined">
                <a-input v-model:value="port.containerPort" placeholder="例如：80" />
              </a-form-item>
            </a-col>
            <a-col :span="4">
              <a-form-item :label="index === 0 ? '操作' : undefined">
                <a-button v-if="index === 0" type="primary" @click="addPortMapping">
                  <template #icon>
                    <PlusOutlined />
                  </template>
                </a-button>
                <a-button v-else type="danger" @click="removePortMapping(index)">
                  <template #icon>
                    <DeleteOutlined />
                  </template>
                </a-button>
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <a-divider orientation="left">数据卷</a-divider>
        <div v-for="(volume, index) in containerForm.volumes" :key="'volume-' + index">
          <a-row :gutter="8">
            <a-col :span="10">
              <a-form-item :label="index === 0 ? '主机路径' : undefined">
                <a-input v-model:value="volume.hostPath" placeholder="例如：/data/mysql" />
              </a-form-item>
            </a-col>
            <a-col :span="10">
              <a-form-item :label="index === 0 ? '容器路径' : undefined">
                <a-input v-model:value="volume.containerPath" placeholder="例如：/var/lib/mysql" />
              </a-form-item>
            </a-col>
            <a-col :span="4">
              <a-form-item :label="index === 0 ? '操作' : undefined">
                <a-button v-if="index === 0" type="primary" @click="addVolumeMapping">
                  <template #icon>
                    <PlusOutlined />
                  </template>
                </a-button>
                <a-button v-else type="danger" @click="removeVolumeMapping(index)">
                  <template #icon>
                    <DeleteOutlined />
                  </template>
                </a-button>
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <a-divider orientation="left">环境变量</a-divider>
        <div v-for="(env, index) in containerForm.envs" :key="'env-' + index">
          <a-row :gutter="8">
            <a-col :span="10">
              <a-form-item :label="index === 0 ? '变量名' : undefined">
                <a-input v-model:value="env.key" placeholder="例如：MYSQL_ROOT_PASSWORD" />
              </a-form-item>
            </a-col>
            <a-col :span="10">
              <a-form-item :label="index === 0 ? '变量值' : undefined">
                <a-input v-model:value="env.value" placeholder="例如：123456" />
              </a-form-item>
            </a-col>
            <a-col :span="4">
              <a-form-item :label="index === 0 ? '操作' : undefined">
                <a-button v-if="index === 0" type="primary" @click="addEnvVar">
                  <template #icon>
                    <PlusOutlined />
                  </template>
                </a-button>
                <a-button v-else type="danger" @click="removeEnvVar(index)">
                  <template #icon>
                    <DeleteOutlined />
                  </template>
                </a-button>
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <a-divider orientation="left">高级设置</a-divider>
        <a-form-item label="启动命令">
          <a-input v-model:value="containerForm.command" placeholder="可选，例如：nginx -g 'daemon off;'" />
        </a-form-item>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="重启策略">
              <a-select v-model:value="containerForm.restart">
                <a-select-option value="no">不自动重启</a-select-option>
                <a-select-option value="always">总是重启</a-select-option>
                <a-select-option value="on-failure">失败时重启</a-select-option>
                <a-select-option value="unless-stopped">除非手动停止</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="网络模式">
              <a-select v-model:value="containerForm.network">
                <a-select-option value="bridge">桥接(bridge)</a-select-option>
                <a-select-option value="host">主机(host)</a-select-option>
                <a-select-option value="none">无网络(none)</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.server-docker-container {
  min-height: calc(100vh - 84px);
  background-color: #f5f5f5;
  padding: 24px;
}

.docker-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.header-left h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: #1f1f1f;
}

.header-left p {
  margin: 8px 0 0;
  color: #8c8c8c;
}

.header-right {
  display: flex;
  gap: 12px;
}

.docker-content {
  background-color: #fff;
  border-radius: 8px;
  padding: 24px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03);
}

.toolbar {
  display: flex;
  justify-content: space-between;
  margin-bottom: 16px;
}

.toolbar-left {
  display: flex;
  gap: 12px;
}

/* Ant Design 覆盖样式 */
:deep(.ant-tabs-nav) {
  margin-bottom: 24px;
}

:deep(.ant-table-wrapper) {
  background-color: #fff;
}

/* 状态徽章 */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.status-text {
  font-size: 14px;
}

/* 运行中 */
.status-running .status-dot {
  background-color: #52c41a;
  box-shadow: 0 0 0 2px rgba(82, 196, 26, 0.2);
}
.status-running .status-text {
  color: #52c41a;
}

/* 已停止 */
.status-exited .status-dot {
  background-color: #d9d9d9;
}
.status-exited .status-text {
  color: #8c8c8c;
}

/* 暂停 */
.status-paused .status-dot {
  background-color: #faad14;
}
.status-paused .status-text {
  color: #faad14;
}

/* 重启中 */
.status-restarting .status-dot {
  background-color: #1890ff;
}
.status-restarting .status-text {
  color: #1890ff;
}

/* 错误/死亡 */
.status-dead .status-dot,
.status-unknown .status-dot {
  background-color: #ff4d4f;
}
.status-dead .status-text,
.status-unknown .status-text {
  color: #ff4d4f;
}

/* 操作按钮 */
.action-btn {
  padding: 0 8px;
}

.action-btn.danger {
  color: #ff4d4f;
}

.action-btn.danger:hover {
  color: #ff7875;
}

.docker-container {
  padding: 0;
  background: transparent;
}

.docker-content {
  margin-top: 16px;
}

.tab-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding: 16px;
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.4);
  border-radius: 16px;
  box-shadow: var(--shadow-sm);
}

.form-help {
  color: var(--text-secondary);
  margin-top: 8px;
  font-size: 12px;
  padding: 8px 12px;
  background: rgba(0, 122, 255, 0.05);
  border-radius: 8px;
  border-left: 3px solid var(--primary-color);
}

.form-help p {
  margin-bottom: 4px;
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
  font-size: 13px;
}

:deep(.ant-table-tbody > tr > td) {
  background: transparent;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03);
  font-size: 13px;
}

:deep(.ant-table-tbody > tr:hover > td) {
  background: rgba(0, 122, 255, 0.05);
}

/* 标签样式优化 */
:deep(.ant-tag) {
  border-radius: 8px;
  font-size: 12px;
  padding: 2px 10px;
  font-weight: 500;
}

/* 标签页样式 */
:deep(.ant-tabs) {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.4);
  border-radius: 16px;
  padding: 16px;
  box-shadow: var(--shadow-md);
}

:deep(.ant-tabs-nav) {
  margin-bottom: 16px;
}

:deep(.ant-tabs-tab) {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-secondary);
}

:deep(.ant-tabs-tab-active) {
  color: var(--primary-color);
}

:deep(.ant-tabs-ink-bar) {
  background: var(--primary-color);
  height: 3px;
  border-radius: 2px;
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

/* 下拉菜单样式 */
:deep(.ant-dropdown-menu) {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.4);
  box-shadow: var(--shadow-lg);
  padding: 6px;
}

:deep(.ant-dropdown-menu-item) {
  border-radius: 8px;
  margin: 2px 0;
  font-size: 13px;
  transition: all 0.2s ease;
}

:deep(.ant-dropdown-menu-item:hover) {
  background: rgba(0, 122, 255, 0.1);
}

:deep(.ant-dropdown-menu-item-danger:hover) {
  background: rgba(255, 59, 48, 0.1);
}

:deep(.ant-dropdown-menu-item-divider) {
  background: rgba(0, 0, 0, 0.05);
}

/* 输入框样式 */
:deep(.ant-input),
:deep(.ant-input-search) {
  border-radius: 10px;
  border: 1px solid rgba(0, 0, 0, 0.1);
}

:deep(.ant-input:focus),
:deep(.ant-input-search:focus) {
  border-color: var(--primary-color);
  box-shadow: 0 0 0 2px rgba(0, 122, 255, 0.1);
}

:deep(.ant-select-selector) {
  border-radius: 10px !important;
  border: 1px solid rgba(0, 0, 0, 0.1) !important;
}

:deep(.ant-select-focused .ant-select-selector) {
  border-color: var(--primary-color) !important;
  box-shadow: 0 0 0 2px rgba(0, 122, 255, 0.1) !important;
}

/* 分割线样式 */
:deep(.ant-divider) {
  border-color: rgba(0, 0, 0, 0.06);
  margin: 24px 0 16px;
}

:deep(.ant-divider-inner-text) {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 14px;
}

/* 表单项样式 */
:deep(.ant-form-item-label > label) {
  font-weight: 500;
  color: var(--text-primary);
  font-size: 13px;
}

:deep(.ant-form-item) {
  margin-bottom: 16px;
}

/* 动态表单行样式 */
:deep(.ant-row) {
  margin-bottom: 0;
}

/* 文本域样式 */
:deep(.ant-input-textarea) {
  border-radius: 10px;
}

:deep(.ant-input-textarea .ant-input) {
  border-radius: 10px;
}

/* 按钮组样式优化 */
:deep(.ant-space) {
  gap: 8px !important;
}

/* 输入组样式 */
:deep(.ant-input-group-compact) {
  display: flex;
  gap: 8px;
}

:deep(.ant-input-group-compact > *:first-child) {
  border-radius: 10px !important;
}

:deep(.ant-input-group-compact > *:last-child) {
  border-radius: 10px !important;
}
</style>
<style>
.dark .server-docker-container {
  background-color: #1e1e1e;
}

.dark .header-left h2 {
  color: #e0e0e0;
}

.dark .header-left p {
  color: #8c8c8c;
}

.dark .docker-content {
  background-color: #252526;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
}

.dark .ant-tabs-tab {
  color: #8c8c8c;
}

.dark .ant-tabs-tab-active .ant-tabs-tab-btn {
  color: #177ddc;
}

.dark .ant-tabs-ink-bar {
  background: #177ddc;
}

.dark .ant-table-wrapper {
  background-color: transparent;
}

.dark .ant-table {
  background: transparent;
  color: #ccc;
}

.dark .ant-table-thead > tr > th {
  background: #2d2d2d;
  color: #ccc;
  border-bottom: 1px solid #333;
}

.dark .ant-table-tbody > tr > td {
  border-bottom: 1px solid #333;
  color: #ccc;
}

.dark .ant-table-tbody > tr:hover > td {
  background: #2a2d2e !important;
}

.dark .ant-modal-content {
  background-color: #252526;
}

.dark .ant-modal-header {
  background-color: #252526;
  border-bottom: 1px solid #333;
}

.dark .ant-modal-title {
  color: #e0e0e0;
}

.dark .ant-modal-close {
  color: #ccc;
}

.dark .ant-modal-footer {
  border-top: 1px solid #333;
}

.dark .ant-select-selector {
  background-color: #3c3c3c !important;
  border-color: #434343 !important;
  color: #ccc !important;
}

.dark .ant-select-selector:focus {
  border-color: #177ddc !important;
}
</style>
