import { computed, ref } from 'vue';
import { defineStore } from 'pinia';

import service from '../utils/request';
import {
  clearLoginInfo,
  getAdmin,
  getToken,
  normalizeAdminProfile,
  setAdmin,
  setToken,
  type AdminProfile,
} from '../utils/auth';

export const useAuthStore = defineStore('auth', () => {
  const adminInfo = ref<AdminProfile | null>(getAdmin());
  const token = ref(getToken() || '');
  const lastFetchTime = ref(0);
  const cacheDuration = 60_000;

  const isLoggedIn = computed(() => Boolean(token.value && adminInfo.value?.id));
  const username = computed(() => adminInfo.value?.username || '');

  const login = async (usernameValue: string, password: string) => {
    try {
      const response = await service.post('/login', { username: usernameValue, password });
      const admin = normalizeAdminProfile(response?.admin);
      if (!response?.token || !admin) return false;

      token.value = response.token;
      adminInfo.value = admin;
      setToken(response.token);
      setAdmin(admin);
      lastFetchTime.value = Date.now();
      return true;
    } catch {
      return false;
    }
  };

  const getAdminInfo = async (force = false) => {
    if (!token.value) return false;
    const now = Date.now();
    if (!force && lastFetchTime.value > 0 && now - lastFetchTime.value < cacheDuration) {
      return true;
    }

    try {
      const admin = normalizeAdminProfile(await service.get('/profile'));
      if (!admin) return false;
      adminInfo.value = admin;
      setAdmin(admin);
      lastFetchTime.value = now;
      return true;
    } catch (error: any) {
      if (error?.response?.status === 401 || error?.response?.status === 403) logout();
      return false;
    }
  };

  const logout = () => {
    token.value = '';
    adminInfo.value = null;
    lastFetchTime.value = 0;
    clearLoginInfo();
  };

  if (token.value) void getAdminInfo();

  return {
    adminInfo,
    token,
    isLoggedIn,
    username,
    login,
    getAdminInfo,
    logout,
  };
});
