import { describe, expect, it, vi } from 'vitest';
import { createStopOnce, waitForWebSocketOpen } from './docker';

describe('Docker websocket lifecycle', () => {
  it('resolves on open without polling', async () => {
    const socket = new EventTarget() as WebSocket;
    Object.defineProperty(socket, 'readyState', { value: WebSocket.CONNECTING, configurable: true });
    const waiting = waitForWebSocketOpen(socket, 1000);
    Object.defineProperty(socket, 'readyState', { value: WebSocket.OPEN, configurable: true });
    socket.dispatchEvent(new Event('open'));
    await expect(waiting).resolves.toBeUndefined();
  });

  it('sends a stream stop at most once', () => {
    const send = vi.fn();
    const stop = createStopOnce(send);
    stop('stream-a');
    stop('stream-a');
    expect(send).toHaveBeenCalledTimes(1);
  });
});
