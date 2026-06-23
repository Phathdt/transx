import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import { getRouter } from './router'
import { QueryProvider } from './integrations/tanstack-query/root-provider'
import './styles.css'

const router = getRouter()

const rootElement = document.getElementById('app')
if (!rootElement) {
  throw new Error('Root element #app not found')
}

createRoot(rootElement).render(
  <StrictMode>
    <QueryProvider>
      <RouterProvider router={router} />
    </QueryProvider>
  </StrictMode>,
)
