"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { useSelectedShop } from "./shop-panel";
import { useServices } from "@/hooks/use-services";
import { ApiError, errorMessage } from "@/lib/api/client";
import { adminShopPath, publicBookingPath } from "@/lib/api/config";
import {
  StaffProfessional,
  createProfessional,
  listStaffProfessionals,
  timeToMinutes,
} from "@/lib/api/professionals";

const days = [
  "Domingo",
  "Segunda-feira",
  "Terça-feira",
  "Quarta-feira",
  "Quinta-feira",
  "Sexta-feira",
  "Sábado",
];

export function ProfessionalsPage() {
  const shop = useSelectedShop();
  const catalog = useServices(shop.slug, "admin");
  const [items, setItems] = useState<StaffProfessional[]>([]);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [name, setName] = useState("");
  const [serviceIds, setServiceIds] = useState<string[]>([]);
  const [weekdays, setWeekdays] = useState<number[]>([1, 2, 3, 4, 5, 6]);
  const [opening, setOpening] = useState("09:30");
  const [closing, setClosing] = useState("18:30");
  const [interval, setInterval] = useState("60");
  const [buffer, setBuffer] = useState("0");
  const [saving, setSaving] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const inFlight = useRef(false);
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
    } catch (error) {
      if (!controller.signal.aborted) setListError(errorMessage(error));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [shop.slug]);
  useEffect(() => {
    void reload();
    return () => activeRequest.current?.abort();
  }, [reload]);

  async function submit() {
    if (inFlight.current || uncertain) return;
    setError("");
    setSuccess("");
    let opens: number, closes: number;
    try {
      opens = timeToMinutes(opening);
      closes = timeToMinutes(closing);
      if (
        !name.trim() ||
        !serviceIds.length ||
        !weekdays.length ||
        closes <= opens ||
        !Number.isInteger(Number(interval)) ||
        Number(interval) < 1 ||
        Number(interval) > 1440 ||
        !Number.isInteger(Number(buffer)) ||
        Number(buffer) < 0 ||
        Number(buffer) > 1440
      )
        throw new Error(
          "Preencha o nome, selecione serviços e dias e confira o expediente. O fechamento deve ser depois da abertura, no mesmo dia.",
        );
    } catch (error) {
      setError(errorMessage(error));
      return;
    }
    inFlight.current = true;
    setSaving(true);
    try {
      const created = await createProfessional(shop.slug, {
        name: name.trim(),
        service_ids: serviceIds,
        schedule: {
          interval_minutes: Number(interval),
          buffer_minutes: Number(buffer),
          weekdays,
          opens_at_minute: opens,
          closes_at_minute: closes,
        },
      });
      setItems((current) => [...current, created]);
      setName("");
      setServiceIds([]);
      setSuccess(
        "Barbeiro cadastrado com os serviços e o expediente. A agenda já está disponível no chat público.",
      );
      await reload();
    } catch (error) {
      setError(errorMessage(error));
      setUncertain(!(error instanceof ApiError) || error.status === 0 || error.status >= 500);
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  }
  const services = catalog.services.filter(
    (service) => service.active && service.duration_minutes <= 1440,
  );

  return (
    <section className="professionals-page">
      <header className="page-heading">
        <div>
          <p className="eyebrow">EQUIPE E AGENDA</p>
          <h1>Barbeiros</h1>
          <p>Cadastre quem atende em {shop.name}, os serviços e o expediente inicial.</p>
        </div>
      </header>
      <p className="booking-note">
        O barbeiro aparece para os clientes agendarem. Esse cadastro não cria uma conta de login ou
        acesso administrativo.
      </p>
      {success && (
        <p className="professional-success" role="status">
          {success}
        </p>
      )}
      <div className="professionals-layout">
        <section className="professional-card" aria-label="Profissionais cadastrados">
          <h2>Equipe cadastrada</h2>
          {loading && <p role="status">Carregando barbeiros…</p>}
          {listError && (
            <p role="alert" className="form-error">
              {listError}
            </p>
          )}
          {!loading && !listError && items.length === 0 && (
            <p>Nenhum barbeiro cadastrado nesta unidade.</p>
          )}
          <ul className="professional-list">
            {items.map((item) => (
              <li key={item.id}>
                <strong>{item.name}</strong>
                <span>{item.active ? "Ativo" : "Inativo"}</span>
                <p>
                  {item.service_ids
                    .map(
                      (id) =>
                        catalog.services.find((service) => service.id === id)?.name ??
                        "Serviço cadastrado",
                    )
                    .join(" · ") || "Sem serviços vinculados"}
                </p>
              </li>
            ))}
          </ul>
          <button
            className="button button-secondary"
            disabled={loading || saving}
            onClick={() => void reload()}
          >
            Atualizar barbeiros
          </button>
          <Link
            className="button button-primary"
            href={publicBookingPath(shop.slug)}
            target="_blank"
            rel="noopener noreferrer"
          >
            Testar agendamento no chat
          </Link>
        </section>
        <section className="professional-card">
          <h2>Cadastrar barbeiro</h2>
          {catalog.loading ? (
            <p role="status">Carregando serviços…</p>
          ) : catalog.error ? (
            <>
              <p className="form-error" role="alert">
                {catalog.error}
              </p>
              <button className="button button-secondary" onClick={() => void catalog.reload()}>
                Recarregar serviços
              </button>
            </>
          ) : !services.length ? (
            <p>
              Cadastre primeiro um{" "}
              <Link href={adminShopPath(shop.slug) + "/servicos"}>serviço ativo</Link> com duração
              de até um dia.
            </p>
          ) : (
            <form
              className="booking-form"
              onSubmit={(event) => {
                event.preventDefault();
                void submit();
              }}
            >
              <fieldset disabled={saving || uncertain} className="professional-fields">
                <label className="booking-field">
                  Nome do barbeiro
                  <input
                    required
                    maxLength={100}
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    autoComplete="off"
                  />
                </label>
                <fieldset className="professional-choices">
                  <legend>Serviços que atende</legend>
                  {services.map((service) => (
                    <label key={service.id}>
                      <input
                        type="checkbox"
                        checked={serviceIds.includes(service.id)}
                        onChange={(event) =>
                          setServiceIds(
                            event.target.checked
                              ? [...serviceIds, service.id]
                              : serviceIds.filter((id) => id !== service.id),
                          )
                        }
                      />
                      {service.name} · {service.duration_minutes} min
                    </label>
                  ))}
                </fieldset>
                <fieldset className="professional-choices">
                  <legend>Dias de atendimento</legend>
                  {days.map((day, index) => (
                    <label key={day}>
                      <input
                        type="checkbox"
                        checked={weekdays.includes(index)}
                        onChange={(event) =>
                          setWeekdays(
                            event.target.checked
                              ? [...weekdays, index]
                              : weekdays.filter((value) => value !== index),
                          )
                        }
                      />
                      {day}
                    </label>
                  ))}
                </fieldset>
                <div className="professional-time-fields">
                  <label className="booking-field">
                    Abertura
                    <input
                      type="time"
                      required
                      value={opening}
                      onChange={(event) => setOpening(event.target.value)}
                    />
                  </label>
                  <label className="booking-field">
                    Fechamento
                    <input
                      type="time"
                      required
                      value={closing}
                      onChange={(event) => setClosing(event.target.value)}
                    />
                  </label>
                  <label className="booking-field">
                    Intervalo da grade (minutos)
                    <input
                      type="number"
                      min={1}
                      max={1440}
                      step={1}
                      required
                      value={interval}
                      onChange={(event) => setInterval(event.target.value)}
                    />
                  </label>
                  <label className="booking-field">
                    Margem adicional (minutos)
                    <input
                      type="number"
                      min={0}
                      max={1440}
                      step={1}
                      required
                      value={buffer}
                      onChange={(event) => setBuffer(event.target.value)}
                    />
                  </label>
                </div>
                <p className="booking-note">
                  Os horários seguem o fuso cadastrado da barbearia. Com abertura 09:30 e intervalo
                  60, os inícios são 09:30, 10:30, 11:30… Um serviço de 40 minutos ocupa 60; um de
                  80 ocupa 120. A margem é acrescentada antes do arredondamento. Esta tela cria um
                  período por dia selecionado; edição, pausas e folgas ficam para a administração de
                  agenda.
                </p>
                <button
                  className="button button-primary"
                  type="submit"
                  disabled={saving || uncertain}
                >
                  {saving ? "Cadastrando…" : "Cadastrar barbeiro"}
                </button>
              </fieldset>
            </form>
          )}
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          {uncertain && (
            <div className="cancellation-confirm">
              <p>
                O resultado pode ter sido salvo. Atualize e confira a equipe antes de fazer outro
                cadastro. Não repita automaticamente.
              </p>
              <button
                className="button button-secondary"
                disabled={loading || saving}
                onClick={() => {
                  setUncertain(false);
                  setName("");
                  setServiceIds([]);
                  setError("");
                }}
              >
                Já conferi a lista; encerrar esta tentativa
              </button>
            </div>
          )}
        </section>
      </div>
    </section>
  );
}
