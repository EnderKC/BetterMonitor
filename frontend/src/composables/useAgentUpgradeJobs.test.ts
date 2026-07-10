import { defineComponent, nextTick } from 'vue';
import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const apiMocks = vi.hoisted(() => ({
  createAgentUpgrades: vi.fn(),
  getAgentUpgradeJob: vi.fn(),
  listAgentUpgradeJobs: vi.fn(),
}));

vi.mock('../utils/version', async () => {
  const actual = await vi.importActual<typeof import('../utils/version')>('../utils/version');
  return {
    ...actual,
    createAgentUpgrades: apiMocks.createAgentUpgrades,
    getAgentUpgradeJob: apiMocks.getAgentUpgradeJob,
    listAgentUpgradeJobs: apiMocks.listAgentUpgradeJobs,
  };
});

import { useAgentUpgradeJobs } from './useAgentUpgradeJobs';
import type { AgentUpgradeJob } from '../utils/version';

const job = (id: string, serverId: number, status: AgentUpgradeJob['status']): AgentUpgradeJob => ({
  id,
  server_id: serverId,
  trigger: 'manual',
  from_version: '1.3.0',
  target_version: '1.4.0',
  from_agent_type: 'full',
  target_agent_type: 'full',
  channel: 'stable',
  asset_name: 'better-monitor-agent-1.4.0-linux-amd64',
  asset_size: 1024,
  status,
  last_message: '',
  error_code: '',
  bytes_downloaded: 0,
  deadline_at: '2026-07-10T12:20:00Z',
  dispatched_at: null,
  completed_at: null,
  created_at: '2026-07-10T12:00:00Z',
  updated_at: '2026-07-10T12:00:00Z',
});

describe('useAgentUpgradeJobs', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    apiMocks.createAgentUpgrades.mockReset();
    apiMocks.getAgentUpgradeJob.mockReset();
    apiMocks.listAgentUpgradeJobs.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('loads immediately, polls every two seconds, and notifies terminal once', async () => {
    const onTerminal = vi.fn();
    apiMocks.getAgentUpgradeJob
      .mockResolvedValueOnce(job('job-1', 1, 'downloading'))
      .mockResolvedValueOnce(job('job-1', 1, 'succeeded'));

    let upgrades!: ReturnType<typeof useAgentUpgradeJobs>;
    const wrapper = mount(defineComponent({
      setup() {
        upgrades = useAgentUpgradeJobs({ onTerminal });
        return () => null;
      },
    }));

    await upgrades.track('job-1');
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(1);
    expect(upgrades.statusForServer(1)?.status).toBe('downloading');

    await vi.advanceTimersByTimeAsync(2000);
    await nextTick();
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(2);
    expect(upgrades.statusForServer(1)?.status).toBe('succeeded');
    expect(onTerminal).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(6000);
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(2);
    expect(onTerminal).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it('pauses while hidden, resumes when visible, and stops on unmount', async () => {
    let visibility = 'visible';
    vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visibility as DocumentVisibilityState);
    apiMocks.getAgentUpgradeJob.mockResolvedValue(job('job-1', 1, 'downloading'));

    let upgrades!: ReturnType<typeof useAgentUpgradeJobs>;
    const wrapper = mount(defineComponent({
      setup() {
        upgrades = useAgentUpgradeJobs();
        return () => null;
      },
    }));
    upgrades.track(job('job-1', 1, 'downloading'));

    visibility = 'hidden';
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(4000);
    expect(apiMocks.getAgentUpgradeJob).not.toHaveBeenCalled();

    visibility = 'visible';
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(2000);
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(1);

    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(4000);
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(1);
  });

  it('tracks independent batch jobs without duplicate timers', async () => {
    const jobs = [job('job-1', 1, 'dispatched'), job('job-2', 2, 'received')];
    apiMocks.createAgentUpgrades.mockResolvedValue({ jobs, noop: [], rejected: [] });
    apiMocks.getAgentUpgradeJob.mockImplementation(async (id: string) => jobs.find((item) => item.id === id));

    let upgrades!: ReturnType<typeof useAgentUpgradeJobs>;
    const wrapper = mount(defineComponent({
      setup() {
        upgrades = useAgentUpgradeJobs();
        return () => null;
      },
    }));

    await upgrades.submit({ server_ids: [1, 2], channel: 'stable' });
    upgrades.track(jobs[0]);
    expect(vi.getTimerCount()).toBe(2);

    await vi.advanceTimersByTimeAsync(2000);
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(2);
    expect(upgrades.statusForServer(1)?.id).toBe('job-1');
    expect(upgrades.statusForServer(2)?.id).toBe('job-2');
    wrapper.unmount();
  });

  it('restores persisted active jobs after a page reload', async () => {
    apiMocks.listAgentUpgradeJobs.mockResolvedValue([
      job('job-1', 1, 'downloading'),
      job('job-2', 2, 'received'),
    ]);

    let upgrades!: ReturnType<typeof useAgentUpgradeJobs>;
    const wrapper = mount(defineComponent({
      setup() {
        upgrades = useAgentUpgradeJobs();
        return () => null;
      },
    }));

    await upgrades.loadActive([1]);
    expect(apiMocks.listAgentUpgradeJobs).toHaveBeenCalledWith({ active: true, limit: 500 });
    expect(upgrades.statusForServer(1)?.id).toBe('job-1');
    expect(upgrades.statusForServer(2)).toBeUndefined();
    expect(vi.getTimerCount()).toBe(1);
    wrapper.unmount();
  });

  it('continues polling after a transient refresh failure', async () => {
    apiMocks.getAgentUpgradeJob
      .mockRejectedValueOnce(new Error('temporary network failure'))
      .mockResolvedValueOnce(job('job-1', 1, 'succeeded'));

    let upgrades!: ReturnType<typeof useAgentUpgradeJobs>;
    const wrapper = mount(defineComponent({
      setup() {
        upgrades = useAgentUpgradeJobs();
        return () => null;
      },
    }));

    upgrades.track(job('job-1', 1, 'downloading'));
    await vi.advanceTimersByTimeAsync(2000);
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(1);

    await vi.advanceTimersByTimeAsync(2000);
    expect(apiMocks.getAgentUpgradeJob).toHaveBeenCalledTimes(2);
    expect(upgrades.statusForServer(1)?.status).toBe('succeeded');
    wrapper.unmount();
  });
});
