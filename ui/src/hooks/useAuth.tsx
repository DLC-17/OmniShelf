/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useMemo } from 'react'
import type { ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchMe } from '../api/auth'
import type { User } from '../api/auth'

export const AUTH_ME_KEY = ['auth', 'me'] as const

export interface AuthContextValue {
  /** Current user, or null when signed out. */
  user: User | null
  /** True while the initial /api/auth/me probe is in flight. */
  isLoading: boolean
  /** Re-fetch /api/auth/me (after login/register). */
  refresh: () => Promise<void>
  /** Drop auth state locally (after logout or a global 401). */
  clear: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const { data, isPending } = useQuery({
    queryKey: AUTH_ME_KEY,
    queryFn: fetchMe,
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: AUTH_ME_KEY })
  }, [queryClient])

  const clear = useCallback(() => {
    queryClient.setQueryData(AUTH_ME_KEY, null)
  }, [queryClient])

  const value = useMemo<AuthContextValue>(
    () => ({ user: data ?? null, isLoading: isPending, refresh, clear }),
    [data, isPending, refresh, clear],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (ctx === null) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return ctx
}
