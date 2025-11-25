const TokenKey = 'server_ops_token';
const UserKey = 'server_ops_user';

// 获取令牌
export function getToken(): string | null {
  const token = localStorage.getItem(TokenKey);
  console.log('获取令牌:', token);
  return token;
}

// 设置令牌
export function setToken(token: string): void {
  console.log('设置令牌:', token);
  localStorage.setItem(TokenKey, token);
}

// 删除令牌
export function removeToken(): void {
  console.log('删除令牌');
  localStorage.removeItem(TokenKey);
}

// 获取用户信息
export function getUser(): any {
  const userStr = localStorage.getItem(UserKey);
  if (userStr) {
    try {
      const user = JSON.parse(userStr);
      console.log('获取用户信息:', user);
      return user;
    } catch (e) {
      console.error('解析用户信息失败:', e);
      return null;
    }
  }
  return null;
}

// 设置用户信息
export function setUser(user: any): void {
  console.log('设置用户信息:', user);
  localStorage.setItem(UserKey, JSON.stringify(user));
}

// 删除用户信息
export function removeUser(): void {
  console.log('删除用户信息');
  localStorage.removeItem(UserKey);
}

// 检查是否是管理员
export function isAdmin(): boolean {
  const user = getUser();
  return user && user.role === 'admin';
}

// 清除所有登录信息
export function clearLoginInfo(): void {
  console.log('清除所有登录信息');
  removeToken();
  removeUser();
} 