import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const readView = (name: string) => readFileSync(`${process.cwd()}/src/views/server/${name}`, 'utf8');

describe('management refresh lifecycle', () => {
  it('refreshes images only after the pull request really completes', () => {
    const source = readView('ServerDocker.vue');
    expect(source).toContain("message.success('镜像拉取完成')");
    expect(source).toContain('await fetchImages();');
    expect(source).not.toContain('setTimeout(() => { fetchImages(); }, 3000)');
  });

  it('refreshes OpenResty state immediately and owns polling cleanup', () => {
    const source = readView('ServerNginx.vue');
    expect(source).toContain('onUnmounted(() =>');
    expect(source).not.toContain('setTimeout(async () => {\n            await fetchOpenRestyStatus()');
  });
});
