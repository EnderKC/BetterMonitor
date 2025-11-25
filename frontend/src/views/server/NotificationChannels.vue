<template>
  <div class="notification-channels-container">
    <a-card title="通知渠道管理" :bordered="false">
      <template #extra>
        <a-button type="primary" @click="showAddChannelModal">添加通知渠道</a-button>
      </template>

      <a-spin :spinning="loading.channels">
        <a-table 
          :dataSource="notificationChannels" 
          :columns="columns" 
          rowKey="id" 
          :pagination="false"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'type'">
              <a-tag :color="getTypeColor(record.type)">{{ getTypeName(record.type) }}</a-tag>
            </template>
            <template v-if="column.key === 'enabled'">
              <a-switch 
                :checked="record.enabled" 
                @change="(checked) => toggleChannel(record.id, checked)"
              />
            </template>
            <template v-if="column.key === 'action'">
              <a-button type="link" size="small" @click="editChannel(record)">编辑</a-button>
              <a-button 
                type="link" 
                size="small" 
                @click="testChannel(record)"
                :disabled="!record.enabled"
              >
                测试
              </a-button>
              <a-popconfirm
                title="确定要删除这个通知渠道吗？"
                ok-text="确定"
                cancel-text="取消"
                @confirm="deleteChannel(record.id)"
              >
                <a-button type="link" danger size="small">删除</a-button>
              </a-popconfirm>
            </template>
          </template>
        </a-table>
        <a-empty v-if="notificationChannels.length === 0" description="暂无通知渠道" />
      </a-spin>
    </a-card>

    <!-- 添加/编辑通知渠道的弹窗 -->
    <a-modal
      v-model:visible="channelModalVisible"
      :title="isEditing ? '编辑通知渠道' : '添加通知渠道'"
      okText="保存"
      cancelText="取消"
      @ok="saveChannel"
      width="650px"
    >
      <a-form :model="formState" :label-col="{ span: 6 }" :wrapper-col="{ span: 16 }">
        <a-form-item label="渠道名称" name="name">
          <a-input v-model:value="formState.name" placeholder="请输入渠道名称" />
        </a-form-item>
        
        <a-form-item label="渠道类型" name="type">
          <a-select 
            v-model:value="formState.type"
            placeholder="选择渠道类型"
            :disabled="isEditing"
            @change="handleTypeChange"
          >
            <a-select-option value="email">邮件</a-select-option>
            <a-select-option value="serverchan">Server酱</a-select-option>
          </a-select>
        </a-form-item>
        
        <a-form-item label="启用" name="enabled">
          <a-switch v-model:checked="formState.enabled" />
        </a-form-item>
        
        <!-- 邮件配置表单 -->
        <template v-if="formState.type === 'email'">
          <a-form-item label="SMTP服务器" name="smtp_host">
            <a-input v-model:value="configForm.smtp_host" placeholder="例如: smtp.gmail.com" />
          </a-form-item>
          
          <a-form-item label="SMTP端口" name="smtp_port">
            <a-input-number v-model:value="configForm.smtp_port" :min="1" :max="65535" style="width: 100%" />
          </a-form-item>
          
          <a-form-item label="用户名" name="username">
            <a-input v-model:value="configForm.username" placeholder="邮箱用户名" />
          </a-form-item>
          
          <a-form-item label="密码" name="password">
            <a-input-password 
              v-model:value="configForm.password" 
              placeholder="邮箱密码或应用专用密码" 
              :visibilityToggle="false"
            />
          </a-form-item>
          
          <a-form-item label="发件人邮箱" name="from_email">
            <a-input v-model:value="configForm.from_email" placeholder="发件人邮箱地址" />
          </a-form-item>
          
          <a-form-item label="发件人名称" name="from_name">
            <a-input v-model:value="configForm.from_name" placeholder="发件人显示名称" />
          </a-form-item>
          
          <a-form-item label="收件人邮箱" name="to_email">
            <a-input v-model:value="configForm.to_email" placeholder="接收通知的邮箱地址" />
          </a-form-item>
          
          <a-form-item label="使用TLS" name="use_tls">
            <a-switch v-model:checked="configForm.use_tls" />
          </a-form-item>
        </template>
        
        <!-- Server酱配置表单 -->
        <template v-if="formState.type === 'serverchan'">
          <a-form-item label="SendKey" name="sendkey">
            <a-input 
              v-model:value="configForm.sendkey" 
              placeholder="Server酱的SendKey" 
            />
            <div class="ant-form-item-extra">
              <a href="https://sct.ftqq.com/" target="_blank">获取SendKey</a>
            </div>
          </a-form-item>
        </template>
      </a-form>
    </a-modal>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, computed, onMounted, reactive, watch } from 'vue';
import { useAlertStore } from '@/stores';
import { message } from 'ant-design-vue';

export default defineComponent({
  name: 'NotificationChannels',
  
  setup() {
    const alertStore = useAlertStore();
    
    const channelModalVisible = ref(false);
    const isEditing = ref(false);
    const editingId = ref<number | null>(null);
    
    const formState = reactive({
      name: '',
      type: 'email',
      enabled: true,
    });
    
    // 配置表单，针对不同类型的通知渠道
    const configForm = reactive({
      // 邮件配置
      smtp_host: '',
      smtp_port: 587,
      username: '',
      password: '',
      from_email: '',
      from_name: '',
      to_email: '',
      use_tls: true,
      
      // Server酱配置
      sendkey: '',
    });
    
    const columns = [
      {
        title: '渠道名称',
        dataIndex: 'name',
        key: 'name',
      },
      {
        title: '渠道类型',
        dataIndex: 'type',
        key: 'type',
      },
      {
        title: '启用',
        key: 'enabled',
      },
      {
        title: '创建时间',
        dataIndex: 'created_at',
        key: 'created_at',
        render: (text: string) => new Date(text).toLocaleString(),
      },
      {
        title: '操作',
        key: 'action',
      },
    ];
    
    // 计算属性
    const loading = computed(() => alertStore.loading);
    const notificationChannels = computed(() => alertStore.notificationChannels);
    
    // 生命周期钩子
    onMounted(async () => {
      await fetchData();
    });
    
    // 监听类型变化，重置表单
    watch(() => formState.type, () => {
      resetConfigForm();
    });
    
    // 方法
    const fetchData = async () => {
      await alertStore.fetchNotificationChannels();
    };
    
    const resetConfigForm = () => {
      if (formState.type === 'email') {
        configForm.smtp_host = '';
        configForm.smtp_port = 587;
        configForm.username = '';
        configForm.password = '';
        configForm.from_email = '';
        configForm.from_name = '';
        configForm.to_email = '';
        configForm.use_tls = true;
      } else if (formState.type === 'serverchan') {
        configForm.sendkey = '';
      }
    };
    
    const getTypeColor = (type: string) => {
      switch (type) {
        case 'email': return 'blue';
        case 'serverchan': return 'orange';
        default: return 'default';
      }
    };
    
    const getTypeName = (type: string) => {
      switch (type) {
        case 'email': return '邮件';
        case 'serverchan': return 'Server酱';
        default: return type;
      }
    };
    
    const handleTypeChange = () => {
      resetConfigForm();
    };
    
    const parseConfig = (configStr: string) => {
      try {
        return JSON.parse(configStr);
      } catch (e) {
        return {};
      }
    };
    
    const showAddChannelModal = () => {
      isEditing.value = false;
      editingId.value = null;
      formState.name = '';
      formState.type = 'email';
      formState.enabled = true;
      resetConfigForm();
      channelModalVisible.value = true;
    };
    
    const editChannel = (record: any) => {
      isEditing.value = true;
      editingId.value = record.id;
      formState.name = record.name;
      formState.type = record.type;
      formState.enabled = record.enabled;
      
      // 解析配置
      const config = parseConfig(record.config);
      if (record.type === 'email') {
        configForm.smtp_host = config.smtp_host || '';
        configForm.smtp_port = parseInt(config.smtp_port) || 587;
        configForm.username = config.username || '';
        configForm.password = ''; // 不回显密码
        configForm.from_email = config.from_email || '';
        configForm.from_name = config.from_name || '';
        configForm.to_email = config.to_email || '';
        configForm.use_tls = config.use_tls === 'true' || false;
      } else if (record.type === 'serverchan') {
        configForm.sendkey = ''; // 不回显密钥
      }
      
      channelModalVisible.value = true;
    };
    
    const getConfigString = () => {
      if (formState.type === 'email') {
        return JSON.stringify({
          smtp_host: configForm.smtp_host,
          smtp_port: configForm.smtp_port.toString(),
          username: configForm.username,
          password: configForm.password,
          from_email: configForm.from_email,
          from_name: configForm.from_name,
          to_email: configForm.to_email,
          use_tls: configForm.use_tls.toString(),
        });
      } else if (formState.type === 'serverchan') {
        return JSON.stringify({
          sendkey: configForm.sendkey,
        });
      }
      return '';
    };
    
    const validateForm = () => {
      if (!formState.name) {
        message.error('请输入渠道名称');
        return false;
      }
      
      if (formState.type === 'email') {
        if (!configForm.smtp_host) {
          message.error('请输入SMTP服务器地址');
          return false;
        }
        if (!configForm.username) {
          message.error('请输入用户名');
          return false;
        }
        if (!isEditing.value && !configForm.password) {
          message.error('请输入密码');
          return false;
        }
        if (!configForm.from_email) {
          message.error('请输入发件人邮箱');
          return false;
        }
        if (!configForm.to_email) {
          message.error('请输入收件人邮箱');
          return false;
        }
      } else if (formState.type === 'serverchan') {
        if (!isEditing.value && !configForm.sendkey) {
          message.error('请输入SendKey');
          return false;
        }
      }
      
      return true;
    };
    
    const saveChannel = async () => {
      if (!validateForm()) return;
      
      try {
        const channelData = {
          name: formState.name,
          type: formState.type,
          enabled: formState.enabled,
          config: getConfigString(),
        };
        
        if (isEditing.value && editingId.value) {
          // 处理更新时的密码/密钥问题
          if ((formState.type === 'email' && !configForm.password) || 
              (formState.type === 'serverchan' && !configForm.sendkey)) {
            channelData.config = "[UNCHANGED]";
          }
          
          await alertStore.updateNotificationChannel(editingId.value, channelData);
        } else {
          await alertStore.createNotificationChannel(channelData);
        }
        
        channelModalVisible.value = false;
        await fetchData();
      } catch (error) {
        console.error('保存通知渠道失败:', error);
      }
    };
    
    const deleteChannel = async (id: number) => {
      try {
        await alertStore.deleteNotificationChannel(id);
      } catch (error) {
        console.error('删除通知渠道失败:', error);
      }
    };
    
    const toggleChannel = async (id: number, enabled: boolean) => {
      try {
        await alertStore.updateNotificationChannel(id, { enabled });
        await fetchData();
      } catch (error) {
        console.error('更新通知渠道状态失败:', error);
      }
    };
    
    const testChannel = async (record: any) => {
      console.log('测试通知渠道ID:', record.id, typeof record.id);
      if (!record.id) {
        message.error('通知渠道ID无效');
        return;
      }
      try {
        await alertStore.testNotificationChannel(record.id);
      } catch (error) {
        console.error('测试通知失败:', error);
      }
    };
    
    return {
      loading,
      columns,
      notificationChannels,
      channelModalVisible,
      formState,
      configForm,
      isEditing,
      
      getTypeColor,
      getTypeName,
      handleTypeChange,
      showAddChannelModal,
      editChannel,
      saveChannel,
      deleteChannel,
      toggleChannel,
      testChannel,
    };
  },
});
</script>

<style scoped>
.notification-channels-container {
  padding: 16px;
}
</style> 