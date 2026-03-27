import { useUIStore } from '../stores/uiStore'

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError
}

export function isUnauthorized(error: unknown): boolean {
  return isApiError(error) && error.status === 401
}

/**
 * Typed fetch wrapper for all API calls.
 * Includes credentials (HTTP-only session cookie), sets Content-Type,
 * and triggers the sessionExpired flag on 401.
 *
 * @param skipSessionExpired - Pass true for the auth-check endpoint (/me)
 *   to avoid marking the session as expired when the user simply isn't
 *   logged in yet.
 */
export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
  skipSessionExpired = false,
): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })

  if (res.status === 401) {
    if (!skipSessionExpired) {
      useUIStore.getState().setSessionExpired(true)
    }
    throw new ApiError(401, 'Unauthorized')
  }

  if (!res.ok) {
    const body = await res.text()
    throw new ApiError(res.status, body || res.statusText)
  }

  return res.json() as Promise<T>
}
