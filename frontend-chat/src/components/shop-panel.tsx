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
import { AccessPage } from "./ui/access-page";
import { Button, buttonClasses } from "./ui";
import styles from "./ui/access-page.module.css";

const SelectedShopContext = createContext<AccessibleShop | null>(null);
const ShopAccessContext = createContext<ShopAccess | null>(null);

export function ShopPanel({ slug, children }: { slug: string; children: React.ReactNode }) {
  const { state } = useAdminAccess();
  if (state.status !== "ready") return null;
  const shop = state.shops.find((item) => item.slug === slug);
  if (!shop) {
    return (
      <AccessPage title="Barbearia não disponível para sua conta" error>
        <p className={styles.text}>
          Escolha uma unidade vinculada ao seu usuário. O endereço informado não libera acesso a
          outra barbearia.
        </p>
        <Link href="/admin" className={buttonClasses()}>
          Escolher barbearia
        </Link>
      </AccessPage>
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
      <AccessPage
        title={error ? "Não foi possível verificar seu acesso" : "Verificando sua barbearia"}
        error={!!error}
      >
        {error ? (
          <>
            <p className={styles.text}>{error}</p>
            <Button variant="primary" onClick={() => setRevision((v) => v + 1)}>
              Verificar novamente
            </Button>
            <Link href="/admin" className={buttonClasses("secondary")}>
              Escolher barbearia
            </Link>
          </>
        ) : (
          <p role="status" className={styles.loading}>
            Verificando seu perfil na barbearia…
          </p>
        )}
      </AccessPage>
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
