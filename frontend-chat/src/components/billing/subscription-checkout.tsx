"use client";
import { ArrowUpRight } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
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
  const openingPayment = useRef(false);
  const [paymentOpened, setPaymentOpened] = useState(false);
  const [paymentError, setPaymentError] = useState("");
  useEffect(() => {
    if (flow.mode === "closed" || flow.mode === "loading") {
      setPaymentOpened(false);
      setPaymentError("");
    }
  }, [flow.mode]);
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
  async function proceedToPayment() {
    if (openingPayment.current || configurationPending || disabled ||
        (flow.mode !== "review" && !readyLink)) return;
    openingPayment.current = true;
    setPaymentError("");
    // Reserve the tab during the user gesture; opening after await can be blocked.
    let paymentWindow: Window | null = null;
    try {
      paymentWindow = window.open("about:blank", "_blank");
      if (!paymentWindow) {
        setPaymentError("Seu navegador bloqueou a aba de pagamento. Permita a abertura de uma nova aba e clique em Prosseguir para pagamento novamente.");
        return;
      }
      paymentWindow.opener = null;
      paymentWindow.document.title = "Pagamento da assinatura";
      paymentWindow.document.body.textContent = "Aguarde enquanto preparamos seu pagamento na Stripe…";
      const checkout = readyLink ? flow.checkout : await flow.confirm();
      if (!checkout) {
        paymentWindow.close();
        return;
      }
      if (paymentWindow.closed) {
        setPaymentError("A aba de pagamento foi fechada. Clique em Prosseguir para pagamento para abrir a mesma tentativa.");
        return;
      }
      if (Date.parse(checkout.expires_at) <= Date.now() + 30_000) {
        paymentWindow.close();
        setPaymentError("O endereço de pagamento expirou. Consulte as condições novamente.");
        return;
      }
      // confirm() returns only the Stripe URL validated by the API client.
      paymentWindow.location.replace(checkout.url);
      setPaymentOpened(true);
    } catch {
      paymentWindow?.close();
      setPaymentError("Não foi possível abrir o pagamento. Confira a situação antes de tentar novamente.");
    } finally {
      openingPayment.current = false;
    }
  }
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
            {!inline && flow.offer && (
              <Badge tone={flow.offer.livemode ? "info" : "warning"}>
                {flow.offer.livemode
                  ? "Assinatura com cobrança real"
                  : "Modo de teste — não use cartão real"}
              </Badge>
            )}
            {flow.mode === "loading" && <p role="status">Consultando as condições na API…</p>}
            {flow.error && <Alert tone="error">{flow.error}</Alert>}
            {paymentError && <Alert tone="error">{paymentError}</Alert>}
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
              <p role="status">{inline ? "Abrindo o pagamento na Stripe…" : "Preparando ou recuperando o Checkout na API…"}</p>
            )}
            {flow.mode === "ready" && (
              <Alert>
                {inline
                  ? "Conclua o pagamento na Stripe e depois confira a assinatura para retomar seu agendamento."
                  : "Checkout preparado. Conclua na Stripe e depois confira a assinatura. Isso não publica a barbearia nem cria agendamento automaticamente."}
              </Alert>
            )}
            {configurationPending && (
              <p className={styles.note}>
                Salve ou descarte as alterações de cadastro antes de continuar para o pagamento.
              </p>
            )}
            <div className={styles.actions}>
              {(flow.mode === "review" || (inline && readyLink && !paymentOpened)) && (
                <Button
                  disabled={configurationPending || disabled}
                  onClick={() => void (inline ? proceedToPayment() : flow.confirm())}
                >
                  {inline ? "Prosseguir para pagamento" : "Preparar Checkout"}
                </Button>
              )}
              {!inline && readyLink && !configurationPending && !disabled && (
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
          Revise as condições e prossiga para o pagamento na Stripe. Ele abre em outra aba para manter os dados do agendamento nesta janela.
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
