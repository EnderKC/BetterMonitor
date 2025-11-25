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
        <h2 class="gradient-text">服务器运维系统</h2>
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
    radial-gradient(at 30% 20%, rgba(0, 122, 255, 0.15) 0px, transparent 50%),
    radial-gradient(at 80% 40%, rgba(52, 199, 89, 0.1) 0px, transparent 50%),
    radial-gradient(at 10% 70%, rgba(255, 149, 0, 0.08) 0px, transparent 50%);
  background-attachment: fixed;
  position: relative;
  overflow: hidden;
}

.login-card {
  width: 420px;
  padding: 48px 40px;
  position: relative;
  z-index: 1;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-radius: 24px;
  box-shadow: var(--shadow-lg);
  border: 1px solid rgba(255, 255, 255, 0.4);
}

.login-card::before {
  content: '';
  position: absolute;
  top: -1px;
  left: 10%;
  width: 80%;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.8), transparent);
  z-index: 2;
}

.login-title {
  text-align: center;
  margin-bottom: 36px;
  position: relative;
  z-index: 2;
}

.login-title h2 {
  margin-bottom: 12px;
  font-weight: 700;
  font-size: 28px;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, #007AFF, #5856D6);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.login-title p {
  color: var(--text-secondary);
  font-size: 16px;
  font-weight: 500;
}

.login-form {
  position: relative;
  z-index: 2;
}


:deep(.ant-form-item-label > label) {
  font-weight: 500;
  color: var(--text-secondary);
}

:deep(.ant-input-affix-wrapper) {
  border-radius: 12px;
  box-shadow: none;
  border: 1px solid rgba(0, 0, 0, 0.1);
  transition: var(--transition);
  background: rgba(255, 255, 255, 0.6);
  padding: 10px 12px;
  backdrop-filter: blur(10px);
}

:deep(.ant-input-affix-wrapper .ant-input) {
  background: transparent !important;
}

:deep(.ant-input-affix-wrapper:hover),
:deep(.ant-input-affix-wrapper:focus),
:deep(.ant-input-affix-wrapper-focused) {
  border-color: var(--primary-color);
  background: #fff;
  box-shadow: 0 0 0 4px rgba(0, 122, 255, 0.1);
}

:deep(.ant-input-prefix) {
  margin-right: 10px;
  color: var(--text-hint);
}

.login-button {
  height: 48px;
  font-size: 16px;
  letter-spacing: 0.5px;
  font-weight: 600;
  transition: var(--transition);
  border-radius: 12px;
  overflow: hidden;
  margin-top: 8px;
  background: var(--primary-color);
  border: none;
  box-shadow: 0 4px 12px rgba(0, 122, 255, 0.3);
}

.login-button:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(0, 122, 255, 0.4);
  background: var(--primary-hover);
}

.login-tips {
  text-align: center;
  margin-top: 24px;
  margin-bottom: 24px;
  color: var(--text-hint);
  font-size: 13px;
  background: rgba(0, 0, 0, 0.03);
  padding: 8px;
  border-radius: 8px;
}

.other-links {
  text-align: center;
  margin-top: 24px;
}

.dashboard-link {
  display: inline-flex;
  align-items: center;
  color: var(--primary-color);
  font-weight: 500;
  transition: var(--transition);
  font-size: 14px;
}

.dashboard-link:hover {
  color: var(--primary-hover);
  text-decoration: none;
  opacity: 0.8;
}

.dashboard-link span {
  margin-right: 4px;
}

/* 装饰元素 */
.decoration-circle {
  position: absolute;
  border-radius: 50%;
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

/* 响应式调整 */
@media (max-width: 480px) {
  .login-card {
    width: 90%;
    max-width: 360px;
    padding: 40px 24px;
  }

  .login-title h2 {
    font-size: 24px;
  }
}
</style>

<style>
.dark .login-form-item .ant-form-item-label > label {
  color: #4d4f54b8 !important;
}
</style>