import axios from 'axios';
import { message } from 'ant-design-vue';
import { getToken, clearLoginInfo, isTokenExpired } from './auth';
import router from '../router';

// 创建axios实例
const service = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api', // API基础路径
  timeout: 60000, // 请求超时时间
});

// 请求拦截器
service.interceptors.request.use(
  (config) => {
    // 添加token到请求头
    const token = getToken();
    if (token) {
      // 检查token是否过期或格式错误
      if (isTokenExpired(token)) {
        clearLoginInfo();
        router.push('/login');
        return Promise.reject(new Error('Token expired or invalid'));
      }
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    console.error('请求错误:', error);
    return Promise.reject(error);
  }
);

// 响应拦截器
service.interceptors.response.use(
  (response) => {
    console.log('请求成功响应:', response.config.url, response.data);

    // 直接返回响应数据，不做任何转换
    return response.data;
  },
  (error) => {
    console.error('请求错误:', error.config?.url, error);

    // 处理错误响应
    const response = error.response;

    if (response) {
      const { status, data } = response;
      console.error('错误响应状态:', status, '响应数据:', data);

      // 处理常见的错误
      switch (status) {
        case 400:
          message.error(data.error || '请求参数错误');
          break;
        case 401:
          message.error(data.error || '未授权，请重新登录');
          clearLoginInfo(); // 清除登录信息
          router.push('/login'); // 跳转到登录页
          break;
        case 403:
          message.error(data.error || '没有权限执行此操作');
          break;
        case 404:
          message.error(data.error || '请求的资源不存在');
          break;
        case 500:
          message.error(data.error || '服务器错误');
          break;
        default:
          message.error(data.error || `请求失败: ${status}`);
      }
    } else {
      message.error('网络错误，请检查网络连接');
    }

    return Promise.reject(error);
  }
);

// 声明模块扩展以修改axios返回类型
declare module 'axios' {
  export interface AxiosInstance {
    get<T = any>(url: string, config?: any): Promise<T>;
    post<T = any>(url: string, data?: any, config?: any): Promise<T>;
    put<T = any>(url: string, data?: any, config?: any): Promise<T>;
    delete<T = any>(url: string, config?: any): Promise<T>;
  }
}

// 导出请求方法
export default service; 