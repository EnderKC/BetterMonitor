import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { extname, join, relative } from 'node:path';
import { describe, expect, it } from 'vitest';

const sourceRoot = join(process.cwd(), 'src');
const sourceExtensions = new Set(['.ts', '.vue']);
const allowedDownloadTokenFiles = new Set([
  'views/server/ServerDockerFile.vue',
  'views/server/ServerFile.vue',
]);
const sensitiveConsolePattern =
  /console\.(?:log|debug|info|warn|error)\([^;\n]*(?:getToken\s*\(|\btoken\b|\bwsUrl\b|\bdownloadUrl\b|登录响应|登录失败|用户资料失败|修改密码失败|服务器详情响应|获取到服务器信息响应|传入的server对象|原始服务器数据|服务器信息API返回格式异常|创建服务器响应|服务器数据格式错误)/i;
const websocketConstructorPattern = /\b(?:new\s+)?WebSocket\s*\(/;
const forbiddenCredentialQueryPattern = /[?&](?:token|jwt|secret_key)=|(?:token|jwt|secret_key)=\$\{/i;
const tokenReaderPattern = /\bgetToken\s*\(\s*\)/;
const consolePattern = /console\.(?:log|debug|info|warn|error)\s*\(/;

function isAllowedDownloadTokenQuery(sourcePath: string, line: string): boolean {
  return allowedDownloadTokenFiles.has(sourcePath)
    && /\/files\/download\?[^\n]*[?&]token=\$\{token\}/i.test(line);
}

function collectSensitiveSourceFindings(directory: string): string[] {
  const findings: string[] = [];

  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      findings.push(...collectSensitiveSourceFindings(path));
      continue;
    }
    if (!sourceExtensions.has(extname(entry.name)) || entry.name === 'sensitiveLogging.test.ts') {
      continue;
    }

    const sourcePath = relative(sourceRoot, path);
    const lines = readFileSync(path, 'utf8').split('\n');

    lines.forEach((line, index) => {
      if (sensitiveConsolePattern.test(line)) {
        findings.push(`${sourcePath}:${index + 1}: ${line.trim()}`);
      }

      if (
        forbiddenCredentialQueryPattern.test(line)
        && !isAllowedDownloadTokenQuery(sourcePath, line)
      ) {
        findings.push(`${sourcePath}:${index + 1}: credential query near websocket code`);
      }

      if (websocketConstructorPattern.test(line)) {
        const nearbyStart = Math.max(0, index - 12);
        const nearbyEnd = Math.min(lines.length, index + 13);
        if (lines.slice(nearbyStart, nearbyEnd).some((nearbyLine) => tokenReaderPattern.test(nearbyLine))) {
          findings.push(`${sourcePath}:${index + 1}: getToken() near WebSocket construction`);
        }
      }

      if (!/\.onerror\s*=/.test(line)) return;

      const parameter = line.match(/=\s*\(\s*([A-Za-z_$][\w$]*)/)?.[1]
        ?? line.match(/=\s*([A-Za-z_$][\w$]*)\s*=>/)?.[1];
      const handlerEnd = Math.min(lines.length, index + 13);
      for (let handlerIndex = index; handlerIndex < handlerEnd; handlerIndex += 1) {
        const handlerLine = lines[handlerIndex];
        const logsErrorObject = consolePattern.test(handlerLine)
          && (
            /\.target\b/.test(handlerLine)
            || (parameter !== undefined && new RegExp(`\\b${parameter}\\b`).test(handlerLine))
          );
        if (logsErrorObject) {
          findings.push(`${sourcePath}:${handlerIndex + 1}: websocket error event logged`);
          break;
        }
        if (handlerIndex > index && /^\s*};?\s*$/.test(handlerLine)) break;
      }
    });
  }

  return findings;
}

describe('sensitive browser logging', () => {
  it('does not print tokens or credential-bearing URLs to the console', () => {
    expect(collectSensitiveSourceFindings(sourceRoot)).toEqual([]);
  });

  it('detects long-lived credentials and error targets in websocket code', () => {
    const fixtureRoot = mkdtempSync(join(tmpdir(), 'better-monitor-sensitive-source-'));
    try {
      writeFileSync(join(fixtureRoot, 'BadSocket.vue'), `
        const token = getToken();
        const socket = new WebSocket(\`/api/private/ws?token=\${token}\`);
        socket.onerror = (event) => {
          console.error('websocket failed', event.target);
        };
        console.log('服务器详情响应:', response);
        console.log('创建服务器响应:', response);
      `);

      expect(collectSensitiveSourceFindings(fixtureRoot)).toHaveLength(5);
    } finally {
      rmSync(fixtureRoot, { recursive: true, force: true });
    }
  });
});
