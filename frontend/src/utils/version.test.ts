import { beforeEach, describe, expect, it, vi } from 'vitest';

const requestMocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}));

vi.mock('./request', () => ({
  default: requestMocks,
}));

import {
  compareAgentVersions,
  createAgentUpgrades,
  getAgentUpgradeJob,
  getLatestAgentRelease,
  listAgentUpgradeJobs,
} from './version';

describe('Agent semantic version comparison', () => {
  it('orders stable and prerelease versions by SemVer precedence', () => {
    expect(compareAgentVersions('1.5.0-beta.10', '1.5.0-rc.2')).toBeLessThan(0);
    expect(compareAgentVersions('1.5.0-rc.10', '1.5.0-rc.2')).toBeGreaterThan(0);
    expect(compareAgentVersions('1.5.0-rc.2', '1.5.0')).toBeLessThan(0);
    expect(compareAgentVersions('1.5.0+build.1', '1.5.0+build.2')).toBe(0);
  });

  it('treats development or unknown current versions as upgrade candidates', () => {
    expect(compareAgentVersions('dev', '1.4.0')).toBeLessThan(0);
    expect(compareAgentVersions('unknown', '1.4.0')).toBeLessThan(0);
    expect(compareAgentVersions('', '1.4.0')).toBeLessThan(0);
  });
});

describe('version API contract', () => {
  beforeEach(() => {
    requestMocks.get.mockReset();
    requestMocks.post.mockReset();
  });

  it('queries the configured latest Agent release endpoint', async () => {
    const release = { success: true, version: '1.2.5', assets: [] };
    requestMocks.get.mockResolvedValue(release);

    await expect(getLatestAgentRelease()).resolves.toEqual(release);
    expect(requestMocks.get).toHaveBeenCalledWith('/agents/releases/latest');
  });

  it('creates typed per-server upgrade jobs with snake_case fields', async () => {
    const request = {
      server_ids: [1, 2],
      target_version: '1.4.0',
      channel: 'stable' as const,
      target_agent_type: 'monitor' as const,
    };
    const response = { jobs: [], noop: [], rejected: [] };
    requestMocks.post.mockResolvedValue(response);

    await expect(createAgentUpgrades(request)).resolves.toEqual(response);
    expect(requestMocks.post).toHaveBeenCalledWith('/agent-upgrades', request);
  });

  it('lists and gets safe upgrade job projections', async () => {
    requestMocks.get
      .mockResolvedValueOnce({ jobs: [{ id: 'job-1', server_id: 1, status: 'downloading' }] })
      .mockResolvedValueOnce({ id: 'job-1', server_id: 1, status: 'succeeded' });

    await expect(listAgentUpgradeJobs({ server_id: 1, active: true })).resolves.toEqual([
      { id: 'job-1', server_id: 1, status: 'downloading' },
    ]);
    expect(requestMocks.get).toHaveBeenNthCalledWith(1, '/agent-upgrades', {
      params: { server_id: 1, active: true },
    });

    await expect(getAgentUpgradeJob('job-1')).resolves.toEqual({
      id: 'job-1',
      server_id: 1,
      status: 'succeeded',
    });
    expect(requestMocks.get).toHaveBeenNthCalledWith(2, '/agent-upgrades/job-1');
  });
});
