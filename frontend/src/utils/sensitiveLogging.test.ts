import { readdirSync, readFileSync } from 'node:fs';
import { extname, join, relative } from 'node:path';
import { describe, expect, it } from 'vitest';

const sourceRoot = join(process.cwd(), 'src');
const sourceExtensions = new Set(['.ts', '.vue']);
const sensitiveConsolePattern =
  /console\.(?:log|debug|info|warn|error)\([^;\n]*(?:getToken\s*\(|\btoken\b|\bwsUrl\b|\bdownloadUrl\b|登录响应|登录失败|用户资料失败|修改密码失败)/i;

function collectSensitiveConsoleCalls(directory: string): string[] {
  const findings: string[] = [];

  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      findings.push(...collectSensitiveConsoleCalls(path));
      continue;
    }
    if (!sourceExtensions.has(extname(entry.name)) || entry.name === 'sensitiveLogging.test.ts') {
      continue;
    }

    readFileSync(path, 'utf8')
      .split('\n')
      .forEach((line, index) => {
        if (sensitiveConsolePattern.test(line)) {
          findings.push(`${relative(sourceRoot, path)}:${index + 1}: ${line.trim()}`);
        }
      });
  }

  return findings;
}

describe('sensitive browser logging', () => {
  it('does not print tokens or credential-bearing URLs to the console', () => {
    expect(collectSensitiveConsoleCalls(sourceRoot)).toEqual([]);
  });
});
