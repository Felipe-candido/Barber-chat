"use client";
import Link from "next/link";
import { useMemo, useState } from "react";
import { useAdminAccess } from "../admin-access-provider";
import { useSelectedShop, useSelectedShopAccess } from "../shop-panel";
import { Alert, Button, buttonClasses } from "../ui";
import type { ShopRequest } from "@/lib/api/products";
import styles from "./commerce.module.css";

export function useCommerceScope(): ShopRequest {
  const shop = useSelectedShop(),
    { state } = useAdminAccess();
  const userID = state.status === "ready" ? state.identity.user_id : "";
  return useMemo(
    () => ({ slug: shop.slug, shopID: shop.shop_id, userID }),
    [shop.slug, shop.shop_id, userID],
  );
}
export function CommandFeedback({
  command,
  onRetry,
}: {
  command: {
    ready: boolean;
    busy: boolean;
    error: string;
    errorCode: string;
    attempt: unknown;
    corrupt: boolean;
    discard: () => void;
  };
  onRetry: () => void;
}) {
  const [confirm, setConfirm] = useState(false),
    shop = useSelectedShop(),
    owner = useSelectedShopAccess().role === "owner";
  return (
    <>
      {!command.ready && <p role="status">Verificando tentativas desta aba…</p>}
      {command.error && <Alert tone="error">{command.error}</Alert>}
      {command.errorCode === "shop_subscription_required" && (
        <Alert tone="warning">
          {owner ? (
            <>
              <p>
                Regularize a assinatura em Configurações. Ao voltar, revise os dados e confirme a
                venda.
              </p>
              <Link
                className={buttonClasses("secondary")}
                href={`/admin/${encodeURIComponent(shop.slug)}/configuracoes`}
                target="_blank"
                rel="noopener noreferrer"
              >
                Gerenciar assinatura
              </Link>
            </>
          ) : (
            "Peça ao proprietário para regularizar a assinatura. Você poderá revisar e confirmar a venda depois."
          )}
        </Alert>
      )}
      {(!!command.attempt || command.corrupt) && !command.busy && (
        <div className={styles.recovery}>
          <Alert tone="warning">
            Há uma tentativa sem confirmação nesta aba. Confira o histórico ou repita a confirmação
            com os mesmos dados. A repetição recupera o resultado sem duplicar a operação.
          </Alert>
          {!command.corrupt && (
            <Button variant="secondary" onClick={onRetry}>
              Repetir confirmação
            </Button>
          )}
          {!confirm ? (
            <Button variant="quiet" onClick={() => setConfirm(true)}>
              Já conferi o histórico
            </Button>
          ) : (
            <>
              <p>
                Encerrar esta tentativa não desfaz uma operação que já tenha sido salva. Confirme
                somente depois de conferir o histórico.
              </p>
              <Button variant="secondary" onClick={() => setConfirm(false)}>
                Continuar recuperando
              </Button>
              <Button
                variant="danger"
                onClick={() => {
                  command.discard();
                  setConfirm(false);
                }}
              >
                Encerrar tentativa conferida
              </Button>
            </>
          )}
        </div>
      )}
    </>
  );
}
export function commerceTime(instant: string, timezone?: string) {
  return new Intl.DateTimeFormat("pt-BR", {
    dateStyle: "short",
    timeStyle: "short",
    ...(timezone ? { timeZone: timezone } : {}),
  }).format(new Date(instant));
}
