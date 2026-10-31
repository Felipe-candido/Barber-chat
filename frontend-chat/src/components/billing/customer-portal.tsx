"use client";
import { ArrowUpRight } from "lucide-react";
import type { useBillingPortal } from "@/hooks/use-billing-portal";
import { Alert, Badge, Button, Modal, buttonClasses } from "../ui";
import styles from "./subscription-checkout.module.css";

export function CustomerPortalControl({
  flow,
  onCheck,
  disabled,
}: {
  flow: ReturnType<typeof useBillingPortal>;
  onCheck: () => void;
  disabled: boolean;
}) {
  const available =
    !disabled &&
    flow.mode === "ready" &&
    flow.session &&
    Date.parse(flow.session.open_until) > Date.now() + 30_000;
  return (
    <>
      <div className={styles.actions}>
        <Button
          variant="secondary"
          disabled={disabled || flow.pending}
          onClick={() => void flow.open()}
        >
          Gerenciar assinatura
        </Button>
      </div>
      <Modal
        open={flow.mode !== "closed"}
        onClose={flow.close}
        title="Gerenciar assinatura na Stripe"
      >
        <div className={styles.feedback}>
          <Badge tone="warning">Modo de teste — não use cartão real</Badge>
          <p>
            Consulte faturas, atualize o cartão ou solicite o cancelamento ao fim do período. O
            acesso já pago permanece até o prazo confirmado.
          </p>
          <p className={styles.note}>
            O portal abre em outra aba. As alterações no cadastro desta página ficam preservadas. A
            situação da assinatura será atualizada após a confirmação da Stripe.
          </p>
          {flow.pending && <p role="status">Preparando acesso seguro ao portal…</p>}
          {flow.error && <Alert tone="error">{flow.error}</Alert>}
          <div className={styles.actions}>
            {available && (
              <a
                href={flow.session!.url}
                target="_blank"
                rel="noopener noreferrer"
                className={buttonClasses()}
                onClick={(event) => {
                  if (disabled || Date.parse(flow.session!.open_until) <= Date.now() + 30_000) {
                    event.preventDefault();
                    flow.close();
                  }
                }}
              >
                Abrir portal da Stripe <ArrowUpRight size={16} aria-hidden="true" />
              </a>
            )}
            {flow.mode === "error" && (
              <Button disabled={disabled} onClick={() => void flow.open()}>
                Preparar novo acesso
              </Button>
            )}
            <Button
              variant="secondary"
              disabled={flow.pending}
              onClick={() => {
                flow.close();
                onCheck();
              }}
            >
              Conferir situação
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
