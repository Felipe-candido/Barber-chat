"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import Image from "next/image";
import {
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  CalendarDays,
  Check,
  CheckCheck,
  ChevronRight,
  Clock3,
  LockKeyhole,
  RotateCcw,
  Scissors,
  Sparkles,
  UserRound,
} from "lucide-react";
import {
  Appointment,
  addDays,
  availableTimes,
  dateKey,
  formatDate,
  money,
  professionals,
} from "@/lib/model";
import { useDemo } from "./demo-provider";
import { Brand, LoadingScreen } from "./ui";

export function BookingChat() {
  const { data, ready, book } = useDemo();
  const [step, setStep] = useState(0);
  const [serviceId, setServiceId] = useState("");
  const [professional, setProfessional] = useState("");
  const [date, setDate] = useState(() => dateKey(new Date()));
  const [time, setTime] = useState("");
  const [customer, setCustomer] = useState("");
  const [phone, setPhone] = useState("");
  const [error, setError] = useState("");
  const [appointment, setAppointment] = useState<Appointment | null>(null);
  const end = useRef<HTMLDivElement>(null);
  const service = data.services.find((s) => s.id === serviceId && s.active);
  const person = professionals.find((p) => p.id === professional);
  const times = availableTimes(data.appointments, date, service?.duration ?? 30, professional);
  useEffect(() => {
    end.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [step]);
  const days = Array.from({ length: 8 }, (_, i) => addDays(new Date(), i))
    .filter((d) => d.getDay() !== 0)
    .slice(0, 6);
  function restart() {
    setStep(0);
    setServiceId("");
    setProfessional("");
    setTime("");
    setCustomer("");
    setPhone("");
    setError("");
    setAppointment(null);
  }
  function goBack() {
    setError("");
    setStep(Math.max(step - 1, 0));
  }
  function confirm() {
    const result = book({
      customer,
      phone,
      serviceId,
      professional,
      date,
      time,
      notes: "",
      source: "chat",
    });
    if (result.error) setError(result.error);
    else {
      setAppointment(result.appointment!);
      setStep(5);
    }
  }
  return (
    <div className="booking-page">
      <header className="booking-nav">
        <Link href="/chat" aria-label="Palma Barbearia">
          <Brand />
        </Link>
        <Link href="/login" className="admin-link">
          Painel administrativo
          <ArrowUpRight size={16} />
        </Link>
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
              alt="Palma Barbearia"
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
              Do clássico ao seu próximo estilo.
              <br />
              Reserve seu horário e deixe o resto com a gente.
            </p>
          </div>
          <div className="story-bottom">
            <div>
              <Clock3 size={19} />
              <span>
                Segunda a sábado<strong>Das 9h às 19h</strong>
              </span>
            </div>
            <div>
              <Scissors size={19} />
              <span>
                Cuidado nos detalhes<strong>Do início ao acabamento</strong>
              </span>
            </div>
          </div>
          <div className="story-circles" aria-hidden="true">
            <i />
            <i />
            <i />
          </div>
        </section>
        <section className="chat-panel" aria-label="Chat de agendamento">
          <header className="chat-header">
            <span className="chat-assistant-avatar">
              <Scissors size={24} />
            </span>
            <div>
              <h2>Vamos marcar seu horário?</h2>
              <p>
                <span />
                Assistente de agendamento · Palma
              </p>
            </div>
            <span className="chat-sparkle">
              <Sparkles size={20} />
            </span>
          </header>
          <div className="chat-progress">
            {["Serviço", "Profissional", "Horário", "Seus dados"].map((label, i) => (
              <div key={label} className={step >= i ? "done" : ""}>
                <span>{step > i ? <Check size={11} /> : i + 1}</span>
                <small>{label}</small>
              </div>
            ))}
          </div>
          {!ready ? (
            <LoadingScreen />
          ) : (
            <div className="chat-conversation">
              <div className="chat-date-label">SEU PRÓXIMO BOM MOMENTO</div>
              <BotMessage>
                <p>
                  Olá! Boas-vindas à <strong>Palma Barbearia</strong> 👋
                </p>
                <p>
                  Vamos reservar um tempo para cuidar de você?
                  <br />
                  Qual serviço combina com o seu dia?
                </p>
              </BotMessage>
              {step === 0 && (
                <div className="chat-options">
                  {data.services
                    .filter((s) => s.active)
                    .map((s) => (
                      <button
                        key={s.id}
                        className="chat-service-option"
                        onClick={() => {
                          setServiceId(s.id);
                          setStep(1);
                        }}
                      >
                        <span className="chat-option-icon">
                          <Scissors size={18} />
                        </span>
                        <span>
                          <strong>{s.name}</strong>
                          <small>
                            {s.duration} min · {s.description}
                          </small>
                        </span>
                        <b>{money(s.price)}</b>
                        <ChevronRight size={16} />
                      </button>
                    ))}
                  {!data.services.some((s) => s.active) && (
                    <p className="inline-hint">
                      Ainda não há serviços disponíveis. Cadastre um serviço no painel para
                      experimentar o chat.
                    </p>
                  )}
                </div>
              )}
              {step >= 1 && (
                <>
                  <UserMessage>
                    {service?.name ?? appointment?.serviceName ?? "Serviço indisponível"}
                  </UserMessage>
                  <BotMessage>Boa escolha! Com quem você gostaria de agendar?</BotMessage>
                  {step === 1 && (
                    <div className="professional-options">
                      {professionals.map((p) => (
                        <button
                          key={p.id}
                          onClick={() => {
                            setProfessional(p.id);
                            setStep(2);
                          }}
                        >
                          <span className={"avatar professional-" + p.id}>{p.initials}</span>
                          <strong>{p.name}</strong>
                          <small>{p.role}</small>
                          <span className="choose-label">
                            Escolher
                            <ArrowRight size={13} />
                          </span>
                        </button>
                      ))}
                    </div>
                  )}
                </>
              )}
              {step >= 2 && (
                <>
                  <UserMessage>{person?.name}</UserMessage>
                  <BotMessage>Perfeito! Agora escolha o melhor dia e horário para você.</BotMessage>
                  {step === 2 && (
                    <div className="chat-date-picker">
                      <div className="date-pills">
                        {days.map((d) => (
                          <button
                            key={dateKey(d)}
                            className={dateKey(d) === date ? "selected" : ""}
                            onClick={() => {
                              setDate(dateKey(d));
                              setTime("");
                            }}
                          >
                            <small>{formatDate(d, { weekday: "short" }).replace(".", "")}</small>
                            <strong>{d.getDate()}</strong>
                          </button>
                        ))}
                      </div>
                      <label className="another-date">
                        <CalendarDays size={15} />
                        Escolher outra data
                        <input
                          aria-label="Escolher outra data"
                          type="date"
                          value={date}
                          min={dateKey(new Date())}
                          onChange={(e) => {
                            setDate(e.target.value);
                            setTime("");
                          }}
                        />
                      </label>
                      <p className="available-label">
                        Horários disponíveis{" "}
                        <span>· {formatDate(date, { day: "numeric", month: "short" })}</span>
                      </p>
                      <div className="time-options">
                        {times.map((t) => (
                          <button
                            key={t}
                            onClick={() => {
                              setTime(t);
                              setStep(3);
                            }}
                          >
                            {t}
                          </button>
                        ))}
                      </div>
                      {!times.length && (
                        <div className="inline-hint">
                          Esse dia já está completo ou fora do expediente. Escolha outra data para
                          encontrar seu horário.
                        </div>
                      )}
                    </div>
                  )}
                </>
              )}
              {step >= 3 && (
                <>
                  <UserMessage>
                    {formatDate(date, { day: "numeric", month: "long" })} às {time}
                  </UserMessage>
                  <BotMessage>
                    Quase lá! Como podemos chamar você e qual é o seu telefone?
                  </BotMessage>
                  {step === 3 && (
                    <form
                      className="chat-contact-form form-stack"
                      onSubmit={(e) => {
                        e.preventDefault();
                        if (
                          customer.trim().length < 3 ||
                          ![10, 11].includes(phone.replace(/\D/g, "").length)
                        ) {
                          setError("Informe seu nome completo e um telefone válido com DDD.");
                          return;
                        }
                        setError("");
                        setStep(4);
                      }}
                    >
                      <label>
                        Seu nome
                        <input
                          autoComplete="name"
                          required
                          minLength={3}
                          maxLength={100}
                          value={customer}
                          onChange={(e) => setCustomer(e.target.value)}
                          placeholder="Nome e sobrenome"
                        />
                      </label>
                      <label>
                        Telefone com DDD
                        <input
                          autoComplete="tel"
                          type="tel"
                          required
                          value={phone}
                          onChange={(e) => setPhone(e.target.value)}
                          placeholder="(11) 99999-9999"
                        />
                      </label>
                      <button className="button button-primary">
                        Revisar agendamento
                        <ArrowRight size={16} />
                      </button>
                      <p className="privacy-note">
                        <LockKeyhole size={13} />
                        Seus dados só são salvos ao confirmar o agendamento.
                      </p>
                    </form>
                  )}
                </>
              )}
              {step >= 4 && (
                <>
                  <UserMessage>{customer}</UserMessage>
                  {step === 4 ? (
                    <>
                      <BotMessage>
                        Tudo certo, {customer.split(" ")[0]}! Confira os detalhes antes de
                        confirmar.
                      </BotMessage>
                      <div className="booking-summary">
                        <span className="eyebrow">SEU MOMENTO NA PALMA</span>
                        <h3>{service?.name ?? "Serviço indisponível"}</h3>
                        <div>
                          <CalendarDays size={16} />
                          {formatDate(date, { weekday: "long", day: "numeric", month: "long" })}
                        </div>
                        <div>
                          <Clock3 size={16} />
                          {time} · {service?.duration} minutos
                        </div>
                        <div>
                          <UserRound size={16} />
                          {person?.name}
                        </div>
                        <div className="booking-total">
                          <span>Total</span>
                          <strong>{service ? money(service.price) : "—"}</strong>
                        </div>
                        <button className="button button-primary" onClick={confirm}>
                          <Check size={17} />
                          Confirmar meu agendamento
                        </button>
                      </div>
                    </>
                  ) : (
                    <div className="booking-success">
                      <span className="success-icon">
                        <Check size={28} />
                      </span>
                      <span className="eyebrow">COMBINADO!</span>
                      <h3>Seu horário está reservado.</h3>
                      <p>
                        {customer.split(" ")[0]}, esperamos você em
                        <br />
                        <strong>
                          {formatDate(date, { day: "numeric", month: "long" })}, às {time}.
                        </strong>
                      </p>
                      <div className="success-ticket">
                        <Scissors size={18} />
                        <div>
                          <strong>{appointment?.serviceName}</strong>
                          <small>
                            Com {person?.name} · {money(appointment?.price ?? 0)}
                          </small>
                        </div>
                        <CheckCheck size={19} />
                      </div>
                      <Link href={"/admin?date=" + date} className="button button-primary">
                        Ver na agenda
                        <ArrowUpRight size={16} />
                      </Link>
                      <button className="button button-quiet" onClick={restart}>
                        <RotateCcw size={15} />
                        Fazer outro agendamento
                      </button>
                    </div>
                  )}
                </>
              )}
              {error && (
                <div role="alert" className="form-error">
                  {error}
                </div>
              )}
              {step > 0 && step < 5 && (
                <button onClick={goBack} className="chat-back">
                  <ArrowLeft size={14} />
                  Voltar à etapa anterior
                </button>
              )}
              <div ref={end} />
            </div>
          )}
          <footer className="chat-footer">
            <LockKeyhole size={13} />
            <span>Demonstração · Nenhuma reserva real ou mensagem é enviada.</span>
          </footer>
        </section>
      </main>
      <footer className="booking-footer">
        <span>PALMA BARBEARIA</span>
        <span>Seu estilo. Nosso cuidado.</span>
      </footer>
    </div>
  );
}
function BotMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="bot-message">
      <span className="bot-avatar">
        <Scissors size={14} />
      </span>
      <div className="bot-bubble">{children}</div>
    </div>
  );
}
function UserMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="user-message">
      <div>
        {children}
        <CheckCheck size={14} />
      </div>
    </div>
  );
}
