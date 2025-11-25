import { createRouter, createWebHistory } from 'vue-router';
import type { RouteRecordRaw } from 'vue-router';
import { getToken } from '../utils/auth';

// 路由配置
const routes: Array<RouteRecordRaw> = [
  {
    path: '/',
    name: 'Root',
    redirect: '/dashboard',
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/auth/Login.vue'),
    meta: {
      title: '登录',
      requiresAuth: false,
    },
  },
  {
    path: '/dashboard',
    name: 'Dashboard',
    component: () => import('../views/dashboard/index.vue'),
    meta: {
      title: '探针',
      requiresAuth: false,
    },
  },
  {
    path: '/admin',
    name: 'Admin',
    component: () => import('../layout/AdminLayout.vue'),
    meta: {
      title: '控制台',
      requiresAuth: true,
    },
    children: [
      {
        path: '',
        name: 'AdminHome',
        redirect: '/admin/servers',
      },
      {
        path: 'servers',
        name: 'ServerList',
        component: () => import('../views/server/ServerList.vue'),
        meta: {
          title: '服务器管理',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id',
        name: 'ServerDetail',
        component: () => import('../views/server/ServerDetail.vue'),
        meta: {
          title: '服务器详情',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/monitor',
        name: 'ServerMonitor',
        component: () => import('../views/server/ServerMonitor.vue'),
        meta: {
          title: '服务器监控',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/terminal',
        name: 'ServerTerminal',
        component: () => import('../views/server/ServerTerminal.vue'),
        meta: {
          title: '终端',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/file',
        name: 'ServerFile',
        component: () => import('../views/server/ServerFile.vue'),
        meta: {
          title: '文件管理',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/process',
        name: 'ServerProcess',
        component: () => import('../views/server/ServerProcess.vue'),
        meta: {
          title: '进程管理',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/docker',
        name: 'ServerDocker',
        component: () => import('../views/server/ServerDocker.vue'),
        meta: {
          title: 'Docker管理',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/docker/:containerId/terminal',
        name: 'ServerDockerTerminal',
        component: () => import('../views/server/ServerDockerTerminal.vue'),
        meta: {
          title: '容器终端',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/docker/:containerId/file',
        name: 'ServerDockerFile',
        component: () => import('../views/server/ServerDockerFile.vue'),
        meta: {
          title: '容器文件',
          requiresAuth: true,
        },
      },
      {
        path: 'servers/:id/nginx',
        name: 'ServerNginx',
        component: () => import('../views/server/ServerNginx.vue'),
        meta: {
          title: '网站管理',
          requiresAuth: true,
        },
      },
      {
        path: 'profile',
        name: 'Profile',
        component: () => import('../views/user/Profile.vue'),
        meta: {
          title: '个人资料',
          requiresAuth: true,
        },
      },
      {
        path: 'users',
        name: 'UserManagement',
        component: () => import('../views/user/UserManagement.vue'),
        meta: {
          title: '用户管理',
          requiresAuth: true,
          admin: true,
        },
      },
      {
        path: 'settings',
        name: 'SystemSettings',
        component: () => import('../views/server/Settings.vue'),
        meta: {
          title: '系统设置',
          requiresAuth: true,
          admin: true,
        },
      },
      {
        path: 'alerts/settings',
        name: 'AlertSettings',
        component: () => import('../views/server/AlertSettings.vue'),
        meta: {
          title: '预警设置',
          requiresAuth: true,
        },
      },
      {
        path: 'alerts/channels',
        name: 'NotificationChannels',
        component: () => import('../views/server/NotificationChannels.vue'),
        meta: {
          title: '通知渠道',
          requiresAuth: true,
        },
      },
      {
        path: 'alerts/records',
        name: 'AlertRecords',
        component: () => import('../views/server/AlertRecords.vue'),
        meta: {
          title: '预警记录',
          requiresAuth: true,
        },
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('../views/error/NotFound.vue'),
    meta: {
      title: '404',
      requiresAuth: false,
    },
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

// 导航守卫
router.beforeEach((to, from, next) => {
  // 设置页面标题
  document.title = to.meta.title ? `${to.meta.title} - 服务器运维系统` : '服务器运维系统';

  console.log('路由守卫：', to.path, '需要认证:', to.matched.some(record => record.meta.requiresAuth));
  
  // 检查是否需要登录权限
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth);
  const token = getToken();
  
  console.log('当前token状态:', token ? '已登录' : '未登录');

  if (requiresAuth && !token) {
    // 需要登录但没有token，重定向到登录页
    console.log('未登录，重定向到登录页');
    next({ path: '/login', query: { redirect: to.fullPath } });
  } else {
    console.log('允许访问:', to.path);
    next();
  }
});

export default router; 
