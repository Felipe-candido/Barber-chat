"use client";
import { useState } from "react";
import Image from "next/image";
import {
  ArrowLeft,
  CalendarDays,
  CheckCheck,
  ChevronRight,
  LoaderCircle,
  Scissors,
} from "lucide-react";
import { Brand } from "./ui";
import { configuredShopSlug } from "@/lib/api/config";
import { useServices } from "@/hooks/use-services";
import { money } from "@/lib/model";
export function BookingChat({ slug = configuredShopSlug }: { slug?: string }) {
  const { services, loading, error, reload } = useServices(slug);
  const [selectedId, setSelectedId] = useState("");
  const selected = services.find((s) => s.id === selectedId);
  return (
    <div className="booking-page">
      <header className="booking-nav">
        <Brand />
        <span className="admin-link">Catálogo · {slug || "barbearia não configurada"}</span>
      </header>
      <main className="booking-layout">
        <section className="booking-story">
          <div className="story-top">
            <span className="story-pill">
              <span />
              SEU MOMENTO, DO SEU JEITO
            </span>
            <Image
              src="/brand/palma-logo.png"
              width={170}
              height={170}
              alt="Palma — identidade visual de referência"
              className="hero-logo"
              priority
            />
          </div>
          <div className="story-copy">
            <div className="eyebrow">ESTILO É SE SENTIR BEM.</div>
            <h1>
              Um tempo
              <br />
              para você.
              <br />
              <em>Um cuidado nosso.</em>
            </h1>
            <p>
              Conheça os serviços disponíveis.
              <br />
              Em breve, escolha seu próximo horário por aqui.
            </p>
          </div>
          <div className="story-bottom">
            <span>Identidade visual de referência · Dados do catálogo por slug</span>
          </div>
          <div className="story-circles" aria-hidden="true">
            <i />
            <i />
            <i />
          </div>
        </section>
        <section className="chat-panel" aria-label="Catálogo público">
          <header className="chat-header">
            <span className="chat-assistant-avatar">
              <Scissors size={24} />
            </span>
            <div>
              <h2>Qual cuidado combina com você?</h2>
              <p>Serviços da barbearia · {slug || "slug não configurado"}</p>
            </div>
          </header>
          <div className="chat-progress">
            <div className="done">
              <span>1</span>
              <small>Serviço</small>
            </div>
            <div>
              <span>2</span>
              <small>Horário · em breve</small>
            </div>
          </div>
          <div className="chat-conversation">
            <div className="chat-date-label">SEU PRÓXIMO BOM MOMENTO</div>
            <div className="bot-message">
              <span className="bot-avatar">
                <Scissors size={14} />
              </span>
              <div className="bot-bubble">
                Boas-vindas! Consulte os serviços e valores disponíveis nesta barbearia.
              </div>
            </div>
            {loading ? (
              <div className="loading-screen" role="status">
                <LoaderCircle size={25} />
                <span>Carregando serviços…</span>
              </div>
            ) : error ? (
              <div className="form-error" role="alert">
                <div>
                  <p>{error}</p>
                  <button className="button button-secondary" onClick={() => void reload()}>
                    Tentar novamente
                  </button>
                </div>
              </div>
            ) : selected ? (
              <>
                <div className="user-message">
                  <div>
                    {selected.name}
                    <CheckCheck size={14} />
                  </div>
                </div>
                <div className="booking-summary">
                  <span className="eyebrow">SERVIÇO SELECIONADO</span>
                  <h3>{selected.name}</h3>
                  <p>{selected.description}</p>
                  <div className="booking-total">
                    <span>{selected.duration_minutes} minutos</span>
                    <strong>{money(selected.price_cents)}</strong>
                  </div>
                </div>
                <div className="inline-hint integration-note">
                  <CalendarDays size={18} />
                  <strong>Agendamento ainda indisponível.</strong>
                  <p>
                    Profissionais, horários e confirmação aguardam integração. Nenhuma reserva foi
                    criada.
                  </p>
                </div>
                <button className="chat-back" onClick={() => setSelectedId("")}>
                  <ArrowLeft size={15} />
                  Escolher outro serviço
                </button>
              </>
            ) : services.length ? (
              <div className="chat-options">
                {services.map((s) => (
                  <button
                    className="chat-service-option"
                    key={s.id}
                    onClick={() => setSelectedId(s.id)}
                  >
                    <span className="chat-option-icon">
                      <Scissors size={18} />
                    </span>
                    <span>
                      <strong>{s.name}</strong>
                      <small>
                        {s.duration_minutes} min · {s.description}
                      </small>
                    </span>
                    <b>{money(s.price_cents)}</b>
                    <ChevronRight size={16} />
                  </button>
                ))}
              </div>
            ) : (
              <div className="inline-hint">
                Esta barbearia ainda não tem serviços ativos disponíveis.
              </div>
            )}
          </div>
          <footer className="chat-footer">
            Catálogo conectado à API · Reservas ainda indisponíveis
          </footer>
        </section>
      </main>
      <footer className="booking-footer">
        <span>BARBER-CHAT</span>
        <span>Seu estilo. Nosso cuidado.</span>
      </footer>
    </div>
  );
}
