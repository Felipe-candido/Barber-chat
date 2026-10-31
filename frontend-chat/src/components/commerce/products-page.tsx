"use client";
import { useCallback, useState } from "react";
import { Package, Plus, RefreshCw } from "lucide-react";
import { listProducts, type Product } from "@/lib/api/products";
import { money } from "@/lib/model";
import { useCommercePage } from "@/hooks/use-commerce-page";
import { useSelectedShopAccess } from "../shop-panel";
import { Alert, Badge, Button, PageHeader, TextField } from "../ui";
import { useCommerceScope } from "./shared";
import { ProductForm } from "./product-form";
import { StockPanel } from "./stock-panel";
import styles from "./commerce.module.css";

export function ProductsPage() {
  const scope = useCommerceScope(),
    owner = useSelectedShopAccess().role === "owner";
  const [query, setQuery] = useState({ search: "", includeInactive: false });
  const load = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      listProducts(scope, { ...query, cursor }, signal),
    [scope, query],
  );
  const page = useCommercePage(load);
  const [editing, setEditing] = useState<Product | "new" | null>(null),
    [stock, setStock] = useState<Product | null>(null);
  function close() {
    setEditing(null);
    page.refresh();
  }
  return (
    <section className={styles.page}>
      <PageHeader
        title="Produtos"
        description={
          owner
            ? "Cuide do catálogo, dos preços e do estoque da barbearia."
            : "Consulte os produtos disponíveis para venda."
        }
        actions={
          owner && (
            <Button onClick={() => setEditing("new")}>
              <Plus size={18} aria-hidden="true" />
              Adicionar produto
            </Button>
          )
        }
      />
      <form
        className={styles.toolbar}
        onSubmit={(event) => {
          event.preventDefault();
          const values = new FormData(event.currentTarget);
          setQuery({
            search: String(values.get("search")).trim(),
            includeInactive: owner && values.get("inactive") === "on",
          });
        }}
      >
        <TextField
          name="search"
          label="Buscar produto"
          type="search"
          maxLength={100}
          placeholder="Nome do produto"
        />
        {owner && (
          <label className={styles.check}>
            <input name="inactive" type="checkbox" />
            Incluir inativos
          </label>
        )}
        <Button type="submit" variant="secondary">
          Buscar
        </Button>
        <Button variant="quiet" disabled={page.loading} onClick={page.refresh}>
          <RefreshCw size={16} aria-hidden="true" />
          Atualizar lista
        </Button>
      </form>
      {page.error && <Alert tone="error">{page.error}</Alert>}
      <div className={styles.list}>
        {page.items.map((p) => (
          <article key={p.id} className={styles.row}>
            <div>
              <div className={styles.rowTop}>
                <h2>{p.name}</h2>
                <Badge tone={p.active ? "success" : "neutral"}>
                  {p.active ? "Ativo" : "Inativo"}
                </Badge>
              </div>
              {p.description && <p>{p.description}</p>}
              <p>
                {p.quantity_on_hand} unidades disponíveis{" "}
                <span className={styles.stockHint}>· mínimo {p.minimum_quantity}</span>
              </p>
              {p.low_stock && <Badge tone="warning">Estoque baixo</Badge>}
            </div>
            <strong className={styles.price}>{money(p.price_cents)}</strong>
            {owner ? (
              <div className={styles.actions}>
                <Button variant="secondary" onClick={() => setStock(p)}>
                  Estoque
                </Button>
                <Button variant="quiet" onClick={() => setEditing(p)}>
                  Editar
                </Button>
              </div>
            ) : (
              <span className={styles.muted}>Estoque gerenciado pelo proprietário</span>
            )}
          </article>
        ))}
      </div>
      {page.loading && <p role="status">Carregando produtos…</p>}
      {!page.loading && !page.error && !page.items.length && (
        <div className={styles.empty}>
          <Package size={30} aria-hidden="true" />
          <h2>Nenhum produto encontrado</h2>
          <p className={styles.muted}>
            {owner
              ? "Adicione um produto ou altere a busca para consultar seu catálogo."
              : "O proprietário ainda não disponibilizou produtos para esta busca."}
          </p>
        </div>
      )}
      {page.next && (
        <Button variant="secondary" disabled={page.loading} onClick={page.more}>
          Carregar mais produtos
        </Button>
      )}
      {owner && editing && (
        <ProductForm
          scope={scope}
          product={editing === "new" ? undefined : editing}
          onClose={close}
          onSaved={close}
        />
      )}
      {owner && stock && (
        <StockPanel
          scope={scope}
          initial={stock}
          onClose={() => {
            setStock(null);
            page.refresh();
          }}
          onUpdated={page.refresh}
        />
      )}
    </section>
  );
}
