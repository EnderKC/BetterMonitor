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
  download_url: string;
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
}

export interface AgentUpgradeRequest {
  serverIds: number[];
  targetVersion?: string;
  channel?: string;
}

export interface AgentUpgradeResult {
  success: number[];
  failure: number[];
  offline: number[];
  missing: number[];
}

export interface AgentUpgradeResponse {
  success: boolean;
  message: string;
  targetVersion?: string;
  channel?: string;
  result: AgentUpgradeResult;
}

export const getLatestAgentRelease = async (): Promise<AgentReleaseInfo> => {
  const response = await service.get<AgentReleaseInfo>('/agents/releases/latest');
  return response;
};

export const forceAgentUpgrade = async (request: AgentUpgradeRequest): Promise<AgentUpgradeResponse> => {
  const response = await service.post<AgentUpgradeResponse>('/servers/upgrade', request);
  return response;
};
