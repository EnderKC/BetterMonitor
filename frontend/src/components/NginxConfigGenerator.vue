<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import {
  Form,
  FormItem,
  Input,
  Select,
  Switch,
  Button,
  Modal,
  Space,
  InputNumber,
  Tabs,
  TabPane,
  message,
  Card,
  Divider,
  Radio,
  RadioGroup,
  Tooltip,
  Alert,
} from 'ant-design-vue';
import {
  PlusOutlined,
  MinusCircleOutlined,
  QuestionCircleOutlined,
  EyeOutlined,
  InfoCircleOutlined,
  BulbOutlined,
} from '@ant-design/icons-vue';

// 定义配置选项
const serverName = ref('');
const port = ref(80);
const useHttps = ref(false);
const httpsPort = ref(443);
const enableGzip = ref(true);
const clientMaxBodySize = ref('1m');

// 证书路径 (HTTPS)
const sslCertPath = ref('');
const sslKeyPath = ref('');

// 前端静态资源配置
const staticEnabled = ref(true);
const staticRoot = ref('/usr/share/nginx/html');
const staticIndex = ref('index.html');
const staticTryFiles = ref(true);
const customTryFiles = ref('/index.html'); // 自定义伪静态重定向路径

// 反向代理配置
const proxyEnabled = ref(false);
const proxyPaths = ref([
  {
    path: '/api',
    target: 'http://localhost:8080',
    changeOrigin: true,
    rewrite: true,
    pathRewrite: '^/api',
    streaming: false, // 新增流式输出选项
  }
]);

// 负载均衡配置
const loadBalancingEnabled = ref(false);
const loadBalancingMethod = ref('round_robin'); // round_robin, ip_hash, least_conn
const upstreamName = ref('backend');
const upstreamServers = ref([
  { server: 'localhost:8080', weight: 1, backup: false, down: false }
]);

// 自定义配置
const customConfig = ref('');
const showAdvanced = ref(false);

// 当前活动标签
const activeTab = ref('basic');

// 解析状态
const parseSuccess = ref(true);
const parseError = ref('');

// 是否有多个server块
const hasMultipleServers = ref(false);
const selectedServerIndex = ref(0);
const serverBlocks = ref([]);

// 监听selectedServerIndex变化，自动解析选中的server块
watch(selectedServerIndex, (newIndex) => {
  console.log(`切换到第 ${newIndex + 1} 个Server块`);
  if (serverBlocks.value && serverBlocks.value.length > newIndex) {
    parseServerBlock(serverBlocks.value[newIndex].block);
  }
});

// 解析Nginx配置
const parseNginxConfig = (configContent) => {
  parseSuccess.value = true;
  parseError.value = '';
  serverBlocks.value = [];
  
  try {
    // 如果内容为空，返回解析失败
    if (!configContent || configContent.trim() === '') {
      parseSuccess.value = false;
      parseError.value = '配置内容为空';
      return false;
    }
    
    console.log("开始解析Nginx配置...");
    
    // 重置所有配置为默认值
    serverName.value = '';
    port.value = 80;
    useHttps.value = false;
    httpsPort.value = 443;
    enableGzip.value = true;
    clientMaxBodySize.value = '1m';
    sslCertPath.value = '';
    sslKeyPath.value = '';
    staticEnabled.value = true;
    staticRoot.value = '/usr/share/nginx/html';
    staticIndex.value = 'index.html';
    staticTryFiles.value = false;
    customTryFiles.value = '/index.html';
    proxyEnabled.value = false;
    proxyPaths.value = [{
      path: '/api',
      target: 'http://localhost:8080',
      changeOrigin: true,
      rewrite: true,
      pathRewrite: '^/api',
      streaming: false
    }];
    loadBalancingEnabled.value = false;
    loadBalancingMethod.value = 'round_robin';
    upstreamName.value = 'backend';
    upstreamServers.value = [{ server: 'localhost:8080', weight: 1, backup: false, down: false }];
    
    // 检查upstream配置
    const upstreamRegex = /upstream\s+([^\s{]+)\s*{([^}]*)}/g;
    const upstreamMatches = [...configContent.matchAll(upstreamRegex)];
    
    if (upstreamMatches.length > 0) {
      // 找到upstream配置
      loadBalancingEnabled.value = true;
      upstreamName.value = upstreamMatches[0][1] || 'backend';
      
      const upstreamContent = upstreamMatches[0][2];
      
      // 检查负载均衡方法
      if (upstreamContent.includes('ip_hash')) {
        loadBalancingMethod.value = 'ip_hash';
      } else if (upstreamContent.includes('least_conn')) {
        loadBalancingMethod.value = 'least_conn';
      } else {
        loadBalancingMethod.value = 'round_robin';
      }
      
      // 解析服务器列表
      const serverRegexInUpstream = /server\s+([^;]+);/g;
      const serverMatchesInUpstream = [...upstreamContent.matchAll(serverRegexInUpstream)];
      
      if (serverMatchesInUpstream.length > 0) {
        upstreamServers.value = [];
        
        for (const match of serverMatchesInUpstream) {
          const serverConfig = match[1];
          const server = {
            server: '',
            weight: 1,
            backup: false,
            down: false
          };
          
          // 提取服务器地址
          const parts = serverConfig.split(/\s+/);
          server.server = parts[0];
          
          // 提取额外参数
          for (let i = 1; i < parts.length; i++) {
            if (parts[i].startsWith('weight=')) {
              server.weight = parseInt(parts[i].substring(7)) || 1;
            } else if (parts[i] === 'backup') {
              server.backup = true;
            } else if (parts[i] === 'down') {
              server.down = true;
            }
          }
          
          upstreamServers.value.push(server);
        }
      }
    }
    
    // 特殊处理：预处理配置文件，标准化server块之间的分隔
    // 这是为了处理Certbot常见场景，如server { ... }server { ... }这种没有空格的情况
    let normalizedConfig = configContent.replace(/}(\s*)server\s*{/g, '}\n\nserver {');
    
    console.log("已预处理配置文件，规范化server块分隔");
    
    // 提取server块 - 使用更健壮的正则表达式
    const extractServerBlocks = (config) => {
      // 尝试多种正则表达式来提取server块
      const patterns = [
        // 标准匹配：包含嵌套花括号的server块
        /server\s*{([^{]*(?:{[^{]*}[^{]*)*?)}/g,
        // 简单匹配：无嵌套的server块
        /server\s*{([^}]*)}/g,
        // 保守匹配：从server开始到下一个server前或文件结束
        /server\s*{(.*?)(?=\s*server\s*{|$)/gs
      ];
      
      for (const pattern of patterns) {
        const matches = [...normalizedConfig.matchAll(pattern)];
        if (matches.length > 0) {
          return matches.map(match => `server {${match[1]}}`);
        }
      }
      
      // 如果所有正则都失败，尝试一个更激进的方法：按'server {'分割并重新组装
      const parts = normalizedConfig.split(/server\s*{/);
      if (parts.length > 1) {
        // 第一部分是server前的内容，忽略
        return parts.slice(1).map(part => {
          // 确保每个部分以}结尾
          let content = part;
          if (!content.trimEnd().endsWith('}')) {
            // 找到最外层的}
            const lastBraceIndex = content.lastIndexOf('}');
            if (lastBraceIndex !== -1) {
              content = content.substring(0, lastBraceIndex + 1);
            }
          }
          return `server {${content}`;
        });
      }
      
      return [];
    };
    
    const extractedServerBlocks = extractServerBlocks(normalizedConfig);
    
    if (extractedServerBlocks.length === 0) {
      parseSuccess.value = false;
      parseError.value = '未检测到有效的server块';
      console.error('无法解析server块，配置内容:', normalizedConfig);
      return false;
    }
    
    console.log(`检测到 ${extractedServerBlocks.length} 个server块`);
    
    // 检测多个server块
    if (extractedServerBlocks.length > 1) {
      hasMultipleServers.value = true;
      
      // 解析每个server块的头部信息
      const processedBlocks = extractedServerBlocks.map((block, index) => {
        const header = extractServerHeader(block);
        console.log(`Server块 ${index+1} 头部信息:`, header);
        return { 
          index, 
          block,
          header 
        };
      });
      
      // 更新全局状态
      serverBlocks.value = processedBlocks;
      
      // 默认选择第一个server块进行解析（通常是主HTTPS块）
      selectedServerIndex.value = 0;
      parseServerBlock(extractedServerBlocks[0]);
      console.log("多个server块模式：已设置默认选中第1个块");
      
      return true;
    } else {
      hasMultipleServers.value = false;
      serverBlocks.value = [{
        index: 0,
        block: extractedServerBlocks[0],
        header: extractServerHeader(extractedServerBlocks[0])
      }];
      selectedServerIndex.value = 0;
      parseServerBlock(extractedServerBlocks[0]);
      console.log("单个server块模式：已解析唯一的server块");
      
      return true;
    }
  } catch (error) {
    console.error('解析Nginx配置失败:', error);
    parseSuccess.value = false;
    parseError.value = `解析失败: ${error.message}`;
    return false;
  }
};

// 提取server块的基本信息用于显示
const extractServerHeader = (serverBlock) => {
  let header = "Server";
  let type = "普通";
  let protocol = "HTTP";
  let port = "80";
  
  // 尝试提取server_name
  const serverNameMatch = serverBlock.match(/server_name\s+([^;]+);/);
  let serverName = '';
  if (serverNameMatch && serverNameMatch[1] && serverNameMatch[1].trim() !== '_') {
    serverName = serverNameMatch[1].trim();
    if (serverName.includes(' ')) {
      // 如果有多个域名，使用第一个
      serverName = serverName.split(' ')[0];
    }
  }
  
  // 尝试提取端口
  const listenMatch = serverBlock.match(/listen\s+([0-9]+)/);
  if (listenMatch && listenMatch[1]) {
    port = listenMatch[1];
  }
  
  // 检查是否为HTTPS
  if (serverBlock.includes('ssl_certificate') || serverBlock.includes('listen 443 ssl')) {
    protocol = "HTTPS";
    port = "443";
  }
  
  // 检查是否为重定向
  if (serverBlock.includes('return 301 https://') || 
      (serverBlock.includes('if ($host =') && serverBlock.includes('return 301'))) {
    type = "重定向";
  }
  
  // 检查是否为Certbot管理
  const isCertbot = serverBlock.includes('managed by Certbot');
  
  // 构建显示名称
  if (serverName) {
    if (type === "重定向") {
      header = `${serverName} (HTTP → HTTPS 重定向)`;
    } else {
      header = `${serverName} (${protocol}:${port})`;
    }
    
    if (isCertbot) {
      header += " [TLS]";
    }
  } else {
    if (type === "重定向") {
      header = `HTTP → HTTPS 重定向 (端口 ${port})`;
    } else {
      header = `${protocol} 服务器 (端口 ${port})`;
    }
  }
  
  return header;
};

// 解析单个server块
const parseServerBlock = (serverBlock) => {
  try {
    // 解析server_name
    const serverNameMatch = serverBlock.match(/server_name\s+([^;]+);/);
    if (serverNameMatch && serverNameMatch[1]) {
      serverName.value = serverNameMatch[1].trim();
      if (serverName.value === '_') serverName.value = '';
    }
    
    // 解析端口
    const listenMatch = serverBlock.match(/listen\s+([0-9]+)/);
    if (listenMatch && listenMatch[1]) {
      port.value = parseInt(listenMatch[1]);
    }
    
    // 检查是否启用HTTPS
    const httpsMatch = serverBlock.match(/listen\s+([0-9]+)\s+ssl/);
    // 也检查是否有SSL证书配置，这是Certbot的标志
    const hasCertificate = serverBlock.includes('ssl_certificate');
    
    if (httpsMatch && httpsMatch[1]) {
      useHttps.value = true;
      httpsPort.value = parseInt(httpsMatch[1]);
    } else if (hasCertificate) {
      // 如果找到证书但没明确指定ssl端口，默认为443
      useHttps.value = true;
      httpsPort.value = 443;
    }
    
    if (useHttps.value) {
      // 解析SSL证书路径 - 支持Certbot风格的注释
      const sslCertMatch = serverBlock.match(/ssl_certificate\s+([^;]+);(\s*#[^;]*)?/);
      if (sslCertMatch && sslCertMatch[1]) {
        sslCertPath.value = sslCertMatch[1].trim();
      }
      
      // 解析SSL密钥路径 - 支持Certbot风格的注释
      const sslKeyMatch = serverBlock.match(/ssl_certificate_key\s+([^;]+);(\s*#[^;]*)?/);
      if (sslKeyMatch && sslKeyMatch[1]) {
        sslKeyPath.value = sslKeyMatch[1].trim();
      }
    }
    
    // 解析gzip设置
    enableGzip.value = serverBlock.includes('gzip on');
    
    // 解析client_max_body_size
    const maxBodySizeMatch = serverBlock.match(/client_max_body_size\s+([^;]+);/);
    if (maxBodySizeMatch && maxBodySizeMatch[1]) {
      clientMaxBodySize.value = maxBodySizeMatch[1].trim();
    }
    
    // 解析root目录
    const rootMatch = serverBlock.match(/root\s+([^;]+);/);
    if (rootMatch && rootMatch[1]) {
      staticRoot.value = rootMatch[1].trim();
      staticEnabled.value = true;
    } else {
      // 尝试在location块中查找root
      const locationRootMatch = serverBlock.match(/location\s+\/\s+{[^}]*root\s+([^;]+);/);
      if (locationRootMatch && locationRootMatch[1]) {
        staticRoot.value = locationRootMatch[1].trim();
        staticEnabled.value = true;
      } else {
        staticEnabled.value = false;
      }
    }
    
    // 解析index
    const indexMatch = serverBlock.match(/index\s+([^;]+);/);
    if (indexMatch && indexMatch[1]) {
      staticIndex.value = indexMatch[1].trim();
    }
    
    // 解析try_files (先检查全局，再检查location / 块)
    let tryFilesMatch = serverBlock.match(/try_files\s+\$uri\s+\$uri\/\s+([^;]+);/);
    if (!tryFilesMatch) {
      // 在location / 块中查找 - 更宽松的匹配
      tryFilesMatch = serverBlock.match(/location\s+\/\s+{[^}]*try_files\s+([^;]+);/);
    }
    
    if (tryFilesMatch && tryFilesMatch[1]) {
      staticTryFiles.value = true;
      // 提取最后一个参数作为回退
      const parts = tryFilesMatch[1].trim().split(/\s+/);
      if (parts.length > 0) {
        customTryFiles.value = parts[parts.length - 1];
      } else {
        customTryFiles.value = tryFilesMatch[1].trim();
      }
    }
    
    // 检查是否为重定向块 (Certbot HTTP→HTTPS重定向)
    const isRedirectBlock = serverBlock.includes('return 301 https://') || 
                          (serverBlock.includes('if ($host =') && serverBlock.includes('return 301'));
    
    if (isRedirectBlock) {
      // 这是重定向块，设置最小配置
      staticEnabled.value = false;
      proxyEnabled.value = false;
      return true;
    }
    
    // 解析反向代理设置 - 改进的正则表达式
    const locationBlocks = serverBlock.match(/location\s+[^{]+{[^}]*}/g) || [];
    const proxyEntries = [];
    
    // 检查每个location块
    for (const locationBlock of locationBlocks) {
      const pathMatch = locationBlock.match(/location\s+([^\s{]+)/);
      const proxyPassMatch = locationBlock.match(/proxy_pass\s+([^;]+);/);
      
      if (pathMatch && proxyPassMatch) {
        const path = pathMatch[1];
        const target = proxyPassMatch[1];
        
        // 检查特定的特性
        const isChangeOrigin = locationBlock.includes('Access-Control-Allow-Origin') || 
                              locationBlock.includes('proxy_set_header Host');
        const isStreaming = locationBlock.includes('proxy_buffering off') || 
                           locationBlock.includes('proxy_http_version 1.1') && 
                           locationBlock.includes('proxy_set_header Upgrade $http_upgrade');
        
        // 尝试检测路径重写
        let hasRewrite = false;
        let pathRewrite = '';
        
        // 简单重写检测
        if (path !== '/' && target.endsWith('/')) {
          hasRewrite = true;
          pathRewrite = `^${path}`;
        }
        
        proxyEntries.push({
          path,
          target,
          changeOrigin: isChangeOrigin,
          rewrite: hasRewrite,
          pathRewrite,
          streaming: isStreaming
        });
      }
    }
    
    if (proxyEntries.length > 0) {
      proxyEnabled.value = true;
      proxyPaths.value = proxyEntries;
    } else {
      proxyEnabled.value = false;
    }
    
    // 检查upstream块使用情况
    if (loadBalancingEnabled.value) {
      // 检查server块中是否引用了upstream
      const proxyToUpstreamMatch = serverBlock.match(new RegExp(`proxy_pass\\s+http://${upstreamName.value}`));
      if (!proxyToUpstreamMatch) {
        // 如果没有引用，则禁用负载均衡
        loadBalancingEnabled.value = false;
      }
    }
    
    return true;
  } catch (error) {
    console.error('解析server块失败:', error);
    return false;
  }
};

// 选择特定的server块解析
const selectServerBlock = (event) => {
  // 支持直接传入index或event对象
  const index = typeof event === 'object' ? event.target.value : event;
  
  if (index >= 0 && index < serverBlocks.value.length) {
    selectedServerIndex.value = index;
    parseServerBlock(serverBlocks.value[index].block);
  }
};

// 设置配置内容供外部访问
const setConfig = (configContent) => {
  console.log("开始设置配置内容:");
  
  const result = parseNginxConfig(configContent);
  
  // 检查数据绑定状态
  console.log("解析后的数据状态:", {
    hasMultipleServers: hasMultipleServers.value,
    selectedServerIndex: selectedServerIndex.value,
    serverBlocks: serverBlocks.value,
    parseSuccess: parseSuccess.value
  });
  
  return result;
};

// 获取生成的配置供外部访问
const getConfig = () => {
  return generatedConfig.value;
};

// 清空所有配置
const clearConfig = () => {
  serverName.value = '';
  port.value = 80;
  useHttps.value = false;
  httpsPort.value = 443;
  enableGzip.value = true;
  clientMaxBodySize.value = '1m';
  sslCertPath.value = '';
  sslKeyPath.value = '';
  staticEnabled.value = true;
  staticRoot.value = '/usr/share/nginx/html';
  staticIndex.value = 'index.html';
  staticTryFiles.value = true;
  customTryFiles.value = '/index.html';
  proxyEnabled.value = false;
  proxyPaths.value = [
    {
      path: '/api',
      target: 'http://localhost:8080',
      changeOrigin: true,
      rewrite: true,
      pathRewrite: '^/api',
      streaming: false,
    }
  ];
  loadBalancingEnabled.value = false;
  loadBalancingMethod.value = 'round_robin';
  upstreamName.value = 'backend';
  upstreamServers.value = [
    { server: 'localhost:8080', weight: 1, backup: false, down: false }
  ];
  customConfig.value = '';
  showAdvanced.value = false;
  activeTab.value = 'basic';
};

// 导出方法 - 合并两个defineExpose成一个
defineExpose({
  // 解析相关
  setConfig,
  getConfig,
  parseSuccess,
  parseError,
  hasMultipleServers,
  selectedServerIndex,
  serverBlocks,
  selectServerBlock,
  
  // 配置相关
  getConfigWithMode: () => showAdvanced.value ? customConfig.value : generatedConfig.value,
  clearConfig
});

// 添加代理路径
const addProxyPath = () => {
  proxyPaths.value.push({
    path: '/api',
    target: 'http://localhost:8080',
    changeOrigin: true,
    rewrite: true,
    pathRewrite: '^/api',
    streaming: false,
  });
};

// 删除代理路径
const removeProxyPath = (index) => {
  proxyPaths.value.splice(index, 1);
};

// 添加上游服务器
const addUpstreamServer = () => {
  upstreamServers.value.push({
    server: 'localhost:8080',
    weight: 1,
    backup: false,
    down: false
  });
};

// 删除上游服务器
const removeUpstreamServer = (index) => {
  upstreamServers.value.splice(index, 1);
};

// 生成配置
const generatedConfig = computed(() => {
  let config = '';

  // 基本设置
  config += `# 基本配置\n`;
  config += `server {\n`;
  config += `    listen ${port.value};\n`;
  
  if (useHttps.value) {
    config += `    listen ${httpsPort.value} ssl;\n`;
    config += `    ssl_certificate ${sslCertPath.value};\n`;
    config += `    ssl_certificate_key ${sslKeyPath.value};\n`;
    config += `    ssl_protocols TLSv1.2 TLSv1.3;\n`;
    config += `    ssl_prefer_server_ciphers on;\n`;
  }
  
  config += `    server_name ${serverName.value || '_'};\n\n`;
  
  // 通用设置
  config += `    # 通用设置\n`;
  config += `    client_max_body_size ${clientMaxBodySize.value};\n`;
  
  if (enableGzip.value) {
    config += `    gzip on;\n`;
    config += `    gzip_disable "msie6";\n`;
    config += `    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;\n`;
  }
  
  config += `\n`;
  
  // 负载均衡配置（如果启用）
  if (loadBalancingEnabled.value && upstreamServers.value.length > 0) {
    config = `# 上游服务器配置\nupstream ${upstreamName.value} {\n`;
    
    // 负载均衡方法
    if (loadBalancingMethod.value === 'ip_hash') {
      config += `    ip_hash;\n`;
    } else if (loadBalancingMethod.value === 'least_conn') {
      config += `    least_conn;\n`;
    }
    
    // 服务器列表
    upstreamServers.value.forEach(server => {
      let serverConfig = `    server ${server.server}`;
      
      if (server.weight > 1) {
        serverConfig += ` weight=${server.weight}`;
      }
      
      if (server.backup) {
        serverConfig += ` backup`;
      }
      
      if (server.down) {
        serverConfig += ` down`;
      }
      
      serverConfig += `;\n`;
      config += serverConfig;
    });
    
    config += `}\n\n# 服务器配置\nserver {\n`;
    config += `    listen ${port.value};\n`;
    
    if (useHttps.value) {
      config += `    listen ${httpsPort.value} ssl;\n`;
      config += `    ssl_certificate ${sslCertPath.value};\n`;
      config += `    ssl_certificate_key ${sslKeyPath.value};\n`;
      config += `    ssl_protocols TLSv1.2 TLSv1.3;\n`;
      config += `    ssl_prefer_server_ciphers on;\n`;
    }
    
    config += `    server_name ${serverName.value || '_'};\n\n`;
    config += `    # 通用设置\n`;
    config += `    client_max_body_size ${clientMaxBodySize.value};\n`;
    
    if (enableGzip.value) {
      config += `    gzip on;\n`;
      config += `    gzip_disable "msie6";\n`;
      config += `    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;\n`;
    }
    
    config += `\n`;
  }
  
  // 前端静态资源配置
  if (staticEnabled.value) {
    config += `    # 静态资源配置\n`;
    config += `    root ${staticRoot.value};\n`;
    config += `    index ${staticIndex.value};\n`;
    
    if (staticTryFiles.value) {
      config += `    # 单页应用支持\n`;
      config += `    location / {\n`;
      config += `        try_files $uri $uri/ ${customTryFiles.value};\n`;
      config += `    }\n\n`;
    }
  }
  
  // 反向代理配置
  if (proxyEnabled.value && proxyPaths.value.length > 0) {
    config += `    # 反向代理配置\n`;
    
    proxyPaths.value.forEach(proxy => {
      config += `    location ${proxy.path} {\n`;
      
      // 如果使用负载均衡
      if (loadBalancingEnabled.value) {
        config += `        proxy_pass http://${upstreamName.value}`;
      } else {
        config += `        proxy_pass ${proxy.target}`;
      }
      
      // 是否需要URL重写
      if (proxy.rewrite) {
        config += `${proxy.pathRewrite}`;
      }
      
      config += `;\n`;
      config += `        proxy_set_header Host $host;\n`;
      config += `        proxy_set_header X-Real-IP $remote_addr;\n`;
      config += `        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n`;
      config += `        proxy_set_header X-Forwarded-Proto $scheme;\n`;
      
      // 流式输出配置
      if (proxy.streaming) {
        config += `        # 流式输出配置\n`;
        config += `        proxy_buffering off;\n`;
        config += `        proxy_cache off;\n`;
      }
      
      if (proxy.changeOrigin) {
        config += `        # 支持跨域\n`;
        config += `        add_header 'Access-Control-Allow-Origin' '*';\n`;
        config += `        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS, PUT, DELETE';\n`;
        config += `        add_header 'Access-Control-Allow-Headers' 'DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Authorization';\n`;
        
        config += `        if ($request_method = 'OPTIONS') {\n`;
        config += `            add_header 'Access-Control-Max-Age' 1728000;\n`;
        config += `            add_header 'Content-Type' 'text/plain charset=UTF-8';\n`;
        config += `            add_header 'Content-Length' 0;\n`;
        config += `            return 204;\n`;
        config += `        }\n`;
      }
      
      config += `    }\n\n`;
    });
  }
  
  config += `}\n`;
  
  return config;
});

// 在高级模式下更新配置
const updateCustomConfig = () => {
  customConfig.value = generatedConfig.value;
};

// 切换到高级模式时生成配置
const handleTabChange = (key) => {
  if (key === 'advanced' && customConfig.value === '') {
    updateCustomConfig();
  }
};

// 调试信息
const debugShowServerBlocks = () => {
  console.log('当前配置包含的Server块:', serverBlocks.value);
  console.log('当前选中: 第', selectedServerIndex.value + 1, '个Server块');
};
</script>

<template>
  <div class="nginx-config-generator">
    <!-- 解析错误提示 -->
    <a-alert
      v-if="!parseSuccess"
      type="error"
      show-icon
      message="解析配置失败"
      :description="parseError || '无法正确解析Nginx配置文件，请检查格式或使用手动编辑模式。'"
      class="glass-card error-card"
    />
    
    <!-- 多个Server块选择 -->
    <div v-if="hasMultipleServers" class="glass-card server-selector-card">
      <div class="section-title">
        <InfoCircleOutlined />
        <span>检测到多个Server块</span>
      </div>
      
      <p class="section-description">
        当前配置文件包含多个Server块，请选择需要编辑的Server
      </p>
      
      <!-- 服务器块列表 - 直接列表形式 -->
      <div class="server-blocks-list">
        <div 
          v-for="(server, index) in serverBlocks" 
          :key="index"
          class="server-block-item glass-card-small"
          :class="{ 'active': selectedServerIndex === index }"
          @click="selectedServerIndex = index; selectServerBlock(index);"
        >
          <div class="server-block-header">
            {{ server.header || `Server块 ${index+1}` }}
          </div>
        </div>
      </div>
      
      <div class="helper-text">
        <BulbOutlined />
        <span>点击选择要编辑的Server块，您可以随时切换不同的Server块进行编辑</span>
      </div>
    </div>
    
    <!-- 配置选项 -->
    <div class="glass-card main-config-card">
      <a-tabs v-model:activeKey="activeTab" class="modern-tabs">
        <!-- 基础设置 -->
        <a-tab-pane key="basic" tab="基础设置">
          <div class="tab-content">
            <div class="config-section glass-card-inner">
              <div class="section-title">服务器配置</div>
              <a-form layout="vertical">
                <a-row :gutter="16">
                  <a-col :span="12">
                    <a-form-item label="Server Name">
                      <a-input v-model:value="serverName" placeholder="example.com" class="modern-input" />
                      <span class="helper-text">多个域名使用空格分隔</span>
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="端口">
                      <a-input-number v-model:value="port" :min="1" :max="65535" style="width: 100%" class="modern-input-number" />
                    </a-form-item>
                  </a-col>
                </a-row>
                
                <a-form-item label="Max Body Size">
                  <a-input v-model:value="clientMaxBodySize" placeholder="1m" class="modern-input" />
                  <span class="helper-text">客户端请求体大小限制</span>
                </a-form-item>
                
                <a-form-item>
                  <a-checkbox v-model:checked="enableGzip" class="modern-checkbox">启用Gzip压缩</a-checkbox>
                </a-form-item>
              </a-form>
            </div>
            
            <div class="config-section glass-card-inner">
              <div class="section-title">HTTPS设置</div>
              <a-form layout="vertical">
                <a-form-item>
                  <a-checkbox v-model:checked="useHttps" class="modern-checkbox">启用HTTPS</a-checkbox>
                </a-form-item>
                
                <a-form-item v-if="useHttps" label="HTTPS端口">
                  <a-input-number v-model:value="httpsPort" :min="1" :max="65535" style="width: 100%" class="modern-input-number" />
                </a-form-item>
                
                <a-form-item v-if="useHttps" label="SSL证书路径">
                  <a-input v-model:value="sslCertPath" placeholder="/etc/letsencrypt/live/example.com/fullchain.pem" class="modern-input" />
                </a-form-item>
                
                <a-form-item v-if="useHttps" label="SSL密钥路径">
                  <a-input v-model:value="sslKeyPath" placeholder="/etc/letsencrypt/live/example.com/privkey.pem" class="modern-input" />
                </a-form-item>
              </a-form>
            </div>
          </div>
        </a-tab-pane>
        
        <!-- 静态资源 -->
        <a-tab-pane key="static" tab="静态资源">
          <div class="tab-content">
            <div class="config-section glass-card-inner">
              <div class="section-title">静态资源配置</div>
              <a-form layout="vertical">
                <a-form-item>
                  <a-checkbox v-model:checked="staticEnabled" class="modern-checkbox">启用静态资源服务</a-checkbox>
                </a-form-item>
                
                <a-form-item v-if="staticEnabled" label="网站根目录">
                  <a-input v-model:value="staticRoot" placeholder="/usr/share/nginx/html" class="modern-input" />
                </a-form-item>
                
                <a-form-item v-if="staticEnabled" label="索引文件">
                  <a-input v-model:value="staticIndex" placeholder="index.html index.htm" class="modern-input" />
                </a-form-item>
                
                <a-form-item v-if="staticEnabled">
                  <a-checkbox v-model:checked="staticTryFiles" class="modern-checkbox">启用单页应用支持 (try_files)</a-checkbox>
                  <a-tooltip v-if="staticTryFiles">
                    <template #title>
                      对于Vue, React等单页应用，可以将所有请求转发到index.html
                    </template>
                    <a-button type="link" size="small" class="info-button">
                      <question-circle-outlined />
                    </a-button>
                  </a-tooltip>
                </a-form-item>
                
                <a-form-item v-if="staticEnabled && staticTryFiles" label="伪静态路径">
                  <a-input v-model:value="customTryFiles" placeholder="/index.html" class="modern-input" />
                </a-form-item>
              </a-form>
            </div>
          </div>
        </a-tab-pane>
        
        <!-- 反向代理 -->
        <a-tab-pane key="proxy" tab="反向代理">
          <div class="tab-content">
            <div class="config-section glass-card-inner">
              <div class="section-title">反向代理配置</div>
              <a-form layout="vertical">
                <a-form-item>
                  <a-checkbox v-model:checked="proxyEnabled" class="modern-checkbox">启用反向代理</a-checkbox>
                </a-form-item>
                
                <div v-if="proxyEnabled">
                  <div class="subsection-title">代理路径设置</div>
                  
                  <div v-for="(proxy, index) in proxyPaths" :key="index" class="proxy-item glass-card-small">
                    <a-row :gutter="16">
                      <a-col :span="8">
                        <a-form-item label="路径">
                          <a-input v-model:value="proxy.path" placeholder="/api" class="modern-input" />
                        </a-form-item>
                      </a-col>
                      <a-col :span="16">
                        <a-form-item label="目标地址">
                          <a-input v-model:value="proxy.target" placeholder="http://localhost:8080" class="modern-input" />
                        </a-form-item>
                      </a-col>
                    </a-row>
                    
                    <a-row :gutter="16">
                      <a-col :span="12">
                        <a-form-item>
                          <a-checkbox v-model:checked="proxy.changeOrigin" class="modern-checkbox">支持跨域 (CORS)</a-checkbox>
                        </a-form-item>
                      </a-col>
                      <a-col :span="12">
                        <a-form-item>
                          <a-checkbox v-model:checked="proxy.streaming" class="modern-checkbox">流式输出 (SSE/WebSocket)</a-checkbox>
                        </a-form-item>
                      </a-col>
                    </a-row>
                    
                    <a-row :gutter="16">
                      <a-col :span="18">
                        <a-form-item>
                          <a-checkbox v-model:checked="proxy.rewrite" class="modern-checkbox">路径重写</a-checkbox>
                          <a-input 
                            v-if="proxy.rewrite" 
                            v-model:value="proxy.pathRewrite" 
                            placeholder="^/api" 
                            class="modern-input"
                            style="margin-top: 8px"
                          />
                        </a-form-item>
                      </a-col>
                      <a-col :span="6" style="text-align: right">
                        <a-button 
                          v-if="index > 0"
                          type="danger" 
                          @click="removeProxyPath(index)"
                          class="modern-button-danger"
                        >
                          <template #icon><minus-circle-outlined /></template>
                          删除
                        </a-button>
                      </a-col>
                    </a-row>
                  </div>
                  
                  <a-button 
                    type="dashed" 
                    block 
                    @click="addProxyPath" 
                    class="modern-button-dashed"
                  >
                    <template #icon><plus-outlined /></template>
                    添加代理路径
                  </a-button>
                </div>
              </a-form>
            </div>
          </div>
        </a-tab-pane>
        
        <!-- 负载均衡 -->
        <a-tab-pane key="upstream" tab="负载均衡">
          <div class="tab-content">
            <div class="config-section glass-card-inner">
              <div class="section-title">负载均衡配置</div>
              <a-form layout="vertical">
                <a-form-item>
                  <a-checkbox v-model:checked="loadBalancingEnabled" class="modern-checkbox">启用负载均衡</a-checkbox>
                </a-form-item>
                
                <div v-if="loadBalancingEnabled">
                  <a-form-item label="Upstream名称">
                    <a-input v-model:value="upstreamName" placeholder="backend" class="modern-input" />
                  </a-form-item>
                  
                  <a-form-item label="负载均衡方法">
                    <a-radio-group v-model:value="loadBalancingMethod" class="modern-radio-group">
                      <a-radio value="round_robin">Round Robin (默认)</a-radio>
                      <a-radio value="ip_hash">IP Hash</a-radio>
                      <a-radio value="least_conn">Least Connections</a-radio>
                    </a-radio-group>
                  </a-form-item>
                  
                  <div class="subsection-title">上游服务器</div>
                  
                  <div v-for="(server, index) in upstreamServers" :key="index" class="upstream-item glass-card-small">
                    <a-row :gutter="16">
                      <a-col :span="12">
                        <a-form-item label="服务器地址">
                          <a-input v-model:value="server.server" placeholder="localhost:8080" class="modern-input" />
                        </a-form-item>
                      </a-col>
                      <a-col :span="12">
                        <a-form-item label="权重">
                          <a-input-number v-model:value="server.weight" :min="1" :max="100" style="width: 100%" class="modern-input-number" />
                        </a-form-item>
                      </a-col>
                    </a-row>
                    
                    <a-row :gutter="16">
                      <a-col :span="8">
                        <a-form-item>
                          <a-checkbox v-model:checked="server.backup" class="modern-checkbox">备用服务器</a-checkbox>
                        </a-form-item>
                      </a-col>
                      <a-col :span="8">
                        <a-form-item>
                          <a-checkbox v-model:checked="server.down" class="modern-checkbox">标记为下线</a-checkbox>
                        </a-form-item>
                      </a-col>
                      <a-col :span="8" style="text-align: right">
                        <a-button 
                          v-if="index > 0"
                          type="danger" 
                          @click="removeUpstreamServer(index)"
                          class="modern-button-danger"
                        >
                          <template #icon><minus-circle-outlined /></template>
                          删除
                        </a-button>
                      </a-col>
                    </a-row>
                  </div>
                  
                  <a-button 
                    type="dashed" 
                    block 
                    @click="addUpstreamServer" 
                    class="modern-button-dashed"
                  >
                    <template #icon><plus-outlined /></template>
                    添加服务器
                  </a-button>
                </div>
              </a-form>
            </div>
          </div>
        </a-tab-pane>
        
        <!-- 高级设置 -->
        <a-tab-pane key="advanced" tab="高级设置">
          <div class="tab-content">
            <div class="config-section glass-card-inner">
              <div class="section-title">自定义配置</div>
              <a-form layout="vertical">
                <a-form-item label="自定义Nginx配置">
                  <a-textarea
                    v-model:value="customConfig"
                    placeholder="在这里添加自定义配置"
                    :rows="10"
                    :autoSize="{ minRows: 10, maxRows: 20 }"
                    class="modern-textarea code-area"
                  />
                  <span class="helper-text">自定义配置将直接添加到server块内</span>
                </a-form-item>
              </a-form>
            </div>
          </div>
        </a-tab-pane>
      </a-tabs>
    </div>
    
    <!-- 预览 -->
    <div class="glass-card preview-card">
      <div class="section-title">
        <EyeOutlined />
        <span>配置预览</span>
      </div>
      <div class="preview-container">
        <a-textarea
          :value="generatedConfig"
          readonly
          :autoSize="{ minRows: 10, maxRows: 20 }"
          class="modern-textarea code-area"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.nginx-config-generator {
  padding: 24px;
  background-color: #f5f8fa;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 毛玻璃效果卡片 */
.glass-card {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(var(--blur-sm));
  border-radius: var(--radius-lg);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.18);
  padding: 24px;
  transition: all 0.3s ease;
}

.glass-card:hover {
  box-shadow: 0 8px 32px rgba(31, 38, 135, 0.15);
  transform: translateY(-2px);
}

.glass-card-inner {
  background: rgba(255, 255, 255, 0.7);
  border-radius: var(--radius-md);
  box-shadow: 0 4px 12px var(--alpha-black-05);
  border: 1px solid var(--alpha-white-50);
  padding: 20px;
  margin-bottom: 20px;
  transition: all 0.3s ease;
}

.glass-card-small {
  background: var(--alpha-white-80);
  border-radius: var(--radius-md);
  box-shadow: 0 4px 12px var(--alpha-black-05);
  border: 1px solid var(--alpha-white-50);
  padding: 16px;
  margin-bottom: 16px;
  transition: all 0.3s ease;
}

.glass-card-small:hover {
  box-shadow: 0 4px 20px var(--alpha-black-10);
}

/* 特殊卡片样式 */
.error-card {
  background: rgba(255, 240, 240, 0.9);
  border-left: 4px solid var(--error-color);
}

.server-selector-card {
  background: rgba(240, 248, 255, 0.9);
}

.main-config-card {
  flex: 1;
}

.preview-card {
  background: rgba(245, 252, 248, 0.9);
}

/* 标题样式 */
.section-title {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: #2c3e50;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.section-title :deep(svg) {
  color: #4096ff;
}

.subsection-title {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-medium);
  color: #2c3e50;
  margin: 16px 0 12px;
}

.section-description {
  color: #5e6d82;
  margin-bottom: 16px;
}

/* 帮助文本 */
.helper-text {
  color: #8492a6;
  font-size: var(--font-size-xs);
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.helper-text :deep(svg) {
  color: #ffc53d;
}

/* 现代表单控件 */
.modern-input {
  border-radius: var(--radius-sm);
  height: 40px;
  transition: all 0.3s;
}

.modern-input:hover, .modern-input:focus {
  border-color: #4096ff;
  box-shadow: 0 0 0 2px rgba(64, 150, 255, 0.1);
}

.modern-input :deep(.ant-input) {
  border: none !important;
  box-shadow: none !important;
  background: transparent !important;
  border-radius: 0 !important;
}

.modern-input-number {
  border-radius: var(--radius-sm);
  height: 40px;
}

.modern-input-number :deep(.ant-input-number-input) {
  border: none !important;
  box-shadow: none !important;
  background: transparent !important;
  border-radius: 0 !important;
}

.modern-textarea {
  border-radius: var(--radius-sm);
  transition: all 0.3s;
}

.modern-textarea:hover, .modern-textarea:focus {
  border-color: #4096ff;
  box-shadow: 0 0 0 2px rgba(64, 150, 255, 0.1);
}

.code-area {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: var(--font-size-sm);
}

.modern-checkbox {
  font-weight: var(--font-weight-medium);
}

.modern-radio-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* 按钮样式 */
.modern-button-dashed {
  border-radius: var(--radius-sm);
  height: 40px;
  border-style: dashed;
  color: #4096ff;
  border-color: #4096ff;
  background: rgba(64, 150, 255, 0.05);
}

.modern-button-dashed:hover {
  color: var(--primary-color);
  border-color: var(--primary-color);
  background: rgba(22, 119, 255, 0.1);
}

.modern-button-danger {
  border-radius: var(--radius-sm);
  background: rgba(255, 77, 79, 0.05);
}

.info-button {
  color: #4096ff;
}

/* 标签页样式 */
.modern-tabs :deep(.ant-tabs-nav::before) {
  border-bottom: none;
}

.modern-tabs :deep(.ant-tabs-tab) {
  padding: 12px 16px;
  font-size: 15px;
  transition: all 0.3s;
}

.modern-tabs :deep(.ant-tabs-tab-active) {
  font-weight: var(--font-weight-semibold);
}

.modern-tabs :deep(.ant-tabs-ink-bar) {
  height: 3px;
  border-radius: 3px;
  background: linear-gradient(90deg, #4096ff, var(--primary-color));
}

.tab-content {
  padding: 16px 0;
}

/* 服务器块选择器样式 */
.server-blocks-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 16px;
}

.server-block-item {
  padding: 16px;
  cursor: pointer;
  transition: all 0.3s;
}

.server-block-item:hover {
  background: rgba(240, 248, 255, 0.95);
  border-color: #4096ff;
}

.server-block-item.active {
  background: rgba(230, 244, 255, 0.9);
  border-color: #4096ff;
  box-shadow: 0 4px 16px rgba(64, 150, 255, 0.2), 0 0 0 1px rgba(64, 150, 255, 0.4);
}

.server-block-header {
  font-weight: var(--font-weight-medium);
  color: #2c3e50;
  font-size: 15px;
}

.server-block-item.active .server-block-header {
  color: var(--primary-color);
  font-weight: var(--font-weight-semibold);
}

/* 代理和上游项目容器样式 */
.proxy-item, .upstream-item {
  transition: all 0.3s;
}

/* 预览容器 */
.preview-container {
  margin-top: 16px;
  position: relative;
}

/* 辉光效果 */
.glass-card.preview-card:hover .code-area {
  box-shadow: 0 0 15px rgba(64, 150, 255, 0.15);
}
</style>
