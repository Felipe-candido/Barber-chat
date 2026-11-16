"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import { adminShopPath } from "@/lib/api/config";
import {
  getCommercialStatus,
  hasCurrentCoverage,
  type CommercialStatus,
} from "@/lib/api/shop-commercial";
import { useSubscriptionCheckout } from "@/hooks/use-subscription-checkout";
import { Alert, Button, Modal, buttonClasses } from "../ui";
import { SubscriptionCheckoutControl } from "./subscription-checkout";
import styles from "./subscription-checkout.module.css";

type Scope = { slug: string; shopID: string; userID: string };

// Asks the API whether the shop has current coverage before an owner-only
// action. This is a courtesy check for the screen; the API stays the authority.
export function useSubscriptionGate(scope: Scope) {
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const [required, setRequired] = useState<CommercialStatus | null>(null);
  const busy = useRef(false);
  const controller = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      controller.current?.abort();
      busy.current = false;
    },
    [scope],
  );
  async function run(onCovered: () => void) {
    if (busy.current) return;
    busy.current = true;
    const abort = new AbortController();
    controller.current = abort;
    setChecking(true);
    setError("");
    try {
      const status = await getCommercialStatus(
        scope.slug,
        scope.shopID,
        scope.userID,
        abort.signal,
      );
      if (abort.signal.aborted) return;
      if (hasCurrentCoverage(status)) onCovered();
      else setRequired(status);
    } catch (failure) {
      if (!abort.signal.aborted) setError(errorMessage(failure));
    } finally {
      if (!abort.signal.aborted) {
        busy.current = false;
        setChecking(false);
      }
    }
  }
  return { checking, error, required, run, dismiss: () => setRequired(null) };
}

export function SubscriptionRequiredDialog({
  scope,
  status,
  purpose,
  onCovered,
  onClose,
}: {
  scope: Scope;
  status: CommercialStatus;
  // Completes "A assinatura é necessária para …".
  purpose: string;
  onCovered: () => void;
  onClose: () => void;
}) {
  const checkout = useSubscriptionCheckout(scope.slug, scope.shopID, scope.userID);
  const [current, setCurrent] = useState(status);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const [confirmedMissing, setConfirmedMissing] = useState(false);
  const busy = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const canStart =
    current.publication_managed && ["none", "canceled"].includes(current.subscription.status);
  async function recheck() {
    if (busy.current) return;
    busy.current = true;
    setChecking(true);
    setError("");
    setConfirmedMissing(false);
    try {
      const next = await getCommercialStatus(scope.slug, scope.shopID, scope.userID);
      if (!alive.current) return;
      if (hasCurrentCoverage(next)) {
        onCovered();
        return;
      }
      setCurrent(next);
      setConfirmedMissing(true);
    } catch (failure) {
      if (alive.current) setError(errorMessage(failure));
    } finally {
      busy.current = false;
      if (alive.current) setChecking(false);
    }
  }
  return (
    <Modal
      open
      title="Assinatura necessária"
      subtitle={`A assinatura da barbearia é necessária para ${purpose}. O que você já cadastrou continua salvo.`}
      onClose={() => {
        if (!checkout.pending) onClose();
      }}
    >
      <div className={styles.feedback}>
        {error && <Alert tone="error">{error}</Alert>}
        {checking && <p role="status">Conferindo a assinatura na API…</p>}
        {confirmedMissing && !checking && (
          <Alert tone="warning">
            A assinatura ainda não foi confirmada. Se você acabou de pagar, aguarde alguns segundos
            e confira de novo.
          </Alert>
        )}
        {canStart ? (
          checkout.mode === "closed" ? (
            <div className={styles.actions}>
              <Button disabled={checking} onClick={() => void checkout.load()}>
                Prosseguir para a assinatura
              </Button>
              <Button variant="secondary" disabled={checking} onClick={() => void recheck()}>
                Conferir assinatura
              </Button>
            </div>
          ) : (
            <SubscriptionCheckoutControl
              inline
              bookingContext={false}
              checkout={checkout}
              configurationPending={false}
              disabled={checking}
              onCheck={() => void recheck()}
              backLabel="Voltar"
            />
          )
        ) : (
          <>
            <p className={styles.note}>
              Já existe uma assinatura para regularizar ou uma confirmação pendente. Resolva em
              Configurações; não é preciso abrir outra assinatura.
            </p>
            <div className={styles.actions}>
              <Link href={adminShopPath(scope.slug) + "/configuracoes"} className={buttonClasses()}>
                Abrir Configurações
              </Link>
              <Button variant="secondary" disabled={checking} onClick={() => void recheck()}>
                Conferir assinatura
              </Button>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
