<script setup lang="ts">
defineOptions({
  name: 'ServerWebsite'
});

import { ref, reactive, computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, Modal } from 'ant-design-vue';
import {
  ReloadOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  FolderOutlined,
  SearchOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';

interface RawSite {
  primary_domain: string;
  extra_domains?: string[];
  root_dir: string;
  php_version?: string;
  proxy?: {
    enable: boolean;
    pass: string;
    websocket?: boolean;
  };
  enable_https: boolean;
  force_ssl: boolean;
  ssl?: {
    certificate?: string;
    certificate_key?: string;
  };
  http_challenge_dir?: string;
  labels?: Record<string, string>;
  updated_at?: string;
}

interface CertificateInfo {
  valid?: boolean;
  expiry?: string;
  issuer?: string;
  days_left?: number;
}

interface WebsiteItem {
  site: RawSite;
  type: string;
  certificate?: CertificateInfo;
  host_root_dir?: string;
}

interface CertificateAccount {
  id: number;
  name: string;
  provider: string;
  config: Record<string, string>;
}

interface ManagedCertificate {
  id: number;
  primary_domain: string;
  domains: string[];
  provider: string;
  account_id?: number;
  status: string;
  certificate_path: string;
  key_path: string;
  expiry: string;
}

const route = useRoute();
const router = useRouter();
const serverId = ref<number>(Number(route.params.id));

const activeTab = ref('websites');
const serverInfo = ref<any>({});
const loading = ref(true);
const openRestyStatus = ref({
  installed: true,
  running: false,
  mode: 'container',
  version: '',
  native_running: false,
  native_version: ''
});
const openRestyChecking = ref(false);
const installingOpenResty = ref(false);
const installLogModalVisible = ref(false);
const installLogs = ref<string[]>([]);
const installSessionId = ref<string>('');
let installLogTimer: number | null = null;

const websites = ref<WebsiteItem[]>([]);
const websitesLoading = ref(false);
const typeFilter = ref('all');
const keyword = ref('');

const certificateAccounts = ref<CertificateAccount[]>([]);
const accountsLoading = ref(false);
const certificates = ref<ManagedCertificate[]>([]);
const certificatesLoading = ref(false);
const certificateContentModalVisible = ref(false);
const certificateContentLoading = ref(false);
const certificateContent = reactive({
  domain: '',
  certificate: '',
  privateKey: '',
  certificatePath: '',
  keyPath: ''
});

const websiteDrawerVisible = ref(false);
const websiteSaving = ref(false);
const editingWebsite = ref<WebsiteItem | null>(null);

const websiteForm = reactive({
  domain: '',
  extraDomains: [] as string[],
  rootDir: '/www/sites/example',
  phpVersion: undefined as string | undefined,
  proxyEnable: false,
  proxyPass: '',
  proxyWebsocket: false,
  enableHTTPS: true,
  forceSSL: true,
  httpChallengeDir: '/www/common',
  indexText: 'index.php,index.html',
  certificateId: undefined as number | undefined
});

const sslModalVisible = ref(false);
const sslLoading = ref(false);
const sslForm = reactive({
  domains: [] as string[],
  email: '',
  provider: 'http01',
  webroot: '/opt/node/openresty/www/common',
  dnsAccountId: undefined as number | undefined,
  aliyunKey: '',
  aliyunSecret: '',
  cloudflareToken: '',
  cloudflareZoneToken: '',
  useStaging: false
});

const accountModalVisible = ref(false);
const accountSubmitting = ref(false);
const accountForm = reactive({
  name: '',
  provider: 'alidns',
  accessKeyId: '',
  accessKeySecret: '',
  apiToken: '',
  apiEmail: '',
  apiKey: '',
  zoneToken: ''
});

const isServerOnline = computed(() => serverInfo.value?.online === true);
const canManageSites = computed(() => openRestyStatus.value.installed);
const canControlContainer = computed(() => openRestyStatus.value.installed);
const showWebrootField = computed(() => sslForm.provider === 'http01');
const showAliyunFields = computed(() => sslForm.provider === 'alidns' || sslForm.provider === 'aliyun');
const showCloudflareField = computed(() => sslForm.provider === 'cloudflare' || sslForm.provider === 'cf');
const showNativeOnlyWarning = computed(
  () => !openRestyStatus.value.installed && openRestyStatus.value.native_running
);
const certificateOptions = computed(() =>
  certificates.value.map((cert) => ({
    label: `${cert.primary_domain} (到期 ${new Date(cert.expiry).toLocaleDateString()})`,
    value: cert.id
  }))
);
const providerAccounts = computed(() =>
  certificateAccounts.value.filter((acc) => acc.provider === sslForm.provider)
);

const filteredWebsites = computed(() => {
  return websites.value.filter((item) => {
    const matchesType = typeFilter.value === 'all' || item.type === typeFilter.value;
    if (!matchesType) {
      return false;
    }

    if (!keyword.value) {
      return true;
    }

    const domain = item.site.primary_domain || '';
    const extras = item.site.extra_domains || [];
    const matcher = keyword.value.toLowerCase();
    if (domain.toLowerCase().includes(matcher)) {
      return true;
    }
    return extras.some((d) => d.toLowerCase().includes(matcher));
  });
});

const fetchServerInfo = async () => {
  loading.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}`);
    if (response && response.server) {
      serverInfo.value = response.server;
    }
  } catch (error) {
    console.error('获取服务器信息失败:', error);
    message.error('获取服务器信息失败');
  } finally {
    loading.value = false;
  }
};

const fetchOpenRestyStatus = async () => {
  openRestyChecking.value = true;
  try {
    const response: any = await request.get(`/servers/${serverId.value}/nginx/openresty/status`);
    openRestyStatus.value = {
      installed: response?.installed ?? false,
      running: response?.running ?? false,
      mode: response?.mode || 'missing',
      version: response?.version || '',
      native_running: response?.native_running ?? false,
      native_version: response?.native_version || ''
    };
  } catch (error) {
    console.error('检测OpenResty状态失败:', error);
    message.error('检查OpenResty状态失败');
    openRestyStatus.value = {
      installed: false,
      running: false,
      mode: 'missing',
      version: '',
      native_running: false,
      native_version: ''
    };
  } finally {
    openRestyChecking.value = false;
  }
};

const fetchCertificateAccounts = async () => {
  accountsLoading.value = true;
  try {
    const response: CertificateAccount[] = await request.get(
      `/servers/${serverId.value}/cert/accounts`
    );
    certificateAccounts.value = Array.isArray(response) ? response : [];
  } catch (error) {
    console.error('获取DNS账号失败:', error);
    message.error('获取DNS账号失败');
  } finally {
    accountsLoading.value = false;
  }
};

const fetchCertificates = async () => {
  certificatesLoading.value = true;
  try {
    const response: ManagedCertificate[] = await request.get(
      `/servers/${serverId.value}/certificates`
    );
    certificates.value = Array.isArray(response) ? response : [];
    if (
      websiteForm.enableHTTPS &&
      websiteForm.certificateId &&
      !certificates.value.some((cert) => cert.id === websiteForm.certificateId)
    ) {
      websiteForm.certificateId = certificates.value[0]?.id;
    }
  } catch (error) {
    console.error('获取证书列表失败:', error);
    message.error('获取证书列表失败');
  } finally {
    certificatesLoading.value = false;
  }
};

const resetAccountForm = () => {
  accountForm.name = '';
  accountForm.provider = 'alidns';
  accountForm.accessKeyId = '';
  accountForm.accessKeySecret = '';
  accountForm.apiToken = '';
  accountForm.apiEmail = '';
  accountForm.apiKey = '';
  accountForm.zoneToken = '';
};

const openAccountModal = () => {
  resetAccountForm();
  accountModalVisible.value = true;
};

const submitAccount = async () => {
  if (!accountForm.name.trim()) {
    message.error('请输入账号名称');
    return;
  }

  const config: Record<string, string> = {};
  if (accountForm.provider === 'alidns' || accountForm.provider === 'aliyun') {
    if (!accountForm.accessKeyId.trim() || !accountForm.accessKeySecret.trim()) {
      message.error('请输入阿里云AccessKey ID和Secret');
      return;
    }
    config.access_key_id = accountForm.accessKeyId.trim();
    config.access_key_secret = accountForm.accessKeySecret.trim();
  } else if (accountForm.provider === 'cloudflare' || accountForm.provider === 'cf') {
    let hasCredential = false;
    if (accountForm.apiToken.trim()) {
      config.api_token = accountForm.apiToken.trim();
      hasCredential = true;
    }
    if (accountForm.apiEmail.trim() && accountForm.apiKey.trim()) {
      config.api_email = accountForm.apiEmail.trim();
      config.api_key = accountForm.apiKey.trim();
      hasCredential = true;
    }
    if (!hasCredential) {
      message.error('请输入Cloudflare API Token或Email+API Key');
      return;
    }
    if (accountForm.zoneToken.trim()) {
      config.zone_token = accountForm.zoneToken.trim();
    }
  } else {
    message.error('暂不支持该提供商');
    return;
  }

  accountSubmitting.value = true;
  try {
    await request.post(`/servers/${serverId.value}/cert/accounts`, {
      name: accountForm.name.trim(),
      provider: accountForm.provider,
      config
    });
    message.success('账号已保存');
    accountModalVisible.value = false;
    await fetchCertificateAccounts();
  } catch (error: any) {
    message.error(error?.message || '保存账号失败');
  } finally {
    accountSubmitting.value = false;
  }
};

const deleteAccount = (account: CertificateAccount) => {
  Modal.confirm({
    title: `确认删除账号「${account.name}」吗？`,
    onOk: async () => {
      try {
        await request.delete(`/servers/${serverId.value}/cert/accounts/${account.id}`);
        message.success('账号已删除');
        fetchCertificateAccounts();
      } catch (error: any) {
        message.error(error?.message || '删除失败');
      }
    }
  });
};

const deleteCertificateRecord = (cert: ManagedCertificate) => {
  Modal.confirm({
    title: `删除证书 ${cert.primary_domain} ?`,
    onOk: async () => {
      try {
        await request.delete(`/servers/${serverId.value}/certificates/${cert.id}`);
        message.success('证书记录已删除');
        fetchCertificates();
      } catch (error: any) {
        message.error(error?.message || '删除证书失败');
      }
    }
  });
};

const renewingCertId = ref<number | null>(null);

const renewCertificate = (cert: ManagedCertificate) => {
  Modal.confirm({
    title: `续期证书 ${cert.primary_domain}`,
    content: '确认要续期此证书吗？续期将使用原有配置重新申请证书。',
    onOk: async () => {
      renewingCertId.value = cert.id;
      try {
        await request.post(`/servers/${serverId.value}/certificates/${cert.id}/renew`);
        message.success('证书续期成功');
        fetchCertificates();
      } catch (error: any) {
        message.error(error?.message || '证书续期失败');
      } finally {
        renewingCertId.value = null;
      }
    }
  });
};

const installOpenResty = async () => {
  installingOpenResty.value = true;
  installLogs.value = [];
  installLogModalVisible.value = true;

  try {
    const response: any = await request.post(`/servers/${serverId.value}/nginx/openresty/install`);

    if (response.session_id) {
      installSessionId.value = response.session_id;
      startPollingInstallLogs();
    } else {
      message.success('OpenResty 已安装并启动');
      installLogModalVisible.value = false;
      await fetchOpenRestyStatus();
      await fetchWebsites();
    }
  } catch (error) {
    console.error('安装OpenResty失败:', error);
    message.error('安装OpenResty失败，请查看节点日志');
    installLogModalVisible.value = false;
  } finally {
    installingOpenResty.value = false;
  }
};

const startPollingInstallLogs = () => {
  if (installLogTimer) {
    clearInterval(installLogTimer);
  }

  // 立即获取一次
  fetchInstallLogs();

  // 每500ms轮询一次
  installLogTimer = window.setInterval(() => {
    fetchInstallLogs();
  }, 500);
};

const fetchInstallLogs = async () => {
  if (!installSessionId.value) return;

  try {
    const resp: any = await request.get(
      `/servers/${serverId.value}/nginx/openresty/install-logs?session_id=${installSessionId.value}`
    );

    if (resp && resp.logs && Array.isArray(resp.logs)) {
      installLogs.value = resp.logs;

      // 根据返回的 status 判断是否完成
      if (resp.status === 'completed' || resp.status === 'not_found') {
        stopPollingInstallLogs();

        // 如果是正常完成，延迟刷新状态
        if (resp.status === 'completed') {
          setTimeout(async () => {
            await fetchOpenRestyStatus();
            await fetchWebsites();
          }, 1000);
        }
      }
    }
  } catch (error) {
    console.error('获取安装日志失败:', error);
  }
};

const stopPollingInstallLogs = () => {
  if (installLogTimer) {
    clearInterval(installLogTimer);
    installLogTimer = null;
  }
  installingOpenResty.value = false;
};

const closeInstallLogModal = () => {
  stopPollingInstallLogs();
  installLogModalVisible.value = false;
  installLogs.value = [];
  installSessionId.value = '';
};

const requestInstallOpenResty = () => {
  if (showNativeOnlyWarning.value) {
    Modal.confirm({
      title: '检测到系统已有 Nginx 服务',
      content:
        '当前服务器上仍有系统级 Nginx 运行，占用了 80/443 端口。请先停止或卸载系统 Nginx 后再安装 OpenResty 容器，以避免端口冲突。',
      okText: '已知晓，继续安装',
      cancelText: '取消',
      onOk: () => installOpenResty()
    });
  } else {
    installOpenResty();
  }
};

const fetchWebsites = async () => {
  websitesLoading.value = true;
  try {
    const response: WebsiteItem[] = await request.get(`/servers/${serverId.value}/websites`);
    websites.value = Array.isArray(response) ? response : [];
  } catch (error) {
    console.error('获取网站列表失败:', error);
    message.error('获取网站列表失败');
  } finally {
    websitesLoading.value = false;
  }
};

const refreshData = async () => {
  await fetchOpenRestyStatus();
  const tasks: Promise<any>[] = [];
  if (openRestyStatus.value.installed) {
    tasks.push(fetchWebsites());
  } else {
    websites.value = [];
  }
  await Promise.all(tasks);
};

const resetWebsiteForm = () => {
  websiteForm.domain = '';
  websiteForm.extraDomains = [];
  websiteForm.rootDir = '/www/sites/example';
  websiteForm.phpVersion = undefined;
  websiteForm.proxyEnable = false;
  websiteForm.proxyPass = '';
  websiteForm.proxyWebsocket = false;
  websiteForm.enableHTTPS = true;
  websiteForm.forceSSL = true;
  websiteForm.httpChallengeDir = '/www/common';
  websiteForm.indexText = 'index.php,index.html';
  websiteForm.certificateId = undefined;
};

const openCreateWebsite = () => {
  editingWebsite.value = null;
  resetWebsiteForm();
  websiteDrawerVisible.value = true;
};

const openEditWebsite = (item: WebsiteItem) => {
  editingWebsite.value = item;
  resetWebsiteForm();
  websiteForm.domain = item.site.primary_domain || '';
  websiteForm.extraDomains = [...(item.site.extra_domains || [])];
  websiteForm.rootDir = item.site.root_dir || '';
  websiteForm.phpVersion = item.site.php_version || undefined;
  websiteForm.proxyEnable = item.site.proxy?.enable || false;
  websiteForm.proxyPass = item.site.proxy?.pass || '';
  websiteForm.proxyWebsocket = item.site.proxy?.websocket || false;
  websiteForm.enableHTTPS = item.site.enable_https;
  websiteForm.forceSSL = item.site.force_ssl;
  websiteForm.httpChallengeDir = item.site.http_challenge_dir || '/www/common';
  websiteDrawerVisible.value = true;
};

const sanitizeDomainList = (domains: string[]) => {
  const seen = new Set<string>();
  return domains
    .map((d) => d.trim())
    .filter((d) => {
      if (!d || seen.has(d)) {
        return false;
      }
      seen.add(d);
      return true;
    });
};

const buildWebsitePayload = () => {
  const domain = websiteForm.domain.trim();
  if (!domain) {
    throw new Error('请输入主域名');
  }
  if (!websiteForm.rootDir.trim()) {
    throw new Error('请输入网站目录');
  }

  const extra = sanitizeDomainList(websiteForm.extraDomains);
  const allDomains = sanitizeDomainList([domain, ...extra]);

  const config: Record<string, any> = {
    primary_domain: domain,
    extra_domains: extra,
    root_dir: websiteForm.rootDir.trim(),
    index: websiteForm.indexText
      .split(',')
      .map((item) => item.trim())
      .filter((item) => !!item),
    proxy: {
      enable: websiteForm.proxyEnable,
      pass: websiteForm.proxyPass.trim(),
      websocket: websiteForm.proxyWebsocket
    },
    enable_https: websiteForm.enableHTTPS,
    force_ssl: websiteForm.forceSSL,
    http_challenge_dir: websiteForm.httpChallengeDir.trim()
  };

  if (websiteForm.phpVersion) {
    config.php_version = websiteForm.phpVersion;
  }
  if (websiteForm.enableHTTPS && websiteForm.certificateId) {
    config.certificate_id = websiteForm.certificateId;
  }

  return {
    domain,
    domains: allDomains,
    extra_domains: extra,
    config
  };
};

const submitWebsite = async () => {
  try {
    const payload = buildWebsitePayload();
    websiteSaving.value = true;
    await request.post(`/servers/${serverId.value}/websites`, payload);
    message.success('网站配置已应用');
    websiteDrawerVisible.value = false;
    await fetchWebsites();
  } catch (error: any) {
    const msg = error?.message || '保存网站失败';
    message.error(msg);
  } finally {
    websiteSaving.value = false;
  }
};

const openSSLModal = (item?: WebsiteItem | string[]) => {
  let domains: string[] = [];
  if (Array.isArray(item)) {
    domains = item;
  } else if (item) {
    domains = sanitizeDomainList([
      item.site.primary_domain,
      ...(item.site.extra_domains || [])
    ]);
  }
  sslForm.domains = domains;
  sslForm.email = '';
  sslForm.provider = 'http01';
  sslForm.webroot = '/opt/node/openresty/www/common';
  sslForm.dnsAccountId = undefined;
  sslForm.aliyunKey = '';
  sslForm.aliyunSecret = '';
  sslForm.cloudflareToken = '';
  sslForm.cloudflareZoneToken = '';
  sslForm.useStaging = false;
  sslModalVisible.value = true;
};

const applySSLFromDrawer = () => {
  const domains = sanitizeDomainList([websiteForm.domain, ...websiteForm.extraDomains]);
  if (domains.length === 0) {
    message.warning('请先填写域名');
    return;
  }
  openSSLModal(domains);
};

const submitSSL = async () => {
  if (!sslForm.domains.length) {
    message.error('请至少输入一个域名');
    return;
  }
  if (!sslForm.email.trim()) {
    message.error('请输入证书通知邮箱');
    return;
  }

  const payload: Record<string, any> = {
    domains: sslForm.domains,
    email: sslForm.email.trim(),
    provider: sslForm.provider,
    use_staging: sslForm.useStaging
  };

  if (sslForm.provider === 'http01') {
    if (!sslForm.webroot.trim()) {
      message.error('请填写Web根目录');
      return;
    }
    payload.webroot = sslForm.webroot.trim();
  } else {
    if (sslForm.dnsAccountId) {
      payload.account_id = sslForm.dnsAccountId;
    } else if (showAliyunFields.value) {
      if (!sslForm.aliyunKey.trim() || !sslForm.aliyunSecret.trim()) {
        message.error('请填写阿里云的AccessKey ID和Secret');
        return;
      }
      payload.dns_config = {
        access_key_id: sslForm.aliyunKey.trim(),
        access_key_secret: sslForm.aliyunSecret.trim()
      };
    } else if (showCloudflareField.value) {
      if (!sslForm.cloudflareToken.trim()) {
        message.error('请填写Cloudflare API Token');
        return;
      }
      const cfConfig: Record<string, string> = {
        api_token: sslForm.cloudflareToken.trim()
      };
      if (sslForm.cloudflareZoneToken.trim()) {
        cfConfig.zone_token = sslForm.cloudflareZoneToken.trim();
      }
      payload.dns_config = cfConfig;
    } else {
      message.error('请选择DNS账号或填写凭据');
      return;
    }
  }

  sslLoading.value = true;
  try {
    await request.post(`/servers/${serverId.value}/websites/ssl`, payload);
    message.success('证书申请任务已创建');
    sslModalVisible.value = false;
    await fetchWebsites();
    await fetchCertificates();
  } catch (error: any) {
    const msg = error?.message || '证书申请失败';
    message.error(msg);
  } finally {
    sslLoading.value = false;
  }
};

const resolveSiteHostPath = (item: WebsiteItem) => {
  const hostPath = item.host_root_dir?.trim();
  if (hostPath) {
    return hostPath;
  }
  const raw = item.site.root_dir?.trim();
  if (!raw) {
    return '/';
  }
  if (raw.startsWith('/www')) {
    return `/opt/node/openresty${raw}`;
  }
  return raw;
};

const openSiteDirectory = (item: WebsiteItem) => {
  const target = resolveSiteHostPath(item);
  if (!target) {
    message.warning('暂无网站目录信息');
    return;
  }
  router.push({
    name: 'ServerFile',
    params: { id: serverId.value },
    query: { path: target }
  });
};

const openCertificateContent = async (cert: ManagedCertificate) => {
  certificateContent.domain = cert.primary_domain;
  certificateContent.certificate = '';
  certificateContent.privateKey = '';
  certificateContent.certificatePath = cert.certificate_path || '';
  certificateContent.keyPath = cert.key_path || '';
  certificateContentModalVisible.value = true;
  certificateContentLoading.value = true;
  try {
    const resp: any = await request.get(
      `/servers/${serverId.value}/certificates/${cert.id}/content`
    );
    certificateContent.certificate = resp?.certificate || '';
    certificateContent.privateKey = resp?.private_key || '';
    if (resp?.certificate_path) {
      certificateContent.certificatePath = resp.certificate_path;
    }
    if (resp?.key_path) {
      certificateContent.keyPath = resp.key_path;
    }
  } catch (error: any) {
    certificateContentModalVisible.value = false;
    message.error(error?.message || '获取证书内容失败');
  } finally {
    certificateContentLoading.value = false;
  }
};

const copyText = async (value: string, label: string) => {
  if (!value) {
    message.warning(`${label}内容为空`);
    return;
  }
  try {
    await navigator.clipboard.writeText(value);
    message.success(`${label}已复制`);
  } catch (error) {
    console.error(error);
    message.error(`复制${label}失败`);
  }
};

const startNginx = async () => {
  try {
    await request.post(`/servers/${serverId.value}/nginx/start`);
    message.success('OpenResty 已启动');
    await fetchOpenRestyStatus();
  } catch (error) {
    message.error('启动失败');
  }
};

const stopNginx = async () => {
  try {
    await request.post(`/servers/${serverId.value}/nginx/stop`);
    message.success('OpenResty 已停止');
    await fetchOpenRestyStatus();
  } catch (error) {
    message.error('停止失败');
  }
};

const reloadNginx = async () => {
  try {
    await request.post(`/servers/${serverId.value}/nginx/restart`);
    message.success('配置已重载');
    await fetchOpenRestyStatus();
  } catch (error) {
    message.error('重载失败');
  }
};

const testNginxConfig = async () => {
  try {
    const response: any = await request.get(`/servers/${serverId.value}/nginx/test`);
    if (response?.success) {
      message.success('配置语法检查通过');
    } else {
      message.error(response?.output || '配置测试失败');
    }
  } catch (error) {
    message.error('配置测试失败');
  }
};

const siteTypeText = (type: string) => {
  if (type === 'proxy') {
    return '反向代理';
  }
  return '静态网站';
};

const providerLabel = (provider: string) => {
  switch (provider) {
    case 'alidns':
    case 'aliyun':
      return '阿里云 DNS';
    case 'cloudflare':
    case 'cf':
      return 'Cloudflare';
    default:
      return provider ? provider.toUpperCase() : '';
  }
};

const protocolText = (item: WebsiteItem) => {
  return item.site.enable_https ? 'HTTPS' : 'HTTP';
};

const certificateText = (item: WebsiteItem) => {
  if (item.certificate?.expiry) {
    return new Date(item.certificate.expiry).toLocaleDateString();
  }
  if (item.site.ssl?.certificate) {
    return item.site.ssl.certificate;
  }
  return '未配置';
};

const certificateStatus = (item: WebsiteItem) => {
  if (!item.site.enable_https) {
    return { text: '未开启', color: '' };
  }
  if (!item.certificate || !item.certificate.expiry) {
    if (item.site.ssl?.certificate) {
      return { text: '已配置', color: 'processing' };
    }
    return { text: '未配置', color: '' };
  }
  if (!item.certificate.valid) {
    return { text: '已过期', color: 'error' };
  }
  if (typeof item.certificate.days_left === 'number') {
    if (item.certificate.days_left <= 7) {
      return { text: `${item.certificate.days_left} 天后过期`, color: 'warning' };
    }
    if (item.certificate.days_left <= 30) {
      return { text: `${item.certificate.days_left} 天后过期`, color: 'processing' };
    }
  }
  return { text: '有效', color: 'success' };
};

const managedCertStatus = (cert: ManagedCertificate) => {
  if (!cert || !cert.expiry) {
    return { text: cert?.status || '未知', color: 'default' };
  }
  const now = Date.now();
  const expiry = new Date(cert.expiry).getTime();
  const diffDays = Math.floor((expiry - now) / (1000 * 60 * 60 * 24));
  if (diffDays < 0) {
    return { text: '已过期', color: 'error' };
  }
  if (diffDays <= 7) {
    return { text: `${diffDays} 天后过期`, color: 'warning' };
  }
  if (diffDays <= 30) {
    return { text: `${diffDays} 天后过期`, color: 'processing' };
  }
  return { text: '有效', color: 'success' };
};

onMounted(async () => {
  await fetchServerInfo();
  await refreshData();
  await fetchCertificateAccounts();
  await fetchCertificates();
});

watch(
  () => sslForm.provider,
  () => {
    sslForm.dnsAccountId = undefined;
    sslForm.aliyunKey = '';
    sslForm.aliyunSecret = '';
    sslForm.cloudflareToken = '';
    sslForm.cloudflareZoneToken = '';
  }
);

watch(
  () => accountForm.provider,
  () => {
    accountForm.accessKeyId = '';
    accountForm.accessKeySecret = '';
    accountForm.apiToken = '';
    accountForm.apiEmail = '';
    accountForm.apiKey = '';
    accountForm.zoneToken = '';
  }
);

watch(activeTab, (tab) => {
  if (tab === 'certificates') {
    fetchCertificateAccounts();
    fetchCertificates();
  }
});

watch(
  () => websiteForm.enableHTTPS,
  (enabled) => {
    if (enabled) {
      if (!websiteForm.certificateId && certificates.value.length > 0) {
        websiteForm.certificateId = certificates.value[0].id;
      }
    } else {
      websiteForm.certificateId = undefined;
    }
  }
);

watch(installLogs, () => {
  // 当日志更新时，自动滚动到底部
  setTimeout(() => {
    const logOutput = document.querySelector('.install-log-output');
    if (logOutput) {
      logOutput.scrollTop = logOutput.scrollHeight;
    }
  }, 100);
});

</script>

<template>
  <div class="website-page">
    <a-page-header title="网站管理" :sub-title="serverInfo.name || ''" class="page-header" @back="router.back">
      <template #extra>
        <a-button type="primary" @click="refreshData" :loading="openRestyChecking || websitesLoading">
          <template #icon>
            <ReloadOutlined />
          </template>
          刷新
        </a-button>
      </template>
    </a-page-header>

    <div class="website-content">
      <a-alert v-if="!openRestyStatus.installed" type="warning" show-icon class="install-alert"
        message="未检测到OpenResty环境" :description="showNativeOnlyWarning
          ? `检测到系统级 Nginx${openRestyStatus.native_version ? ' (版本 ' + openRestyStatus.native_version + ')' : ''} 正在运行。为使用统一的网站管理，需要先停用系统 Nginx 并安装 OpenResty 容器。`
          : '需要在节点上安装 OpenResty 容器后才能创建和管理网站。'
          ">
        <template #action>
          <a-button type="primary" size="small" :loading="installingOpenResty" @click="requestInstallOpenResty">
            一键安装
          </a-button>
        </template>
      </a-alert>

      <a-card class="service-card" :loading="openRestyChecking">
        <div class="service-header">
          <div>
            <a-tag v-if="openRestyStatus.installed && openRestyStatus.running" color="green">
              OpenResty 容器已启动
            </a-tag>
            <a-tag v-else-if="openRestyStatus.installed" color="orange">
              OpenResty 容器已安装，未运行
            </a-tag>
            <a-tag v-else-if="openRestyStatus.native_running" color="orange">
              检测到系统级 Nginx
            </a-tag>
            <a-tag v-else color="red">未安装 OpenResty</a-tag>
            <span class="service-version">
              版本 {{ openRestyStatus.version || openRestyStatus.native_version || '未知' }}
            </span>
          </div>
          <a-space>
            <a-button type="primary" size="small" :disabled="!canControlContainer || openRestyStatus.running"
              @click="startNginx">
              <template #icon>
                <PlayCircleOutlined />
              </template>
              启动
            </a-button>
            <a-button size="small" danger :disabled="!canControlContainer || !openRestyStatus.running"
              @click="stopNginx">
              <template #icon>
                <PauseCircleOutlined />
              </template>
              停止
            </a-button>
            <a-button size="small" :disabled="!canControlContainer" @click="reloadNginx">
              重载
            </a-button>
            <a-button size="small" :disabled="!canControlContainer" @click="testNginxConfig">
              检查
            </a-button>
          </a-space>
        </div>
      </a-card>

      <a-tabs v-model:activeKey="activeTab" class="website-tabs">
        <a-tab-pane key="websites" tab="网站列表">
          <a-card class="websites-card" :loading="websitesLoading || openRestyChecking">
            <template #title>
              <div class="table-header">
                <a-space>
                  <a-button type="primary" @click="openCreateWebsite" :disabled="!canManageSites">
                    <template #icon>
                      <PlusOutlined />
                    </template>
                    创建网站
                  </a-button>
                </a-space>
                <div class="table-filters">
                  <a-select v-model:value="typeFilter" style="width: 140px">
                    <a-select-option value="all">全部类型</a-select-option>
                    <a-select-option value="static">静态网站</a-select-option>
                    <a-select-option value="proxy">反向代理</a-select-option>
                  </a-select>
                  <a-input v-model:value="keyword" allow-clear placeholder="搜索域名" style="width: 220px">
                    <template #prefix>
                      <SearchOutlined />
                    </template>
                  </a-input>
                </div>
              </div>
            </template>

            <a-table v-if="canManageSites" :data-source="filteredWebsites" :pagination="false"
              row-key="site.primary_domain">
              <a-table-column title="名称" key="domain">
                <template #default="{ record }">
                  <div class="domain-cell">
                    <span class="primary-domain">{{ record.site.primary_domain }}</span>
                    <span v-if="record.site.extra_domains?.length" class="extra-hint">
                      +{{ record.site.extra_domains.length }}
                    </span>
                  </div>
                </template>
              </a-table-column>
              <a-table-column title="类型" key="type" :width="120">
                <template #default="{ record }">
                  {{ siteTypeText(record.type) }}
                </template>
              </a-table-column>
              <a-table-column title="网站目录" key="root" :width="220">
                <template #default="{ record }">
                  <span class="path-text">
                    <FolderOutlined style="margin-right: 4px" />
                    <a class="path-link" @click.stop="openSiteDirectory(record)">
                      {{ record.site.root_dir }}
                    </a>
                  </span>
                </template>
              </a-table-column>
              <a-table-column title="协议" key="protocol" :width="100">
                <template #default="{ record }">
                  <a-tag :color="record.site.enable_https ? 'success' : 'default'">
                    {{ protocolText(record) }}
                  </a-tag>
                </template>
              </a-table-column>
              <a-table-column title="证书状态" key="certificate" :width="220">
                <template #default="{ record }">
                  <template v-if="record.site.enable_https">
                    <a-tag :color="certificateStatus(record).color">
                      {{ certificateStatus(record).text }}
                    </a-tag>
                    <span class="expiry-text">{{ certificateText(record) }}</span>
                  </template>
                  <span v-else>未开启</span>
                </template>
              </a-table-column>
              <a-table-column title="最后更新" key="updated" :width="180">
                <template #default="{ record }">
                  {{ record.site.updated_at ? new Date(record.site.updated_at).toLocaleString() : '未知' }}
                </template>
              </a-table-column>
              <a-table-column title="操作" key="actions" :width="120">
                <template #default="{ record }">
                  <a-button type="link" size="small" @click="openEditWebsite(record)">配置</a-button>
                </template>
              </a-table-column>
            </a-table>
            <a-empty v-else description="安装 OpenResty 后即可创建网站" />
          </a-card>
        </a-tab-pane>

        <a-tab-pane key="certificates" tab="证书管理">
          <div class="certificate-grid">
            <a-card title="DNS API 账号" class="dns-card" :loading="accountsLoading" :bordered="false">
              <template #extra>
                <a-button type="primary" size="small" @click="openAccountModal">
                  <template #icon>
                    <PlusOutlined />
                  </template>
                  新增账号
                </a-button>
              </template>
              <p class="hint-text">
                这里保存阿里云、Cloudflare 等 DNS API 密钥，统一托管在节点本地，仅在申请证书时调用。
              </p>
              <a-table :data-source="certificateAccounts" :pagination="false" row-key="id"
                :locale="{ emptyText: '暂未添加DNS账号' }">
                <a-table-column title="名称" key="name">
                  <template #default="{ record }">
                    <div class="account-name">
                      <span class="primary-domain">{{ record.name }}</span>
                      <a-tag>{{ providerLabel(record.provider) }}</a-tag>
                    </div>
                  </template>
                </a-table-column>
                <a-table-column title="凭据字段" key="config">
                  <template #default="{ record }">
                    <span class="config-summary">
                      {{
                        Object.keys(record.config || {}).length
                          ? Object.keys(record.config).join(' / ')
                          : '未知'
                      }}
                    </span>
                  </template>
                </a-table-column>
                <a-table-column title="操作" key="ops" :width="120">
                  <template #default="{ record }">
                    <a-button type="link" danger size="small" @click="deleteAccount(record)">
                      删除
                    </a-button>
                  </template>
                </a-table-column>
              </a-table>
            </a-card>

            <a-card title="证书与申请记录" class="cert-card" :loading="certificatesLoading" :bordered="false">
              <template #extra>
                <a-button type="primary" size="small" @click="openSSLModal()">
                  <template #icon>
                    <SafetyCertificateOutlined />
                  </template>
                  申请证书
                </a-button>
              </template>
              <p class="hint-text">
                证书默认保存在节点 /opt/node/openresty/ssl/&lt;域名&gt;/ 下（容器内路径 /usr/local/openresty/nginx/conf/ssl/），可在下方查看与复制内容。
              </p>
              <a-table :data-source="certificates" :pagination="{ pageSize: 5 }" row-key="id"
                :locale="{ emptyText: '暂无证书记录，先申请一个吧' }">
                <a-table-column title="主域名" key="domain">
                  <template #default="{ record }">
                    <div class="domain-cell">
                      <span class="primary-domain">{{ record.primary_domain }}</span>
                      <div class="extra-hint" v-if="record.domains?.length">
                        {{ record.domains.join(', ') }}
                      </div>
                    </div>
                  </template>
                </a-table-column>
                <a-table-column title="颁发方式" key="provider" :width="130">
                  <template #default="{ record }">
                    <a-tag>{{ providerLabel(record.provider) }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="到期时间" key="expiry" :width="180">
                  <template #default="{ record }">
                    {{ record.expiry ? new Date(record.expiry).toLocaleString() : '未知' }}
                  </template>
                </a-table-column>
                <a-table-column title="状态" key="status" :width="140">
                  <template #default="{ record }">
                    <a-tag :color="managedCertStatus(record).color">
                      {{ managedCertStatus(record).text }}
                    </a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="操作" key="actions" :width="180">
                  <template #default="{ record }">
                    <a-space>
                      <a-button type="link" size="small" @click="renewCertificate(record)"
                        :loading="renewingCertId === record.id">
                        续期
                      </a-button>
                      <a-button type="link" size="small" @click="openCertificateContent(record)">
                        查看
                      </a-button>
                      <a-button type="link" danger size="small" @click="deleteCertificateRecord(record)">
                        删除
                      </a-button>
                    </a-space>
                  </template>
                </a-table-column>
              </a-table>
            </a-card>
          </div>
        </a-tab-pane>
      </a-tabs>
    </div>

    <a-modal v-model:open="websiteDrawerVisible" :title="editingWebsite ? '编辑网站' : '创建网站'" width="600px"
      :confirm-loading="websiteSaving" @ok="submitWebsite" @cancel="websiteDrawerVisible = false" class="glass-modal">
      <a-form layout="vertical" class="website-form">
        <a-form-item label="主域名" required>
          <a-input v-model:value="websiteForm.domain" placeholder="example.com" />
        </a-form-item>
        <a-form-item label="附加域名">
          <a-select v-model:value="websiteForm.extraDomains" mode="tags" placeholder="输入域名后回车"
            :token-separators="[',', ' ']" />
        </a-form-item>
        <a-form-item label="网站目录" required>
          <a-input v-model:value="websiteForm.rootDir" placeholder="/www/sites/example" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="PHP版本">
              <a-select v-model:value="websiteForm.phpVersion" allow-clear placeholder="选择PHP版本（可选）">
                <a-select-option value="8.2">PHP 8.2</a-select-option>
                <a-select-option value="8.1">PHP 8.1</a-select-option>
                <a-select-option value="8.0">PHP 8.0</a-select-option>
                <a-select-option value="7.4">PHP 7.4</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="索引文件">
              <a-input v-model:value="websiteForm.indexText" />
            </a-form-item>
          </a-col>
        </a-row>

        <div class="form-section-title">高级配置</div>

        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item label="反向代理">
              <a-switch v-model:checked="websiteForm.proxyEnable" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="WebSocket">
              <a-switch v-model:checked="websiteForm.proxyWebsocket" :disabled="!websiteForm.proxyEnable" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="启用HTTPS">
              <a-switch v-model:checked="websiteForm.enableHTTPS" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item v-if="websiteForm.proxyEnable" label="反代目标地址" required>
          <a-input v-model:value="websiteForm.proxyPass" placeholder="http://127.0.0.1:3000" />
        </a-form-item>

        <template v-if="websiteForm.enableHTTPS">
          <a-form-item label="强制HTTPS跳转">
            <a-switch v-model:checked="websiteForm.forceSSL" />
          </a-form-item>
          <a-form-item label="SSL证书">
            <div style="display: flex; gap: 8px; align-items: flex-start;">
              <a-select v-model:value="websiteForm.certificateId" :options="certificateOptions" allow-clear
                placeholder="请选择已申请的证书" style="flex: 1">
                <template #notFoundContent>
                  <div class="form-hint">暂无证书，请先在证书管理中申请</div>
                </template>
              </a-select>
              <a-button @click="applySSLFromDrawer">申请</a-button>
            </div>
            <div class="form-hint">证书托管在节点本地，可被多个站点复用。</div>
          </a-form-item>
          <a-form-item label="HTTP验证目录">
            <a-input v-model:value="websiteForm.httpChallengeDir" />
          </a-form-item>
        </template>
      </a-form>
    </a-modal>

    <a-modal v-model:open="sslModalVisible" title="申请SSL证书" :confirm-loading="sslLoading" @ok="submitSSL"
      @cancel="sslModalVisible = false">
      <a-form layout="vertical">
        <a-form-item label="域名">
          <a-select v-model:value="sslForm.domains" mode="tags" placeholder="输入域名后回车" />
        </a-form-item>
        <a-form-item label="通知邮箱">
          <a-input v-model:value="sslForm.email" placeholder="admin@example.com" />
        </a-form-item>
        <a-form-item label="验证方式">
          <a-select v-model:value="sslForm.provider">
            <a-select-option value="http01">HTTP-01（Webroot）</a-select-option>
            <a-select-option value="alidns">阿里云 DNS</a-select-option>
            <a-select-option value="cloudflare">Cloudflare DNS</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item v-if="showWebrootField" label="Web根目录">
          <a-input v-model:value="sslForm.webroot" placeholder="/opt/node/openresty/www/common" />
        </a-form-item>
        <template v-else>
          <a-form-item v-if="providerAccounts.length" label="DNS API 账号">
            <a-select v-model:value="sslForm.dnsAccountId" allow-clear placeholder="选择已保存的账号">
              <a-select-option v-for="account in providerAccounts" :key="account.id" :value="account.id">
                {{ account.name }}（{{ providerLabel(account.provider) }}）
              </a-select-option>
            </a-select>
            <div class="form-hint">账号密钥仅保存在节点，可在“证书管理”标签页新增。</div>
          </a-form-item>
          <template v-if="!sslForm.dnsAccountId && showAliyunFields">
            <a-form-item label="AccessKey ID">
              <a-input v-model:value="sslForm.aliyunKey" placeholder="阿里云AccessKey ID" />
            </a-form-item>
            <a-form-item label="AccessKey Secret">
              <a-input v-model:value="sslForm.aliyunSecret" placeholder="阿里云AccessKey Secret" />
            </a-form-item>
          </template>
          <template v-if="!sslForm.dnsAccountId && showCloudflareField">
            <a-form-item label="API Token">
              <a-input v-model:value="sslForm.cloudflareToken" placeholder="Cloudflare API Token" />
            </a-form-item>
            <a-form-item label="Zone Token（可选）">
              <a-input v-model:value="sslForm.cloudflareZoneToken" placeholder="用于 Zone:Read 权限的 Token" />
              <div class="form-hint">若API Token仅拥有 DNS:Edit 权限，请额外提供一个具备 Zone:Read 权限的 Zone Token。</div>
            </a-form-item>
          </template>
        </template>
        <a-form-item label="使用测试环境">
          <a-switch v-model:checked="sslForm.useStaging" />
        </a-form-item>
      </a-form>
    </a-modal>
    <a-modal v-model:open="accountModalVisible" title="新增 DNS API 账号" :confirm-loading="accountSubmitting"
      @ok="submitAccount" @cancel="accountModalVisible = false">
      <a-form layout="vertical">
        <a-form-item label="账号名称">
          <a-input v-model:value="accountForm.name" placeholder="例如：阿里云主账号" />
        </a-form-item>
        <a-form-item label="提供商">
          <a-select v-model:value="accountForm.provider">
            <a-select-option value="alidns">阿里云（AliDNS）</a-select-option>
            <a-select-option value="cloudflare">Cloudflare</a-select-option>
          </a-select>
        </a-form-item>
        <template v-if="accountForm.provider === 'alidns' || accountForm.provider === 'aliyun'">
          <a-form-item label="AccessKey ID">
            <a-input v-model:value="accountForm.accessKeyId" placeholder="AK ID" />
          </a-form-item>
          <a-form-item label="AccessKey Secret">
            <a-input v-model:value="accountForm.accessKeySecret" placeholder="AK Secret" />
          </a-form-item>
        </template>
        <template v-else>
          <a-form-item label="API Token">
            <a-input v-model:value="accountForm.apiToken" placeholder="优先推荐使用 API Token" />
          </a-form-item>
          <a-form-item label="Zone Token（可选）">
            <a-input v-model:value="accountForm.zoneToken" placeholder="仅包含 Zone:Read 权限的 Token" />
            <div class="form-hint">若DNS Token无Zone权限，可在此填写仅具备Zone:Read权限的令牌。</div>
          </a-form-item>
          <a-form-item label="备用 Email + Global API Key">
            <a-input v-model:value="accountForm.apiEmail" placeholder="Cloudflare 登录邮箱（可选）" />
            <a-input style="margin-top: 8px" v-model:value="accountForm.apiKey" placeholder="Global API Key（可选）" />
          </a-form-item>
        </template>
      </a-form>
    </a-modal>
    <a-modal v-model:open="certificateContentModalVisible" :title="`证书内容 - ${certificateContent.domain || ''}`"
      :footer="null" width="720px" @cancel="certificateContentModalVisible = false">
      <a-spin :spinning="certificateContentLoading">
        <div class="cert-content-section" v-if="certificateContent.certificate">
          <div class="cert-content-header">
            <div>
              <strong>证书 (fullchain.pem)</strong>
              <div class="cert-path">
                路径: {{ certificateContent.certificatePath || '未知' }}
              </div>
            </div>
            <a-button size="small" @click="copyText(certificateContent.certificate, '证书')">
              复制
            </a-button>
          </div>
          <a-textarea :value="certificateContent.certificate" readonly :auto-size="{ minRows: 8 }" />
        </div>
        <div class="cert-content-section" v-if="certificateContent.privateKey">
          <div class="cert-content-header">
            <div>
              <strong>私钥 (privkey.pem)</strong>
              <div class="cert-path">
                路径: {{ certificateContent.keyPath || '未知' }}
              </div>
            </div>
            <a-button size="small" @click="copyText(certificateContent.privateKey, '私钥')">
              复制
            </a-button>
          </div>
          <a-textarea :value="certificateContent.privateKey" readonly :auto-size="{ minRows: 8 }" />
        </div>
        <a-empty v-if="!certificateContent.certificate && !certificateContent.privateKey" />
        <div class="modal-footer">
          <a-button type="primary" @click="certificateContentModalVisible = false">关闭</a-button>
        </div>
      </a-spin>
    </a-modal>

    <a-modal v-model:open="installLogModalVisible" title="OpenResty 安装进度" :footer="null" width="720px"
      :maskClosable="false" @cancel="closeInstallLogModal">
      <div class="install-log-container">
        <div class="install-log-output" ref="installLogOutput">
          <div v-if="installLogs.length === 0" class="install-log-empty">
            正在初始化安装...
          </div>
          <div v-else>
            <div v-for="(log, index) in installLogs" :key="index" class="install-log-line">
              {{ log }}
            </div>
          </div>
        </div>
        <div class="install-log-footer">
          <a-button type="primary" @click="closeInstallLogModal"
            :disabled="installingOpenResty || (installLogs.length > 0 && !installLogs[installLogs.length - 1]?.includes('完成'))">
            关闭
          </a-button>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.website-page {
  padding-bottom: 40px;
  min-height: 100%;
  color: var(--text-primary);
}

/* Page Header */
.page-header {
  background: var(--header-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid var(--card-border);
  border-radius: 16px;
  margin-bottom: 24px;
  box-shadow: var(--shadow-sm);
}

:deep(.ant-page-header-heading-title) {
  color: var(--text-primary);
  font-weight: 600;
  font-size: 20px;
}

:deep(.ant-page-header-heading-sub-title) {
  color: var(--text-secondary);
}

/* Content Layout */
.website-content {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* Alerts */
.install-alert {
  border-radius: 16px;
  border: 1px solid rgba(250, 173, 20, 0.3);
  background: rgba(250, 173, 20, 0.1);
  backdrop-filter: blur(10px);
}

/* Cards (Glassmorphism) */
.service-card,
.websites-card,
.dns-card,
.cert-card {
  background: var(--card-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid var(--card-border);
  border-radius: 20px;
  box-shadow: var(--shadow-sm);
  transition: transform 0.3s ease, box-shadow 0.3s ease;
}

.service-card:hover,
.websites-card:hover,
.dns-card:hover,
.cert-card:hover {
  box-shadow: var(--shadow-md);
  border-color: var(--primary-light);
}

:deep(.ant-card-head) {
  border-bottom: 1px solid var(--card-border);
  color: var(--text-primary);
  font-weight: 600;
}

:deep(.ant-card-body) {
  padding: 24px;
}

/* Service Header */
.service-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.service-version {
  margin-left: 12px;
  color: var(--text-secondary);
  font-size: 13px;
}

/* Table Header & Filters */
.table-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0;
  /* Card title handles spacing */
}

.table-filters {
  display: flex;
  gap: 12px;
}

/* Table Styles */
:deep(.ant-table) {
  background: transparent;
  color: var(--text-primary);
}

:deep(.ant-table-thead > tr > th) {
  background: rgba(0, 0, 0, 0.02);
  color: var(--text-secondary);
  border-bottom: 1px solid var(--card-border);
  font-weight: 500;
}



:deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid var(--card-border);
  color: var(--text-primary);
  transition: background 0.3s;
}

:deep(.ant-table-tbody > tr:hover > td) {
  background: var(--primary-bg) !important;
}

:deep(.ant-empty-description) {
  color: var(--text-secondary);
}

/* Domain Cell */
.domain-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.primary-domain {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 15px;
}

.extra-hint {
  font-size: 12px;
  color: var(--text-secondary);
  background: rgba(0, 0, 0, 0.05);
  padding: 2px 6px;
  border-radius: 6px;
}



/* Path Link */
.path-text {
  color: var(--text-secondary);
  display: flex;
  align-items: center;
}

.path-link {
  color: var(--primary-color);
  transition: color 0.3s;
}

.path-link:hover {
  color: var(--primary-hover);
  text-decoration: none;
}

/* Expiry Text */
.expiry-text {
  margin-left: 8px;
  color: var(--text-hint);
  font-size: 12px;
}

/* Tabs */
:deep(.ant-tabs-nav) {
  margin-bottom: 24px;
}

:deep(.ant-tabs-tab) {
  color: var(--text-secondary);
  font-size: 15px;
  padding: 12px 0;
  transition: color 0.3s;
}

:deep(.ant-tabs-tab:hover) {
  color: var(--text-primary);
}

:deep(.ant-tabs-tab-active .ant-tabs-tab-btn) {
  color: var(--primary-color);
  text-shadow: 0 0 10px var(--primary-light);
}

:deep(.ant-tabs-ink-bar) {
  background: var(--primary-color);
  box-shadow: 0 0 10px var(--primary-light);
}

/* Certificate Grid */
.certificate-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  gap: 24px;
}

/* Hint Text */
.hint-text {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 16px;
  line-height: 1.6;
}

/* Account Name */
.account-name {
  display: flex;
  align-items: center;
  gap: 8px;
}

.config-summary {
  color: var(--text-secondary);
  font-family: monospace;
  background: rgba(0, 0, 0, 0.05);
  padding: 2px 6px;
  border-radius: 4px;
}



/* Form Hint */
.form-hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-hint);
}

/* Cert Content Modal */
.cert-content-section {
  margin-bottom: 24px;
  background: rgba(0, 0, 0, 0.02);
  padding: 16px;
  border-radius: 12px;
  border: 1px solid var(--card-border);
}



.cert-content-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.cert-content-header strong {
  color: var(--text-primary);
}

.cert-path {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 4px;
}

.modal-footer {
  text-align: right;
  margin-top: 24px;
}

/* Buttons */
:deep(.ant-btn) {
  border-radius: 8px;
}

:deep(.ant-btn-primary) {
  background: var(--primary-color);
  border: none;
  box-shadow: 0 4px 12px var(--primary-light);
}

:deep(.ant-btn-primary:hover) {
  background: var(--primary-hover);
  box-shadow: 0 6px 16px var(--primary-light);
}

:deep(.ant-btn-default) {
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid rgba(0, 0, 0, 0.1);
  color: var(--text-primary);
}



:deep(.ant-btn-default:hover) {
  background: #fff;
  border-color: var(--primary-color);
  color: var(--primary-color);
}



:deep(.ant-btn-text) {
  color: var(--text-secondary);
}

:deep(.ant-btn-text:hover) {
  background: rgba(0, 0, 0, 0.05);
  color: var(--text-primary);
}



:deep(.ant-btn-link) {
  color: var(--primary-color);
}

:deep(.ant-btn-link:hover) {
  color: var(--primary-hover);
}

/* Inputs & Selects */
:deep(.ant-input),
:deep(.ant-select-selector) {
  background: rgba(255, 255, 255, 0.6) !important;
  border: 1px solid rgba(0, 0, 0, 0.1) !important;
  color: var(--text-primary) !important;
  border-radius: 8px !important;
}



:deep(.ant-input:focus),
:deep(.ant-select-focused .ant-select-selector) {
  border-color: var(--primary-color) !important;
  box-shadow: 0 0 0 2px var(--primary-light) !important;
}

:deep(.ant-select-arrow) {
  color: var(--text-secondary);
}

/* Tags */
:deep(.ant-tag) {
  border-radius: 6px;
  border: none;
}

:deep(.ant-tag-success) {
  background: rgba(82, 196, 26, 0.2);
  color: #73d13d;
}

:deep(.ant-tag-processing) {
  background: rgba(24, 144, 255, 0.2);
  color: #40a9ff;
}

:deep(.ant-tag-warning) {
  background: rgba(250, 173, 20, 0.2);
  color: #ffc53d;
}

:deep(.ant-tag-error) {
  background: rgba(255, 77, 79, 0.2);
  color: #ff7875;
}

/* Responsive */
@media (max-width: 768px) {
  .table-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }

  .table-filters {
    width: 100%;
    flex-direction: column;
  }

  .certificate-grid {
    grid-template-columns: 1fr;
  }
}

/* Install Log Modal */
.install-log-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.install-log-output {
  background: #1e1e1e;
  color: #d4d4d4;
  border-radius: 8px;
  padding: 16px;
  min-height: 400px;
  max-height: 600px;
  overflow-y: auto;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
}

.install-log-empty {
  color: #888;
  text-align: center;
  padding: 20px;
}

.install-log-line {
  margin-bottom: 4px;
  white-space: pre-wrap;
  word-break: break-word;
}

.install-log-line:empty {
  min-height: 4px;
}

.install-log-footer {
  display: flex;
  justify-content: flex-end;
  padding-top: 8px;
}

/* Scroll to bottom */
/* Scroll to bottom */
.install-log-output {
  scroll-behavior: smooth;
}
</style>

<style>
/* Global Dark Mode Overrides for ServerNginx */
.dark .website-page {
  color: #e0e0e0;
}

.dark .page-header {
  background: rgba(30, 30, 30, 0.7);
  border-color: rgba(255, 255, 255, 0.05);
}

.dark .ant-page-header-heading-title {
  color: #e0e0e0;
}

.dark .ant-page-header-heading-sub-title {
  color: #aaa;
}

.dark .service-card,
.dark .websites-card,
.dark .dns-card,
.dark .cert-card {
  background: rgba(30, 30, 30, 0.7);
  border-color: rgba(255, 255, 255, 0.05);
}

.dark .ant-card-head {
  border-bottom-color: rgba(255, 255, 255, 0.05);
  color: #e0e0e0;
}

.dark .ant-table {
  color: #e0e0e0;
}

.dark .ant-table-thead>tr>th {
  background: rgba(255, 255, 255, 0.05);
  color: #ccc;
  border-bottom-color: rgba(255, 255, 255, 0.05);
}

.dark .ant-table-tbody>tr>td {
  border-bottom-color: rgba(255, 255, 255, 0.05);
  color: #e0e0e0;
}

.dark .ant-table-tbody>tr:hover>td {
  background: rgba(255, 255, 255, 0.05) !important;
}

.dark .ant-empty-description {
  color: #888;
}

.dark .primary-domain {
  color: #e0e0e0;
}

.dark .extra-hint {
  background: rgba(255, 255, 255, 0.1);
  color: #aaa;
}

.dark .path-text {
  color: #aaa;
}

.dark .path-link {
  color: #177ddc;
}

.dark .path-link:hover {
  color: #40a9ff;
}

.dark .expiry-text {
  color: #888;
}

.dark .ant-tabs-tab {
  color: #aaa;
}

.dark .ant-tabs-tab:hover {
  color: #e0e0e0;
}

.dark .ant-tabs-tab-active .ant-tabs-tab-btn {
  color: #177ddc;
  text-shadow: 0 0 10px rgba(23, 125, 220, 0.5);
}

.dark .ant-tabs-ink-bar {
  background: #177ddc;
  box-shadow: 0 0 10px rgba(23, 125, 220, 0.5);
}

.dark .hint-text {
  color: #aaa;
}

.dark .config-summary {
  background: rgba(255, 255, 255, 0.1);
  color: #ccc;
}

.dark .form-hint {
  color: #888;
}

.dark .cert-content-section {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(255, 255, 255, 0.1);
}

.dark .cert-content-header strong {
  color: #e0e0e0;
}

.dark .cert-path {
  color: #aaa;
}

.dark .ant-btn-default {
  background: rgba(255, 255, 255, 0.1);
  border-color: rgba(255, 255, 255, 0.1);
  color: #e0e0e0;
}

.dark .ant-btn-default:hover {
  background: rgba(255, 255, 255, 0.2);
  border-color: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.dark .ant-btn-text {
  color: #aaa;
}

.dark .ant-btn-text:hover {
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
}

.dark .ant-btn-link {
  color: #177ddc;
}

.dark .ant-btn-link:hover {
  color: #40a9ff;
}

.dark .ant-input,
.dark .ant-select-selector {
  background: rgba(0, 0, 0, 0.2) !important;
  border-color: rgba(255, 255, 255, 0.1) !important;
  color: #e0e0e0 !important;
}

.dark .ant-input:focus,
.dark .ant-select-focused .ant-select-selector {
  border-color: #177ddc !important;
  box-shadow: 0 0 0 2px rgba(23, 125, 220, 0.2) !important;
}

.dark .ant-select-arrow {
  color: #aaa;
}

.dark .ant-tag-success {
  background: rgba(82, 196, 26, 0.2);
  color: #95de64;
}

.dark .ant-tag-processing {
  background: rgba(24, 144, 255, 0.2);
  color: #69c0ff;
}

.dark .ant-tag-warning {
  background: rgba(250, 173, 20, 0.2);
  color: #ffc53d;
}

.dark .ant-tag-error {
  background: rgba(255, 77, 79, 0.2);
  color: #ff9c6e;
}

/* Glass Modal Styles (Copied from ServerDocker.vue for consistency) */
.glass-modal .ant-modal-content {
  background: rgba(255, 255, 255, 0.8);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: 16px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.glass-modal .ant-modal-header {
  background: transparent;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 16px 16px 0 0;
}

.glass-modal .ant-modal-title {
  font-weight: 600;
}

.glass-modal .ant-input,
.glass-modal .ant-select-selector,
.glass-modal .ant-input-number {
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.5);
  border-color: rgba(0, 0, 0, 0.1);
}

.glass-modal .ant-btn {
  border-radius: 8px;
}

/* Dark Mode Modal */
.dark .glass-modal .ant-modal-content {
  background: rgba(40, 40, 40, 0.8);
  border-color: rgba(255, 255, 255, 0.1);
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.3);
}

.dark .glass-modal .ant-modal-header {
  border-bottom-color: rgba(255, 255, 255, 0.05);
}

.dark .glass-modal .ant-modal-title {
  color: #e0e0e0;
}

.dark .glass-modal .ant-modal-close {
  color: #aaa;
}

.dark .glass-modal .ant-input,
.dark .glass-modal .ant-select-selector,
.dark .glass-modal .ant-input-number {
  background: rgba(0, 0, 0, 0.2);
  border-color: rgba(255, 255, 255, 0.1);
  color: #e0e0e0;
}

.dark .glass-modal .ant-form-item-label>label {
  color: #ccc;
}

.form-section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 16px 0 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--card-border);
}

.dark .form-section-title {
  color: #e0e0e0;
  border-bottom-color: rgba(255, 255, 255, 0.1);
}
</style>
