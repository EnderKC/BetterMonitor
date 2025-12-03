import { defineStore } from 'pinia';
import { ref } from 'vue';
import service from '../utils/request';

// 设置接口定义
interface SystemSettings {
  ui_refresh_interval?: string;
  heartbeat_interval?: string;
  monitor_interval?: string;
  data_retention_days?: number;
  life_data_retention_days?: number;
  chart_history_hours?: number;
  agent_release_repo?: string;
  agent_release_channel?: string;
  agent_release_mirror?: string;
}

export const useSettingsStore = defineStore('settings', () => {
  // 默认值
  const uiRefreshInterval = ref('10s');
  const heartbeatInterval = ref('10s');
  const monitorInterval = ref('30s');
  const dataRetentionDays = ref(7);
  const lifeDataRetentionDays = ref(7);
  const chartHistoryHours = ref(24);
  const agentReleaseRepo = ref('');
  const agentReleaseChannel = ref('stable');
  const agentReleaseMirror = ref('');
  
  // 是否已加载设置
  const loaded = ref(false);

  // 将持续时间字符串转换为毫秒数
  const durationToMs = (duration: string): number => {
    const numericPart = parseFloat(duration);
    const unit = duration.replace(/^[\d.]+/, '');

    switch (unit) {
      case 's':
        return numericPart * 1000;
      case 'm':
        return numericPart * 60 * 1000;
      case 'h':
        return numericPart * 60 * 60 * 1000;
      default:
        return numericPart * 1000; // 默认为秒
    }
  };

  // 获取UI刷新间隔（毫秒）
  const getUiRefreshIntervalMs = (): number => {
    return durationToMs(uiRefreshInterval.value);
  };

  // 加载设置
  const loadSettings = async () => {
    try {
      // 调整API路径
      const settings = await service.get<SystemSettings>('admin/settings');
      console.log('加载系统设置响应:', settings);
      
      if (settings) {
        console.log('加载的系统设置:', settings);
        
        if (settings.ui_refresh_interval !== undefined) {
          uiRefreshInterval.value = settings.ui_refresh_interval;
        }
        
        if (settings.heartbeat_interval !== undefined) {
          heartbeatInterval.value = settings.heartbeat_interval;
        }
        
        if (settings.monitor_interval !== undefined) {
          monitorInterval.value = settings.monitor_interval;
        }
        
        if (settings.data_retention_days !== undefined) {
          dataRetentionDays.value = settings.data_retention_days;
        }

        if (settings.life_data_retention_days !== undefined) {
          lifeDataRetentionDays.value = settings.life_data_retention_days;
        }
        
        if (settings.chart_history_hours !== undefined) {
          chartHistoryHours.value = settings.chart_history_hours;
        }
        
        if (settings.agent_release_repo !== undefined) {
          agentReleaseRepo.value = settings.agent_release_repo;
        }
        
        if (settings.agent_release_channel !== undefined) {
          agentReleaseChannel.value = settings.agent_release_channel;
        }
        
        if (settings.agent_release_mirror !== undefined) {
          agentReleaseMirror.value = settings.agent_release_mirror;
        }
        
        loaded.value = true;
      }
    } catch (error) {
      console.error('加载系统设置失败:', error);
      // 出错时使用默认值
    }
  };
  
  // 如果是公共页面，加载公共设置
  const loadPublicSettings = async () => {
    try {
      // 调整API路径
      const settings = await service.get<SystemSettings>('public/settings');
      console.log('加载公共设置响应:', settings);
      
      if (settings) {
        console.log('加载的公共设置:', settings);
        
        if (settings.ui_refresh_interval !== undefined) {
          uiRefreshInterval.value = settings.ui_refresh_interval;
        }
        
        if (settings.chart_history_hours !== undefined) {
          chartHistoryHours.value = settings.chart_history_hours;
        }
        
        loaded.value = true;
      }
    } catch (error) {
      console.error('加载公共设置失败:', error);
      // 出错时使用默认值
    }
  };

  // 手动更新设置值
  const updateSettings = (settings: SystemSettings) => {
    if (settings.ui_refresh_interval !== undefined) {
      uiRefreshInterval.value = settings.ui_refresh_interval;
    }
    
    if (settings.heartbeat_interval !== undefined) {
      heartbeatInterval.value = settings.heartbeat_interval;
    }
    
    if (settings.monitor_interval !== undefined) {
      monitorInterval.value = settings.monitor_interval;
    }
    
    if (settings.data_retention_days !== undefined) {
      dataRetentionDays.value = settings.data_retention_days;
    }

    if (settings.life_data_retention_days !== undefined) {
      lifeDataRetentionDays.value = settings.life_data_retention_days;
    }
    
    if (settings.chart_history_hours !== undefined) {
      chartHistoryHours.value = settings.chart_history_hours;
    }
    
    if (settings.agent_release_repo !== undefined) {
      agentReleaseRepo.value = settings.agent_release_repo;
    }
    
    if (settings.agent_release_channel !== undefined) {
      agentReleaseChannel.value = settings.agent_release_channel;
    }
    
    if (settings.agent_release_mirror !== undefined) {
      agentReleaseMirror.value = settings.agent_release_mirror;
    }
    
    console.log('手动更新设置完成:', {
      chartHistoryHours: chartHistoryHours.value,
      uiRefreshInterval: uiRefreshInterval.value
    });
  };

  return {
    uiRefreshInterval,
    heartbeatInterval,
    monitorInterval,
    dataRetentionDays,
    lifeDataRetentionDays,
    chartHistoryHours,
    agentReleaseRepo,
    agentReleaseChannel,
    agentReleaseMirror,
    loaded,
    getUiRefreshIntervalMs,
    loadSettings,
    loadPublicSettings,
    updateSettings
  };
}); 
