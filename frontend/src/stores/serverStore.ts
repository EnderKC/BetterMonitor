import { defineStore } from 'pinia';
import request from '../utils/request';

// 服务器状态接口定义
interface ServerState {
  id: number;
  status: string;
  lastUpdate: number;
  secret_key?: string;
  secretKey?: string;
  name?: string;
  ip?: string;
  public_ip?: string;
  port?: number;
  os?: string;
  arch?: string;
  cpu_cores?: number;
  cpu_model?: string;
  memory_total?: number;
  disk_total?: number;
  online?: boolean;
  description?: string;
  tags?: string;
  system_info?: any;
  sort_order?: number;
  agent_type?: string; // Agent类型: "full" 或 "monitor"
  // 可选：最新的监控数据
  monitorData?: {
    cpu_usage?: number;
    memory_used?: number;
    disk_used?: number;
    network_in?: number;
    network_out?: number;
    load_avg_1?: number;
    load_avg_5?: number;
    load_avg_15?: number;
  };
}

// 全局服务器状态存储
export const useServerStore = defineStore('serverStore', {
  state: () => ({
    servers: {} as Record<number, ServerState>,
    lastFetchTime: 0, // 上次获取列表的时间戳
    cacheDuration: 30000, // 缓存有效期 30秒
  }),

  getters: {
    // 判断特定服务器是否在线
    isServerOnline: (state) => (serverId: number): boolean => {
      const server = state.servers[serverId];
      if (!server) return false;
      return server.status?.toLowerCase() === 'online';
    },

    // 获取特定服务器状态
    getServerStatus: (state) => (serverId: number): string => {
      return state.servers[serverId]?.status || 'offline';
    },

    // 获取特定服务器的完整状态信息
    getServerInfo: (state) => (serverId: number): ServerState | null => {
      return state.servers[serverId] || null;
    },

    // 获取特定服务器的监控数据
    getServerMonitorData: (state) => (serverId: number) => {
      return state.servers[serverId]?.monitorData || null;
    },

    // 获取所有服务器列表
    getAllServers: (state) => {
      // 将对象转换为数组，并按 sort_order 排序
      return Object.values(state.servers).sort((a, b) => {
        const orderA = a.sort_order || 0;
        const orderB = b.sort_order || 0;
        if (orderA !== orderB) {
          return orderA - orderB;
        }
        // sort_order 相同时按 id 排序
        return a.id - b.id;
      });
    }
  },

  actions: {
    // 更新服务器状态
    updateServerStatus(serverId: number, status: string) {
      if (!this.servers[serverId]) {
        this.servers[serverId] = {
          id: serverId,
          status,
          lastUpdate: Date.now()
        };
      } else {
        this.servers[serverId].status = status;
        this.servers[serverId].lastUpdate = Date.now();
      }
      console.log(`[Store] 服务器 ${serverId} 状态已更新为: ${status}`);
    },

    // 更新服务器监控数据
    updateServerMonitorData(serverId: number, data: any) {
      if (!this.servers[serverId]) {
        this.servers[serverId] = {
          id: serverId,
          status: data.status || 'unknown',
          lastUpdate: Date.now(),
          monitorData: {}
        };
      }

      // 确保monitorData对象存在
      if (!this.servers[serverId].monitorData) {
        this.servers[serverId].monitorData = {};
      }

      // 更新服务器基本信息
      if (data.name !== undefined) this.servers[serverId].name = data.name;
      if (data.ip !== undefined) this.servers[serverId].ip = data.ip;
      if (data.public_ip !== undefined) this.servers[serverId].public_ip = data.public_ip;
      if (data.PublicIP !== undefined) this.servers[serverId].public_ip = data.PublicIP;
      if (data.os !== undefined) this.servers[serverId].os = data.os;
      if (data.arch !== undefined) this.servers[serverId].arch = data.arch;
      if (data.cpu_cores !== undefined) this.servers[serverId].cpu_cores = data.cpu_cores;
      if (data.cpu_model !== undefined) this.servers[serverId].cpu_model = data.cpu_model;
      if (data.memory_total !== undefined) this.servers[serverId].memory_total = data.memory_total;
      if (data.disk_total !== undefined) this.servers[serverId].disk_total = data.disk_total;
      if (data.online !== undefined) this.servers[serverId].online = data.online;
      if (data.description !== undefined) this.servers[serverId].description = data.description;
      if (data.tags !== undefined) this.servers[serverId].tags = data.tags;
      if (data.system_info !== undefined) this.servers[serverId].system_info = data.system_info;
      if (data.agent_type !== undefined) this.servers[serverId].agent_type = data.agent_type;

      // 更新监控数据
      const monitorData = this.servers[serverId].monitorData!;

      // 更新基本指标
      if (data.cpu_usage !== undefined) monitorData.cpu_usage = data.cpu_usage;
      if (data.memory_used !== undefined) monitorData.memory_used = data.memory_used;
      if (data.disk_used !== undefined) monitorData.disk_used = data.disk_used;
      if (data.network_in !== undefined) monitorData.network_in = data.network_in;
      if (data.network_out !== undefined) monitorData.network_out = data.network_out;
      if (data.load_avg_1 !== undefined) monitorData.load_avg_1 = data.load_avg_1;
      if (data.load_avg_5 !== undefined) monitorData.load_avg_5 = data.load_avg_5;
      if (data.load_avg_15 !== undefined) monitorData.load_avg_15 = data.load_avg_15;

      // 更新状态（如果提供了）
      if (data.status) {
        this.servers[serverId].status = data.status;
      }
      if (data.secret_key !== undefined) {
        this.servers[serverId].secret_key = data.secret_key;
        this.servers[serverId].secretKey = data.secret_key;
      } else if (data.SecretKey !== undefined) {
        this.servers[serverId].secret_key = data.SecretKey;
        this.servers[serverId].secretKey = data.SecretKey;
      }

      // 更新时间戳
      this.servers[serverId].lastUpdate = Date.now();

      // console.log(`[Store] 服务器 ${serverId} 监控数据已更新`);
    },

    // 获取所有服务器 (带缓存策略)
    async fetchServers(force = false) {
      const now = Date.now();

      // 如果不是强制刷新，且缓存未过期，且有数据，则直接返回
      if (!force &&
        this.lastFetchTime > 0 &&
        (now - this.lastFetchTime < this.cacheDuration) &&
        Object.keys(this.servers).length > 0) {
        console.log('[Store] 使用缓存的服务器列表数据');
        return Object.values(this.servers);
      }

      try {
        console.log('[Store] 开始获取服务器列表 (网络请求)');
        const response = await request.get('/servers');

        // Axios 响应数据在 data 属性中
        const responseData = response?.data ? response.data : response;

        if (responseData && responseData.servers && Array.isArray(responseData.servers)) {
          // 注意：这里不再清空 this.servers，而是进行合并/更新
          // 这样可以防止UI闪烁，并保留已有的监控数据

          // 标记当前批次ID，用于清理已删除的服务器（可选）
          const currentIds = new Set<number>();

          // 处理服务器数据
          responseData.servers.forEach((server: any) => {
            const serverId = server.ID || server.id;
            currentIds.add(serverId);

            // 如果已存在，保留原有monitorData
            const existingServer = this.servers[serverId];

            this.servers[serverId] = {
              ...existingServer, // 保留现有属性
              id: serverId,
              name: server.Name || server.name,
              status: server.Status || server.status || 'unknown',
              ip: server.IP || server.ip,
              public_ip: server.PublicIP || server.public_ip || existingServer?.public_ip,
              os: server.OS || server.os,
              arch: server.Arch || server.arch,
              cpu_cores: server.CPUCores || server.cpu_cores,
              cpu_model: server.CPUModel || server.cpu_model,
              memory_total: server.MemoryTotal || server.memory_total,
              disk_total: server.DiskTotal || server.disk_total,
              online: server.Online || server.online,
              description: server.Description || server.description,
              tags: server.Tags || server.tags,
              system_info: server.SystemInfo || server.system_info,
              secret_key: server.secret_key || server.SecretKey || existingServer?.secret_key,
              secretKey: server.secret_key || server.SecretKey || existingServer?.secretKey,
              sort_order: server.SortOrder || server.sort_order || 0,
              agent_type: server.AgentType || server.agent_type || 'full',
              lastUpdate: Date.now(),
              // 确保 monitorData 不被覆盖为空
              monitorData: existingServer?.monitorData || {}
            };
          });

          // 更新获取时间
          this.lastFetchTime = Date.now();

          console.log(`[Store] 服务器列表已更新: ${Object.keys(this.servers).length} 台服务器`);
          return Object.values(this.servers);
        } else {
          console.error('[Store] 服务器数据格式错误', responseData);
          return [];
        }
      } catch (error) {
        console.error('[Store] 获取服务器列表失败:', error);
        throw error;
      }
    },

    // 强制刷新服务器列表
    async forceRefreshServers() {
      return this.fetchServers(true);
    },

    // 删除服务器 (乐观更新)
    async deleteServer(serverId: number) {
      // 保存副本以便回滚
      const serverBackup = this.servers[serverId];

      // 立即从状态中删除
      if (this.servers[serverId]) {
        delete this.servers[serverId];
        console.log(`[Store] 乐观删除服务器 ${serverId}`);
      }

      try {
        // 调用API
        await request.delete(`/servers/${serverId}`);
        console.log(`[Store] 服务器 ${serverId} 删除成功`);
      } catch (error) {
        // 失败回滚
        console.error(`[Store] 删除服务器 ${serverId} 失败，正在回滚`, error);
        if (serverBackup) {
          this.servers[serverId] = serverBackup;
        }
        throw error;
      }
    },

    // 清除服务器状态（如用户登出或切换服务器时）
    clearServerStatus(serverId?: number) {
      if (serverId !== undefined) {
        if (this.servers[serverId]) {
          delete this.servers[serverId];
          console.log(`[Store] 服务器 ${serverId} 状态已清除`);
        }
      } else {
        this.servers = {};
        this.lastFetchTime = 0;
        console.log('[Store] 所有服务器状态已清除');
      }
    },

    // 批量更新服务器顺序
    async reorderServers(orderedIds: number[]) {
      try {
        console.log('[Store] 正在更新服务器顺序:', orderedIds);

        // 调用API更新顺序
        await request.put('/servers/reorder', {
          orderedIds: orderedIds
        });

        // 更新本地状态中的 sort_order
        orderedIds.forEach((serverId, index) => {
          if (this.servers[serverId]) {
            // TypeScript需要我们先读取再修改
            const server = this.servers[serverId];
            // @ts-ignore - 动态添加 sort_order 字段
            server.sort_order = index + 1;
          }
        });

        console.log('[Store] 服务器顺序更新成功');
      } catch (error) {
        console.error('[Store] 更新服务器顺序失败:', error);
        throw error;
      }
    }
  }
});
