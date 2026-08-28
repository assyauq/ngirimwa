import axios from 'axios';
import { inboxDebugLog, isInboxDebugActive } from './inboxDebug';

const debugRequestStartedAt = new WeakMap<object, number>();

function shouldDebugInboxRequest(url?: string) {
  return Boolean(url && /\/(contacts|conversation|typing|send(?:-media)?|history-sync)/.test(url));
}

const api = axios.create({
  baseURL: '/api',
  timeout: 25_000,
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  if (isInboxDebugActive() && shouldDebugInboxRequest(config.url)) {
    debugRequestStartedAt.set(config, window.performance.now());
    inboxDebugLog('http.request.start', {
      method: (config.method || 'get').toUpperCase(),
      url: config.url,
    });
  }
  return config;
});

api.interceptors.response.use(
  (res) => {
    const requestStartedAt = debugRequestStartedAt.get(res.config);
    if (requestStartedAt !== undefined && shouldDebugInboxRequest(res.config.url)) {
      inboxDebugLog('http.request.success', {
        method: (res.config.method || 'get').toUpperCase(),
        url: res.config.url,
        status: res.status,
        duration_ms: Math.round(window.performance.now() - requestStartedAt),
      });
    }
    return res;
  },
  (err) => {
    const requestUrl = err.config?.url || '';
    const requestStartedAt = err.config ? debugRequestStartedAt.get(err.config) : undefined;
    if (requestStartedAt !== undefined && shouldDebugInboxRequest(requestUrl)) {
      inboxDebugLog('http.request.error', {
        method: (err.config?.method || 'get').toUpperCase(),
        url: requestUrl,
        status: err.response?.status || 0,
        code: err.code || '',
        duration_ms: Math.round(window.performance.now() - requestStartedAt),
      });
    }
    const isLoginRequest = requestUrl === '/login' || requestUrl.endsWith('/login');
    if (err.response?.status === 401 && !isLoginRequest) {
      localStorage.removeItem('token');
      window.location.href = '/login';
    }
    return Promise.reject(err);
  }
);

export default api;
