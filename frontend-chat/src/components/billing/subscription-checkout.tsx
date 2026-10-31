"use client";
import { ArrowUpRight } from "lucide-react";
import type { ReactNode } from "react";
import type { useSubscriptionCheckout } from "@/hooks/use-subscription-checkout";
import { Alert, Badge, Button, Modal, buttonClasses } from "../ui";
import styles from "./subscription-checkout.module.css";

export function SubscriptionCheckoutControl({
  checkout: flow,
  configurationPending,
  disabled,
  onCheck,
  inline = false,
  backLabel = "Voltar às configurações",
}: {
  checkout: ReturnType<typeof useSubscriptionCheckout>;
  configurationPending: boolean;
  disabled: boolean;
  onCheck: () => void;
  inline?: boolean;
  backLabel?: string;
}) {
  const offer = flow.offer;
  const amount = offer
    ? new Intl.NumberFormat("pt-BR", { style: "currency", currency: offer.currency }).format(
        offer.amount_cents / 100,
      )
    : "";
  const readyLink =
    flow.mode === "ready" &&
    flow.checkout &&
    Date.parse(flow.checkout.expires_at) > Date.now() + 30_000;
  return (
    <>
      {!inline && (
        <div className={styles.actions}>
          <Button
            variant="secondary"
            disabled={
              disabled || configurationPending || flow.mode === "loading" || flow.mode === "opening"
            }
            onClick={() => void flow.load()}
          >
            Ver plano e assinar
          </Button>
        </div>
      )}
      {flow.mode !== "closed" && (
        <CheckoutContainer inline={inline} onClose={flow.close}>
          <div className={styles.feedback}>
            <Badge tone="warning">Modo de teste — não use cartão real</Badge>
            {flow.mode === "loading" && <p role="status">Consultando as condições na API…</p>}
            {flow.error && <Alert tone="error">{flow.error}</Alert>}
            {flow.uncertain && (
              <Alert tone="warning">
                A tentativa pode ter sido registrada. Consulte novamente as condições para recuperar
                o Checkout da mesma barbearia. Não crie outra assinatura por fora.
              </Alert>
            )}
            {offer && (
              <>
                <p className={styles.subscriptionPrice}>
                  {amount}
                  <span> por mês, por barbearia</span>
                </p>
                <p className={styles.note}>
                  Cobrança recorrente mensal em BRL, no cartão, sem período gratuito nesta oferta.
                </p>
                {offer.recovering && (
                  <Alert>
                    Já existe uma tentativa para esta barbearia. Estas são as condições preservadas
                    daquela tentativa.
                  </Alert>
                )}
              </>
            )}
            {flow.mode === "opening" && (
              <p role="status">Preparando ou recuperando o Checkout na API…</p>
            )}
            {flow.mode === "ready" && (
              <Alert>
                Checkout preparado. Conclua na Stripe e depois confira a assinatura. Isso não
                publica a barbearia nem cria agendamento automaticamente.
              </Alert>
            )}
            {configurationPending && (
              <p className={styles.note}>
                Salve ou descarte as alterações de cadastro antes de continuar para o pagamento.
              </p>
            )}
            <div className={styles.actions}>
              {flow.mode === "review" && (
                <Button
                  disabled={configurationPending || disabled}
                  onClick={() => void flow.confirm()}
                >
                  Preparar Checkout
                </Button>
              )}
              {readyLink && !configurationPending && !disabled && (
                <a
                  href={flow.checkout!.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={buttonClasses()}
                  onClick={(event) => {
                    if (
                      configurationPending ||
                      disabled ||
                      Date.parse(flow.checkout!.expires_at) <= Date.now() + 30_000
                    ) {
                      event.preventDefault();
                      return;
                    }
                  }}
                >
                  Continuar na Stripe <ArrowUpRight size={16} aria-hidden="true" />
                </a>
              )}
              {(flow.mode === "error" || (flow.mode === "ready" && !readyLink)) && (
                <Button variant="secondary" onClick={() => void flow.load()}>
                  Consultar condições novamente
                </Button>
              )}
              <Button
                variant="secondary"
                disabled={flow.mode === "loading" || flow.mode === "opening"}
                onClick={() => {
                  flow.close();
                  onCheck();
                }}
              >
                Conferir assinatura
              </Button>
              <Button
                variant="quiet"
                disabled={flow.mode === "loading" || flow.mode === "opening"}
                onClick={flow.close}
              >
                {backLabel}
              </Button>
            </div>
          </div>
        </CheckoutContainer>
      )}
    </>
  );
}

function CheckoutContainer({
  inline,
  onClose,
  children,
}: {
  inline: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  if (inline)
    return (
      <section aria-label="Assinatura mensal da barbearia" className={styles.feedback}>
        <h3>Assinatura mensal da barbearia</h3>
        <p className={styles.note}>
          Revise as condições antes de preparar o Checkout. O pagamento abre em outra aba; mantenha
          este agendamento aberto.
        </p>
        {children}
      </section>
    );
  return (
    <Modal
      open
      title="Assinatura mensal da barbearia"
      onClose={onClose}
      subtitle="Revise as condições antes de preparar o Checkout. Nenhum cartão é informado neste painel."
    >
      {children}
    </Modal>
  );
}
