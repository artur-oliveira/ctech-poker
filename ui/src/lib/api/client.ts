import axios from 'axios';
import {doRefresh} from '@/lib/auth/oauth'

let token: string | null = null;
const listeners = new Set<(v: string | null) => void>()

export function setAccessToken(v: string | null) {
  token = v;
  listeners.forEach(f => f(v))
}

export function getAccessToken() {
  return token
}

export function subscribeAccessToken(f: (v: string | null) => void) {
  listeners.add(f);
  return () => {
    listeners.delete(f)
  }
}

export const apiClient = axios.create({baseURL: process.env.NEXT_PUBLIC_API_URL || ''})
apiClient.interceptors.request.use(c => {
  if (token) c.headers.Authorization = `Bearer ${token}`;
  return c
})
apiClient.interceptors.response.use(r => r, async e => {
  if (e.response?.status === 401 && !e.config._retried) {
    e.config._retried = true;
    const r = await doRefresh();
    if (r) {
      setAccessToken(r.accessToken);
      e.config.headers.Authorization = `Bearer ${r.accessToken}`;
      return apiClient.request(e.config)
    }
  }
  return Promise.reject(e)
})
