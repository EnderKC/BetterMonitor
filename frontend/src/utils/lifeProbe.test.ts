import { beforeEach, describe, expect, it, vi } from 'vitest';

const requestMocks = vi.hoisted(() => ({
  post: vi.fn(),
}));

vi.mock('./request', () => ({
  default: requestMocks,
}));

import { createLifeProbe, rotateLifeProbeSecret } from './lifeProbe';

describe('life probe secret API contract', () => {
  beforeEach(() => {
    requestMocks.post.mockReset();
  });

  it('creates a private probe through the authenticated collection endpoint', async () => {
    const payload = {
      name: 'phone',
      device_id: 'phone-1',
      description: '',
      tags: '',
      allow_public_view: false,
    };
    requestMocks.post.mockResolvedValue({ life_probe: { id: 1 }, ingest_secret: 'secret' });

    await createLifeProbe(payload);

    expect(requestMocks.post).toHaveBeenCalledWith('/life-probes', payload);
  });

  it('rotates the selected probe secret', async () => {
    requestMocks.post.mockResolvedValue({ ingest_secret: 'new-secret', ingest_secret_version: 2 });

    await rotateLifeProbeSecret(7);

    expect(requestMocks.post).toHaveBeenCalledWith('/life-probes/7/rotate-secret');
  });
});
