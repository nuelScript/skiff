import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryProvider } from '@/lib/query-provider'
import App from '@/components/app'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryProvider>
      <App />
    </QueryProvider>
  </StrictMode>,
)
