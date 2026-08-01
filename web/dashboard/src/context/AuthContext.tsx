import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  changePasswordRequest,
  loginRequest,
  logoutRequest,
  meRequest,
  type AuthUser,
} from "@/lib/api";
import { clearStoredToken, getStoredToken, setStoredToken } from "@/lib/storage";

interface AuthContextValue {
  user: AuthUser | null;
  token: string | null;
  isAuthenticated: boolean;
  mustChangePassword: boolean;
  isLoading: boolean;
  login: (username: string, password: string) => Promise<AuthUser>;
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshSession: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [token, setToken] = useState<string | null>(() => getStoredToken());
  const [isLoading, setIsLoading] = useState(true);

  const refreshSession = useCallback(async () => {
    const storedToken = getStoredToken();
    setToken(storedToken);

    try {
      // Session cookie is sent via credentials: "include"; Bearer header is optional fallback.
      const session = await meRequest(storedToken);
      if (session?.user) {
        setUser(session.user);
        return;
      }
    } catch {
      // Fall through to unauthenticated state.
    }

    setUser(null);
    clearStoredToken();
    setToken(null);
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        await refreshSession();
      } finally {
        setIsLoading(false);
      }
    })();
  }, [refreshSession]);

  const login = useCallback(async (username: string, password: string) => {
    const response = await loginRequest(username, password);

    if (response.token) {
      setStoredToken(response.token);
      setToken(response.token);
    }

    setUser(response.user);
    return response.user;
  }, []);

  const changePassword = useCallback(
    async (currentPassword: string, newPassword: string) => {
      const activeToken = token ?? getStoredToken();
      const response = await changePasswordRequest(currentPassword, newPassword, activeToken);
      setUser(response.user);
    },
    [token],
  );

  const logout = useCallback(async () => {
    const activeToken = token ?? getStoredToken();
    try {
      await logoutRequest(activeToken);
    } finally {
      clearStoredToken();
      setToken(null);
      setUser(null);
    }
  }, [token]);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      token,
      isAuthenticated: Boolean(user),
      mustChangePassword: Boolean(user?.must_change_password),
      isLoading,
      login,
      changePassword,
      logout,
      refreshSession,
    }),
    [user, token, isLoading, login, changePassword, logout, refreshSession],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return context;
}
