const TokenKey = 'server_ops_token';
const AdminKey = 'server_ops_admin';
const LegacyUserKey = 'server_ops_user';

export interface AdminProfile {
  id: number;
  username: string;
  email?: string;
  phone?: string;
  last_login_at?: string;
  last_login?: string;
}

export function getToken(): string | null {
  return localStorage.getItem(TokenKey);
}

export function setToken(token: string): void {
  localStorage.setItem(TokenKey, token);
}

export function removeToken(): void {
  localStorage.removeItem(TokenKey);
}

export function normalizeAdminProfile(value: unknown): AdminProfile | null {
  if (!value || typeof value !== 'object') return null;
  const raw = value as Record<string, unknown>;
  if (typeof raw.id !== 'number' || raw.id <= 0 || typeof raw.username !== 'string' || !raw.username) {
    return null;
  }

  const admin: AdminProfile = {
    id: raw.id,
    username: raw.username,
    email: typeof raw.email === 'string' ? raw.email : '',
    phone: typeof raw.phone === 'string' ? raw.phone : '',
  };
  if (typeof raw.last_login_at === 'string') admin.last_login_at = raw.last_login_at;
  if (typeof raw.last_login === 'string') admin.last_login = raw.last_login;
  return admin;
}

export function getAdmin(): AdminProfile | null {
  const stored = localStorage.getItem(AdminKey);
  if (!stored) return null;
  try {
    return normalizeAdminProfile(JSON.parse(stored));
  } catch {
    localStorage.removeItem(AdminKey);
    return null;
  }
}

export function setAdmin(value: AdminProfile): void {
  const admin = normalizeAdminProfile(value);
  if (!admin) throw new Error('invalid administrator profile');
  localStorage.setItem(AdminKey, JSON.stringify(admin));
}

export function removeAdmin(): void {
  localStorage.removeItem(AdminKey);
}

export function clearLoginInfo(): void {
  removeToken();
  removeAdmin();
  localStorage.removeItem(LegacyUserKey);
}

export function isTokenExpired(token: string): boolean {
  try {
    const base64Url = token.split('.')[1];
    if (!base64Url) return true;
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const jsonPayload = decodeURIComponent(atob(base64).split('').map((character) => {
      return `%${(`00${character.charCodeAt(0).toString(16)}`).slice(-2)}`;
    }).join(''));
    const payload = JSON.parse(jsonPayload);
    return payload.exp ? Date.now() >= payload.exp * 1000 : false;
  } catch {
    return true;
  }
}
