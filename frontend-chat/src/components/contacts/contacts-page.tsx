"use client";

import { useCallback, useMemo, useState } from "react";
import { Plus, RefreshCw, Search, UsersRound } from "lucide-react";
import {
  contactPhone,
  listContacts,
  type ContactFilters,
  type ContactScope,
} from "@/lib/api/contacts";
import { useCommercePage as usePaginatedList } from "@/hooks/use-commerce-page";
import { useSelectedShop } from "../shop-panel";
import { useAdminAccess } from "../admin-access-provider";
import { useFeedback } from "../feedback-provider";
import { Alert, Button, PageHeader, TextField } from "../ui";
import { ContactForm } from "./contact-form";
import styles from "./contacts.module.css";

export function ContactsPage() {
  const shop = useSelectedShop(),
    { state } = useAdminAccess(),
    { notify } = useFeedback();
  const userID = state.status === "ready" ? state.identity.user_id : "";
  const scope = useMemo<ContactScope>(
    () => ({ slug: shop.slug, shopID: shop.shop_id, userID }),
    [shop.slug, shop.shop_id, userID],
  );
  const [filters, setFilters] = useState<ContactFilters>({}),
    [editing, setEditing] = useState(false),
    [search, setSearch] = useState("");
  const load = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      listContacts(scope, { ...filters, cursor }, signal),
    [scope, filters],
  );
  const page = usePaginatedList(load);
  const filtered = !!(filters.name || filters.phone);
  return (
    <section className={styles.page}>
      <PageHeader
        title="Contatos"
        description="Os clientes dos agendamentos e os contatos cadastrados pela equipe, em uma só lista."
        actions={
          <Button disabled={!!page.error} onClick={() => setEditing(true)}>
            <Plus size={18} aria-hidden="true" />
            Novo contato
          </Button>
        }
      />
      <form
        className={styles.toolbar}
        aria-label="Buscar contatos"
        onSubmit={(e) => {
          e.preventDefault();
          const q = String(new FormData(e.currentTarget).get("search") ?? "").trim();
          setFilters(
            q && /^[+\d\s().-]+$/.test(q)
              ? { phone: q, phone_country: q.startsWith("+") ? undefined : "BR" }
              : { name: q },
          );
        }}
      >
        <TextField
          label="Buscar contato"
          name="search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          type="search"
          maxLength={201}
          placeholder="Nome ou telefone completo"
          hint="Para buscar por telefone, informe o número completo com DDD."
        />
        <Button type="submit" variant="secondary">
          <Search size={16} aria-hidden="true" />
          Buscar
        </Button>
        <Button variant="quiet" disabled={page.loading} onClick={page.refresh}>
          <RefreshCw size={16} aria-hidden="true" />
          Atualizar lista
        </Button>
      </form>
      {page.error && <Alert tone="error">{page.error}</Alert>}
      {page.loading && <p role="status">Carregando contatos…</p>}
      {!page.error && page.items.length > 0 && (
        <div className={styles.list} role="list" aria-label="Contatos da barbearia">
          {page.items.map((c) => (
            <article key={c.id} className={styles.row} role="listitem">
              <div className={styles.person}>
                <span className={styles.avatar} aria-hidden="true">
                  <UsersRound size={20} />
                </span>
                <h2>{c.name}</h2>
              </div>
              <a href={"tel:" + c.phone} aria-label={"Ligar para " + c.name}>
                {contactPhone(c.phone)}
              </a>
              <p>Cadastrado em {new Date(c.created_at).toLocaleDateString("pt-BR")}</p>
            </article>
          ))}
        </div>
      )}
      {!page.loading && !page.error && page.items.length === 0 && (
        <div className={styles.empty}>
          <UsersRound size={28} aria-hidden="true" />
          <h2>{filtered ? "Nenhum contato encontrado" : "Sua lista de contatos começa aqui"}</h2>
          <p>
            {filtered
              ? "Confira o nome ou o telefone completo e faça outra busca."
              : "Clientes aparecem aqui depois do agendamento. Você também pode cadastrar um contato manualmente."}
          </p>
        </div>
      )}
      {page.error && (
        <Button variant="secondary" disabled={page.loading} onClick={page.refresh}>
          Tentar novamente
        </Button>
      )}
      {!page.error && page.next && (
        <Button variant="secondary" loading={page.loading} onClick={page.more}>
          Carregar mais contatos
        </Button>
      )}
      {editing && (
        <ContactForm
          key={scope.shopID + ":" + scope.userID}
          scope={scope}
          onClose={() => setEditing(false)}
          onAccessDenied={() => {
            setEditing(false);
            page.refresh();
          }}
          onSaved={(contact, created) => {
            setEditing(false);
            notify(created ? "Contato cadastrado." : "Contato encontrado. Confira a lista.");
            setSearch(created ? "" : contact.phone);
            setFilters(created ? {} : { phone: contact.phone });
          }}
        />
      )}
    </section>
  );
}
