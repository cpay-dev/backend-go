"use client";

import {
  createContext,
  useContext,
  useCallback,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useRouter } from "next/navigation";

interface User {
  id: string;
  wallet_address: string;
  role: string | null;
  created_at: string;
  updated_at: string;
}

interface AuthContext {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  signIn: (token: string, user: User) => void;
  signOut: () => void;
  setUser: (user: User) => void;
  updateAuth: (token: string, user: User) => void;
}

const AuthCtx = createContext<AuthContext | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUserState] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const router = useRouter();

  useEffect(() => {
    const storedToken = localStorage.getItem("token");
    const storedUser = localStorage.getItem("user");
    if (storedToken && storedUser) {
      try {
        setToken(storedToken);
        setUserState(JSON.parse(storedUser));
      } catch {
        localStorage.removeItem("token");
        localStorage.removeItem("user");
      }
    }
    setIsLoading(false);
  }, []);

  const signIn = useCallback(
    (newToken: string, newUser: User) => {
      localStorage.setItem("token", newToken);
      localStorage.setItem("user", JSON.stringify(newUser));
      setToken(newToken);
      setUserState(newUser);

      if (!newUser.role) {
        router.push("/onboarding");
      } else {
        router.push("/dashboard");
      }
    },
    [router]
  );

  const signOut = useCallback(() => {
    localStorage.removeItem("token");
    localStorage.removeItem("user");
    setToken(null);
    setUserState(null);
    router.push("/");
  }, [router]);

  const setUser = useCallback((u: User) => {
    localStorage.setItem("user", JSON.stringify(u));
    setUserState(u);
  }, []);

  const updateAuth = useCallback((newToken: string, u: User) => {
    localStorage.setItem("token", newToken);
    localStorage.setItem("user", JSON.stringify(u));
    setToken(newToken);
    setUserState(u);
  }, []);

  return (
    <AuthCtx.Provider
      value={{
        user,
        token,
        isAuthenticated: !!token,
        isLoading,
        signIn,
        signOut,
        setUser,
        updateAuth,
      }}
    >
      {children}
    </AuthCtx.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthCtx);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
