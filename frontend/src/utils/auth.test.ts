import { beforeEach, describe, expect, it } from 'vitest';

import {
  clearLoginInfo,
  getAdmin,
  getToken,
  setAdmin,
  setToken,
} from './auth';

describe('single administrator auth storage', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('stores only supported administrator profile fields', () => {
    setAdmin({
      id: 1,
      username: 'admin',
      email: 'admin@example.com',
      role: 'admin',
      arbitrary: 'discard-me',
    } as never);

    expect(getAdmin()).toEqual({
      id: 1,
      username: 'admin',
      email: 'admin@example.com',
      phone: '',
    });
    expect(localStorage.getItem('server_ops_admin')).not.toContain('role');
    expect(localStorage.getItem('server_ops_admin')).not.toContain('arbitrary');
  });

  it('clears the administrator, token, and legacy user key', () => {
    localStorage.setItem('server_ops_user', JSON.stringify({ id: 2, role: 'user' }));
    setAdmin({ id: 1, username: 'admin' });
    setToken('token');

    clearLoginInfo();

    expect(getAdmin()).toBeNull();
    expect(getToken()).toBeNull();
    expect(localStorage.getItem('server_ops_user')).toBeNull();
  });
});
