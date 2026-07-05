import axios from 'axios'

// The single axios instance every HTTP request in the app routes through.
// Same-origin in production; the Vite dev server proxies /api to the panel.
export const http = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
})

http.interceptors.response.use(
  (res) => res,
  (err) => Promise.reject(err),
)
