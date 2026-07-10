import service from './request';

// 版本信息接口
export interface VersionInfo {
  version: string;
  buildTime: string;
  goVersion: string;
}

// 系统信息接口  
export interface SystemInfo {
  version: string;
  buildTime: string;
  goVersion: string;
  startTime: string;
  uptime: string;
  osInfo: string;
  arch: string;
  cpuCount: number;
  memoryTotal: string;
}

// 服务器版本信息接口
export interface ServerVersion {
  id: number;
  name: string;
  host: string;
  agentVersion?: string;
  agentType?: 'full' | 'monitor';
  lastHeartbeat?: string;
  status: number;
}

// 获取Dashboard版本信息
export const getDashboardVersion = async (): Promise<VersionInfo> => {
  const response = await service.get<VersionInfo>('/version');
  return response;
};

// 获取系统信息（包含详细版本信息）
export const getSystemInfo = async (): Promise<SystemInfo> => {
  const response = await service.get<SystemInfo>('/system/info');
  return response;
};

// 获取所有服务器的版本信息
export const getServersVersions = async (): Promise<ServerVersion[]> => {
  const response = await service.get<ServerVersion[]>('/servers/versions');
  return response;
};

export interface ReleaseAsset {
  name: string;
  os?: string;
  arch?: string;
  size?: number;
}

export interface AgentReleaseInfo {
  success: boolean;
  version: string;
  name?: string;
  notes?: string;
  publishedAt?: string;
  assets: ReleaseAsset[];
  release_repo?: string;
  channel?: AgentReleaseChannel;
}

export type AgentUpgradeStatus =
  | 'queued'
  | 'dispatched'
  | 'received'
  | 'downloading'
  | 'verifying'
  | 'applying'
  | 'restarting'
  | 'succeeded'
  | 'failed'
  | 'timed_out';

export type AgentReleaseChannel = 'stable' | 'prerelease' | 'nightly';
export type AgentType = 'full' | 'monitor';

interface ParsedAgentVersion {
  core: [string, string, string];
  prerelease: string[];
}

const strictSemVerPattern = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

const parseAgentVersion = (raw: string): ParsedAgentVersion | undefined => {
  const match = strictSemVerPattern.exec(raw.trim());
  if (!match) return undefined;
  const prerelease = match[4] ? match[4].split('.') : [];
  if (prerelease.some((identifier) => /^[0-9]+$/.test(identifier) && identifier.length > 1 && identifier.startsWith('0'))) {
    return undefined;
  }
  return {
    core: [match[1], match[2], match[3]],
    prerelease,
  };
};

const compareNumericIdentifier = (left: string, right: string) => {
  if (left.length !== right.length) return left.length < right.length ? -1 : 1;
  return left === right ? 0 : left < right ? -1 : 1;
};

export const compareAgentVersions = (current: string | undefined, target: string | undefined): number => {
  const targetVersion = parseAgentVersion(target || '');
  if (!targetVersion) return 0;

  const currentText = (current || '').trim().toLowerCase();
  if (!currentText || currentText === 'dev' || currentText === 'unknown') return -1;
  const currentVersion = parseAgentVersion(currentText);
  if (!currentVersion) return -1;

  for (let index = 0; index < currentVersion.core.length; index += 1) {
    const compared = compareNumericIdentifier(currentVersion.core[index], targetVersion.core[index]);
    if (compared !== 0) return compared;
  }
  if (currentVersion.prerelease.length === 0 && targetVersion.prerelease.length === 0) return 0;
  if (currentVersion.prerelease.length === 0) return 1;
  if (targetVersion.prerelease.length === 0) return -1;

  const limit = Math.min(currentVersion.prerelease.length, targetVersion.prerelease.length);
  for (let index = 0; index < limit; index += 1) {
    const left = currentVersion.prerelease[index];
    const right = targetVersion.prerelease[index];
    if (left === right) continue;
    const leftNumeric = /^[0-9]+$/.test(left);
    const rightNumeric = /^[0-9]+$/.test(right);
    if (leftNumeric && rightNumeric) return compareNumericIdentifier(left, right);
    if (leftNumeric !== rightNumeric) return leftNumeric ? -1 : 1;
    return left < right ? -1 : 1;
  }
  if (currentVersion.prerelease.length === targetVersion.prerelease.length) return 0;
  return currentVersion.prerelease.length < targetVersion.prerelease.length ? -1 : 1;
};

export interface CreateAgentUpgradeRequest {
  server_ids: number[];
  target_version?: string;
  channel?: AgentReleaseChannel;
  target_agent_type?: AgentType;
}

export interface AgentUpgradeJob {
  id: string;
  server_id: number;
  trigger: string;
  from_version: string;
  target_version: string;
  from_agent_type: AgentType;
  target_agent_type: AgentType;
  channel: AgentReleaseChannel;
  asset_name: string;
  asset_size: number;
  status: AgentUpgradeStatus;
  last_message: string;
  error_code: string;
  bytes_downloaded: number;
  deadline_at: string;
  dispatched_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface AgentUpgradeNoop {
  server_id: number;
  version: string;
  agent_type: AgentType;
  reason: string;
}

export interface AgentUpgradeRejection {
  server_id: number;
  code: string;
  message: string;
}

export interface CreateAgentUpgradeResponse {
  jobs: AgentUpgradeJob[];
  noop: AgentUpgradeNoop[];
  rejected: AgentUpgradeRejection[];
}

export interface AgentUpgradeJobQuery {
  server_id?: number;
  status?: AgentUpgradeStatus;
  active?: boolean;
  limit?: number;
}

export const getLatestAgentRelease = async (): Promise<AgentReleaseInfo> => {
  const response = await service.get<AgentReleaseInfo>('/agents/releases/latest');
  return response;
};

export const createAgentUpgrades = async (
  request: CreateAgentUpgradeRequest,
): Promise<CreateAgentUpgradeResponse> => {
  const response = await service.post<CreateAgentUpgradeResponse>('/agent-upgrades', request);
  return response;
};

export const listAgentUpgradeJobs = async (
  query: AgentUpgradeJobQuery = {},
): Promise<AgentUpgradeJob[]> => {
  const response = await service.get<{ jobs: AgentUpgradeJob[] }>('/agent-upgrades', { params: query });
  return response.jobs;
};

export const getAgentUpgradeJob = async (jobId: string): Promise<AgentUpgradeJob> => {
  return service.get<AgentUpgradeJob>(`/agent-upgrades/${encodeURIComponent(jobId)}`);
};
