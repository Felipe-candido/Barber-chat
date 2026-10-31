"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowUpRight, RefreshCw, Users } from "lucide-react";
import { useSelectedShop, useSelectedShopAccess } from "./shop-panel";
import { useServices } from "@/hooks/use-services";
import { errorMessage } from "@/lib/api/client";
import { adminShopPath, publicBookingPath } from "@/lib/api/config";
import { type StaffProfessional, listStaffProfessionals } from "@/lib/api/professionals";
import { Alert, Badge, Button, PageHeader, buttonClasses } from "./ui";
import { ProfessionalForm } from "./professionals/professional-form";
import styles from "./admin/management.module.css";

export function ProfessionalsPage() {
  const access = useSelectedShopAccess();
  if (access.role !== "owner")
    return (
      <section className={styles.page}>
        <PageHeader
          title="Acesso restrito ao proprietário"
          description="Use a agenda para consultar e criar seus agendamentos."
        />
      </section>
    );
  return <OwnerProfessionalsPage />;
}

function OwnerProfessionalsPage() {
  const shop = useSelectedShop();
  const catalog = useServices(shop.slug, "admin");
  const [items, setItems] = useState<StaffProfessional[]>([]);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState("");
  const activeRequest = useRef<AbortController | null>(null);
  const reload = useCallback(async () => {
    activeRequest.current?.abort();
    const controller = new AbortController();
    activeRequest.current = controller;
    setLoading(true);
    setListError("");
    try {
      const result = await listStaffProfessionals(shop.slug, controller.signal);
      if (!controller.signal.aborted) setItems(result);
    } catch (cause) {
      if (!controller.signal.aborted) setListError(errorMessage(cause));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [shop.slug]);
  useEffect(() => {
    void reload();
    return () => activeRequest.current?.abort();
  }, [reload]);

  return (
    <section className={styles.page}>
      <PageHeader
        title="Barbeiros"
        description={`Equipe, serviços e expediente de ${shop.name}.`}
        actions={
          <Link
            href={publicBookingPath(shop.slug)}
            className={buttonClasses("secondary")}
            target="_blank"
            rel="noopener noreferrer"
          >
            Testar agendamento no chat
            <ArrowUpRight size={16} aria-hidden="true" />
          </Link>
        }
      />
      <p className={styles.hint}>
        O cadastro disponibiliza o barbeiro para agendamentos, mas não cria um login.{" "}
        <Link href={adminShopPath(shop.slug) + "/contas"}>
          Criar conta de acesso para um barbeiro
        </Link>
        .
      </p>
      <div className={styles.layout}>
        <section className={styles.panel} aria-label="Profissionais cadastrados">
          <div className={styles.sectionHeader}>
            <h2>Equipe cadastrada</h2>
            <Button variant="quiet" disabled={loading} onClick={() => void reload()}>
              <RefreshCw size={16} aria-hidden="true" />
              Atualizar barbeiros
            </Button>
          </div>
          {loading ? (
            <p className={styles.loading} role="status">
              Carregando barbeiros…
            </p>
          ) : listError ? (
            <Alert tone="error">{listError}</Alert>
          ) : !items.length ? (
            <div className={styles.empty}>
              <Users size={28} aria-hidden="true" />
              <h3>Sua equipe começa aqui</h3>
              <p>
                Nenhum barbeiro cadastrado nesta unidade. Preencha o cadastro para disponibilizar os
                primeiros horários.
              </p>
            </div>
          ) : (
            <ul className={styles.list}>
              {items.map((item) => (
                <li key={item.id}>
                  <div className={styles.itemHeader}>
                    <strong>{item.name}</strong>
                    <Badge tone={item.active ? "success" : "neutral"}>
                      {item.active ? "Ativo" : "Inativo"}
                    </Badge>
                  </div>
                  <div className={styles.tags}>
                    {item.service_ids.length ? (
                      item.service_ids.map((id) => (
                        <span className={styles.tag} key={id}>
                          {catalog.services.find((service) => service.id === id)?.name ??
                            "Serviço cadastrado"}
                        </span>
                      ))
                    ) : (
                      <p className={styles.hint}>Sem serviços vinculados</p>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>
        <ProfessionalForm
          slug={shop.slug}
          catalog={catalog}
          checking={loading}
          onCreated={async (created) => {
            setItems((current) => [...current, created]);
            await reload();
          }}
        />
      </div>
    </section>
  );
}
