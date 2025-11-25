import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { getToken, getUser, removeToken, setToken, setUser } from '../utils/auth';
import service from '../utils/request';

export const useUserStore = defineStore('user', () => {
  // 用户信息
  const userInfo = ref<any>(getUser() || {});
  const token = ref<string>(getToken() || '');
  const lastFetchTime = ref(0);
  const cacheDuration = 60000; // 缓存1分钟

  // 计算属性：是否为管理员
  const isAdmin = computed(() => userInfo.value?.role === 'admin');

  // 计算属性：是否已登录
  const isLoggedIn = computed(() => {
    return !!token.value && !!userInfo.value?.id;
  });

  // 用户名
  const username = computed(() => userInfo.value?.username || '');

  // 登录
  const login = async (username: string, password: string) => {
    try {
      console.log('尝试登录:', username);
      const response = await service.post('/login', { username, password });
      console.log('登录响应:', response);

      // 检查是否有token和用户数据
      if (response && response.token) {
        token.value = response.token;
        setToken(response.token);

        // 如果响应中包含用户信息，直接使用
        if (response.user) {
          userInfo.value = response.user;
          setUser(response.user);
          lastFetchTime.value = Date.now();
          console.log('从登录响应中获取用户信息:', userInfo.value);
          return true;
        } else {
          // 否则，获取用户信息
          console.log('登录成功，尝试获取用户信息');
          return await getUserInfo(true);
        }
      }
      return false;
    } catch (error) {
      console.error('登录失败:', error);
      return false;
    }
  };

  // 获取用户信息
  const getUserInfo = async (force = false) => {
    try {
      if (!token.value) {
        console.log('没有token，无法获取用户信息');
        return false;
      }

      const now = Date.now();
      // 如果不是强制刷新，且缓存未过期，则直接返回true
      if (!force && lastFetchTime.value > 0 && (now - lastFetchTime.value < cacheDuration)) {
        console.log('使用缓存的用户信息');
        return true;
      }

      console.log('尝试获取用户信息，使用token:', token.value ? '已设置' : '未设置');
      const response = await service.get('/profile');
      console.log('获取用户信息响应:', response);

      if (response) {
        // 接口可能直接返回用户数据，而不是包装在user字段中
        userInfo.value = response;
        setUser(response); // 保存到本地存储
        lastFetchTime.value = Date.now();
        console.log('用户信息已更新:', userInfo.value);
        return true;
      }
      return false;
    } catch (error) {
      console.error('获取用户信息失败:', error);
      // 如果获取用户信息失败，可能是token过期
      if (error.response && (error.response.status === 401 || error.response.status === 403)) {
        logout(); // 清除登录状态
      }
      return false;
    }
  };

  // 退出登录
  const logout = () => {
    token.value = '';
    userInfo.value = {};
    lastFetchTime.value = 0;
    removeToken();
  };

  // 尝试初始化时自动检查登录状态
  if (token.value) {
    getUserInfo().catch(() => {
      console.warn('初始化用户信息失败，可能需要重新登录');
    });
  }

  return {
    userInfo,
    token,
    isAdmin,
    isLoggedIn,
    username,
    login,
    getUserInfo,
    logout
  };
});