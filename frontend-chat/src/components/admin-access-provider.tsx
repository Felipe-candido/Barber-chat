"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import { getMe, listMyShops, type AccessibleShop, type StaffIdentity } from "@/lib/api/identity";
import { getSupabaseBrowserClient } from "@/lib/supabase/browser";
import { signOut } from "@/lib/supabase/auth";

type VerifiedAccess = { identity: StaffIdentity; shops: AccessibleShop[] };
type AccessState =
  | { status: "idle" | "loading" }
  | { status: "error"; error: ApiError }
  | ({ status: "ready" } & VerifiedAccess);
type AdminAccess = {
  state: AccessState;
  refresh: () => Promise<VerifiedAccess>;
  logout: () => Promise<void>;
  syncShopName: (shopID: string, name: string, expectedUserID: string) => void;
};
const AccessContext = createContext<AdminAccess | null>(null);

export function AdminAccessProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AccessState>({ status: "idle" });
  const generation = useRef(0);
  const request = useRef<AbortController | null>(null);
  const sessionUser = useRef<string | null | undefined>(undefined);

  const invalidate = useCallback(() => {
    generation.current++;
    request.current?.abort();
    setState({ status: "idle" });
  }, []);

  // Cosmetic cache update from a verified settings response. Never adds shops,
  // changes slugs, identity or permissions, or treats browser state as authority.
  const syncShopName = useCallback((shopID: string, name: string, expectedUserID: string) => {
    setState((current) => {
      if (
        current.status !== "ready" ||
        current.identity.user_id !== expectedUserID ||
        (sessionUser.current !== undefined && sessionUser.current !== expectedUserID) ||
        !current.shops.some((shop) => shop.shop_id === shopID && shop.name !== name)
      )
        return current;
      return {
        ...current,
        shops: current.shops.map((shop) => (shop.shop_id === shopID ? { ...shop, name } : shop)),
      };
    });
  }, []);

  const refresh = useCallback(async (): Promise<VerifiedAccess> => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const attempt = ++generation.current;
    setState({ status: "loading" });
    try {
      // The Go API, not browser-stored claims, decides the local identity and memberships.
      const identity = await getMe(controller.signal);
      const shops = await listMyShops(controller.signal);
      if (controller.signal.aborted || attempt !== generation.current) {
        throw new DOMException("Superseded access request", "AbortError");
      }
      if (
        sessionUser.current &&
        sessionUser.current.toLowerCase() !== identity.user_id.toLowerCase()
      ) {
        throw new ApiError(
          "invalid_access_token",
          "A sessão mudou durante a validação. Entre novamente.",
          401,
        );
      }
      const access = { identity, shops };
      setState({ status: "ready", ...access });
      return access;
    } catch (error) {
      if (!controller.signal.aborted && attempt === generation.current) {
        setState({
          status: "error",
          error:
            error instanceof ApiError ? error : new ApiError("access_failed", errorMessage(error)),
        });
      }
      throw error;
    }
  }, []);

  const logout = useCallback(async () => {
    invalidate();
    setState({ status: "loading" });
    try {
      await signOut();
      invalidate();
    } catch (error) {
      setState({ status: "error", error: new ApiError("sign_out_failed", errorMessage(error)) });
      throw error;
    }
  }, [invalidate]);

  useEffect(() => {
    // Strict Mode may clean up and recreate this effect while keeping its state.
    // If cleanup canceled verification, let the guard start a fresh attempt.
    if (request.current?.signal.aborted) {
      request.current = null;
      setState((current) => (current.status === "loading" ? { status: "idle" } : current));
    }
    let unsubscribe: (() => void) | undefined;
    try {
      const { data } = getSupabaseBrowserClient().auth.onAuthStateChange((event, session) => {
        const nextUser = session?.user.id ?? null;
        const changed = sessionUser.current !== undefined && sessionUser.current !== nextUser;
        sessionUser.current = nextUser;
        // Keep this callback synchronous: nested Supabase auth calls can deadlock.
        if (event === "SIGNED_OUT" || changed) invalidate();
      });
      unsubscribe = () => data.subscription.unsubscribe();
    } catch {
      // Public pages still work without Supabase. Administrative calls surface configuration errors.
    }
    return () => {
      unsubscribe?.();
      generation.current++;
      request.current?.abort();
    };
  }, [invalidate]);

  const value = useMemo(
    () => ({ state, refresh, logout, syncShopName }),
    [state, refresh, logout, syncShopName],
  );
  return <AccessContext.Provider value={value}>{children}</AccessContext.Provider>;
}

export function useAdminAccess() {
  const access = useContext(AccessContext);
  if (!access) throw new Error("Admin access requires AdminAccessProvider.");
  return access;
}
