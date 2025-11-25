<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { message, Modal } from 'ant-design-vue';
import {
  UserOutlined,
  PlusOutlined,
  LockOutlined,
  DeleteOutlined,
  EditOutlined,
  ReloadOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';

// 用户列表
const userList = ref<any[]>([]);
const loading = ref(true);

// 新增/编辑用户表单
const userForm = reactive({
  id: null,
  username: '',
  password: '',
  email: '',
  phone: '',
  role: 'user'
});

// 对话框控制
const modalVisible = ref(false);
const modalTitle = ref('新增用户');
const modalLoading = ref(false);
const isEdit = ref(false);

// 获取用户列表
const fetchUsers = async () => {
  loading.value = true;
  try {
    const response = await request.get('/users');
    userList.value = response.data || [];
  } catch (error) {
    console.error('获取用户列表失败:', error);
    message.error('获取用户列表失败');
  } finally {
    loading.value = false;
  }
};

// 删除用户
const deleteUser = (userId: number, username: string) => {
  Modal.confirm({
    title: '确认删除',
    content: `确定要删除用户 ${username} 吗？此操作不可恢复。`,
    okText: '确认',
    cancelText: '取消',
    okType: 'danger',
    onOk: async () => {
      try {
        await request.delete(`/users/${userId}`);
        message.success('用户已删除');
        fetchUsers();
      } catch (error) {
        console.error('删除用户失败:', error);
        message.error('删除用户失败');
      }
    }
  });
};

// 打开新增用户对话框
const openAddUserModal = () => {
  resetUserForm();
  modalTitle.value = '新增用户';
  isEdit.value = false;
  modalVisible.value = true;
};

// 打开编辑用户对话框
const openEditUserModal = (user: any) => {
  resetUserForm();
  userForm.id = user.id;
  userForm.username = user.username;
  userForm.email = user.email || '';
  userForm.phone = user.phone || '';
  userForm.role = user.role || 'user';
  
  modalTitle.value = '编辑用户';
  isEdit.value = true;
  modalVisible.value = true;
};

// 重置用户表单
const resetUserForm = () => {
  userForm.id = null;
  userForm.username = '';
  userForm.password = '';
  userForm.email = '';
  userForm.phone = '';
  userForm.role = 'user';
};

// 提交用户表单
const submitUserForm = async () => {
  // 表单验证
  if (!userForm.username) {
    message.error('请输入用户名');
    return;
  }
  
  if (!isEdit.value && !userForm.password) {
    message.error('请输入密码');
    return;
  }
  
  modalLoading.value = true;
  
  try {
    if (isEdit.value) {
      // 编辑用户
      await request.put(`/users/${userForm.id}`, {
        username: userForm.username,
        email: userForm.email,
        phone: userForm.phone,
        role: userForm.role,
        password: userForm.password || undefined
      });
      message.success('用户已更新');
    } else {
      // 新增用户
      await request.post('/users', {
        username: userForm.username,
        password: userForm.password,
        email: userForm.email,
        phone: userForm.phone,
        role: userForm.role
      });
      message.success('用户已创建');
    }
    
    modalVisible.value = false;
    fetchUsers();
  } catch (error) {
    console.error('操作失败:', error);
    message.error('操作失败');
  } finally {
    modalLoading.value = false;
  }
};

// 重置用户密码
const resetUserPassword = (userId: number, username: string) => {
  Modal.confirm({
    title: '重置密码',
    content: `确定要重置用户 ${username} 的密码吗？`,
    okText: '确认',
    cancelText: '取消',
    onOk: async () => {
      try {
        const response = await request.post(`/users/${userId}/reset-password`);
        Modal.success({
          title: '密码已重置',
          content: `新密码: ${response.data.password}`
        });
      } catch (error) {
        console.error('重置密码失败:', error);
        message.error('重置密码失败');
      }
    }
  });
};

// 页面挂载时获取用户列表
onMounted(() => {
  fetchUsers();
});
</script>

<template>
  <div class="user-management-container">
    <a-page-header
      title="用户管理"
      sub-title="管理系统用户"
    >
      <template #extra>
        <a-space>
          <a-button
            type="primary"
            :icon="PlusOutlined"
            @click="openAddUserModal"
          >
            新增用户
          </a-button>
          <a-button
            :icon="ReloadOutlined"
            @click="fetchUsers"
            :loading="loading"
          >
            刷新
          </a-button>
        </a-space>
      </template>
    </a-page-header>
    
    <div class="user-management-content">
      <a-table
        :dataSource="userList"
        :loading="loading"
        :pagination="{ pageSize: 10 }"
        rowKey="id"
      >
        <a-table-column title="用户名" dataIndex="username">
          <template #customRender="{ text }">
            <span>
              <UserOutlined style="margin-right: 8px" />
              {{ text }}
            </span>
          </template>
        </a-table-column>
        <a-table-column title="邮箱" dataIndex="email">
          <template #customRender="{ text }">
            {{ text || '未设置' }}
          </template>
        </a-table-column>
        <a-table-column title="电话" dataIndex="phone">
          <template #customRender="{ text }">
            {{ text || '未设置' }}
          </template>
        </a-table-column>
        <a-table-column title="角色" dataIndex="role">
          <template #customRender="{ text }">
            <a-tag :color="text === 'admin' ? '#f50' : '#108ee9'">
              {{ text === 'admin' ? '管理员' : '普通用户' }}
            </a-tag>
          </template>
        </a-table-column>
        <a-table-column title="最后登录时间" dataIndex="last_login">
          <template #customRender="{ text }">
            {{ text ? new Date(text).toLocaleString() : '从未登录' }}
          </template>
        </a-table-column>
        <a-table-column title="操作">
          <template #customRender="{ record }">
            <a-space>
              <a-button
                type="primary"
                size="small"
                :icon="EditOutlined"
                @click="openEditUserModal(record)"
              >
                编辑
              </a-button>
              <a-button
                type="primary"
                size="small"
                :icon="LockOutlined"
                @click="resetUserPassword(record.id, record.username)"
              >
                重置密码
              </a-button>
              <a-button
                type="primary"
                danger
                size="small"
                :icon="DeleteOutlined"
                @click="deleteUser(record.id, record.username)"
                :disabled="record.role === 'admin' && record.username === 'admin'"
              >
                删除
              </a-button>
            </a-space>
          </template>
        </a-table-column>
      </a-table>
    </div>
    
    <!-- 用户表单对话框 -->
    <a-modal
      v-model:visible="modalVisible"
      :title="modalTitle"
      @ok="submitUserForm"
      :confirmLoading="modalLoading"
      :maskClosable="false"
    >
      <a-form layout="vertical">
        <a-form-item label="用户名" required>
          <a-input
            v-model:value="userForm.username"
            placeholder="请输入用户名"
            :prefix="UserOutlined"
          />
        </a-form-item>
        
        <a-form-item :label="isEdit ? '密码 (不修改请留空)' : '密码'" :required="!isEdit">
          <a-input-password
            v-model:value="userForm.password"
            :placeholder="isEdit ? '不修改请留空' : '请输入密码'"
            :prefix="LockOutlined"
          />
        </a-form-item>
        
        <a-form-item label="电子邮箱">
          <a-input
            v-model:value="userForm.email"
            placeholder="请输入电子邮箱"
          />
        </a-form-item>
        
        <a-form-item label="手机号码">
          <a-input
            v-model:value="userForm.phone"
            placeholder="请输入手机号码"
          />
        </a-form-item>
        
        <a-form-item label="角色">
          <a-select
            v-model:value="userForm.role"
            placeholder="请选择角色"
          >
            <a-select-option value="user">普通用户</a-select-option>
            <a-select-option value="admin">管理员</a-select-option>
          </a-select>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.user-management-container {
  padding: 24px;
  background: #fff;
  border-radius: 4px;
}

.user-management-content {
  margin-top: 24px;
}
</style> 