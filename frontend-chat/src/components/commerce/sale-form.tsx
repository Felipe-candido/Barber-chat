"use client";
import { useCallback, useEffect, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import { listProducts, quantity, type Product } from "@/lib/api/products";
import { isSaleInput, registerSale, type Sale, type SaleInput } from "@/lib/api/sales";
import { listStaffProfessionals, type StaffProfessional } from "@/lib/api/professionals";
import type { AdminAppointment } from "@/lib/api/agenda";
import { money } from "@/lib/model";
import { useCommercePage } from "@/hooks/use-commerce-page";
import { useCommerceCommand } from "@/hooks/use-commerce-command";
import { useSelectedShopAccess } from "../shop-panel";
import { Alert, Button, Modal, SelectField, TextField } from "../ui";
import { useCommerceScope, CommandFeedback } from "./shared";
import { SaleReceipt } from "./sale-receipt";
import styles from "./commerce.module.css";

type BasketLine = { product: Product; amount: string };
export function SaleForm({
  appointment,
  onClose,
  onSaved,
}: {
  appointment?: AdminAppointment;
  onClose: () => void;
  onSaved?: () => void;
}) {
  const scope = useCommerceScope(),
    access = useSelectedShopAccess(),
    owner = access.role === "owner";
  const [search, setSearch] = useState(""),
    [basket, setBasket] = useState<BasketLine[]>([]),
    [professionals, setProfessionals] = useState<StaffProfessional[]>([]),
    [professionalError, setProfessionalError] = useState("");
  const [professional, setProfessional] = useState(appointment?.professional_id ?? ""),
    [review, setReview] = useState<SaleInput | null>(null),
    [error, setError] = useState(""),
    [receipt, setReceipt] = useState<Sale | null>(null);
  const load = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      listProducts(scope, { search, cursor }, signal),
    [scope, search],
  );
  const catalog = useCommercePage(load);
  const command = useCommerceCommand(scope, "sale:create", isSaleInput, (body, key, signal) =>
    registerSale(scope, body, key, signal),
  );
  useEffect(() => {
    if (!owner || appointment) return;
    const c = new AbortController();
    void listStaffProfessionals(scope.slug, c.signal)
      .then((value) => {
        if (!c.signal.aborted) setProfessionals(value.filter((p) => p.active));
      })
      .catch((cause) => {
        if (!c.signal.aborted) setProfessionalError(errorMessage(cause));
      });
    return () => c.abort();
  }, [owner, appointment, scope.slug]);
  const effective = command.attempt ?? review;
  const safeTotal = basket.reduce((sum, line) => {
    const amount = Number(line.amount),
      price = line.product.price_cents;
    if (
      !Number.isSafeInteger(amount) ||
      amount < 1 ||
      price > Math.floor(Number.MAX_SAFE_INTEGER / amount) ||
      sum > Number.MAX_SAFE_INTEGER - price * amount
    )
      return NaN;
    return sum + price * amount;
  }, 0);
  async function confirm() {
    const saved = await command.submit(effective ?? undefined);
    if (saved) {
      setReceipt(saved);
      onSaved?.();
    }
  }
  return (
    <Modal
      open
      title={
        receipt ? (receipt.replayed ? "Venda recuperada" : "Venda registrada") : "Registrar venda"
      }
      subtitle={
        appointment
          ? "Produtos vinculados ao atendimento selecionado."
          : "Selecione os produtos, revise os valores e confirme."
      }
      wide
      onClose={() => {
        if (!command.busy) onClose();
      }}
    >
      {receipt ? (
        <div className={styles.stack}>
          <SaleReceipt sale={receipt} timezone={appointment?.shop_timezone} />
          <div className={styles.actions}>
            <Button onClick={onClose}>Fechar comprovante</Button>
          </div>
        </div>
      ) : (
        <div className={styles.stack}>
          {appointment && !command.attempt && (
            <Alert>
              Agendamento de {appointment.customer_name}, com {appointment.professional_name}.
              Registrar produtos não conclui o atendimento.
            </Alert>
          )}
          <CommandFeedback command={command} onRetry={() => void confirm()} />
          {error && <Alert tone="error">{error}</Alert>}
          {effective ? (
            <>
              <section className={styles.receipt} aria-label="Revisão da venda">
                <h3>
                  {command.attempt ? "Dados da tentativa original" : "Revise antes de confirmar"}
                </h3>
                <p>
                  {effective.appointment_id
                    ? "Vinculada ao agendamento " + effective.appointment_id
                    : "Venda avulsa"}
                </p>
                <p className={styles.muted}>
                  {owner
                    ? effective.professional_id
                      ? (professionals.find((p) => p.id === effective.professional_id)?.name ??
                        (effective.professional_id === appointment?.professional_id
                          ? appointment.professional_name
                          : "Barbeiro selecionado na tentativa"))
                      : "Sem barbeiro"
                    : "Venda atribuída ao seu barbeiro"}
                </p>
                <ul className={styles.history}>
                  {effective.items.map((item) => (
                    <li key={item.product_id}>
                      <div className={styles.rowTop}>
                        <strong>
                          {basket.find((line) => line.product.id === item.product_id)?.product
                            .name ??
                            catalog.items.find((p) => p.id === item.product_id)?.name ??
                            "Produto " + item.product_id}
                        </strong>
                        <strong>{money(item.quantity * item.expected_unit_price_cents)}</strong>
                      </div>
                      <p>
                        {item.quantity} un. × {money(item.expected_unit_price_cents)}
                      </p>
                    </li>
                  ))}
                </ul>
                <div className={styles.total}>
                  <span>Total revisado</span>
                  <strong>
                    {money(
                      effective.items.reduce(
                        (sum, i) => sum + i.quantity * i.expected_unit_price_cents,
                        0,
                      ),
                    )}
                  </strong>
                </div>
                <p className={styles.muted}>
                  O estoque e os preços serão conferidos ao confirmar. Nenhum pagamento será cobrado
                  por esta ação.
                </p>
              </section>
              <div className={styles.actions}>
                {(command.attempt || command.corrupt) && (
                  <Button variant="secondary" disabled={command.busy} onClick={onClose}>
                    Fechar
                  </Button>
                )}
                {!command.attempt && !command.corrupt && (
                  <>
                    <Button
                      variant="secondary"
                      disabled={command.locked}
                      onClick={() => setReview(null)}
                    >
                      Voltar à seleção
                    </Button>
                    <Button
                      disabled={command.locked}
                      loading={command.busy}
                      onClick={() => void confirm()}
                    >
                      Confirmar venda
                    </Button>
                  </>
                )}
              </div>
              {!command.attempt &&
                ["sale_price_changed", "sale_product_unavailable", "insufficient_stock"].includes(
                  command.errorCode,
                ) && (
                  <Button
                    variant="secondary"
                    onClick={() => {
                      setReview(null);
                      setBasket([]);
                      catalog.refresh();
                    }}
                  >
                    Limpar seleção e atualizar produtos
                  </Button>
                )}
            </>
          ) : (
            <>
              {owner && !appointment && (
                <>
                  <SelectField
                    label="Barbeiro responsável"
                    value={professional}
                    disabled={command.locked}
                    onChange={(e) => setProfessional(e.target.value)}
                  >
                    <option value="">Sem barbeiro</option>
                    {professionals.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </SelectField>
                  {professionalError && (
                    <Alert tone="warning">
                      Não foi possível consultar a equipe. Você pode registrar uma venda sem
                      barbeiro ou reabrir a janela para tentar novamente.
                    </Alert>
                  )}
                </>
              )}
              {!owner && !appointment && (
                <p className={styles.muted}>Esta venda será atribuída ao seu barbeiro.</p>
              )}
              <form
                className={styles.toolbar}
                onSubmit={(event) => {
                  event.preventDefault();
                  const data = new FormData(event.currentTarget);
                  setSearch(String(data.get("search")).trim());
                }}
              >
                <TextField
                  name="search"
                  label="Buscar produto para venda"
                  type="search"
                  maxLength={100}
                />
                <Button type="submit" variant="secondary" disabled={command.locked}>
                  Buscar produtos
                </Button>
              </form>
              {catalog.error && <Alert tone="error">{catalog.error}</Alert>}
              <div className={styles.list}>
                {catalog.items.map((p) => (
                  <div key={p.id} className={styles.row}>
                    <div>
                      <strong>{p.name}</strong>
                      <p>{p.quantity_on_hand} un. disponíveis</p>
                    </div>
                    <strong>{money(p.price_cents)}</strong>
                    <Button
                      variant="secondary"
                      disabled={
                        command.locked ||
                        p.quantity_on_hand < 1 ||
                        basket.some((i) => i.product.id === p.id) ||
                        basket.length >= 50
                      }
                      aria-label={"Adicionar " + p.name}
                      onClick={() => setBasket((prior) => [...prior, { product: p, amount: "1" }])}
                    >
                      Adicionar
                    </Button>
                  </div>
                ))}
              </div>
              {catalog.loading && <p role="status">Carregando catálogo…</p>}
              {!catalog.loading && !catalog.error && !catalog.items.length && (
                <p>
                  Nenhum produto ativo encontrado. Peça ao proprietário para conferir o catálogo.
                </p>
              )}
              {catalog.error && (
                <Button variant="secondary" onClick={catalog.refresh} disabled={catalog.loading}>
                  Tentar carregar produtos
                </Button>
              )}
              {catalog.next && (
                <Button variant="secondary" disabled={catalog.loading} onClick={catalog.more}>
                  Carregar mais produtos
                </Button>
              )}
              <section className={styles.basket} aria-label="Produtos selecionados">
                <h3>Produtos selecionados</h3>
                {!basket.length && (
                  <p className={styles.muted}>
                    Adicione pelo menos um produto para revisar a venda.
                  </p>
                )}
                {basket.map((line) => (
                  <div key={line.product.id} className={styles.basketItem}>
                    <div>
                      <strong>{line.product.name}</strong>
                      <p className={styles.muted}>{money(line.product.price_cents)} por unidade</p>
                    </div>
                    <TextField
                      label={"Quantidade de " + line.product.name}
                      type="number"
                      min={1}
                      max={2147483647}
                      step={1}
                      required
                      value={line.amount}
                      disabled={command.locked}
                      onChange={(e) =>
                        setBasket((prior) =>
                          prior.map((i) =>
                            i.product.id === line.product.id ? { ...i, amount: e.target.value } : i,
                          ),
                        )
                      }
                    />
                    <Button
                      variant="quiet"
                      disabled={command.locked}
                      aria-label={"Remover " + line.product.name}
                      onClick={() =>
                        setBasket((prior) => prior.filter((i) => i.product.id !== line.product.id))
                      }
                    >
                      Remover
                    </Button>
                  </div>
                ))}
                <div className={styles.total}>
                  <span>Total previsto</span>
                  <strong>
                    {Number.isSafeInteger(safeTotal) ? money(safeTotal) : "Revise as quantidades"}
                  </strong>
                </div>
              </section>
              <div className={styles.actions}>
                {(command.attempt || command.corrupt) && (
                  <Button variant="secondary" disabled={command.busy} onClick={onClose}>
                    Fechar
                  </Button>
                )}
                <Button
                  disabled={!basket.length || command.locked}
                  onClick={() => {
                    setError("");
                    try {
                      if (!Number.isSafeInteger(safeTotal) || safeTotal < 1)
                        throw new Error("O total está fora do limite. Revise as quantidades.");
                      const items = basket.map((line) => {
                        const amount = quantity(line.amount, 1);
                        if (amount > line.product.quantity_on_hand)
                          throw new Error(
                            "O saldo consultado de “" +
                              line.product.name +
                              "” é insuficiente. Atualize os produtos.",
                          );
                        return {
                          product_id: line.product.id,
                          quantity: amount,
                          expected_product_version: line.product.version,
                          expected_unit_price_cents: line.product.price_cents,
                        };
                      });
                      setReview({
                        items,
                        ...(professional ? { professional_id: professional } : {}),
                        ...(appointment ? { appointment_id: appointment.id } : {}),
                      });
                    } catch (cause) {
                      setError(errorMessage(cause));
                    }
                  }}
                >
                  Revisar venda
                </Button>
              </div>
            </>
          )}
        </div>
      )}
    </Modal>
  );
}
