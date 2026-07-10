import { flushPromises, mount } from '@vue/test-utils';
import { reactive } from 'vue';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMocks = vi.hoisted(() => ({
  getDashboardVersion: vi.fn(),
  getSystemInfo: vi.fn(),
  getServersVersions: vi.fn(),
  getLatestAgentRelease: vi.fn(),
}));

const upgradeMocks = vi.hoisted(() => ({
  submit: vi.fn(),
  loadActive: vi.fn(),
  track: vi.fn(),
  statusForServer: vi.fn(),
  stop: vi.fn(),
  jobs: new Map(),
}));

const messageMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

vi.mock('../utils/version', async () => {
  const actual = await vi.importActual<typeof import('../utils/version')>('../utils/version');
  return { ...actual, ...apiMocks };
});
vi.mock('../composables/useAgentUpgradeJobs', () => ({
  useAgentUpgradeJobs: () => upgradeMocks,
}));
vi.mock('ant-design-vue', () => {
  const container = { template: '<div><slot name="title"/><slot/></div>' };
  const descriptions = Object.assign({}, container, {
    Item: { template: '<div><slot/></div>' },
  });
  const modal = Object.assign({}, container, { confirm: vi.fn() });
  return {
    Card: container,
    Descriptions: descriptions,
    Tag: { template: '<span><slot/></span>' },
    Button: { template: '<button><slot/></button>' },
    Space: container,
    Table: {
      props: ['dataSource', 'columns'],
      template: `
        <div>
          <div v-for="record in dataSource" :key="record.id">
            <div v-for="column in columns" :key="column.key">
              <slot name="bodyCell" :column="column" :record="record" />
            </div>
          </div>
        </div>
      `,
    },
    Spin: container,
    Modal: modal,
    Select: container,
    message: messageMocks,
  };
});
vi.mock('@ant-design/icons-vue', () => ({
  InfoCircleOutlined: { template: '<i />' },
  SyncOutlined: { template: '<i />' },
  DownloadOutlined: { template: '<i />' },
  ExclamationCircleOutlined: { template: '<i />' },
}));

import VersionInfo from './VersionInfo.vue';
import type { AgentUpgradeJob } from '../utils/version';

const job = (serverId: number, status: AgentUpgradeJob['status'], errorCode = '') => reactive({
  id: `job-${serverId}`,
  server_id: serverId,
  trigger: 'manual',
  from_version: '1.3.0',
  target_version: '1.4.0',
  from_agent_type: 'full',
  target_agent_type: serverId === 1 ? 'monitor' : 'full',
  channel: 'stable',
  asset_name: 'better-monitor-agent-1.4.0-linux-amd64',
  asset_size: 1024,
  status,
  last_message: errorCode ? 'upgrade failed safely' : '',
  error_code: errorCode,
  bytes_downloaded: 0,
  deadline_at: '2026-07-10T12:20:00Z',
  dispatched_at: null,
  completed_at: null,
  created_at: '2026-07-10T12:00:00Z',
  updated_at: '2026-07-10T12:00:00Z',
}) as AgentUpgradeJob;

describe('VersionInfo upgrade truthfulness', () => {
  beforeEach(() => {
    apiMocks.getDashboardVersion.mockResolvedValue({ version: '1.0.0', buildTime: '', goVersion: '' });
    apiMocks.getSystemInfo.mockResolvedValue({});
    apiMocks.getServersVersions.mockResolvedValue([
      { id: 1, name: 'server-1', host: '10.0.0.1', agentVersion: '1.3.0', agentType: 'full', status: 1 },
      { id: 2, name: 'server-2', host: '10.0.0.2', agentVersion: '1.3.0', agentType: 'full', status: 1 },
    ]);
    apiMocks.getLatestAgentRelease.mockResolvedValue({ success: true, version: '1.4.0', assets: [] });
    upgradeMocks.submit.mockReset();
    upgradeMocks.loadActive.mockReset();
    upgradeMocks.loadActive.mockResolvedValue([]);
    upgradeMocks.track.mockReset();
    upgradeMocks.statusForServer.mockReset();
    messageMocks.success.mockReset();
    messageMocks.error.mockReset();
  });

  it('renders dispatched as submitted, then renders stable failure details per server', async () => {
    const first = job(1, 'dispatched');
    const second = job(2, 'failed', 'download_sha_mismatch');
    upgradeMocks.statusForServer.mockImplementation((serverId: number) => serverId === 1 ? first : second);

    const wrapper = mount(VersionInfo);
    await flushPromises();

    expect(wrapper.text()).toContain('已下发');
    expect(wrapper.text()).not.toContain('升级成功');
    expect(wrapper.text()).toContain('download_sha_mismatch');
    expect(wrapper.text()).toContain('目标 monitor / 1.4.0');
  });

  it('does not use the Dashboard version as an Agent release fallback', async () => {
    apiMocks.getDashboardVersion.mockResolvedValue({ version: '9.9.9', buildTime: '', goVersion: '' });
    apiMocks.getLatestAgentRelease.mockRejectedValue(new Error('release unavailable'));
    upgradeMocks.statusForServer.mockReturnValue(undefined);

    const wrapper = mount(VersionInfo);
    await flushPromises();

    expect(wrapper.text()).not.toContain('需要更新');
    expect(wrapper.findAll('button').some((button) => button.text().trim() === '升级')).toBe(false);
  });
});
