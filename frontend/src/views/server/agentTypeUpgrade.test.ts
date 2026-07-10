import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const readView = (name: string) => readFileSync(resolve(process.cwd(), 'src/views/server', name), 'utf8');

describe('server Agent type upgrade integration', () => {
  it('uses the job API in detail and list views without prewriting current type', () => {
    const detail = readView('ServerDetail.vue');
    const list = readView('ServerList.vue');

    for (const source of [detail, list]) {
      expect(source).toContain('useAgentUpgradeJobs');
      expect(source).toContain('target_agent_type');
      expect(source).not.toContain('/switch-agent-' + 'type');
    }
    expect(detail).not.toContain('serverInfo.value.agent_type = targetType');
    expect(detail).toContain('target_agent_type }}');
  });
});
