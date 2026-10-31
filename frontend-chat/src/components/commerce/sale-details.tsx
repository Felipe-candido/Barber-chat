"use client";
import { useEffect, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import {
  cancelSale,
  getSale,
  isCancellationInput,
  type CancelSaleInput,
  type Sale,
} from "@/lib/api/sales";
import { useCommerceCommand } from "@/hooks/use-commerce-command";
import { useSelectedShopAccess } from "../shop-panel";
import { Alert, Button, Modal, TextareaField } from "../ui";
import { CommandFeedback, useCommerceScope } from "./shared";
import { SaleReceipt } from "./sale-receipt";
import styles from "./commerce.module.css";

export function SaleDetails({
  id,
  timezone,
  onClose,
  onUpdated,
}: {
  id: string;
  timezone: string;
  onClose: () => void;
  onUpdated: () => void;
}) {
  const scope = useCommerceScope(),
    owner = useSelectedShopAccess().role === "owner";
  const [sale, setSale] = useState<Sale | null>(null),
    [error, setError] = useState(""),
    [revision, setRevision] = useState(0),
    [loading, setLoading] = useState(true);
  const command = useCommerceCommand(
    scope,
    "sale:cancel:" + id,
    isCancellationInput,
    (body, key, signal) => cancelSale(scope, id, body, key, signal),
  );
  const [last, setLast] = useState<CancelSaleInput | null>(null);
  const draft = command.attempt ?? last;
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    setSale(null);
    void getSale(scope, id, c.signal)
      .then((out) => {
        if (!c.signal.aborted) setSale(out);
      })
      .catch((cause) => {
        if (!c.signal.aborted) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [scope, id, revision]);
  async function cancel(body?: CancelSaleInput) {
    if (body) setLast(body);
    const out = await command.submit(body);
    if (out) {
      setLast(null);
      setSale(out);
      onUpdated();
    }
  }
  return (
    <Modal
      open
      title="Detalhes da venda"
      wide
      onClose={() => {
        if (!command.busy) onClose();
      }}
    >
      <div className={styles.stack}>
        <div className={styles.actions}>
          <Button
            variant="quiet"
            disabled={loading || command.busy}
            onClick={() => setRevision((v) => v + 1)}
          >
            Atualizar detalhes
          </Button>
        </div>
        {loading && <p role="status">Consultando venda…</p>}
        {error && (
          <>
            <Alert tone="error">{error}</Alert>
            <Button variant="secondary" onClick={() => setRevision((v) => v + 1)}>
              Consultar venda novamente
            </Button>
          </>
        )}
        {sale && <SaleReceipt sale={sale} timezone={timezone} />}
        {owner && <CommandFeedback command={command} onRetry={() => void cancel()} />}
        {owner && sale?.status === "recorded" && (
          <details className={styles.panel}>
            <summary>Anular venda</summary>
            <form
              className={styles.form}
              onSubmit={(event) => {
                event.preventDefault();
                if (command.locked) return;
                const values = new FormData(event.currentTarget),
                  restock = values.get("restock"),
                  reason = String(values.get("reason")).trim();
                if (!reason || (restock !== "true" && restock !== "false")) {
                  setError("Informe o motivo e escolha o que acontece com o estoque.");
                  return;
                }
                void cancel({
                  expected_version: sale.version,
                  reason,
                  restock: restock === "true",
                });
              }}
            >
              <fieldset
                className={styles.fields}
                key={command.attempt ? "restored" : "new"}
                disabled={command.locked}
              >
                <div className={styles.full}>
                  <TextareaField
                    name="reason"
                    label="Motivo da anulação"
                    required
                    maxLength={500}
                    rows={3}
                    defaultValue={draft?.reason ?? ""}
                  />
                </div>
                <fieldset className={styles.choice + " " + styles.full}>
                  <legend>O que acontece com os produtos?</legend>
                  <label>
                    <input
                      type="radio"
                      name="restock"
                      value="true"
                      required
                      defaultChecked={draft?.restock === true}
                    />
                    <span>
                      Devolver ao estoque
                      <br />
                      <small>Mercadoria devolvida ou saída registrada por engano.</small>
                    </span>
                  </label>
                  <label>
                    <input
                      type="radio"
                      name="restock"
                      value="false"
                      required
                      defaultChecked={draft?.restock === false}
                    />
                    <span>
                      Manter fora do estoque
                      <br />
                      <small>A mercadoria não voltou para a barbearia.</small>
                    </span>
                  </label>
                </fieldset>
              </fieldset>
              <p className={styles.muted}>
                A anulação é definitiva e não processa reembolso de pagamento. Os itens e preços
                originais serão preservados.
              </p>
              <div className={styles.actions}>
                <Button
                  type="submit"
                  variant="danger"
                  disabled={command.locked}
                  loading={command.busy}
                >
                  Confirmar anulação
                </Button>
              </div>
            </form>
          </details>
        )}
        <div className={styles.actions}>
          <Button variant="secondary" disabled={command.busy} onClick={onClose}>
            Fechar
          </Button>
        </div>
      </div>
    </Modal>
  );
}
