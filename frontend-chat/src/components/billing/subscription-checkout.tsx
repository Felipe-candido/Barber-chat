"use client";
import { ArrowUpRight } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { useSubscriptionCheckout } from "@/hooks/use-subscription-checkout";
import { Alert, Badge, Button, Modal } from "../ui";
import styles from "./subscription-checkout.module.css";

export function SubscriptionCheckoutControl({
  checkout: flow,
  configurationPending,
  disabled,
  onCheck,
  inline = false,
  autoStart = false,
  bookingContext = true,
  backLabel = "Voltar às configurações",
}: {
  checkout: ReturnType<typeof useSubscriptionCheckout>;
  configurationPending: boolean;
  disabled: boolean;
  onCheck: () => void;
  inline?: boolean;
  // The caller already showed the first step and loads the offer itself.
  autoStart?: boolean;
  // Inline copy mentions the manual booking being kept; other callers turn it off.
  bookingContext?: boolean;
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
    if (
      openingPayment.current ||
      configurationPending ||
      disabled ||
      (flow.mode !== "review" && !readyLink)
    )
      return;
    openingPayment.current = true;
    setPaymentError("");
    // Reserve the tab during the user gesture; opening after await can be blocked.
    let paymentWindow: Window | null = null;
    try {
      paymentWindow = window.open("about:blank", "_blank");
      if (!paymentWindow) {
        setPaymentError(
          "Seu navegador bloqueou a página de pagamento. Permita abrir uma nova aba e clique em Abrir pagamento.",
        );
        return;
      }
      paymentWindow.opener = null;
      paymentWindow.document.title = "Pagamento da assinatura";
      paymentWindow.document.body.textContent = "Estamos preparando seu pagamento…";
      const checkout = readyLink ? flow.checkout : await flow.confirm();
      if (!checkout) {
        paymentWindow.close();
        return;
      }
      if (paymentWindow.closed) {
        setPaymentError(
          "A página de pagamento foi fechada. Clique em Abrir pagamento para continuar de onde parou.",
        );
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
      setPaymentError(
        "Não foi possível abrir o pagamento. Confira a situação antes de tentar novamente.",
      );
    } finally {
      openingPayment.current = false;
    }
  }
  return (
    <>
      {!inline && !autoStart && (
        <div className={styles.actions}>
          <Button
            disabled={
              disabled || configurationPending || flow.mode === "loading" || flow.mode === "opening"
            }
            onClick={() => void flow.load()}
          >
            Prosseguir para a assinatura
          </Button>
        </div>
      )}
      {flow.mode !== "closed" && (
        <CheckoutContainer inline={inline} bookingContext={bookingContext} onClose={flow.close}>
          <div className={styles.feedback}>
            {!inline && flow.offer && (
              <Badge tone={flow.offer.livemode ? "info" : "warning"}>
                {flow.offer.livemode ? "Assinatura mensal" : "Não use cartão real nesta página"}
              </Badge>
            )}
            {flow.mode === "loading" && <p role="status">Carregando seu plano…</p>}
            {flow.error && <Alert tone="error">{flow.error}</Alert>}
            {paymentError && <Alert tone="error">{paymentError}</Alert>}
            {flow.uncertain && (
              <Alert tone="warning">
                Você pode já ter começado a assinatura. Confira o plano novamente para continuar de
                onde parou, sem criar outra assinatura.
              </Alert>
            )}
            {offer && (
              <>
                <p className={styles.subscriptionPrice}>
                  {amount}
                  <span> por mês, por barbearia</span>
                </p>
                <p className={styles.note}>
                  Pagamento mensal no cartão, em reais. Este plano não inclui período grátis.
                </p>
                {offer.recovering && (
                  <Alert>
                    Você já começou uma assinatura para esta barbearia. Confira os dados abaixo para
                    continuar.
                  </Alert>
                )}
              </>
            )}
            {flow.mode === "opening" && <p role="status">Abrindo o pagamento…</p>}
            {flow.mode === "ready" && (
              <Alert>
                {inline && bookingContext
                  ? "Conclua o pagamento na outra aba e confira sua assinatura para continuar o agendamento."
                  : "Conclua o pagamento na outra aba e confira sua assinatura. Depois, ative seu link de agendamento."}
              </Alert>
            )}
            {configurationPending && (
              <p className={styles.note}>
                Salve ou descarte as alterações de cadastro antes de continuar para o pagamento.
              </p>
            )}
            <div className={styles.actions}>
              {(flow.mode === "review" || (readyLink && !paymentOpened)) && (
                <Button
                  disabled={configurationPending || disabled}
                  onClick={() => void proceedToPayment()}
                >
                  Abrir pagamento <ArrowUpRight size={16} aria-hidden="true" />
                </Button>
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
  bookingContext,
  onClose,
  children,
}: {
  inline: boolean;
  bookingContext: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  if (inline)
    return (
      <section aria-label="Assinatura mensal da barbearia" className={styles.feedback}>
        <h3>Assinatura mensal da barbearia</h3>
        <p className={styles.note}>
          {bookingContext
            ? "Confira o plano e continue para o pagamento. Ele abre em outra aba, mantendo seu agendamento aqui."
            : "Confira o valor e continue para o pagamento em uma nova aba."}
        </p>
        {children}
      </section>
    );
  return (
    <Modal
      open
      title="Assinatura mensal da barbearia"
      onClose={onClose}
      subtitle="Confira o valor. Você informará os dados do cartão na página segura de pagamento, em outra aba."
    >
      {children}
    </Modal>
  );
}
