"use client";

import { useState } from "react";
import {
  ArrowUpRight,
  Clock3,
  LoaderCircle,
  Plus,
  RefreshCw,
  Scissors,
  Search,
} from "lucide-react";
import Link from "next/link";
import { publicBookingPath } from "@/lib/api/config";
import { useServices } from "@/hooks/use-services";
import { money } from "@/lib/model";
import { useSelectedShop, useSelectedShopAccess } from "./shop-panel";
import { Alert, Badge, Button, PageHeader, TextField, buttonClasses } from "./ui";
import { ServiceForm } from "./services/service-form";
import styles from "./admin/management.module.css";

export function ServicesPage() {
  const shop = useSelectedShop();
  const owner = useSelectedShopAccess().role === "owner";
  const { services, loading, error, errorStatus, reload } = useServices(shop.slug, "admin");
  const [query, setQuery] = useState("");
  const [creating, setCreating] = useState(false);
  const filtered = services.filter((service) =>
    service.name.toLocaleLowerCase("pt-BR").includes(query.trim().toLocaleLowerCase("pt-BR")),
  );
  return (
    <section className={styles.page}>
      <PageHeader
        title="Serviços"
        description={`Catálogo de serviços ativos de ${shop.name}.`}
        actions={
          owner && (
            <Button
              disabled={errorStatus === 401 || errorStatus === 403}
              onClick={() => setCreating(true)}
            >
              <Plus size={18} aria-hidden="true" />
              Adicionar serviço
            </Button>
          )
        }
      />
      <div className={styles.toolbar}>
        <TextField
          label="Buscar serviço"
          type="search"
          placeholder="Buscar pelo nome"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          endAdornment={<Search size={18} aria-hidden="true" />}
        />
        <Button variant="secondary" disabled={loading} onClick={() => void reload()}>
          <RefreshCw size={16} aria-hidden="true" />
          Atualizar lista
        </Button>
        <Link
          href={publicBookingPath(shop.slug)}
          className={buttonClasses("quiet")}
          target="_blank"
          rel="noopener noreferrer"
        >
          Ver como cliente
          <ArrowUpRight size={16} aria-hidden="true" />
        </Link>
      </div>
      {loading ? (
        <div className={styles.loading} role="status">
          <LoaderCircle size={22} aria-hidden="true" />
          Carregando serviços…
        </div>
      ) : error ? (
        <div className={styles.feedback}>
          <Alert tone="error">
            <strong>Não foi possível carregar o catálogo.</strong>
            <p>{error}</p>
          </Alert>
          <div className={styles.actions}>
            <Button variant="secondary" onClick={() => void reload()}>
              Tentar novamente
            </Button>
            {(errorStatus === 401 || errorStatus === 403) && (
              <Link
                className={buttonClasses("secondary")}
                href={errorStatus === 401 ? "/login" : "/admin"}
              >
                {errorStatus === 401 ? "Entrar novamente" : "Escolher outra barbearia"}
              </Link>
            )}
          </div>
        </div>
      ) : (
        <>
          <p className={styles.muted}>
            {filtered.length} {filtered.length === 1 ? "serviço ativo" : "serviços ativos"}
            {query.trim() ? ` para “${query.trim()}”` : ""}
          </p>
          {filtered.length ? (
            <div className={styles.grid}>
              {filtered.map((service) => (
                <article className={styles.service} key={service.id}>
                  <div className={styles.itemHeader}>
                    <div className={styles.serviceName}>
                      <Scissors size={20} aria-hidden="true" />
                      <h2>{service.name}</h2>
                    </div>
                    <Badge tone={service.active ? "success" : "neutral"}>
                      {service.active ? "Ativo" : "Inativo"}
                    </Badge>
                  </div>
                  <p>{service.description || "Sem descrição."}</p>
                  <div className={styles.serviceData}>
                    <strong>{money(service.price_cents)}</strong>
                    <span>
                      <Clock3 size={16} aria-hidden="true" />
                      {service.duration_minutes} min
                    </span>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <div className={styles.empty}>
              <Scissors size={28} aria-hidden="true" />
              <h2>{query.trim() ? "Nenhum resultado" : "Nenhum serviço ativo"}</h2>
              <p>
                {query.trim()
                  ? "Tente buscar por outro nome ou limpe a busca."
                  : owner
                    ? "Adicione o primeiro serviço para começar a montar sua equipe e receber agendamentos."
                    : "O proprietário ainda não cadastrou serviços ativos nesta barbearia."}
              </p>
              {query.trim() && (
                <Button variant="secondary" onClick={() => setQuery("")}>
                  Limpar busca
                </Button>
              )}
            </div>
          )}
          <p className={styles.hint}>
            Este catálogo exibe apenas serviços ativos. Edição e exclusão ainda não estão
            disponíveis.{!owner && " Somente o proprietário pode cadastrar serviços."}
          </p>
        </>
      )}
      {owner && creating && (
        <ServiceForm
          slug={shop.slug}
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false);
            void reload();
          }}
        />
      )}
    </section>
  );
}
