const TokenKey = 'server_ops_token';
const UserKey = 'server_ops_user';

// 获取令牌
export function getToken(): string | null {
  return localStorage.getItem(TokenKey);
}

// 设置令牌
export function setToken(token: string): void {
  localStorage.setItem(TokenKey, token);
}

// 删除令牌
export function removeToken(): void {
  localStorage.removeItem(TokenKey);
}

// 获取用户信息
export function getUser(): any {
  const userStr = localStorage.getItem(UserKey);
  if (userStr) {
    try {
      return JSON.parse(userStr);
    } catch (e) {
      console.error('解析用户信息失败:', e);
      return null;
    }
  }
  return null;
}

// 设置用户信息
export function setUser(user: any): void {
  localStorage.setItem(UserKey, JSON.stringify(user));
}

// 删除用户信息
export function removeUser(): void {
  localStorage.removeItem(UserKey);
}

// 检查是否是管理员
export function isAdmin(): boolean {
  const user = getUser();
  return user && user.role === 'admin';
}

// 清除所有登录信息
export function clearLoginInfo(): void {
  removeToken();
  removeUser();
}

// 检查Token是否过期
export function isTokenExpired(token: string): boolean {
  try {
    const base64Url = token.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const jsonPayload = decodeURIComponent(atob(base64).split('').map(function (c) {
      return '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2);
    }).join(''));

    const payload = JSON.parse(jsonPayload);
    if (payload.exp) {
      // exp is in seconds, Date.now() is in ms
      return Date.now() >= payload.exp * 1000;
    }
    return false;
  } catch (e) {
    console.error('Token解析失败:', e);
    return true; // 解析失败视为过期/无效
  }
} 
