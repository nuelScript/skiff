import { createContext, useContext, type ReactNode } from 'react'
import { useAuth } from '@/hooks/use-auth'

// Distributes the auth hook's state across the routed shell without prop-drilling
// through <Outlet>. The logic still lives in use-auth; this only shares it.
type AuthValue = ReturnType<typeof useAuth>

const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const auth = useAuth()
  return <AuthContext.Provider value={auth}>{children}</AuthContext.Provider>
}

export function useAuthContext(): AuthValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuthContext must be used within AuthProvider')
  return ctx
}
