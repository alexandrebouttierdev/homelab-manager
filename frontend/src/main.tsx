import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router-dom'
import { router } from './app/router'
import { ApiError, isUnauthenticated } from './lib/api/client'
import { RefreshProvider } from './lib/refresh'
import { ThemeProvider } from './lib/theme'
import '@fontsource-variable/instrument-sans'
import '@fontsource/ibm-plex-mono/latin-400.css'
import '@fontsource/ibm-plex-mono/latin-500.css'
import './styles.css'

// A revoked or expired session (password change, 12 h expiry, logout in
// another tab) refreshes the session query so RequireAuth redirects to login.
// The session query itself is excluded: its 401 already triggers the redirect.
function onApiError(error: unknown, queryKey?: readonly unknown[]) {
  if (isUnauthenticated(error) && queryKey?.[0] !== 'session') {
    void queryClient.invalidateQueries({ queryKey: ['session'] })
  }
}

const queryClient: QueryClient = new QueryClient({
  queryCache: new QueryCache({ onError: (error, query) => onApiError(error, query.queryKey) }),
  mutationCache: new MutationCache({ onError: (error) => onApiError(error) }),
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      // Client errors (4xx) are final; only network/5xx failures are retried once.
      retry: (failureCount, error) =>
        failureCount < 1 &&
        !(error instanceof ApiError && error.status >= 400 && error.status < 500),
    },
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
