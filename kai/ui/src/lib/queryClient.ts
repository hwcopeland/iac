import { QueryClient } from '@tanstack/react-query'

/**
 * Singleton QueryClient shared between the router context and React tree.
 * Retry logic: skip retry on 401 (handled by route guard / sessionExpired banner).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        // Don't retry auth errors — they need user action
        if (
          error instanceof Error &&
          'status' in error &&
          (error as { status: number }).status === 401
        ) {
          return false
        }
        return failureCount < 3
      },
      staleTime: 30_000,
    },
  },
})
