export const waitForWebSocketOpen = (socket: WebSocket, timeoutMs = 5000): Promise<void> => {
  if (socket.readyState === WebSocket.OPEN) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const cleanup = () => {
      clearTimeout(timer);
      socket.removeEventListener('open', onOpen);
      socket.removeEventListener('close', onClose);
      socket.removeEventListener('error', onError);
    };
    const onOpen = () => { cleanup(); resolve(); };
    const onClose = () => { cleanup(); reject(new Error('WebSocket 已关闭')); };
    const onError = () => { cleanup(); reject(new Error('WebSocket 连接失败')); };
    const timer = window.setTimeout(() => { cleanup(); reject(new Error('WebSocket 连接超时')); }, timeoutMs);
    socket.addEventListener('open', onOpen, { once: true });
    socket.addEventListener('close', onClose, { once: true });
    socket.addEventListener('error', onError, { once: true });
  });
};

export const createStopOnce = (send: (streamID: string) => void) => {
  const stopped = new Set<string>();
  return (streamID: string) => {
    if (!streamID || stopped.has(streamID)) return;
    stopped.add(streamID);
    send(streamID);
  };
};
