import { beforeEach, describe, expect, it, vi } from 'vitest';

const requestMocks = vi.hoisted(() => ({
  post: vi.fn(),
}));

vi.mock('@/utils/request', () => ({ default: requestMocks }));

import {
  buildAuthenticatedWebSocketURL,
  buildWebSocketURL,
  issueWSTicket,
} from './websocket';

describe('websocket ticket utilities', () => {
  beforeEach(() => {
    requestMocks.post.mockReset();
  });

  it('issues an exact scoped ticket request', async () => {
    requestMocks.post.mockResolvedValue({
      ticket: 'one-time-ticket',
      expires_at: '2026-07-10T16:00:30Z',
    });

    await expect(issueWSTicket({
      purpose: 'terminal',
      server_id: 7,
      session_id: 'session-1',
    })).resolves.toEqual({
      ticket: 'one-time-ticket',
      expires_at: '2026-07-10T16:00:30Z',
    });
    expect(requestMocks.post).toHaveBeenCalledWith('/ws-tickets', {
      purpose: 'terminal',
      server_id: 7,
      session_id: 'session-1',
    });
  });

  it('builds a one-time terminal websocket URL without long-lived credentials', async () => {
    requestMocks.post.mockResolvedValue({
      ticket: 'one-time-ticket',
      expires_at: '2026-07-10T16:00:30Z',
    });

    const url = await buildAuthenticatedWebSocketURL(
      '/api/servers/7/ws?session=session-1',
      { purpose: 'terminal', server_id: 7, session_id: 'session-1' },
    );
    const parsed = new URL(url);
    expect(parsed.protocol).toBe(window.location.protocol === 'https:' ? 'wss:' : 'ws:');
    expect(parsed.host).toBe(window.location.host);
    expect(parsed.pathname).toBe('/api/servers/7/ws');
    expect(parsed.searchParams.get('session')).toBe('session-1');
    expect(parsed.searchParams.get('ticket')).toBe('one-time-ticket');
    expect(parsed.searchParams.has('token')).toBe(false);
    expect(url).not.toContain('secret');
  });

  it('builds anonymous public URLs without a ticket', () => {
    const url = new URL(buildWebSocketURL('/api/servers/public/7/ws'));
    expect(url.pathname).toBe('/api/servers/public/7/ws');
    expect(url.search).toBe('');
  });

  it('requests a fresh ticket for every authenticated URL build', async () => {
    requestMocks.post
      .mockResolvedValueOnce({ ticket: 'ticket-1', expires_at: '2026-07-10T16:00:30Z' })
      .mockResolvedValueOnce({ ticket: 'ticket-2', expires_at: '2026-07-10T16:00:31Z' });

    const input = { purpose: 'server' as const, server_id: 7 };
    const first = new URL(await buildAuthenticatedWebSocketURL('/api/servers/7/ws', input));
    const second = new URL(await buildAuthenticatedWebSocketURL('/api/servers/7/ws', input));

    expect(first.searchParams.get('ticket')).toBe('ticket-1');
    expect(second.searchParams.get('ticket')).toBe('ticket-2');
    expect(requestMocks.post).toHaveBeenCalledTimes(2);
  });
});
