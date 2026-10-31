"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import {
  getCommercialStatus,
  hasCurrentCoverage,
  type CommercialStatus,
} from "@/lib/api/shop-commercial";
import { adminShopPath } from "@/lib/api/config";
import { useSubscriptionCheckout } from "@/hooks/use-subscription-checkout";
import { SubscriptionCheckoutControl } from "../billing/subscription-checkout";
import { Alert, Button, buttonClasses } from "../ui";
import type { StaffBookingContext, useBookingFlow } from "./use-booking-flow";
import styles from "./booking.module.css";

export function StaffSubscriptionRecovery({
  slug,
  staff,
  flow,
}: {
  slug: string;
  staff: StaffBookingContext;
  flow: ReturnType<typeof useBookingFlow>;
}) {
  if (staff.role !== "owner")
    return (
      <>
        <Alert tone="warning">
          A assinatura da barbearia precisa ser regularizada para criar novos agendamentos. Procure
          o proprietário. Os dados preenchidos continuam nesta janela.
        </Alert>
        <Button loading={flow.busy} onClick={() => void flow.resumeAfterSubscription()}>
          Atualizar horário e revisar
        </Button>
      </>
    );
  return (
    <OwnerRecovery key={staff.shopID + ":" + staff.userID} slug={slug} staff={staff} flow={flow} />
  );
}

function OwnerRecovery({
  slug,
  staff,
  flow,
}: {
  slug: string;
  staff: StaffBookingContext;
  flow: ReturnType<typeof useBookingFlow>;
}) {
  const checkout = useSubscriptionCheckout(slug, staff.shopID, staff.userID, "manual_booking");
  const [status, setStatus] = useState<CommercialStatus | null>(null);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const busy = useRef(false);
  const revision = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const check = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    const version = ++revision.current;
    const abort = new AbortController();
    controller.current = abort;
    setChecking(true);
    setStatus(null);
    setError("");
    try {
      const current = await getCommercialStatus(slug, staff.shopID, staff.userID, abort.signal);
      if (revision.current === version) setStatus(current);
    } catch (failure) {
      if (revision.current === version) setError(errorMessage(failure));
    } finally {
      if (revision.current === version) {
        busy.current = false;
        setChecking(false);
      }
    }
  }, [slug, staff.shopID, staff.userID]);
  useEffect(() => {
    void check();
    return () => {
      revision.current++;
      controller.current?.abort();
      busy.current = false;
    };
  }, [check]);
  useEffect(() => {
    flow.setBillingBusy(checkout.pending);
    return () => flow.setBillingBusy(false);
  }, [checkout.pending, flow.setBillingBusy]);
  const covered = !!status && hasCurrentCoverage(status);
  const canStart =
    !!status &&
    status.publication_managed &&
    ["none", "canceled"].includes(status.subscription.status) &&
    !covered;
  return (
    <section className={styles.flow} aria-label="Assinatura para agendamento manual">
      <Alert tone="warning">
        {flow.uncertain
          ? "O resultado da tentativa anterior continua incerto. Seus dados e a mesma chave foram preservados para recuperação após regularizar a assinatura."
          : "A API não liberou esta reserva sem assinatura. Seus dados foram mantidos nesta janela; o horário ainda não está reservado."}{" "}
        Mantenha esta aba aberta durante o pagamento.
      </Alert>
      {checking && <p role="status">Conferindo a assinatura na API…</p>}
      {error && <Alert tone="error">{error}</Alert>}
      {covered ? (
        <>
          <Alert tone="success">
            Assinatura confirmada. Retome a revisão para conferir o horário antes de confirmar a
            reserva.
          </Alert>
          <Button
            loading={flow.busy}
            disabled={checkout.pending}
            onClick={() => void flow.resumeAfterSubscription()}
          >
            Retomar revisão do agendamento
          </Button>
        </>
      ) : (
        <>
          {status && !canStart && (
            <p className={styles.hint}>
              Já existe uma assinatura para regularizar ou uma confirmação pendente. Confira a
              situação em Configurações; não abra outra assinatura.
            </p>
          )}
          {canStart && checkout.mode === "closed" && (
            <Button disabled={checking || flow.busy} onClick={() => void checkout.load()}>
              Ver plano e assinar
            </Button>
          )}
        </>
      )}
      {checkout.mode !== "closed" && (
        <SubscriptionCheckoutControl
          inline
          checkout={checkout}
          configurationPending={false}
          disabled={checking || covered || flow.busy}
          onCheck={() => void check()}
          backLabel="Voltar ao agendamento"
        />
      )}
      {checkout.mode === "closed" && (
        <Button variant="secondary" disabled={checking || flow.busy} onClick={() => void check()}>
          Conferir assinatura
        </Button>
      )}
      <Link
        href={adminShopPath(slug) + "/configuracoes#publication"}
        target="_blank"
        rel="noopener noreferrer"
        className={buttonClasses("quiet")}
      >
        Abrir Configurações em outra aba
      </Link>
      <p className={styles.hint}>
        Conferir a assinatura apenas consulta a API. O pagamento não cria o agendamento nem publica
        o link da barbearia.
      </p>
    </section>
  );
}
