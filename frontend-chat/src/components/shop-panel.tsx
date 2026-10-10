"use client";

import { createContext, useContext, useEffect, useState } from "react";
import Link from "next/link";
import { useAdminAccess } from "./admin-access-provider";
import { AdminShell } from "./admin-shell";
import {
  getShopAccess,
  type AccessibleShop,
  type ShopAccess,
  type StaffIdentity,
} from "@/lib/api/identity";
import { errorMessage } from "@/lib/api/client";

const SelectedShopContext = createContext<AccessibleShop | null>(null);
const ShopAccessContext = createContext<ShopAccess | null>(null);

export function ShopPanel({ slug, children }: { slug: string; children: React.ReactNode }) {
  const { state } = useAdminAccess();
  if (state.status !== "ready") return null;
  const shop = state.shops.find((item) => item.slug === slug);
  if (!shop) {
    return (
      <main className="access-screen">
        <section className="api-error" role="alert">
          <h1>Barbearia não disponível para sua conta</h1>
          <p>
            Escolha uma unidade vinculada ao seu usuário. Um slug digitado na URL não libera acesso.
          </p>
          <Link href="/admin" className="button button-primary">
            Escolher barbearia
          </Link>
        </section>
      </main>
    );
  }
  return (
    <AuthorizedPanel
      key={shop.shop_id + ":" + state.identity.user_id}
      shop={shop}
      identity={state.identity}
    >
      {children}
    </AuthorizedPanel>
  );
}

function AuthorizedPanel({
  shop,
  identity,
  children,
}: {
  shop: AccessibleShop;
  identity: StaffIdentity;
  children: React.ReactNode;
}) {
  const [access, setAccess] = useState<ShopAccess | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setAccess(null);
    setError("");
    void getShopAccess(shop.slug, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) setAccess(value);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [shop.slug, revision]);
  if (!access)
    return (
      <main className="access-screen">
        <section className="api-error">
          {error ? (
            <>
              <h1>Não foi possível verificar seu acesso</h1>
              <p role="alert">{error}</p>
              <button className="button button-secondary" onClick={() => setRevision((v) => v + 1)}>
                Verificar novamente
              </button>
              <Link href="/admin">Escolher barbearia</Link>
            </>
          ) : (
            <p role="status">Verificando seu perfil na barbearia…</p>
          )}
        </section>
      </main>
    );
  return (
    <SelectedShopContext.Provider value={shop}>
      <ShopAccessContext.Provider value={access}>
        <AdminShell shop={shop} identity={identity} access={access}>
          {children}
        </AdminShell>
      </ShopAccessContext.Provider>
    </SelectedShopContext.Provider>
  );
}

export function useSelectedShopAccess() {
  const access = useContext(ShopAccessContext);
  if (!access) throw new Error("A shop page requires verified shop access.");
  return access;
}

export function useSelectedShop() {
  const shop = useContext(SelectedShopContext);
  if (!shop) throw new Error("A shop page requires a validated ShopPanel.");
  return shop;
}
