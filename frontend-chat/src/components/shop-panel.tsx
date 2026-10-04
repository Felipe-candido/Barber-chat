"use client";

import { createContext, useContext } from "react";
import Link from "next/link";
import { useAdminAccess } from "./admin-access-provider";
import { AdminShell } from "./admin-shell";
import type { AccessibleShop } from "@/lib/api/identity";

const SelectedShopContext = createContext<AccessibleShop | null>(null);

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
    <SelectedShopContext.Provider value={shop}>
      <AdminShell shop={shop} identity={state.identity}>
        {children}
      </AdminShell>
    </SelectedShopContext.Provider>
  );
}

export function useSelectedShop() {
  const shop = useContext(SelectedShopContext);
  if (!shop) throw new Error("A shop page requires a validated ShopPanel.");
  return shop;
}
