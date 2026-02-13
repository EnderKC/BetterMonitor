<script setup lang="ts">
import { ref, reactive, h } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { message } from 'ant-design-vue';
import request from '../../utils/request';
import { setToken, setUser } from '../../utils/auth';
import { UserOutlined, LockOutlined, RightOutlined } from '@ant-design/icons-vue';

// 图标组件
const UserOutlinedIcon = () => h(UserOutlined);
const LockOutlinedIcon = () => h(LockOutlined);
const RightOutlinedIcon = () => h(RightOutlined);

// 路由
const router = useRouter();
const route = useRoute();

// 登录表单
const formRef = ref();
const formState = reactive({
  username: '',
  password: '',
});

// 加载状态
const loading = ref(false);

// 处理登录
const handleLogin = () => {
  formRef.value.validate().then(() => {
    loading.value = true;
    console.log('开始登录请求...');

    request.post('/login', formState)
      .then((response: any) => {
        console.log('登录响应:', response);

        // 确保响应中包含token和user
        if (!response.token) {
          message.error('登录成功但缺少令牌信息');
          loading.value = false;
          return;
        }

        // 保存登录信息
        setToken(response.token);
        setUser(response.user);

        message.success('登录成功');

        // 跳转到重定向页面或默认页面
        const redirectPath = (route.query.redirect as string) || '/admin';
        console.log('准备跳转到:', redirectPath);

        // 使用setTimeout确保跳转在下一个事件循环执行
        setTimeout(() => {
          router.push(redirectPath);
        }, 100);
      })
      .catch((error) => {
        console.error('登录失败:', error);
        // 错误已经在拦截器中处理
      })
      .finally(() => {
        loading.value = false;
      });
  });
};

// 表单验证规则
const rules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
  ],
};
</script>

<template>
  <div class="login-container">
    <div class="login-card glass-card">
      <div class="login-title">
        <h2 class="gradient-text">Better Monitor</h2>
        <p>登录账号</p>
      </div>

      <a-form :model="formState" :rules="rules" ref="formRef" layout="vertical" @finish="handleLogin"
        class="login-form">
        <a-form-item name="username" label="用户名" class="login-form-item">
          <a-input v-model:value="formState.username" placeholder="请输入用户名" size="large" :prefix="UserOutlinedIcon()" />
        </a-form-item>

        <a-form-item name="password" label="密码" class="login-form-item">
          <a-input-password v-model:value="formState.password" placeholder="请输入密码" size="large"
            :prefix="LockOutlinedIcon()" />
        </a-form-item>

        <a-form-item>
          <a-button type="primary" html-type="submit" :loading="loading" block size="large"
            class="login-button glow-effect">
            登录
          </a-button>
        </a-form-item>

        <div class="login-tips">
          您必须要登录才能管理服务器
        </div>

        <div class="other-links">
          <router-link to="/dashboard" class="dashboard-link">
            <span>无需登录，直接查看探针页面</span>
            <component :is="RightOutlinedIcon" />
          </router-link>
        </div>
      </a-form>

      <div class="decoration-circle decoration-1"></div>
      <div class="decoration-circle decoration-2"></div>
    </div>
  </div>
</template>

<style scoped>
.login-container {
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background-color: var(--body-bg);
  background-image:
    radial-gradient(at 30% 20%, var(--bg-radial-primary) 0px, transparent 50%),
    radial-gradient(at 80% 40%, var(--bg-radial-success) 0px, transparent 50%),
    radial-gradient(at 10% 70%, rgba(255, 149, 0, 0.08) 0px, transparent 50%);
  background-attachment: fixed;
  position: relative;
  overflow: hidden;
}

.login-card {
  width: 420px;
  padding: var(--spacing-3xl) var(--spacing-2xl);
  position: relative;
  z-index: 1;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.7) !important;
  backdrop-filter: blur(var(--blur-md));
  -webkit-backdrop-filter: blur(var(--blur-md));
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-lg);
  border: 1px solid var(--alpha-white-40);
}

.login-card::before {
  content: '';
  position: absolute;
  top: -1px;
  left: 10%;
  width: 80%;
  height: 1px;
  background: linear-gradient(90deg, transparent, var(--alpha-white-80), transparent);
  z-index: 2;
}

.login-title {
  text-align: center;
  margin-bottom: 36px;
  position: relative;
  z-index: 2;
}

.login-title h2 {
  margin-bottom: var(--spacing-sm);
  font-weight: var(--font-weight-bold);
  font-size: var(--font-size-4xl);
  letter-spacing: -0.5px;
  background: var(--gradient-brand);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.login-title p {
  color: var(--text-secondary);
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-medium);
}

.login-form {
  position: relative;
  z-index: 2;
}

:deep(.ant-form-item-label > label) {
  font-weight: var(--font-weight-medium);
  color: var(--text-secondary);
}

:deep(.ant-input-affix-wrapper) {
  border-radius: var(--radius-md);
  box-shadow: none;
  border: 1px solid var(--input-border);
  transition: var(--transition);
  background: var(--input-bg);
  padding: 10px var(--spacing-sm);
  backdrop-filter: blur(var(--blur-sm));
}

:deep(.ant-input-affix-wrapper .ant-input) {
  background: transparent !important;
}

:deep(.ant-input-affix-wrapper:hover),
:deep(.ant-input-affix-wrapper:focus),
:deep(.ant-input-affix-wrapper-focused) {
  border-color: var(--primary-color);
  background: var(--input-focus-bg);
  box-shadow: var(--input-focus-shadow);
}

:deep(.ant-input-prefix) {
  margin-right: 10px;
  color: var(--text-hint);
}

.login-button {
  height: 48px;
  font-size: var(--font-size-lg);
  letter-spacing: 0.5px;
  font-weight: var(--font-weight-semibold);
  transition: var(--transition);
  border-radius: var(--radius-md);
  overflow: hidden;
  margin-top: var(--spacing-sm);
  background: var(--primary-color);
  border: none;
  box-shadow: var(--btn-primary-shadow);
}

.login-button:hover {
  transform: translateY(-1px);
  box-shadow: var(--btn-primary-hover-shadow);
  background: var(--primary-hover);
}

.login-tips {
  text-align: center;
  margin-top: var(--spacing-lg);
  margin-bottom: var(--spacing-lg);
  color: var(--text-hint);
  font-size: var(--font-size-sm);
  background: var(--alpha-black-03);
  padding: var(--spacing-sm);
  border-radius: var(--radius-sm);
}

.other-links {
  text-align: center;
  margin-top: var(--spacing-lg);
}

.dashboard-link {
  display: inline-flex;
  align-items: center;
  color: var(--primary-color);
  font-weight: var(--font-weight-medium);
  transition: var(--transition);
  font-size: var(--font-size-md);
}

.dashboard-link:hover {
  color: var(--primary-hover);
  text-decoration: none;
  opacity: 0.8;
}

.dashboard-link span {
  margin-right: var(--spacing-xs);
}

/* Decoration circles */
.decoration-circle {
  position: absolute;
  border-radius: var(--radius-circle);
  background: linear-gradient(135deg, var(--primary-color), var(--info-color));
  opacity: 0.15;
  filter: blur(60px);
  z-index: 0;
}

.decoration-1 {
  width: 300px;
  height: 300px;
  top: -100px;
  right: -100px;
}

.decoration-2 {
  width: 250px;
  height: 250px;
  bottom: -50px;
  left: -50px;
  background: linear-gradient(135deg, var(--success-color), var(--primary-color));
}

/* Responsive */
@media (max-width: 480px) {
  .login-card {
    width: 90%;
    max-width: 360px;
    padding: var(--spacing-2xl) var(--spacing-lg);
  }

  .login-title h2 {
    font-size: var(--font-size-3xl);
  }
}
</style>

<style>
/* Dark Mode Adaptation */
.dark .login-container {
  background-color: #0f0f12;
  background-image:
    radial-gradient(at 30% 20%, var(--bg-radial-primary) 0px, transparent 50%),
    radial-gradient(at 80% 40%, var(--bg-radial-success) 0px, transparent 50%),
    radial-gradient(at 10% 70%, rgba(255, 149, 0, 0.08) 0px, transparent 50%);
}

.dark .login-card {
  background: rgba(30, 30, 35, 0.65) !important;
  border: 1px solid var(--alpha-white-08);
  box-shadow: 0 20px 40px var(--alpha-black-40);
}

.dark .login-card::before {
  background: linear-gradient(90deg, transparent, var(--alpha-white-15), transparent);
}

.dark .login-title p {
  color: var(--alpha-white-60);
}

.dark .login-form-item .ant-form-item-label>label {
  color: var(--alpha-white-80) !important;
}

.dark .ant-input-affix-wrapper {
  background: var(--alpha-black-30) !important;
  border-color: var(--alpha-white-08) !important;
}

.dark .ant-input-affix-wrapper .ant-input {
  color: var(--alpha-white-90) !important;
}

.dark .ant-input-affix-wrapper .ant-input::placeholder {
  color: var(--alpha-white-30);
}

.dark .ant-input-prefix {
  color: var(--alpha-white-40) !important;
}

.dark .ant-input-affix-wrapper:hover,
.dark .ant-input-affix-wrapper:focus,
.dark .ant-input-affix-wrapper-focused {
  background: var(--alpha-black-50) !important;
  border-color: var(--primary-color) !important;
}

.dark .login-tips {
  background: var(--alpha-white-05);
  color: var(--alpha-white-50);
}

.dark .decoration-circle {
  opacity: 0.2;
}
</style>