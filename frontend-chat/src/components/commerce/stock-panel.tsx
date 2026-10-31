"use client";
import { useCallback, useRef, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import {
  isStockInput,
  listMovements,
  quantity,
  recordStock,
  updateMinimum,
  type Product,
  type ShopRequest,
  type StockInput,
} from "@/lib/api/products";
import { useCommerceCommand } from "@/hooks/use-commerce-command";
import { useCommercePage } from "@/hooks/use-commerce-page";
import { Alert, Button, Modal, SelectField, TextField, TextareaField } from "../ui";
import { CommandFeedback, commerceTime } from "./shared";
import { useFeedback } from "../feedback-provider";
import styles from "./commerce.module.css";

const movementNames = {
  opening: "Estoque inicial",
  receipt: "Entrada",
  adjustment: "Ajuste",
  sale: "Venda",
  sale_reversal: "Devolução de venda",
};
export function StockPanel({
  scope,
  initial,
  onClose,
  onUpdated,
}: {
  scope: ShopRequest;
  initial: Product;
  onClose: () => void;
  onUpdated: () => void;
}) {
  const { notify } = useFeedback();
  const [product, setProduct] = useState(initial),
    [error, setError] = useState(""),
    [minimumBusy, setMinimumBusy] = useState(false),
    [review, setReview] = useState(false);
  const busy = useRef(false);
  const [last, setLast] = useState<StockInput | null>(null);
  const load = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      listMovements(scope, initial.id, cursor, signal),
    [scope, initial.id],
  );
  const history = useCommercePage(load);
  const command = useCommerceCommand(
    scope,
    "stock:" + initial.id,
    isStockInput,
    (body, key, signal) => recordStock(scope, initial.id, body, key, signal),
  );
  const draft = command.attempt ?? last;
  async function save(body?: StockInput) {
    if (body) setLast(body);
    const out = await command.submit(body);
    if (out) {
      setLast(null);
      setProduct(out.product);
      history.refresh();
      onUpdated();
      notify(out.replayed ? "Movimentação recuperada." : "Movimentação registrada.");
    }
  }
  return (
    <Modal
      open
      title={"Estoque de " + product.name}
      subtitle="Entradas e ajustes ficam registrados no histórico."
      wide
      onClose={() => {
        if (!command.busy && !minimumBusy) onClose();
      }}
    >
      <div className={styles.stack}>
        <div className={styles.rowTop}>
          <strong className={styles.price}>{product.quantity_on_hand} unidades disponíveis</strong>
          <span className={styles.muted}>Mínimo: {product.minimum_quantity}</span>
        </div>
        <form
          className={styles.form}
          onSubmit={(event) => {
            event.preventDefault();
            if (command.locked || minimumBusy) return;
            const values = new FormData(event.currentTarget);
            setError("");
            try {
              const operation = String(values.get("operation")),
                amount = quantity(String(values.get("quantity")), 1),
                reason = String(values.get("reason")).trim();
              if (!reason) throw new Error("Informe o motivo da movimentação.");
              void save({
                operation: operation === "receipt" ? "receipt" : "adjustment",
                quantity_delta: operation === "remove" ? -amount : amount,
                reason,
              });
            } catch (cause) {
              setError(errorMessage(cause));
            }
          }}
        >
          <fieldset
            key={command.attempt ? "restored" : "new"}
            className={styles.fields}
            disabled={command.locked || minimumBusy}
          >
            <SelectField
              name="operation"
              label="Movimentação"
              defaultValue={
                draft
                  ? draft.operation === "receipt"
                    ? "receipt"
                    : draft.quantity_delta < 0
                      ? "remove"
                      : "add"
                  : "receipt"
              }
            >
              <option value="receipt">Entrada de mercadoria</option>
              <option value="add">Ajuste: adicionar</option>
              <option value="remove">Ajuste: retirar</option>
            </SelectField>
            <TextField
              name="quantity"
              label="Quantidade"
              type="number"
              min={1}
              max={2147483647}
              step={1}
              required
              defaultValue={draft ? Math.abs(draft.quantity_delta) : 1}
            />
            <div className={styles.full}>
              <TextareaField
                name="reason"
                label="Motivo da movimentação"
                required
                maxLength={500}
                rows={2}
                defaultValue={draft?.reason ?? ""}
              />
            </div>
          </fieldset>
          {error && <Alert tone="error">{error}</Alert>}
          <CommandFeedback command={command} onRetry={() => void save()} />
          <div className={styles.actions}>
            <Button type="submit" disabled={command.locked || minimumBusy} loading={command.busy}>
              Registrar movimentação
            </Button>
          </div>
        </form>
        <details className={styles.panel}>
          <summary>Alterar estoque mínimo</summary>
          <form
            className={styles.form}
            onSubmit={async (event) => {
              event.preventDefault();
              if (busy.current || command.locked || review) return;
              const values = new FormData(event.currentTarget);
              setError("");
              try {
                const amount = quantity(String(values.get("minimum")));
                busy.current = true;
                setMinimumBusy(true);
                const out = await updateMinimum(scope, product, amount);
                setProduct(out);
                onUpdated();
                notify("Estoque mínimo atualizado.");
              } catch (cause) {
                setError(errorMessage(cause));
                setReview(true);
              } finally {
                busy.current = false;
                setMinimumBusy(false);
              }
            }}
          >
            <TextField
              key={product.stock_version}
              name="minimum"
              label="Estoque mínimo"
              type="number"
              min={0}
              max={2147483647}
              step={1}
              required
              defaultValue={product.minimum_quantity}
              disabled={minimumBusy || command.locked || review}
            />
            {review ? (
              <Alert tone="warning">
                Feche esta janela e atualize os produtos antes de alterar o mínimo novamente.
              </Alert>
            ) : (
              <Button
                type="submit"
                variant="secondary"
                disabled={command.locked}
                loading={minimumBusy}
              >
                Salvar estoque mínimo
              </Button>
            )}
          </form>
        </details>
        <section className={styles.stack} aria-label="Histórico de estoque">
          <div className={styles.rowTop}>
            <h3>Movimentações</h3>
            <Button variant="quiet" disabled={history.loading} onClick={history.refresh}>
              Atualizar histórico
            </Button>
          </div>
          <p className={styles.muted}>
            Horários do dispositivo. Cada venda e devolução mantém sua referência original.
          </p>
          {history.error && <Alert tone="error">{history.error}</Alert>}
          <ul className={styles.history}>
            {history.items.map((m) => (
              <li key={m.id}>
                <div className={styles.rowTop}>
                  <strong>{movementNames[m.kind]}</strong>
                  <strong>
                    {m.quantity_delta > 0 ? "+" : ""}
                    {m.quantity_delta} un.
                  </strong>
                </div>
                <p>{m.reason}</p>
                <p className={styles.muted}>{commerceTime(m.created_at)}</p>
                {m.sale_item_id && (
                  <details>
                    <summary>Referências</summary>
                    <p className={styles.muted}>Item da venda: {m.sale_item_id}</p>
                    {m.reverses_movement_id && (
                      <p className={styles.muted}>Saída estornada: {m.reverses_movement_id}</p>
                    )}
                  </details>
                )}
              </li>
            ))}
          </ul>
          {history.loading && <p role="status">Carregando movimentações…</p>}
          {!history.loading && !history.error && !history.items.length && (
            <p>Nenhuma movimentação registrada.</p>
          )}
          {history.next && (
            <Button variant="secondary" disabled={history.loading} onClick={history.more}>
              Carregar mais movimentações
            </Button>
          )}
        </section>
        <div className={styles.actions}>
          <Button variant="secondary" disabled={command.busy || minimumBusy} onClick={onClose}>
            Fechar
          </Button>
        </div>
      </div>
    </Modal>
  );
}
