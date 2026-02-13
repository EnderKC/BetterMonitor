<script setup lang="ts">
import { ref, computed } from 'vue';
import { message } from 'ant-design-vue';
import {
    CopyOutlined,
    CheckOutlined,
    AppleOutlined,
    WindowsOutlined,
    CodeOutlined,
    InfoCircleOutlined
} from '@ant-design/icons-vue';

const props = defineProps<{
    visible: boolean;
    server: any;
    agentReleaseRepo?: string;
}>();

const emit = defineEmits(['update:visible', 'close']);

const activeTab = ref('linux');
const copiedId = ref(false);
const copiedKey = ref(false);
const copiedCmd = ref(false);

const serverId = computed(() => props.server?.id || props.server?.ID || '');
const secretKey = computed(() => props.server?.secret_key || props.server?.SecretKey || '');
const agentType = computed(() => props.server?.agent_type || props.server?.AgentType || 'full');
const dashboardUrl = window.location.origin;
const releaseRepo = computed(() => props.agentReleaseRepo || 'EnderKC/BetterMonitor');

const agentTypeFlag = computed(() => agentType.value === 'monitor' ? ' --agent-type monitor' : '');

const installCmdLinux = computed(() =>
    `curl -fsSL https://raw.githubusercontent.com/${releaseRepo.value}/refs/heads/main/install-agent.sh | bash -s -- --server-id ${serverId.value} --secret-key ${secretKey.value} --server ${dashboardUrl}${agentTypeFlag.value}`
);

const installCmdWindows = computed(() =>
    `irm https://raw.githubusercontent.com/${releaseRepo.value}/main/install-agent.ps1 | iex -ServerUrl "${dashboardUrl}" -ServerId ${serverId.value} -SecretKey "${secretKey.value}"${agentTypeFlag.value}`
);

const handleClose = () => {
    emit('update:visible', false);
    emit('close');
};

const copyToClipboard = async (text: string, type: 'id' | 'key' | 'cmd') => {
    try {
        await navigator.clipboard.writeText(text);
        if (type === 'id') {
            copiedId.value = true;
            setTimeout(() => copiedId.value = false, 2000);
        } else if (type === 'key') {
            copiedKey.value = true;
            setTimeout(() => copiedKey.value = false, 2000);
        } else if (type === 'cmd') {
            copiedCmd.value = true;
            setTimeout(() => copiedCmd.value = false, 2000);
        }
        message.success('已复制到剪贴板');
    } catch (err) {
        message.error('复制失败');
    }
};
</script>

<template>
    <a-modal :visible="visible" :footer="null" :width="680" @cancel="handleClose" class="deploy-agent-modal"
        :maskClosable="true" centered>
        <div class="modal-content-wrapper">
            <!-- Header -->
            <div class="modal-header">
                <div class="header-icon">
                    <CodeOutlined />
                </div>
                <div class="header-text">
                    <h3>部署 Agent</h3>
                    <p>在您的服务器上安装 Agent 以开始监控</p>
                </div>
            </div>

            <!-- Server Info Card -->
            <div class="info-card glass-panel">
                <div class="info-row">
                    <div class="info-label">Server ID</div>
                    <div class="info-value-container">
                        <code class="info-value">{{ serverId }}</code>
                        <button class="copy-btn-mini" @click="copyToClipboard(String(serverId), 'id')">
                            <CheckOutlined v-if="copiedId" />
                            <CopyOutlined v-else />
                        </button>
                    </div>
                </div>
                <div class="info-divider"></div>
                <div class="info-row">
                    <div class="info-label">Secret Key</div>
                    <div class="info-value-container">
                        <code class="info-value">{{ secretKey }}</code>
                        <button class="copy-btn-mini" @click="copyToClipboard(secretKey, 'key')">
                            <CheckOutlined v-if="copiedKey" />
                            <CopyOutlined v-else />
                        </button>
                    </div>
                </div>
            </div>

            <!-- OS Selection Tabs -->
            <div class="os-tabs">
                <div class="os-tab" :class="{ active: activeTab === 'linux' }" @click="activeTab = 'linux'">
                    <AppleOutlined class="os-icon" />
                    <span>Linux / macOS</span>
                </div>
                <div class="os-tab" :class="{ active: activeTab === 'windows' }" @click="activeTab = 'windows'">
                    <WindowsOutlined class="os-icon" />
                    <span>Windows</span>
                </div>
            </div>

            <!-- Command Block -->
            <div class="command-panel glass-panel">
                <div class="panel-header">
                    <div class="traffic-lights">
                        <span class="light red"></span>
                        <span class="light yellow"></span>
                        <span class="light green"></span>
                    </div>
                    <span class="panel-title">{{ activeTab === 'linux' ? 'Terminal' : 'PowerShell' }}</span>
                </div>
                <div class="code-block">
                    <pre>{{ activeTab === 'linux' ? installCmdLinux : installCmdWindows }}</pre>
                </div>
                <div class="panel-footer">
                    <div class="cmd-tip">
                        <InfoCircleOutlined />
                        <span>{{ activeTab === 'linux' ? '一键安装脚本，自动配置开机自启' : '请以管理员身份运行 PowerShell' }}</span>
                    </div>
                    <div v-if="agentType === 'monitor'" class="cmd-tip" style="margin-top: 4px;">
                        <InfoCircleOutlined />
                        <span>此服务器将安装最小监控版 Agent</span>
                    </div>
                    <button class="action-copy-btn"
                        @click="copyToClipboard(activeTab === 'linux' ? installCmdLinux : installCmdWindows, 'cmd')">
                        <CheckOutlined v-if="copiedCmd" />
                        <span v-if="copiedCmd">已复制</span>
                        <template v-else>
                            <CopyOutlined />
                            <span>复制命令</span>
                        </template>
                    </button>
                </div>
            </div>
        </div>
    </a-modal>
</template>

<style scoped>
/* Modal Base Styles */
.deploy-agent-modal :deep(.ant-modal-content) {
    background: transparent;
    box-shadow: none;
    padding: 0;
}

.deploy-agent-modal :deep(.ant-modal-close) {
    top: 24px;
    right: 24px;
    color: var(--text-secondary);
    background: var(--alpha-black-05);
    border-radius: var(--radius-circle);
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.3s ease;
}

.deploy-agent-modal :deep(.ant-modal-close:hover) {
    background: var(--alpha-black-10);
    color: var(--text-primary);
}

.modal-content-wrapper {
    background: var(--alpha-white-80);
    backdrop-filter: blur(24px);
    -webkit-backdrop-filter: blur(24px);
    border-radius: var(--radius-xl);
    padding: 32px;
    box-shadow:
        0 25px 50px -12px rgba(0, 0, 0, 0.25),
        0 0 0 1px var(--alpha-white-50) inset;
    border: 1px solid var(--alpha-white-30);
}

/* Header */
.modal-header {
    display: flex;
    align-items: flex-start;
    gap: 16px;
    margin-bottom: 32px;
}

.header-icon {
    width: 48px;
    height: 48px;
    border-radius: 14px;
    background: linear-gradient(135deg, var(--primary-color), var(--info-color));
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
    font-size: var(--font-size-3xl);
    box-shadow: 0 8px 16px rgba(0, 122, 255, 0.25);
}

.header-text h3 {
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-bold);
    margin: 0 0 4px 0;
    color: #1d1d1f;
    letter-spacing: -0.01em;
}

.header-text p {
    margin: 0;
    color: #86868b;
    font-size: var(--font-size-md);
}

/* Glass Panel */
.glass-panel {
    background: var(--alpha-white-50);
    border-radius: var(--radius-lg);
    border: 1px solid var(--alpha-black-05);
    overflow: hidden;
}

/* Info Card */
.info-card {
    margin-bottom: 24px;
    background: rgba(245, 245, 247, 0.5);
}

.info-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px;
}

.info-label {
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-semibold);
    color: #86868b;
    text-transform: uppercase;
    letter-spacing: 0.02em;
}

.info-value-container {
    display: flex;
    align-items: center;
    gap: 12px;
}

.info-value {
    font-family: 'SF Mono', SFMono-Regular, ui-monospace, monospace;
    font-size: var(--font-size-md);
    color: #1d1d1f;
    background: var(--alpha-black-05);
    padding: 4px 8px;
    border-radius: var(--radius-xs);
}

.copy-btn-mini {
    border: none;
    background: transparent;
    color: var(--primary-color);
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
    transition: all 0.2s;
    display: flex;
    align-items: center;
}

.copy-btn-mini:hover {
    background: var(--primary-light);
}

.info-divider {
    height: 1px;
    background: var(--alpha-black-05);
    margin: 0 20px;
}

/* Tabs */
.os-tabs {
    display: flex;
    gap: 8px;
    margin-bottom: 16px;
    background: rgba(118, 118, 128, 0.12);
    padding: 4px;
    border-radius: var(--radius-md);
    width: fit-content;
}

.os-tab {
    padding: 8px 16px;
    border-radius: var(--radius-sm);
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-semibold);
    color: #86868b;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 8px;
    transition: all 0.3s ease;
}

.os-tab.active {
    background: white;
    color: #1d1d1f;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

/* Command Panel */
.command-panel {
    background: #1e1e1e;
    /* Dark terminal background */
    border: 1px solid var(--alpha-white-10);
    box-shadow: 0 20px 40px var(--alpha-black-20);
}

.panel-header {
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    padding: 12px 16px;
    border-bottom: 1px solid var(--alpha-white-10);
}

.traffic-lights {
    position: absolute;
    left: 16px;
    display: flex;
    gap: 6px;
}

.light {
    width: 10px;
    height: 10px;
    border-radius: var(--radius-circle);
}

.light.red {
    background: #FF5F57;
}

.light.yellow {
    background: #FEBC2E;
}

.light.green {
    background: #28C840;
}

.panel-title {
    color: var(--alpha-white-40);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
}

.code-block {
    padding: 20px;
    overflow-x: auto;
}

.code-block pre {
    margin: 0;
    font-family: 'SF Mono', SFMono-Regular, ui-monospace, monospace;
    font-size: var(--font-size-sm);
    line-height: 1.6;
    color: #fff;
    white-space: pre-wrap;
    word-break: break-all;
}

.panel-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    background: var(--alpha-white-05);
    border-top: 1px solid var(--alpha-white-10);
}

.cmd-tip {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--alpha-white-50);
    font-size: var(--font-size-xs);
}

.action-copy-btn {
    background: var(--alpha-white-10);
    border: 1px solid var(--alpha-white-10);
    color: white;
    padding: 6px 12px;
    border-radius: var(--radius-xs);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 6px;
    transition: all 0.2s;
}

.action-copy-btn:hover {
    background: var(--alpha-white-20);
}
</style>

<style>
/* Dark Mode Overrides */
.dark .modal-content-wrapper {
    background: rgba(30, 30, 32, 0.85);
    border: 1px solid var(--alpha-white-10);
    box-shadow:
        0 25px 50px -12px var(--alpha-black-50),
        0 0 0 1px var(--alpha-white-10) inset;
}

.dark .header-text h3 {
    color: #fff;
}

.dark .header-text p {
    color: #a1a1a6;
}

.dark .info-card {
    background: var(--alpha-white-05);
    border-color: var(--alpha-white-05);
}

.dark .info-label {
    color: #86868b;
}

.dark .info-value {
    color: #fff;
    background: var(--alpha-black-30);
}

.dark .info-divider {
    background: var(--alpha-white-05);
}

.dark .os-tabs {
    background: var(--alpha-black-30);
}

.dark .os-tab.active {
    background: var(--alpha-white-10);
    color: #fff;
}

.dark .deploy-agent-modal .ant-modal-close {
    color: var(--alpha-white-50);
    background: var(--alpha-white-05);
}

.dark .deploy-agent-modal .ant-modal-close:hover {
    background: var(--alpha-white-15);
    color: #fff;
}

.dark .copy-btn-mini {
    color: #0A84FF;
    /* Apple systemBlue dark mode */
}

.dark .copy-btn-mini:hover {
    background: rgba(10, 132, 255, 0.15);
}

.dark .command-panel {
    background: #1e1e1e;
    border-color: var(--alpha-white-10);
}

.dark .panel-header {
    border-bottom-color: var(--alpha-white-10);
}

.dark .panel-footer {
    background: rgba(255, 255, 255, 0.02);
    border-top-color: var(--alpha-white-10);
}

.dark .action-copy-btn {
    background: var(--alpha-white-08);
    border-color: var(--alpha-white-10);
}

.dark .action-copy-btn:hover {
    background: var(--alpha-white-15);
}
</style>
