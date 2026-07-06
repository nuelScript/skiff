import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryProvider } from '@/providers/query-provider'
import { ConfirmProvider } from '@/providers/confirm-provider'
import App from '@/components/app'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryProvider>
      <ConfirmProvider>
        <App />
      </ConfirmProvider>
    </QueryProvider>
  </StrictMode>,
)
