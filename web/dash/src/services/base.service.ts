import type { AxiosRequestConfig } from 'axios'
import { http } from '@/config/axios.config'

// Every service extends this. It centralizes the HTTP verbs (and unwraps the
// axios response) so each service only declares its endpoints — never axios
// plumbing. All requests still route through the one configured instance.
export abstract class BaseService {
  protected get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return http.get<T>(url, config).then((r) => r.data)
  }

  protected post<T = void>(url: string, body?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return http.post<T>(url, body, config).then((r) => r.data)
  }

  protected put<T = void>(url: string, body?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return http.put<T>(url, body, config).then((r) => r.data)
  }

  protected delete<T = void>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return http.delete<T>(url, config).then((r) => r.data)
  }
}
