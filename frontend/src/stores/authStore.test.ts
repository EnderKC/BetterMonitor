import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

const requestMocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}));

vi.mock('../utils/request', () => ({
  default: requestMocks,
}));

import { getAdmin, getToken } from '../utils/auth';
import { useAuthStore } from './authStore';

describe('single administrator auth store', () => {
  beforeEach(() => {
    localStorage.clear();
    requestMocks.get.mockReset();
    requestMocks.post.mockReset();
    setActivePinia(createPinia());
  });

  it('persists a normalized administrator after login', async () => {
    requestMocks.post.mockResolvedValue({
      token: 'signed-token',
      admin: {
        id: 1,
        username: 'admin',
        email: 'admin@example.com',
        role: 'admin',
      },
    });

    const store = useAuthStore();

    await expect(store.login('admin', 'password')).resolves.toBe(true);
    expect(getToken()).toBe('signed-token');
    expect(getAdmin()).toEqual({
      id: 1,
      username: 'admin',
      email: 'admin@example.com',
      phone: '',
    });
    expect(store.isLoggedIn).toBe(true);
    expect(store.adminInfo).not.toHaveProperty('role');
  });

  it('refreshes the administrator profile without role state', async () => {
    localStorage.setItem('server_ops_token', 'signed-token');
    requestMocks.get.mockResolvedValue({
      id: 1,
      username: 'renamed-admin',
      phone: '10010',
      role: 'admin',
    });

    const store = useAuthStore();

    await expect(store.getAdminInfo(true)).resolves.toBe(true);
    expect(store.adminInfo).toMatchObject({
      id: 1,
      username: 'renamed-admin',
      phone: '10010',
    });
    expect(store.adminInfo).not.toHaveProperty('role');
  });
});
