"use client";
import { useCallback, useEffect, useState } from "react";
import { Plus, ReceiptText, RefreshCw } from "lucide-react";
import { listSales, type SalesFilters } from "@/lib/api/sales";
import { listStaffProfessionals, type StaffProfessional } from "@/lib/api/professionals";
import { dateKey, money } from "@/lib/model";
import { useCommercePage } from "@/hooks/use-commerce-page";
import { useSelectedShopAccess } from "../shop-panel";
import { Alert, Badge, Button, PageHeader, SelectField, TextField } from "../ui";
import { commerceTime, useCommerceScope } from "./shared";
import { SaleForm } from "./sale-form";
import { SaleDetails } from "./sale-details";
import styles from "./commerce.module.css";

export function SalesPage() {
  const [initial, setInitial] = useState<SalesFilters | null>(null);
  useEffect(() => {
    const now = new Date();
    setInitial({
      from: dateKey(new Date(now.getFullYear(), now.getMonth(), 1)),
      through: dateKey(new Date(now.getFullYear(), now.getMonth() + 1, 0)),
      status: "",
    });
  }, []);
  return initial ? <SalesContent initial={initial} /> : <p role="status">Preparando período…</p>;
}
function SalesContent({ initial }: { initial: SalesFilters }) {
  const scope = useCommerceScope(),
    owner = useSelectedShopAccess().role === "owner";
  const [filters, setFilters] = useState(initial),
    [draft, setDraft] = useState(initial),
    [timezone, setTimezone] = useState("");
  const [creating, setCreating] = useState(false),
    [selected, setSelected] = useState<string | null>(null),
    [professionals, setProfessionals] = useState<StaffProfessional[]>([]),
    [teamError, setTeamError] = useState(false);
  const load = useCallback(
    async (cursor: string | undefined, signal: AbortSignal) => {
      const out = await listSales(scope, filters, cursor, signal);
      if (!signal.aborted) setTimezone(out.timezone);
      return out;
    },
    [scope, filters],
  );
  const page = useCommercePage(load);
  useEffect(() => {
    if (!owner) return;
    const c = new AbortController();
    void listStaffProfessionals(scope.slug, c.signal)
      .then((out) => {
        if (!c.signal.aborted) setProfessionals(out);
      })
      .catch(() => {
        if (!c.signal.aborted) setTeamError(true);
      });
    return () => c.abort();
  }, [owner, scope.slug]);
  return (
    <section className={styles.page}>
      <PageHeader
        title="Vendas"
        description={
          owner
            ? "Registre produtos vendidos e consulte o histórico da unidade."
            : "Registre produtos vendidos e consulte suas vendas."
        }
        actions={
          <Button onClick={() => setCreating(true)}>
            <Plus size={18} aria-hidden="true" />
            Registrar venda
          </Button>
        }
      />
      <form
        className={styles.toolbar}
        onSubmit={(event) => {
          event.preventDefault();
          setTimezone("");
          setFilters({ ...draft });
        }}
      >
        <TextField
          label="De"
          type="date"
          min="1900-01-01"
          max="9998-12-31"
          required
          value={draft.from}
          onChange={(e) => setDraft((p) => ({ ...p, from: e.target.value }))}
        />
        <TextField
          label="Até"
          type="date"
          min={draft.from || "1900-01-01"}
          max="9998-12-31"
          required
          value={draft.through}
          onChange={(e) => setDraft((p) => ({ ...p, through: e.target.value }))}
        />
        <SelectField
          label="Situação da venda"
          value={draft.status ?? ""}
          onChange={(e) =>
            setDraft((p) => ({ ...p, status: e.target.value as SalesFilters["status"] }))
          }
        >
          <option value="">Todas</option>
          <option value="recorded">Registradas</option>
          <option value="cancelled">Anuladas</option>
        </SelectField>
        {owner && (
          <SelectField
            label="Filtrar por barbeiro"
            value={draft.professional_id ?? ""}
            onChange={(e) => setDraft((p) => ({ ...p, professional_id: e.target.value }))}
          >
            <option value="">Todos e vendas sem barbeiro</option>
            {professionals.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </SelectField>
        )}
        <Button type="submit" variant="secondary">
          Filtrar vendas
        </Button>
        <Button variant="quiet" disabled={page.loading} onClick={page.refresh}>
          <RefreshCw size={16} aria-hidden="true" />
          Atualizar vendas
        </Button>
      </form>
      <p className={styles.muted}>
        {timezone
          ? "Datas e horários em " + timezone + ". "
          : "As datas serão interpretadas no fuso cadastrado da barbearia. "}
        Consulte até um ano por vez.
        {!owner && " A lista inclui apenas as vendas atribuídas ao seu barbeiro."}
      </p>
      {teamError && (
        <Alert tone="warning">
          Não foi possível carregar o filtro de barbeiros. A consulta geral continua disponível.
        </Alert>
      )}
      {page.error && <Alert tone="error">{page.error}</Alert>}
      <div className={styles.list}>
        {page.items.map((s) => (
          <article key={s.id} className={styles.row}>
            <div>
              <div className={styles.rowTop}>
                <h2>{s.items.map((i) => `${i.quantity}× ${i.product_name}`).join(", ")}</h2>
                <Badge tone={s.status === "recorded" ? "success" : "neutral"}>
                  {s.status === "recorded" ? "Registrada" : "Anulada"}
                </Badge>
              </div>
              <p>{s.professional_name || "Sem barbeiro"}</p>
              <p>{commerceTime(s.sold_at, timezone || undefined)}</p>
            </div>
            <strong className={styles.price}>{money(s.total_cents)}</strong>
            <div className={styles.actions}>
              <Button variant="secondary" onClick={() => setSelected(s.id)}>
                Ver venda
              </Button>
            </div>
          </article>
        ))}
      </div>
      {page.loading && <p role="status">Carregando vendas…</p>}
      {!page.loading && !page.error && !page.items.length && (
        <div className={styles.empty}>
          <ReceiptText size={30} aria-hidden="true" />
          <h2>Nenhuma venda neste período</h2>
          <p className={styles.muted}>
            Altere os filtros ou registre os produtos vendidos para começar seu histórico.
          </p>
        </div>
      )}
      {!!page.items.length && (
        <p className={styles.muted}>
          {page.items.length} registros carregados.{page.next && " A lista ainda é parcial."}{" "}
          Valores de vendas anuladas continuam visíveis para conferência.
        </p>
      )}
      {page.next && (
        <Button variant="secondary" disabled={page.loading} onClick={page.more}>
          Carregar mais vendas
        </Button>
      )}
      {creating && <SaleForm onClose={() => setCreating(false)} onSaved={page.refresh} />}
      {selected && (
        <SaleDetails
          key={selected}
          id={selected}
          timezone={timezone || Intl.DateTimeFormat().resolvedOptions().timeZone}
          onClose={() => setSelected(null)}
          onUpdated={page.refresh}
        />
      )}
    </section>
  );
}
