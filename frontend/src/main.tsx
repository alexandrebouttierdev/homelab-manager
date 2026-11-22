import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router-dom'
import { router } from './app/router'
import { RefreshProvider } from './lib/refresh'
import { ThemeProvider } from './lib/theme'
import './styles.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 30_000 },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <RefreshProvider>
          <RouterProvider router={router} />
        </RefreshProvider>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
)
