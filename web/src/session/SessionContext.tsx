import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useApi } from '@/api/context';
import type { Role, Scope, User } from '@/api/types';

interface Session {
  user: User | null;
  roles: Role[];
  signIn(email: string): Promise<void>;
  signOut(): void;
  can(permissionId: string, scope?: Scope): boolean;
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const api = useApi();
  const [user, setUser] = useState<User | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);

  useEffect(() => {
    void api.listRoles().then(setRoles);
  }, [api]);

  const signIn = useCallback(
    async (email: string) => {
      const users = await api.listUsers();
      const found = users.find((u) => u.email.toLowerCase() === email.toLowerCase());
      if (!found) throw new Error(`${email} has not been invited. Ask an admin to invite it.`);
      setUser(found);
    },
    [api],
  );

  const signOut = useCallback(() => setUser(null), []);

  const can = useCallback(
    (permissionId: string, scope?: Scope) => {
      if (!user) return false;
      return user.grants.some((g) => {
        const role = roles.find((r) => r.id === g.roleId);
        if (!role?.permissionIds.includes(permissionId)) return false;
        if (g.scope.kind === 'organisation') return true;
        if (!scope) return false;
        return scope.kind === 'group' && scope.groupId === g.scope.groupId;
      });
    },
    [user, roles],
  );

  const value = useMemo(
    () => ({ user, roles, signIn, signOut, can }),
    [user, roles, signIn, signOut, can],
  );
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error('useSession needs SessionProvider');
  return ctx;
}

export function RequireSession({ children }: { children: ReactNode }) {
  const { user } = useSession();
  const location = useLocation();
  if (!user) return <Navigate to="/sign-in" replace state={{ from: location.pathname }} />;
  return <>{children}</>;
}
