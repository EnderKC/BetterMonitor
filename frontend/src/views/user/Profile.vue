<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { message } from 'ant-design-vue';
import {
  UserOutlined,
  LockOutlined,
  MailOutlined,
  PhoneOutlined,
  SaveOutlined,
  CameraOutlined
} from '@ant-design/icons-vue';
import request from '../../utils/request';

// 用户资料
const userInfo = ref<any>({});
const loading = ref(true);

// 编辑表单
const formState = reactive({
  username: '',
  email: '',
  phone: '',
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
});

// 是否显示密码修改表单
const showPasswordForm = ref(false);

// 获取用户资料
const fetchUserProfile = async () => {
  loading.value = true;
  try {
    const response = await request.get('/profile');
    userInfo.value = response.data || {};

    // 填充表单数据
    formState.username = userInfo.value.username || '';
    formState.email = userInfo.value.email || '';
    formState.phone = userInfo.value.phone || '';
  } catch (error) {
    console.error('获取用户资料失败:', error);
    message.error('获取用户资料失败');
  } finally {
    loading.value = false;
  }
};

// 更新用户资料
const updateProfile = async () => {
  try {
    await request.put('/profile', {
      username: formState.username,
      email: formState.email,
      phone: formState.phone
    });

    message.success('个人资料已更新');
    fetchUserProfile();
  } catch (error) {
    console.error('更新用户资料失败:', error);
    message.error('更新用户资料失败');
  }
};

// 修改密码
const changePassword = async () => {
  // 验证密码
  if (!formState.oldPassword) {
    message.error('请输入当前密码');
    return;
  }

  if (!formState.newPassword) {
    message.error('请输入新密码');
    return;
  }

  if (formState.newPassword !== formState.confirmPassword) {
    message.error('两次输入的新密码不一致');
    return;
  }

  try {
    await request.post('/change-password', {
      old_password: formState.oldPassword,
      new_password: formState.newPassword
    });

    message.success('密码已修改，请重新登录');

    // 清空密码表单
    formState.oldPassword = '';
    formState.newPassword = '';
    formState.confirmPassword = '';

    // 隐藏密码表单
    showPasswordForm.value = false;

    // 清除登录状态，跳转到登录页
    localStorage.removeItem('server_ops_token');
    localStorage.removeItem('server_ops_user');
    setTimeout(() => {
      window.location.href = '/login';
    }, 1500);
  } catch (error) {
    console.error('修改密码失败:', error);
    message.error('修改密码失败，请确认当前密码是否正确');
  }
};

// 页面挂载时获取用户资料
onMounted(() => {
  fetchUserProfile();
});
</script>

<template>
  <div class="profile-container">
    <div class="page-header">
      <h1 class="page-title">个人资料</h1>
      <p class="page-subtitle">管理您的个人信息和安全设置</p>
    </div>

    <div class="profile-content">
      <a-spin :spinning="loading">
        <a-row :gutter="[24, 24]">
          <!-- 左侧个人卡片 -->
          <a-col :xs="24" :md="8" :lg="8">
            <div class="ios-card profile-card">
              <div class="avatar-section">
                <div class="avatar-wrapper">
                  <a-avatar :size="100" class="user-avatar">
                    <template #icon><user-outlined /></template>
                  </a-avatar>
                  <div class="avatar-glow"></div>
                  <div class="avatar-edit-btn">
                    <camera-outlined />
                  </div>
                </div>
                <h2 class="user-name">{{ userInfo.username }}</h2>
                <div class="user-role">
                  <span class="role-badge">{{ userInfo.role === 'admin' ? '管理员' : '普通用户' }}</span>
                </div>
              </div>

              <div class="info-list">
                <div class="info-item">
                  <div class="info-icon"><mail-outlined /></div>
                  <div class="info-content">
                    <div class="info-label">邮箱</div>
                    <div class="info-value">{{ userInfo.email || '未设置' }}</div>
                  </div>
                </div>
                <div class="info-item">
                  <div class="info-icon"><phone-outlined /></div>
                  <div class="info-content">
                    <div class="info-label">手机</div>
                    <div class="info-value">{{ userInfo.phone || '未设置' }}</div>
                  </div>
                </div>
                <div class="info-item">
                  <div class="info-icon"><user-outlined /></div>
                  <div class="info-content">
                    <div class="info-label">最后登录</div>
                    <div class="info-value">{{ userInfo.last_login ? new Date(userInfo.last_login).toLocaleString() :
                      '未知' }}</div>
                  </div>
                </div>
              </div>

              <div class="card-actions">
                <a-button class="ios-btn ios-btn-secondary" block @click="showPasswordForm = !showPasswordForm">
                  {{ showPasswordForm ? '取消修改' : '修改密码' }}
                </a-button>
              </div>
            </div>
          </a-col>

          <!-- 右侧编辑表单 -->
          <a-col :xs="24" :md="16" :lg="16">
            <!-- 基本信息表单 -->
            <div class="ios-card form-card">
              <div class="card-header">
                <h3 class="card-title">基本信息</h3>
              </div>
              <div class="card-body">
                <a-form layout="vertical" class="ios-form">
                  <a-row :gutter="24">
                    <a-col :span="24">
                      <a-form-item label="用户名">
                        <a-input v-model:value="formState.username" placeholder="请输入用户名" class="ios-input">
                          <template #prefix><user-outlined class="input-icon" /></template>
                        </a-input>
                      </a-form-item>
                    </a-col>

                    <a-col :span="12">
                      <a-form-item label="电子邮箱">
                        <a-input v-model:value="formState.email" placeholder="请输入电子邮箱" class="ios-input">
                          <template #prefix><mail-outlined class="input-icon" /></template>
                        </a-input>
                      </a-form-item>
                    </a-col>

                    <a-col :span="12">
                      <a-form-item label="手机号码">
                        <a-input v-model:value="formState.phone" placeholder="请输入手机号码" class="ios-input">
                          <template #prefix><phone-outlined class="input-icon" /></template>
                        </a-input>
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <div class="form-actions">
                    <a-button type="primary" class="ios-btn ios-btn-primary" @click="updateProfile">
                      <template #icon><save-outlined /></template>
                      保存更改
                    </a-button>
                  </div>
                </a-form>
              </div>
            </div>

            <!-- 修改密码表单 -->
            <transition name="slide-fade">
              <div v-if="showPasswordForm" class="ios-card form-card mt-24">
                <div class="card-header">
                  <h3 class="card-title">安全设置</h3>
                </div>
                <div class="card-body">
                  <a-form layout="vertical" class="ios-form">
                    <a-form-item label="当前密码" required>
                      <a-input-password v-model:value="formState.oldPassword" placeholder="请输入当前密码" class="ios-input">
                        <template #prefix><lock-outlined class="input-icon" /></template>
                      </a-input-password>
                    </a-form-item>

                    <a-row :gutter="24">
                      <a-col :span="12">
                        <a-form-item label="新密码" required>
                          <a-input-password v-model:value="formState.newPassword" placeholder="请输入新密码"
                            class="ios-input">
                            <template #prefix><lock-outlined class="input-icon" /></template>
                          </a-input-password>
                        </a-form-item>
                      </a-col>
                      <a-col :span="12">
                        <a-form-item label="确认新密码" required>
                          <a-input-password v-model:value="formState.confirmPassword" placeholder="请再次输入新密码"
                            class="ios-input">
                            <template #prefix><lock-outlined class="input-icon" /></template>
                          </a-input-password>
                        </a-form-item>
                      </a-col>
                    </a-row>

                    <div class="form-actions">
                      <a-button type="primary" danger class="ios-btn ios-btn-danger" @click="changePassword">
                        确认修改密码
                      </a-button>
                    </div>
                  </a-form>
                </div>
              </div>
            </transition>
          </a-col>
        </a-row>
      </a-spin>
    </div>
  </div>
</template>

<style scoped>
.profile-container {
  padding: 24px;
  min-height: 100%;
}

.page-header {
  margin-bottom: 32px;
}

.page-title {
  font-size: 28px;
  font-weight: 700;
  color: rgba(0, 0, 0, 0.85);
  margin-bottom: 8px;
  letter-spacing: -0.5px;
}

.page-subtitle {
  font-size: 14px;
  color: rgba(0, 0, 0, 0.45);
}

/* iOS Card Style */
.ios-card {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: 20px;
  border: 1px solid rgba(255, 255, 255, 0.3);
  box-shadow:
    0 4px 24px -1px rgba(0, 0, 0, 0.05),
    0 0 1px 0 rgba(0, 0, 0, 0.1);
  overflow: hidden;
  transition: all 0.3s ease;
}

.ios-card:hover {
  transform: translateY(-2px);
  box-shadow:
    0 8px 32px -4px rgba(0, 0, 0, 0.08),
    0 0 1px 0 rgba(0, 0, 0, 0.1);
}

.profile-card {
  padding: 32px 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
}

.avatar-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-bottom: 32px;
  position: relative;
}

.avatar-wrapper {
  position: relative;
  margin-bottom: 16px;
}

.user-avatar {
  background: linear-gradient(135deg, #1890ff 0%, #096dd9 100%);
  box-shadow: 0 8px 24px rgba(24, 144, 255, 0.25);
  border: 4px solid rgba(255, 255, 255, 0.8);
}

.avatar-glow {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 100%;
  height: 100%;
  border-radius: 50%;
  background: #1890ff;
  filter: blur(20px);
  opacity: 0.2;
  z-index: -1;
}

.avatar-edit-btn {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 32px;
  height: 32px;
  background: #fff;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  cursor: pointer;
  color: #666;
  transition: all 0.2s;
}

.avatar-edit-btn:hover {
  color: #1890ff;
  transform: scale(1.1);
}

.user-name {
  font-size: 24px;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.85);
  margin-bottom: 8px;
}

.role-badge {
  display: inline-block;
  padding: 4px 12px;
  background: rgba(24, 144, 255, 0.1);
  color: #1890ff;
  border-radius: 100px;
  font-size: 12px;
  font-weight: 500;
}

.info-list {
  width: 100%;
  margin-bottom: 32px;
}

.info-item {
  display: flex;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
}

.info-item:last-child {
  border-bottom: none;
}

.info-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.04);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-right: 16px;
  color: #666;
}

.info-content {
  flex: 1;
}

.info-label {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 2px;
}

.info-value {
  font-size: 14px;
  color: rgba(0, 0, 0, 0.85);
  font-weight: 500;
}

.card-actions {
  width: 100%;
  margin-top: auto;
}

/* Form Card */
.form-card {
  padding: 0;
}

.card-header {
  padding: 20px 24px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
}

.card-title {
  font-size: 18px;
  font-weight: 600;
  margin: 0;
  color: rgba(0, 0, 0, 0.85);
}

.card-body {
  padding: 24px;
}

.ios-form :deep(.ant-form-item-label > label) {
  font-size: 14px;
  color: rgba(0, 0, 0, 0.65);
  font-weight: 500;
}

.ios-input {
  border-radius: 10px;
  border-color: rgba(0, 0, 0, 0.15);
  padding: 8px 12px;
  background: rgba(255, 255, 255, 0.5);
  transition: all 0.3s;
}

.ios-input:hover,
.ios-input:focus {
  background: #fff;
  border-color: #1890ff;
  box-shadow: 0 0 0 2px rgba(24, 144, 255, 0.1);
}

.input-icon {
  color: rgba(0, 0, 0, 0.25);
}

.form-actions {
  margin-top: 24px;
  display: flex;
  justify-content: flex-end;
}

/* iOS Buttons */
.ios-btn {
  height: 40px;
  border-radius: 20px;
  font-weight: 500;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  border: none;
  transition: all 0.3s;
}

.ios-btn-primary {
  background: linear-gradient(135deg, #1890ff 0%, #096dd9 100%);
}

.ios-btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(24, 144, 255, 0.3);
}

.ios-btn-secondary {
  background: rgba(0, 0, 0, 0.04);
  color: rgba(0, 0, 0, 0.65);
  box-shadow: none;
}

.ios-btn-secondary:hover {
  background: rgba(0, 0, 0, 0.08);
  color: rgba(0, 0, 0, 0.85);
}

.ios-btn-danger {
  background: linear-gradient(135deg, #ff4d4f 0%, #cf1322 100%);
  color: #fff;
}

.ios-btn-danger:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(255, 77, 79, 0.3);
}

.mt-24 {
  margin-top: 24px;
}

/* Transitions */
.slide-fade-enter-active,
.slide-fade-leave-active {
  transition: all 0.3s ease-out;
}

.slide-fade-enter-from,
.slide-fade-leave-to {
  transform: translateY(-20px);
  opacity: 0;
}
</style>

<style>
.dark .page-title {
  color: #e0e0e0;
}

.dark .page-subtitle {
  color: #8c8c8c;
}

.dark .ios-card {
  background: rgba(30, 30, 30, 0.7);
  border: 1px solid rgba(255, 255, 255, 0.05);
  box-shadow: 0 4px 24px -1px rgba(0, 0, 0, 0.2);
}

.dark .ios-card:hover {
  box-shadow: 0 8px 32px -4px rgba(0, 0, 0, 0.3);
}

.dark .user-avatar {
  border: 4px solid rgba(255, 255, 255, 0.1);
}

.dark .avatar-edit-btn {
  background: #333;
  color: #ccc;
}

.dark .user-name {
  color: #e0e0e0;
}

.dark .info-item {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.dark .info-icon {
  background: rgba(255, 255, 255, 0.05);
  color: #aaa;
}

.dark .info-label {
  color: #8c8c8c;
}

.dark .info-value {
  color: #e0e0e0;
}

.dark .card-header {
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.dark .card-title {
  color: #e0e0e0;
}

.dark .ios-form .ant-form-item-label > label {
  color: #ccc;
}

.dark .ios-input {
  background: rgba(0, 0, 0, 0.2);
  border-color: rgba(255, 255, 255, 0.1);
  color: #e0e0e0;
}

.dark .ios-input::placeholder {
  color: #666;
}

.dark .ios-input:hover,
.dark .ios-input:focus {
  background: rgba(0, 0, 0, 0.4);
  border-color: #177ddc;
}

.dark .input-icon {
  color: #666;
}

.dark .ios-btn-secondary {
  background: rgba(255, 255, 255, 0.1);
  color: #ccc;
}

.dark .ios-btn-secondary:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}
</style>