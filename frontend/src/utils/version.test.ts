import { beforeEach, describe, expect, it, vi } from 'vitest';

const requestMocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}));

vi.mock('./request', () => ({
  default: requestMocks,
}));

import { forceAgentUpgrade, getLatestAgentRelease } from './version';

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

  it('posts the selected servers and target version', async () => {
    const request = {
      serverIds: [1, 2],
      targetVersion: '1.2.5',
      channel: 'stable',
    };
    requestMocks.post.mockResolvedValue({
      success: true,
      message: 'accepted',
      result: {},
    });

    await forceAgentUpgrade(request);

    expect(requestMocks.post).toHaveBeenCalledWith('/servers/upgrade', request);
  });
});
