<template>
  <div class="alert-settings-container">
    <a-card title="预警设置" :bordered="false">
      <template #extra>
        <a-button type="primary" @click="showAddSettingModal">添加预警设置</a-button>
      </template>

      <a-tabs v-model:activeKey="activeTab">
        <a-tab-pane key="global" tab="全局设置">
          <a-spin :spinning="loading.settings">
            <a-table 
              :dataSource="globalSettings" 
              :columns="columns" 
              rowKey="id" 
              :pagination="false"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'type'">
                  <a-tag :color="getTypeColor(record.type)">{{ getTypeName(record.type) }}</a-tag>
                </template>
                <template v-if="column.key === 'threshold'">
                  {{ getFormattedThreshold(record) }}
                </template>
                <template v-if="column.key === 'enabled'">
                  <a-switch 
                    :checked="record.enabled" 
                    @change="(checked) => toggleAlertSetting(record.id, checked)"
                  />
                </template>
                <template v-if="column.key === 'action'">
                  <a-button type="link" size="small" @click="editSetting(record)">编辑</a-button>
                  <a-popconfirm
                    title="确定要删除这个预警设置吗？"
                    ok-text="确定"
                    cancel-text="取消"
                    @confirm="deleteSetting(record.id)"
                  >
                    <a-button type="link" danger size="small">删除</a-button>
                  </a-popconfirm>
                </template>
              </template>
            </a-table>
            <a-empty v-if="globalSettings.length === 0" description="暂无预警设置" />
          </a-spin>
        </a-tab-pane>
        
        <a-tab-pane key="server" tab="服务器特定设置" v-if="showServerSettings">
          <a-spin :spinning="loading.settings">
            <a-row style="margin-bottom: 16px">
              <a-col :span="8">
                <a-select
                  v-model:value="selectedServerId"
                  style="width: 100%"
                  placeholder="选择服务器"
                  @change="handleServerChange"
                >
                  <a-select-option v-for="server in servers" :key="server.id" :value="server.id">
                    {{ server.name }}
                  </a-select-option>
                </a-select>
              </a-col>
            </a-row>
            
            <a-table 
              :dataSource="serverSettings" 
              :columns="columns" 
              rowKey="id" 
              :pagination="false"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'type'">
                  <a-tag :color="getTypeColor(record.type)">{{ getTypeName(record.type) }}</a-tag>
                </template>
                <template v-if="column.key === 'threshold'">
                  {{ getFormattedThreshold(record) }}
                </template>
                <template v-if="column.key === 'enabled'">
                  <a-switch 
                    :checked="record.enabled" 
                    @change="(checked) => toggleAlertSetting(record.id, checked)"
                  />
                </template>
                <template v-if="column.key === 'action'">
                  <a-button type="link" size="small" @click="editSetting(record)">编辑</a-button>
                  <a-popconfirm
                    title="确定要删除这个预警设置吗？"
                    ok-text="确定"
                    cancel-text="取消"
                    @confirm="deleteSetting(record.id)"
                  >
                    <a-button type="link" danger size="small">删除</a-button>
                  </a-popconfirm>
                </template>
              </template>
            </a-table>
            <a-empty v-if="serverSettings.length === 0" description="暂无预警设置" />
          </a-spin>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <!-- 添加/编辑预警设置的弹窗 -->
    <a-modal
      v-model:visible="settingModalVisible"
      :title="isEditing ? '编辑预警设置' : '添加预警设置'"
      okText="保存"
      cancelText="取消"
      @ok="saveSetting"
    >
      <a-form :model="formState" :label-col="{ span: 6 }" :wrapper-col="{ span: 16 }">
        <a-form-item label="预警类型" name="type">
          <a-select 
            v-model:value="formState.type"
            placeholder="选择预警类型"
            :disabled="isEditing"
          >
            <a-select-option value="cpu">CPU 使用率</a-select-option>
            <a-select-option value="memory">内存使用率</a-select-option>
            <a-select-option value="network">网络流量</a-select-option>
            <a-select-option value="status">服务器状态</a-select-option>
          </a-select>
        </a-form-item>
        
        <a-form-item label="预警阈值" name="threshold">
          <template v-if="formState.type === 'status'">
            <a-select 
              v-model:value="formState.threshold" 
              style="width: 100%"
            >
              <a-select-option :value="1">服务器上线时报警</a-select-option>
              <a-select-option :value="2">服务器离线时报警</a-select-option>
              <a-select-option :value="3">服务器上线和离线都报警</a-select-option>
            </a-select>
          </template>
          <template v-else>
            <a-input-number 
              v-model:value="formState.threshold" 
              :min="0" 
              style="width: 100%"
              :addonAfter="getThresholdUnit(formState.type)" 
            />
          </template>
        </a-form-item>
        
        <a-form-item label="持续时间" name="duration" v-if="formState.type !== 'status'">
          <a-input-number 
            v-model:value="formState.duration" 
            :min="1" 
            style="width: 100%"
            addonAfter="秒" 
          />
        </a-form-item>
        
        <a-form-item label="持续时间" name="duration" v-else>
          <a-input-number 
            v-model:value="formState.duration" 
            :min="0" 
            style="width: 100%"
            addonAfter="秒" 
          />
          <div class="ant-form-item-extra">
            <template v-if="formState.threshold === 1">上线状态将立即通知（持续时间无效）</template>
            <template v-else-if="formState.threshold === 2">服务器离线超过此时间后触发报警</template>
            <template v-else>上线立即通知，离线需持续此时间才通知</template>
          </div>
        </a-form-item>
        
        <a-form-item label="启用" name="enabled">
          <a-switch v-model:checked="formState.enabled" />
        </a-form-item>
        
        <a-form-item label="适用服务器" name="server_id" v-if="showServerSettings">
          <a-select
            v-model:value="formState.server_id"
            placeholder="选择服务器"
            allowClear
          >
            <a-select-option :value="0">全局设置</a-select-option>
            <a-select-option v-for="server in servers" :key="server.id" :value="server.id">
              {{ server.name }}
            </a-select-option>
          </a-select>
          <div class="ant-form-item-extra">留空则为全局设置</div>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, computed, onMounted, reactive, watch } from 'vue';
import { useAlertStore, useServerStore } from '@/stores';
import { useUIStore } from '@/stores/uiStore';
import request from '@/utils/request';

export default defineComponent({
  name: 'AlertSettings',
  
  setup() {
    const alertStore = useAlertStore();
    const serverStore = useServerStore();
    const uiStore = useUIStore();
    
    const activeTab = ref('global');
    const selectedServerId = ref<number | undefined>(undefined);
    const settingModalVisible = ref(false);
    const isEditing = ref(false);
    const editingId = ref<number | null>(null);
    
    const formState = reactive({
      type: 'cpu',
      threshold: 80,
      duration: 60,
      enabled: true,
      server_id: 0,
    });
    
    const columns = [
      {
        title: '预警类型',
        dataIndex: 'type',
        key: 'type',
      },
      {
        title: '阈值',
        dataIndex: 'threshold',
        key: 'threshold',
      },
      {
        title: '持续时间',
        dataIndex: 'duration',
        key: 'duration',
        render: (duration: number) => `${duration} 秒`,
      },
      {
        title: '启用',
        key: 'enabled',
      },
      {
        title: '操作',
        key: 'action',
      },
    ];
    
    // 计算属性
    const loading = computed(() => alertStore.loading);
    const globalSettings = computed(() => 
      alertStore.alertSettings.filter(s => s.server_id === 0)
    );
    const serverSettings = computed(() => 
      alertStore.alertSettings.filter(s => s.server_id === selectedServerId.value)
    );
    const servers = computed(() => {
      // 将Record<number, ServerState>转换为数组格式
      return Object.entries(serverStore.servers).map(([id, server]) => ({
        id: parseInt(id),
        name: server.name || `服务器 #${id}`, // 优先使用服务器名称，如果没有则使用ID
        ...server
      }));
    });
    const showServerSettings = computed(() => Object.keys(serverStore.servers).length > 0);
    
    // 生命周期钩子
    onMounted(async () => {
      try {
        await fetchData();
        // 获取服务器列表
        await fetchServers();
      } finally {
        uiStore.stopLoading();
      }
    });
    
    // 方法
    const fetchData = async () => {
      await alertStore.fetchAlertSettings();
    };
    
    // 获取服务器列表
    const fetchServers = async () => {
      try {
        // 由于serverStore没有fetchServers方法，这里直接调用API获取服务器列表
        const response = await request.get('/servers');
        if (response && (response as any).servers) {
          // 处理后端返回的字段名与前端期望的字段名不一致的问题
          (response as any).servers.forEach((server: any) => {
            // 转换字段名并更新服务器状态
            const serverData = {
              id: server.ID,
              name: server.name,
              ip: server.ip,
              port: server.port,
              os: server.os, 
              arch: server.arch,
              cpu_cores: server.cpu_cores,
              cpu_model: server.cpu_model,
              memory_total: server.memory_total,
              disk_total: server.disk_total,
              last_heartbeat: server.last_heartbeat,
              online: server.online,
              status: server.status || 'unknown',
              tags: server.tags,
              description: server.description
            };
            
            // 更新服务器状态，使用转换后的数据
            serverStore.updateServerStatus(serverData.id, serverData.status);
            
            // 如果需要更新更多数据，可以在此处添加
            if (server.system_info) {
              try {
                const sysInfo = JSON.parse(server.system_info);
                // 可以根据需要处理系统信息
                serverStore.updateServerMonitorData(serverData.id, {
                  ...serverData,
                  system_info: sysInfo
                });
              } catch (e) {
                console.error('解析系统信息失败:', e);
              }
            }
          });
        }
      } catch (error) {
        console.error('获取服务器列表失败:', error);
      }
    };
    
    const handleServerChange = async (serverId: number) => {
      await alertStore.fetchAlertSettings(serverId);
    };
    
    const getTypeColor = (type: string) => {
      switch (type) {
        case 'cpu': return 'blue';
        case 'memory': return 'orange';
        case 'network': return 'green';
        case 'status': return 'purple';
        default: return 'default';
      }
    };
    
    const getTypeName = (type: string) => {
      switch (type) {
        case 'cpu': return 'CPU 使用率';
        case 'memory': return '内存使用率';
        case 'network': return '网络流量';
        case 'status': return '服务器状态';
        default: return type;
      }
    };
    
    const getFormattedThreshold = (record: any) => {
      switch (record.type) {
        case 'cpu':
        case 'memory':
          return `${record.threshold}%`;
        case 'network':
          return `${record.threshold} MB/s`;
        case 'status':
          switch (record.threshold) {
            case 1: return '服务器上线时';
            case 2: return '服务器离线时';
            case 3: return '服务器状态变化时';
            default: return '未知状态';
          }
        default:
          return record.threshold;
      }
    };
    
    const getThresholdUnit = (type: string) => {
      switch (type) {
        case 'cpu':
        case 'memory':
          return '%';
        case 'network':
          return 'MB/s';
        case 'status':
          return '';
        default:
          return '';
      }
    };
    
    const showAddSettingModal = () => {
      isEditing.value = false;
      editingId.value = null;
      formState.type = 'cpu';
      formState.threshold = 80;
      formState.duration = 60;
      formState.enabled = true;
      formState.server_id = activeTab.value === 'global' ? 0 : (selectedServerId.value || 0);
      settingModalVisible.value = true;
    };
    
    // 监听预警类型的变化，自动设置默认值
    watch(() => formState.type, (newType) => {
      if (newType === 'status') {
        formState.threshold = 2; // 默认为离线报警
        formState.duration = 10;  // 默认离线超过10秒
      } else if (newType === 'cpu' || newType === 'memory') {
        formState.threshold = 80;
        formState.duration = 60;
      } else if (newType === 'network') {
        formState.threshold = 100;
        formState.duration = 60;
      }
    });
    
    // 监听状态预警阈值的变化，调整持续时间提示
    watch(() => formState.threshold, (newValue) => {
      if (formState.type === 'status') {
        // 如果是上线报警，持续时间可以设为0（立即通知）
        if (newValue === 1) {
          formState.duration = 0;
        } 
        // 如果是离线报警，默认设置为10秒
        else if (newValue === 2 && formState.duration === 0) {
          formState.duration = 10;
        }
        // 如果是状态变化都报警，默认设置为10秒
        else if (newValue === 3 && formState.duration === 0) {
          formState.duration = 10;
        }
      }
    });
    
    const editSetting = (record: any) => {
      isEditing.value = true;
      editingId.value = record.id;
      formState.type = record.type;
      formState.threshold = record.threshold;
      formState.duration = record.duration;
      formState.enabled = record.enabled;
      formState.server_id = record.server_id;
      settingModalVisible.value = true;
    };
    
    const saveSetting = async () => {
      try {
        if (isEditing.value && editingId.value) {
          await alertStore.updateAlertSetting(editingId.value, formState);
        } else {
          await alertStore.createAlertSetting(formState);
        }
        settingModalVisible.value = false;
        await fetchData();
      } catch (error) {
        console.error('保存设置失败:', error);
      }
    };
    
    const deleteSetting = async (id: number) => {
      try {
        await alertStore.deleteAlertSetting(id);
        // 刷新数据，确保列表更新
        if (activeTab.value === 'global') {
          await alertStore.fetchAlertSettings(0);
        } else if (selectedServerId.value) {
          await alertStore.fetchAlertSettings(selectedServerId.value);
        }
      } catch (error) {
        console.error('删除设置失败:', error);
      }
    };
    
    const toggleAlertSetting = async (id: number, enabled: boolean) => {
      try {
        await alertStore.updateAlertSetting(id, { enabled });
        await fetchData();
      } catch (error) {
        console.error('更新设置状态失败:', error);
      }
    };
    
    return {
      activeTab,
      selectedServerId,
      loading,
      columns,
      globalSettings,
      serverSettings,
      servers,
      showServerSettings,
      settingModalVisible,
      formState,
      isEditing,
      
      handleServerChange,
      getTypeColor,
      getTypeName,
      getFormattedThreshold,
      getThresholdUnit,
      showAddSettingModal,
      editSetting,
      saveSetting,
      deleteSetting,
      toggleAlertSetting,
    };
  },
});
</script>

<style scoped>
.alert-settings-container {
  padding: 16px;
}
</style> 