import { defineStore } from 'pinia';
import { message } from 'ant-design-vue';
import request from '../utils/request';

interface AlertSetting {
  id: number;
  type: string;
  threshold: number;
  duration: number;
  enabled: boolean;
  server_id: number;
  created_at: string;
  updated_at: string;
}

interface NotificationChannel {
  id: number;
  type: string;
  name: string;
  config: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

interface AlertRecord {
  id: number;
  server_id: number;
  server_name: string;
  alert_type: string;
  value: number;
  threshold: number;
  resolved: boolean;
  resolved_at: string;
  notified_at: string;
  channel_ids: string;
  created_at: string;
  updated_at: string;
}

// 后端API响应类型
interface ApiResponse<T> {
  [key: string]: any;
}

export const useAlertStore = defineStore('alert', {
  state: () => ({
    alertSettings: [] as AlertSetting[],
    notificationChannels: [] as NotificationChannel[],
    alertRecords: [] as AlertRecord[],
    totalRecords: 0,
    currentPage: 1,
    pageSize: 10,
    loading: {
      settings: false,
      channels: false,
      records: false,
    },
  }),
  
  actions: {
    // 获取预警设置
    async fetchAlertSettings(serverId: number = 0) {
      this.loading.settings = true;
      try {
        const response = await request.get<ApiResponse<{ settings: AlertSetting[] }>>(`/alerts/settings`, {
          params: { server_id: serverId }
        });
        
        // 处理后端返回的字段名与前端期望的字段名不一致的问题
        const settings = ((response as any).settings || []).map((setting: any) => ({
          id: setting.ID,
          type: setting.type,
          threshold: setting.threshold,
          duration: setting.duration,
          enabled: setting.enabled,
          server_id: setting.server_id,
          created_at: setting.CreatedAt,
          updated_at: setting.UpdatedAt
        }));
        
        this.alertSettings = settings;
      } catch (error) {
        console.error('获取预警设置失败:', error);
        message.error('获取预警设置失败');
      } finally {
        this.loading.settings = false;
      }
    },
    
    // 创建预警设置
    async createAlertSetting(setting: Partial<AlertSetting>) {
      try {
        const response = await request.post<ApiResponse<{ setting: AlertSetting }>>('/alerts/settings', setting);
        message.success('创建预警设置成功');
        // 转换字段名
        const returnedSetting = (response as any).setting ? {
          id: (response as any).setting.ID,
          type: (response as any).setting.type,
          threshold: (response as any).setting.threshold,
          duration: (response as any).setting.duration,
          enabled: (response as any).setting.enabled,
          server_id: (response as any).setting.server_id,
          created_at: (response as any).setting.CreatedAt,
          updated_at: (response as any).setting.UpdatedAt
        } : null;
        return returnedSetting;
      } catch (error) {
        console.error('创建预警设置失败:', error);
        message.error('创建预警设置失败');
        throw error;
      }
    },
    
    // 更新预警设置
    async updateAlertSetting(id: number, setting: Partial<AlertSetting>) {
      try {
        const response = await request.put<ApiResponse<{ setting: AlertSetting }>>(`/alerts/settings/${id}`, setting);
        message.success('更新预警设置成功');
        // 转换字段名
        const returnedSetting = (response as any).setting ? {
          id: (response as any).setting.ID,
          type: (response as any).setting.type,
          threshold: (response as any).setting.threshold,
          duration: (response as any).setting.duration,
          enabled: (response as any).setting.enabled,
          server_id: (response as any).setting.server_id,
          created_at: (response as any).setting.CreatedAt,
          updated_at: (response as any).setting.UpdatedAt
        } : null;
        return returnedSetting;
      } catch (error) {
        console.error('更新预警设置失败:', error);
        message.error('更新预警设置失败');
        throw error;
      }
    },
    
    // 删除预警设置
    async deleteAlertSetting(id: number) {
      try {
        await request.delete(`/alerts/settings/${id}`);
        message.success('删除预警设置成功');
      } catch (error) {
        console.error('删除预警设置失败:', error);
        message.error('删除预警设置失败');
        throw error;
      }
    },
    
    // 获取通知渠道
    async fetchNotificationChannels(onlyEnabled: boolean = false) {
      this.loading.channels = true;
      try {
        const response = await request.get<ApiResponse<{ channels: NotificationChannel[] }>>('/alerts/channels', {
          params: { enabled: onlyEnabled }
        });
        
        // 处理后端返回的字段名与前端期望的字段名不一致的问题
        const channels = ((response as any).channels || []).map((channel: any) => ({
          id: channel.ID,
          type: channel.type,
          name: channel.name,
          config: channel.config,
          enabled: channel.enabled,
          created_at: channel.CreatedAt,
          updated_at: channel.UpdatedAt
        }));
        
        this.notificationChannels = channels;
      } catch (error) {
        console.error('获取通知渠道失败:', error);
        message.error('获取通知渠道失败');
      } finally {
        this.loading.channels = false;
      }
    },
    
    // 创建通知渠道
    async createNotificationChannel(channel: Partial<NotificationChannel>) {
      try {
        const response = await request.post<ApiResponse<{ channel: NotificationChannel }>>('/alerts/channels', channel);
        message.success('创建通知渠道成功');
        
        // 转换字段名
        const returnedChannel = (response as any).channel ? {
          id: (response as any).channel.ID,
          type: (response as any).channel.type,
          name: (response as any).channel.name,
          config: (response as any).channel.config,
          enabled: (response as any).channel.enabled,
          created_at: (response as any).channel.CreatedAt,
          updated_at: (response as any).channel.UpdatedAt
        } : null;
        
        return returnedChannel;
      } catch (error) {
        console.error('创建通知渠道失败:', error);
        message.error('创建通知渠道失败');
        throw error;
      }
    },
    
    // 更新通知渠道
    async updateNotificationChannel(id: number, channel: Partial<NotificationChannel>) {
      try {
        const response = await request.put<ApiResponse<{ channel: NotificationChannel }>>(`/alerts/channels/${id}`, channel);
        message.success('更新通知渠道成功');
        
        // 转换字段名
        const returnedChannel = (response as any).channel ? {
          id: (response as any).channel.ID,
          type: (response as any).channel.type,
          name: (response as any).channel.name,
          config: (response as any).channel.config,
          enabled: (response as any).channel.enabled,
          created_at: (response as any).channel.CreatedAt,
          updated_at: (response as any).channel.UpdatedAt
        } : null;
        
        return returnedChannel;
      } catch (error) {
        console.error('更新通知渠道失败:', error);
        message.error('更新通知渠道失败');
        throw error;
      }
    },
    
    // 删除通知渠道
    async deleteNotificationChannel(id: number) {
      try {
        await request.delete(`/alerts/channels/${id}`);
        message.success('删除通知渠道成功');
      } catch (error) {
        console.error('删除通知渠道失败:', error);
        message.error('删除通知渠道失败');
        throw error;
      }
    },
    
    // 测试通知渠道
    async testNotificationChannel(id: number) {
      if (!id) {
        message.error('通知渠道ID无效');
        return;
      }
      
      try {
        console.log('发送测试通知请求，渠道ID:', id);
        await request.post(`/alerts/channels/${id}/test`);
        message.success('测试通知已发送，请检查接收情况');
      } catch (error: any) {
        console.error('测试通知失败:', error);
        if (error.response) {
          message.error(`测试通知失败: ${error.response.data?.error || '未知错误'}`);
        } else {
          message.error('测试通知失败，请检查网络连接');
        }
        throw error;
      }
    },
    
    // 获取预警记录
    async fetchAlertRecords(params: { 
      server_id?: number;
      type?: string;
      unresolved?: boolean;
      page?: number;
      limit?: number;
    } = {}) {
      this.loading.records = true;
      try {
        const { page = 1, limit = 10 } = params;
        this.currentPage = page;
        this.pageSize = limit;
        
        const response = await request.get<ApiResponse<{ records: AlertRecord[], total: number }>>('/alerts/records', { params });
        
        // 处理后端返回的字段名与前端期望的字段名不一致的问题
        const records = ((response as any).records || []).map((record: any) => ({
          id: record.ID,
          server_id: record.server_id,
          server_name: record.server_name,
          alert_type: record.alert_type,
          value: record.value,
          threshold: record.threshold,
          resolved: record.resolved,
          resolved_at: record.resolved_at || record.ResolvedAt,
          notified_at: record.notified_at || record.NotifiedAt,
          channel_ids: record.channel_ids,
          created_at: record.CreatedAt,
          updated_at: record.UpdatedAt
        }));
        
        this.alertRecords = records;
        this.totalRecords = (response as any).total || 0;
      } catch (error) {
        console.error('获取预警记录失败:', error);
        message.error('获取预警记录失败');
      } finally {
        this.loading.records = false;
      }
    },
    
    // 手动解决预警记录
    async resolveAlertRecord(id: number) {
      try {
        const response = await request.put<ApiResponse<{ record: AlertRecord }>>(`/alerts/records/${id}/resolve`);
        message.success('已标记为已解决');
        
        // 转换字段名
        const record = (response as any).record ? {
          id: (response as any).record.ID,
          server_id: (response as any).record.server_id,
          server_name: (response as any).record.server_name,
          alert_type: (response as any).record.alert_type,
          value: (response as any).record.value,
          threshold: (response as any).record.threshold,
          resolved: (response as any).record.resolved,
          resolved_at: (response as any).record.resolved_at || (response as any).record.ResolvedAt,
          notified_at: (response as any).record.notified_at || (response as any).record.NotifiedAt,
          channel_ids: (response as any).record.channel_ids,
          created_at: (response as any).record.CreatedAt,
          updated_at: (response as any).record.UpdatedAt
        } : null;
        
        // 更新记录状态
        if (record) {
          const index = this.alertRecords.findIndex(r => r.id === id);
          if (index !== -1) {
            this.alertRecords[index] = record;
          }
        }
        
        return record;
      } catch (error) {
        console.error('解决预警记录失败:', error);
        message.error('解决预警记录失败');
        throw error;
      }
    },
  },
}); 