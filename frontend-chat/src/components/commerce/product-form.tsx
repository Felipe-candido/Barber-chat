"use client";
import { useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  createProduct,
  isProductDraft,
  priceCents,
  quantity,
  updateProduct,
  type Product,
  type ProductDraft,
  type ShopRequest,
} from "@/lib/api/products";
import { useCommerceCommand } from "@/hooks/use-commerce-command";
import { Alert, Button, Modal, TextField, TextareaField } from "../ui";
import { CommandFeedback } from "./shared";
import { useFeedback } from "../feedback-provider";
import styles from "./commerce.module.css";

export function ProductForm({
  scope,
  product,
  onClose,
  onSaved,
}: {
  scope: ShopRequest;
  product?: Product;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { notify } = useFeedback();
  const command = useCommerceCommand(scope, "product:create", isProductDraft, (body, key, signal) =>
    createProduct(scope, body, key, signal),
  );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [reload, setReload] = useState(false);
  const [last, setLast] = useState<ProductDraft | null>(null);
  const running = useRef(false),
    locked = product ? busy || reload : command.locked;
  const draft = product ?? command.attempt ?? last;
  async function recover() {
    const out = await command.submit();
    if (out) {
      notify("Cadastro do produto recuperado.");
      onSaved();
    }
  }
  return (
    <Modal
      open
      title={product ? "Editar produto" : "Adicionar produto"}
      subtitle={
        product
          ? "Editar o cadastro preserva os preços das vendas anteriores."
          : "Informe o produto e a quantidade disponível para começar."
      }
      onClose={() => {
        if (!busy && !command.busy) onClose();
      }}
    >
      <form
        className={styles.form}
        onSubmit={async (event) => {
          event.preventDefault();
          if (running.current || locked) return;
          const values = new FormData(event.currentTarget);
          setError("");
          try {
            const base = {
              name: String(values.get("name")).trim(),
              description: String(values.get("description")).trim(),
              price_cents: priceCents(String(values.get("price"))),
            };
            if (!base.name) throw new Error("Informe o nome do produto.");
            if (product) {
              running.current = true;
              setBusy(true);
              await updateProduct(scope, product.id, {
                ...base,
                active: values.get("active") === "on",
                expected_version: product.version,
              });
              notify("Produto atualizado.");
              onSaved();
            } else {
              const body: ProductDraft = {
                ...base,
                initial_quantity: quantity(String(values.get("initial"))),
                minimum_quantity: quantity(String(values.get("minimum"))),
              };
              setLast(body);
              const out = await command.submit(body);
              if (out) {
                notify(out.replayed ? "Cadastro do produto recuperado." : "Produto cadastrado.");
                onSaved();
              }
            }
          } catch (cause) {
            setError(errorMessage(cause));
            if (
              product &&
              cause instanceof ApiError &&
              (cause.status === 409 || cause.status === 0 || cause.status >= 500)
            )
              setReload(true);
          } finally {
            running.current = false;
            setBusy(false);
          }
        }}
      >
        <fieldset
          key={product?.version ?? (command.attempt ? "restored" : "new")}
          disabled={locked}
          className={styles.fields}
        >
          <div className={styles.full}>
            <TextField
              name="name"
              label="Nome do produto"
              maxLength={100}
              required
              defaultValue={draft?.name ?? ""}
              placeholder="Ex.: Pomada modeladora"
            />
          </div>
          <div className={styles.full}>
            <TextareaField
              name="description"
              label="Descrição (opcional)"
              maxLength={1000}
              rows={2}
              defaultValue={draft?.description ?? ""}
            />
          </div>
          <TextField
            name="price"
            label="Preço de venda (R$)"
            inputMode="decimal"
            required
            defaultValue={draft ? (draft.price_cents / 100).toFixed(2).replace(".", ",") : ""}
            placeholder="39,90"
          />
          {product ? (
            <label className={styles.check}>
              <input name="active" type="checkbox" defaultChecked={product.active} />
              Produto ativo
            </label>
          ) : (
            <>
              <TextField
                name="initial"
                label="Quantidade inicial"
                type="number"
                min={0}
                max={2147483647}
                step={1}
                required
                defaultValue={(command.attempt ?? last)?.initial_quantity ?? 0}
              />
              <TextField
                name="minimum"
                label="Estoque mínimo"
                type="number"
                min={0}
                max={2147483647}
                step={1}
                required
                defaultValue={(command.attempt ?? last)?.minimum_quantity ?? 0}
              />
            </>
          )}
        </fieldset>
        {error && <Alert tone="error">{error}</Alert>}
        {!product && <CommandFeedback command={command} onRetry={() => void recover()} />}
        {reload && (
          <Alert tone="warning">
            Confira a lista atual antes de editar novamente. Uma resposta perdida pode indicar que a
            alteração já foi salva.
          </Alert>
        )}
        <div className={styles.actions}>
          <Button variant="secondary" disabled={busy || command.busy} onClick={onClose}>
            {reload ? "Conferir lista atual" : "Fechar"}
          </Button>
          {!reload && (
            <Button type="submit" loading={busy || command.busy} disabled={locked}>
              {product ? "Salvar produto" : "Adicionar produto"}
            </Button>
          )}
        </div>
      </form>
    </Modal>
  );
}
