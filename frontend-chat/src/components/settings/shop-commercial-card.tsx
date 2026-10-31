"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowUpRight, Copy, RefreshCw } from "lucide-react";
import type { useShopCommercial } from "@/hooks/use-shop-commercial";
import type { useSubscriptionCheckout } from "@/hooks/use-subscription-checkout";
import type { useBillingPortal } from "@/hooks/use-billing-portal";
import { CustomerPortalControl } from "../billing/customer-portal";
import { publicBookingPath } from "@/lib/api/config";
import { hasCurrentCoverage, type CommercialReason } from "@/lib/api/shop-commercial";
import { Alert, Badge, Button, Modal, buttonClasses } from "../ui";
import styles from "./shop-settings.module.css";
import { SubscriptionCheckoutControl } from "../billing/subscription-checkout";

const subscriptionLabels: Record<CommercialReason, string> = {
  legacy_access: "Acesso anterior mantido",
  subscribed: "Assinatura confirmada",
  grace_period: "Acesso em prazo de regularização",
  subscription_required: "Sem assinatura confirmada",
  payment_pending: "Aguardando confirmação",
  payment_required: "Pagamento necessário",
  subscription_suspended: "Assinatura suspensa",
  subscription_canceled: "Assinatura encerrada",
};
const readinessLabels: Record<string, string> = {
  address_required: "Complete e salve o endereço da barbearia.",
  service_required: "Cadastre pelo menos um serviço ativo e agendável.",
  professional_required: "Cadastre um barbeiro ativo vinculado a um serviço.",
  schedule_required: "Configure um expediente que comporte o serviço, a grade e a margem.",
};

export function ShopCommercialCard({
  commercial,
  configurationPending,
  timezone,
  checkout,
  portal,
}: {
  commercial: ReturnType<typeof useShopCommercial>;
  configurationPending: boolean;
  timezone: string;
  checkout: ReturnType<typeof useSubscriptionCheckout>;
  portal: ReturnType<typeof useBillingPortal>;
}) {
  const { mode, status, error, notice, canShare } = commercial;
  const [confirmation, setConfirmation] = useState<boolean | null>(null);
  const [copyNotice, setCopyNotice] = useState("");
  const [returned, setReturned] = useState(false);
  const [portalReturned, setPortalReturned] = useState(false);
  useEffect(() => {
    const checkout = new URLSearchParams(window.location.search).get("checkout");
    setReturned(checkout === "returned" || checkout === "canceled");
    setPortalReturned(new URLSearchParams(window.location.search).get("portal") === "returned");
  }, []);
  const ready = mode === "ready" && !!status;
  const canPublish =
    ready && status.can_publish && hasCurrentCoverage(status) && !configurationPending;
  const until = status?.subscription.coverage_until;
  const deadline = until
    ? new Intl.DateTimeFormat("pt-BR", {
        dateStyle: "long",
        timeStyle: "short",
        timeZone: timezone,
      }).format(new Date(until))
    : null;

  return (
    <section
      className={`${styles.section} ${styles.commercial}`}
      id="publication"
      aria-labelledby="commercial-title"
      aria-busy={mode === "loading" || mode === "saving"}
    >
      <div className={styles.commercialHeading}>
        <div>
          <h2 id="commercial-title">Assinatura e agendamento online</h2>
          <p className={styles.note}>
            Configure sem pagar. Publicar o link é uma decisão separada da assinatura.
          </p>
        </div>
        <Button
          variant="secondary"
          disabled={mode === "saving" || mode === "loading" || mode === "blocked"}
          onClick={() => {
            setCopyNotice("");
            void commercial.refresh();
          }}
        >
          <RefreshCw size={16} aria-hidden="true" />
          {mode === "uncertain" ? "Conferir situação na API" : "Atualizar situação"}
        </Button>
      </div>
      {returned && (
        <Alert>
          O retorno do Checkout não confirma pagamento nem publica sua barbearia. A situação abaixo
          vem da API.
        </Alert>
      )}
      {mode === "loading" && <p role="status">Consultando assinatura e publicação…</p>}
      {portalReturned && (
        <Alert>
          Você voltou do portal. A situação abaixo vem da API após a confirmação da Stripe; o
          retorno não altera a assinatura nem publica o link.
        </Alert>
      )}
      {mode === "saving" && (
        <p role="status">Atualizando publicação e conferindo a situação na API…</p>
      )}
      {notice && <Alert tone="info">{notice}</Alert>}
      {error && <Alert tone="error">{error}</Alert>}
      {mode === "uncertain" && (
        <Alert tone="warning">
          O resultado ainda precisa ser conferido. Não publique nem retire do ar novamente antes de
          consultar a situação. Seu formulário foi mantido.
        </Alert>
      )}
      {mode === "blocked" && (
        <Link href="/admin" className={buttonClasses("secondary")}>
          Validar acesso à barbearia
        </Link>
      )}
      {ready && (
        <>
          <div className={styles.commercialStatuses}>
            <Badge tone={status.subscription.operation_allowed ? "success" : "warning"}>
              {subscriptionLabels[status.subscription.reason]}
            </Badge>
            <Badge tone={canShare ? "success" : "neutral"}>
              {canShare
                ? "Link ativo"
                : status.publication_requested
                  ? "Link indisponível"
                  : "Não publicada"}
            </Badge>
          </div>
          {!status.publication_managed ? (
            <p className={styles.note}>
              Esta barbearia mantém o acesso anterior da plataforma. Nenhuma assinatura foi iniciada
              ou cobrada. Mudanças na publicação são gerenciadas pelo responsável pela plataforma.
            </p>
          ) : (
            <>
              {status.subscription.status !== "none" && (
                <CustomerPortalControl
                  flow={portal}
                  disabled={mode !== "ready"}
                  onCheck={() => void commercial.refresh()}
                />
              )}
              {deadline && (
                <p className={styles.note}>
                  Acesso confirmado até {deadline}, no fuso {timezone}. Novos agendamentos dependem
                  da cobertura vigente.
                </p>
              )}
              {status.subscription.cancel_at_period_end && (
                <Alert tone="warning">
                  Há um cancelamento programado. A cobertura já confirmada permanece válida até o
                  prazo informado.
                </Alert>
              )}
              {status.publication_requested && !status.public_booking_enabled && (
                <Alert tone="warning">
                  A intenção de publicação foi mantida, mas o link está indisponível sem cobertura.
                  Uma renovação confirmada pode reativá-lo. Retire do ar se não quiser essa
                  reativação.
                </Alert>
              )}
              {status.blockers.length > 0 && (
                <div>
                  <h3 className={styles.commercialSubheading}>Pendências para publicar</h3>
                  <ul className={styles.commercialList}>
                    {status.blockers.map((reason) => (
                      <li key={reason}>
                        {readinessLabels[reason] ??
                          "Confirme ou regularize a assinatura da barbearia."}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {!status.subscription.operation_allowed && (
                <p className={styles.note}>
                  Você pode continuar configurando a barbearia e consultando os agendamentos
                  existentes. A assinatura habilita novas reservas; publicar o link exige uma
                  confirmação separada.
                </p>
              )}
              {["none", "canceled"].includes(status.subscription.status) && (
                <SubscriptionCheckoutControl
                  checkout={checkout}
                  configurationPending={configurationPending}
                  disabled={mode !== "ready"}
                  onCheck={() => void commercial.refresh()}
                />
              )}
              {configurationPending && (
                <p className={styles.note}>
                  Salve e confira as alterações do formulário antes de publicar. A retirada do ar
                  continua disponível.
                </p>
              )}
              <div className={styles.actions}>
                {!status.publication_requested && (
                  <Button disabled={!canPublish} onClick={() => setConfirmation(true)}>
                    Publicar barbearia
                  </Button>
                )}
                {status.publication_requested && (
                  <Button variant="danger" onClick={() => setConfirmation(false)}>
                    Retirar do ar
                  </Button>
                )}
              </div>
            </>
          )}
          {canShare && (
            <div className={styles.commercialLink}>
              <h3 className={styles.commercialSubheading}>Link para seus clientes</h3>
              <p className={styles.note}>
                Compartilhe nas redes sociais ou envie diretamente aos clientes.
              </p>
              <code>{publicBookingPath(status.slug)}</code>
              <div className={styles.actions}>
                <Link
                  href={publicBookingPath(status.slug)}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={buttonClasses("secondary")}
                >
                  Abrir agendamento <ArrowUpRight size={16} aria-hidden="true" />
                </Link>
                <Button
                  variant="secondary"
                  onClick={async () => {
                    setCopyNotice("");
                    if (!hasCurrentCoverage(status)) {
                      void commercial.refresh();
                      return;
                    }
                    try {
                      await navigator.clipboard.writeText(
                        window.location.origin + publicBookingPath(status.slug),
                      );
                      setCopyNotice("Link de agendamento copiado.");
                    } catch {
                      setCopyNotice(
                        "Não foi possível copiar. Abra o agendamento e copie o endereço na barra do navegador.",
                      );
                    }
                  }}
                >
                  <Copy size={16} aria-hidden="true" />
                  Copiar link de agendamento
                </Button>
              </div>
              {copyNotice && (
                <p className={styles.note} role="status">
                  {copyNotice}
                </p>
              )}
            </div>
          )}
        </>
      )}
      {confirmation !== null && (
        <Modal
          open
          title={confirmation ? "Publicar barbearia?" : "Retirar barbearia do ar?"}
          subtitle={
            confirmation
              ? "A API vai revalidar a assinatura e a configuração salva antes de liberar o agendamento online. Esta ação não inicia cobrança."
              : "Novos clientes não poderão agendar pelo link. A assinatura não será cancelada e as reservas existentes serão mantidas."
          }
          onClose={() => setConfirmation(null)}
        >
          <div className={styles.actions}>
            <Button variant="secondary" onClick={() => setConfirmation(null)}>
              Voltar
            </Button>
            <Button
              variant={confirmation ? "primary" : "danger"}
              disabled={!ready || (confirmation && !canPublish)}
              onClick={() => {
                const desired = confirmation;
                setConfirmation(null);
                setCopyNotice("");
                void commercial.setRequested(desired);
              }}
            >
              {confirmation ? "Confirmar publicação" : "Confirmar retirada do ar"}
            </Button>
          </div>
        </Modal>
      )}
    </section>
  );
}
