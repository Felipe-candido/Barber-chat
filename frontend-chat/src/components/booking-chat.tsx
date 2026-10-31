"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronRight, Clock3, LoaderCircle } from "lucide-react";
import { Alert, Button } from "./ui";
import { useServices } from "@/hooks/use-services";
import { money } from "@/lib/model";
import { BookingForm } from "./booking-form";
import { AppointmentLookup } from "./booking/appointment-lookup";
import { BookingProgress, type BookingStage } from "./booking/progress";
import { PublicShopHeader } from "./booking/public-shop-header";
import styles from "./booking/booking.module.css";

export function BookingChat({ slug }: { slug: string }) {
  const { services, loading, error, reload } = useServices(slug);
  const [selectedId, setSelectedId] = useState("");
  const [mode, setMode] = useState<"book" | "lookup">("book");
  const [stage, setStage] = useState<BookingStage>("service");
  const [shopName, setShopName] = useState("");
  const [locked, setLocked] = useState(false);
  const serviceButtons = useRef(new Map<string, HTMLButtonElement>());
  const lastService = useRef("");
  useEffect(() => {
    if (!selectedId && lastService.current)
      serviceButtons.current.get(lastService.current)?.focus();
  }, [selectedId]);
  const selected = services.find((service) => service.id === selectedId);
  return (
    <div className={styles.page} data-in-flow={!!selected || mode === "lookup" || undefined}>
      <PublicShopHeader name={shopName} slug={slug} />
      <main className={styles.layout}>
        <section className={styles.panel} aria-label="Agendamento público">
          <div className={styles.intro}>
            <h1>
              {mode === "lookup"
                ? "Consulte seu agendamento"
                : stage === "complete" && selected
                  ? "Seu agendamento"
                  : "Reserve seu horário"}
            </h1>
            <p className={styles.muted}>
              {mode === "lookup"
                ? "Use o código recebido ao finalizar a reserva."
                : stage === "complete" && selected
                  ? "Confira a reserva e guarde o código para consultá-la."
                  : "Escolha o serviço e encontre um horário para você."}
            </p>
          </div>
          <div className={styles.mode} aria-label="O que você deseja fazer?">
            <Button
              variant="secondary"
              disabled={locked}
              aria-pressed={mode === "book"}
              onClick={() => {
                if (!locked) setMode("book");
              }}
            >
              Agendar horário
            </Button>
            <Button
              variant="secondary"
              disabled={locked}
              aria-pressed={mode === "lookup"}
              onClick={() => {
                if (!locked) setMode("lookup");
              }}
            >
              Já tenho um código
            </Button>
          </div>
          {mode === "book" && <BookingProgress stage={selected ? stage : "service"} />}
          {mode === "lookup" ? (
            <AppointmentLookup slug={slug} onShopName={setShopName} onLockedChange={setLocked} />
          ) : loading ? (
            <div className={styles.loading} role="status">
              <LoaderCircle size={22} aria-hidden="true" />
              Carregando serviços…
            </div>
          ) : error ? (
            <div className={styles.stack}>
              <Alert tone="error">{error}</Alert>
              <Button variant="secondary" onClick={() => void reload()}>
                Tentar novamente
              </Button>
            </div>
          ) : selected ? (
            <BookingForm
              key={selected.id}
              slug={slug}
              service={selected}
              onBack={() => {
                if (!locked) {
                  setSelectedId("");
                  setStage("service");
                }
              }}
              onLockedChange={setLocked}
              onStageChange={setStage}
              onShopName={setShopName}
            />
          ) : services.length ? (
            <ul className={styles.serviceList} aria-label="Serviços disponíveis">
              {services.map((service) => (
                <li key={service.id}>
                  <button
                    ref={(element) => {
                      if (element) serviceButtons.current.set(service.id, element);
                      else serviceButtons.current.delete(service.id);
                    }}
                    className={styles.serviceButton}
                    type="button"
                    onClick={() => {
                      lastService.current = service.id;
                      setSelectedId(service.id);
                      setStage("schedule");
                    }}
                  >
                    <div>
                      <strong>{service.name}</strong>
                      {service.description && <p className={styles.muted}>{service.description}</p>}
                      <div className={styles.serviceMeta}>
                        <span>
                          <Clock3 size={15} aria-hidden="true" />
                          {service.duration_minutes} min
                        </span>
                        <b>{money(service.price_cents)}</b>
                      </div>
                    </div>
                    <ChevronRight size={18} aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <div className={styles.empty}>
              <h2>Nenhum serviço disponível</h2>
              <p className={styles.hint}>
                Esta barbearia ainda não tem serviços ativos para agendamento. Consulte o
                estabelecimento ou tente novamente mais tarde.
              </p>
              <Button variant="secondary" onClick={() => void reload()}>
                Atualizar serviços
              </Button>
            </div>
          )}
        </section>
        <aside className={styles.aside} aria-label="Sobre o agendamento">
          <h2>Seu próximo atendimento</h2>
          <ol>
            <li>
              <strong>Escolha com calma</strong>Veja os serviços, os profissionais e os horários
              disponíveis.
            </li>
            <li>
              <strong>Confira antes de reservar</strong>O horário é reservado somente após a
              confirmação.
            </li>
            <li>
              <strong>Guarde seu código</strong>Use-o neste mesmo link para consultar ou cancelar.
            </li>
          </ol>
          <p className={styles.hint}>
            Não é preciso criar uma conta. Nesta etapa, o comprovante é exibido aqui, sem envio pelo
            WhatsApp.
          </p>
        </aside>
      </main>
      <footer className={styles.footer}>Agendamento por Barber-chat</footer>
    </div>
  );
}
