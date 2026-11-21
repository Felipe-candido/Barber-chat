"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { useServices } from "@/hooks/use-services";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  getProfessionalDetails,
  updateProfessionalDetails,
  parseScheduleTime,
  scheduleTime,
  type ProfessionalDetails,
  type ProfessionalEditInput,
} from "@/lib/api/professional-edit";
import { Alert, Button, Modal, TextField } from "../ui";
import { useFeedback } from "../feedback-provider";
import styles from "../admin/management.module.css";

const days = [
  "Domingo",
  "Segunda-feira",
  "Terça-feira",
  "Quarta-feira",
  "Quinta-feira",
  "Sexta-feira",
  "Sábado",
];
type DayDraft = { enabled: boolean; periods: { key: string; opening: string; closing: string }[] };
type Draft = {
  name: string;
  active: boolean;
  services: string[];
  interval: string;
  buffer: string;
  days: DayDraft[];
};
function draftOf(p: ProfessionalDetails): Draft {
  return {
    name: p.name,
    active: p.active,
    services: [...p.service_ids],
    interval: String(p.schedule.interval_minutes),
    buffer: String(p.schedule.buffer_minutes),
    days: days.map((_, weekday) => {
      const hours = p.schedule.working_hours.filter((w) => w.weekday === weekday);
      return {
        enabled: hours.length > 0,
        periods: hours.length
          ? hours.map((w) => ({
              key: crypto.randomUUID(),
              opening: scheduleTime(w.opens_at_minute),
              closing: scheduleTime(w.closes_at_minute),
            }))
          : [{ key: crypto.randomUUID(), opening: "09:00", closing: "18:00" }],
      };
    }),
  };
}
export function ProfessionalEditDialog({
  slug,
  id,
  catalog,
  onClose,
  onSaved,
}: {
  slug: string;
  id: string;
  catalog: ReturnType<typeof useServices>;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { notify } = useFeedback();
  const [current, setCurrent] = useState<ProfessionalDetails | null>(null),
    [draft, setDraft] = useState<Draft | null>(null);
  const [loading, setLoading] = useState(true),
    [saving, setSaving] = useState(false),
    [error, setError] = useState(""),
    [conflicted, setConflicted] = useState(false),
    [dirty, setDirty] = useState(false);
  const [attempt, setAttempt] = useState<ProfessionalEditInput | null>(null);
  const inFlight = useRef(false),
    request = useRef<AbortController | null>(null);
  const errorAnchor = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (error) {
      errorAnchor.current?.scrollIntoView({ block: "nearest" });
      errorAnchor.current?.focus({ preventScroll: true });
    }
  }, [error]);
  const load = useCallback(async () => {
    request.current?.abort();
    const c = new AbortController();
    request.current = c;
    setLoading(true);
    setError("");
    setCurrent(null);
    setDraft(null);
    setConflicted(false);
    setAttempt(null);
    try {
      const p = await getProfessionalDetails(slug, id, c.signal);
      if (c.signal.aborted) return;
      setCurrent(p);
      setDraft(draftOf(p));
      setDirty(false);
    } catch (cause) {
      if (!c.signal.aborted) setError(errorMessage(cause));
    } finally {
      if (!c.signal.aborted) setLoading(false);
    }
  }, [slug, id]);
  useEffect(() => {
    void load();
    return () => request.current?.abort();
  }, [load]);
  function change(next: Partial<Draft>) {
    setDraft((d) => (d ? { ...d, ...next } : d));
    setDirty(true);
  }
  function changeDay(index: number, update: (d: DayDraft) => DayDraft) {
    setDraft((d) =>
      d ? { ...d, days: d.days.map((day, i) => (i === index ? update(day) : day)) } : d,
    );
    setDirty(true);
  }
  async function save() {
    if (inFlight.current || !draft || !current || conflicted || catalog.loading || catalog.error)
      return;
    setError("");
    let sent = attempt;
    try {
      if (!sent) {
        const interval = Number(draft.interval),
          buffer = Number(draft.buffer);
        if (!draft.name.trim() || Array.from(draft.name.trim()).length > 100)
          throw new Error("Informe um nome de até 100 caracteres.");
        if (
          !Number.isInteger(interval) ||
          interval < 1 ||
          interval > 1440 ||
          !Number.isInteger(buffer) ||
          buffer < 0 ||
          buffer > 1440
        )
          throw new Error(
            "Confira o espaço entre os horários e o tempo extra. Use minutos inteiros.",
          );
        const hours = draft.days
          .flatMap((day, weekday) =>
            day.enabled
              ? day.periods.map((w) => ({
                  weekday,
                  opens_at_minute: parseScheduleTime(w.opening),
                  closes_at_minute: parseScheduleTime(w.closing, true),
                }))
              : [],
          )
          .sort((a, b) => a.weekday - b.weekday || a.opens_at_minute - b.opens_at_minute);
        for (let i = 0; i < hours.length; i++) {
          const w = hours[i];
          if (
            w.closes_at_minute <= w.opens_at_minute ||
            (i &&
              hours[i - 1].weekday === w.weekday &&
              hours[i - 1].closes_at_minute > w.opens_at_minute)
          )
            throw new Error(
              "Confira o expediente: o fechamento deve ser depois da abertura e os períodos do mesmo dia não podem se sobrepor.",
            );
        }
        sent = {
          name: draft.name.trim(),
          active: draft.active,
          service_ids: [...draft.services],
          revision: current.revision,
          schedule: { interval_minutes: interval, buffer_minutes: buffer, working_hours: hours },
        };
      }
    } catch (cause) {
      setError(errorMessage(cause));
      return;
    }
    inFlight.current = true;
    setSaving(true);
    const c = new AbortController();
    request.current = c;
    try {
      await updateProfessionalDetails(slug, id, sent, c.signal);
      if (c.signal.aborted) return;
      notify(
        "Cadastro do barbeiro atualizado. Os atendimentos já marcados e as folgas foram mantidos.",
      );
      onSaved();
    } catch (cause) {
      if (c.signal.aborted) return;
      setError(errorMessage(cause));
      setConflicted(cause instanceof ApiError && cause.code === "professional_changed");
      setAttempt(
        !(cause instanceof ApiError) ||
          cause.code === "invalid_response" ||
          cause.status === 0 ||
          cause.status >= 500
          ? sent
          : null,
      );
    } finally {
      inFlight.current = false;
      if (!c.signal.aborted) setSaving(false);
    }
  }
  const disabled =
    saving || loading || !!attempt || conflicted || catalog.loading || !!catalog.error;
  const available = catalog.services.filter(
    (s) => (s.active && s.duration_minutes <= 1440) || draft?.services.includes(s.id),
  );
  const missing =
    current?.service_ids.filter((service) => !catalog.services.some((s) => s.id === service)) ?? [];
  return (
    <Modal
      open
      wide
      title="Editar barbeiro"
      subtitle="Altere os serviços e a escala para novos agendamentos. Os atendimentos já feitos ou marcados e as folgas serão preservados."
      onClose={() => {
        if (inFlight.current) return;
        if (dirty && !attempt && !window.confirm("Sair sem salvar as alterações do barbeiro?"))
          return;
        onClose();
      }}
    >
      <div className={styles.form}>
        {loading && <p role="status">Carregando o cadastro…</p>}
        {error && (
          <div ref={errorAnchor} tabIndex={-1}>
            <Alert tone="error">{error}</Alert>
          </div>
        )}
        {catalog.error && <Alert tone="error">{catalog.error}</Alert>}
        {catalog.loading && <p role="status">Carregando serviços…</p>}
        {catalog.error && (
          <Button variant="secondary" onClick={() => void catalog.reload()}>
            Recarregar serviços
          </Button>
        )}
        {attempt && (
          <Alert tone="warning">
            Ainda não conseguimos confirmar se foi salvo. Suas alterações foram mantidas. Use
            Repetir confirmação para conferir o mesmo envio com segurança.
          </Alert>
        )}
        {conflicted && (
          <Alert tone="warning">
            Outra pessoa alterou este cadastro. Suas alterações continuam no formulário. Recarregue
            os dados atuais antes de editar novamente.
          </Alert>
        )}
        {draft && current && (
          <form
            className={styles.form}
            noValidate
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <fieldset className={styles.fields} disabled={disabled}>
              <TextField
                label="Nome do barbeiro"
                value={draft.name}
                required
                maxLength={100}
                onChange={(e) => change({ name: e.target.value })}
              />
              <label className={styles.choice}>
                <input
                  type="checkbox"
                  checked={draft.active}
                  onChange={(e) => change({ active: e.target.checked })}
                />
                <span>Barbeiro ativo</span>
              </label>
              {!draft.active && (
                <Alert tone="warning">
                  Enquanto estiver inativo, este barbeiro não recebe novos agendamentos nem acessa
                  sua conta da equipe. Os atendimentos já marcados continuam registrados.
                </Alert>
              )}
              <fieldset className={styles.choices}>
                <legend>Serviços que atende</legend>
                {available.map((service) => (
                  <label className={styles.choice} key={service.id}>
                    <input
                      type="checkbox"
                      checked={draft.services.includes(service.id)}
                      onChange={(e) =>
                        change({
                          services: e.target.checked
                            ? [...draft.services, service.id]
                            : draft.services.filter((value) => value !== service.id),
                        })
                      }
                    />
                    <span>
                      {service.name} · {service.duration_minutes} min
                    </span>
                  </label>
                ))}
                {missing.map((service) => (
                  <label className={styles.choice} key={service}>
                    <input
                      type="checkbox"
                      checked={draft.services.includes(service)}
                      onChange={(e) =>
                        change({
                          services: e.target.checked
                            ? [...draft.services, service]
                            : draft.services.filter((value) => value !== service),
                        })
                      }
                    />
                    <span>Serviço fora do catálogo — não disponível para novos agendamentos</span>
                  </label>
                ))}
                {!draft.services.length && (
                  <p className={styles.hint}>
                    Sem serviços selecionados, este barbeiro não aparece para novos agendamentos.
                  </p>
                )}
              </fieldset>
              <div className={styles.grid}>
                <TextField
                  label="Espaço entre inícios (minutos)"
                  type="number"
                  min={1}
                  max={1440}
                  step={1}
                  required
                  value={draft.interval}
                  onChange={(e) => change({ interval: e.target.value })}
                  hint="Ex.: 30 gera horários às 09:00, 09:30, 10:00…"
                />
                <TextField
                  label="Tempo extra entre atendimentos (minutos)"
                  type="number"
                  min={0}
                  max={1440}
                  step={1}
                  required
                  value={draft.buffer}
                  onChange={(e) => change({ buffer: e.target.value })}
                  hint="Acrescentado à duração do serviço antes de reservar os horários. Use 0 se não precisar."
                />
              </div>
              <section className={styles.form} aria-label="Expediente semanal">
                <h3>Dias e horários de atendimento</h3>
                <p className={styles.hint}>
                  Abra cada dia para ajustar os horários. Para uma pausa de almoço, cadastre um
                  período antes e outro depois. Os horários seguem o fuso da barbearia.
                </p>
                {draft.days.map((day, weekday) => (
                  <details className={styles.details} key={weekday}>
                    <summary>
                      {days[weekday]} ·{" "}
                      {day.enabled
                        ? day.periods.map((w) => `${w.opening}–${w.closing}`).join(" / ")
                        : "Sem atendimento"}
                    </summary>
                    <div className={styles.form}>
                      <label className={styles.choice}>
                        <input
                          type="checkbox"
                          aria-label={`Atendimento: ${days[weekday]}`}
                          checked={day.enabled}
                          onChange={(e) =>
                            changeDay(weekday, (d) => ({ ...d, enabled: e.target.checked }))
                          }
                        />
                        <span>Atender neste dia</span>
                      </label>
                      {day.enabled &&
                        day.periods.map((period, index) => (
                          <div className={styles.panel} key={period.key}>
                            <TextField
                              label={`Abertura ${index + 1} — ${days[weekday]}`}
                              type="time"
                              required
                              value={period.opening}
                              onChange={(e) =>
                                changeDay(weekday, (d) => ({
                                  ...d,
                                  periods: d.periods.map((w, i) =>
                                    i === index ? { ...w, opening: e.target.value } : w,
                                  ),
                                }))
                              }
                            />
                            <TextField
                              label={`Fechamento ${index + 1} — ${days[weekday]}`}
                              required
                              value={period.closing}
                              maxLength={5}
                              placeholder="18:00"
                              hint="Use 24:00 para terminar à meia-noite."
                              onChange={(e) =>
                                changeDay(weekday, (d) => ({
                                  ...d,
                                  periods: d.periods.map((w, i) =>
                                    i === index ? { ...w, closing: e.target.value } : w,
                                  ),
                                }))
                              }
                            />
                            {day.periods.length > 1 && (
                              <Button
                                variant="secondary"
                                onClick={() =>
                                  changeDay(weekday, (d) => ({
                                    ...d,
                                    periods: d.periods.filter((_, i) => i !== index),
                                  }))
                                }
                              >
                                Remover período {index + 1} — {days[weekday]}
                              </Button>
                            )}
                          </div>
                        ))}
                      {day.enabled && (
                        <Button
                          variant="secondary"
                          disabled={day.periods.length >= 4}
                          onClick={() =>
                            changeDay(weekday, (d) => ({
                              ...d,
                              periods: [
                                ...d.periods,
                                { key: crypto.randomUUID(), opening: "13:00", closing: "18:00" },
                              ],
                            }))
                          }
                        >
                          Adicionar período — {days[weekday]}
                        </Button>
                      )}
                    </div>
                  </details>
                ))}
              </section>
              {!draft.days.some((day) => day.enabled) && (
                <Alert>
                  Nenhum dia de atendimento selecionado. Não serão oferecidos novos horários até
                  você definir um expediente.
                </Alert>
              )}
            </fieldset>
            <div className={styles.actions}>
              <Button
                type="submit"
                loading={saving}
                disabled={loading || conflicted || catalog.loading || !!catalog.error}
              >
                {attempt ? "Repetir confirmação" : "Salvar alterações"}
              </Button>
              <Button
                variant="secondary"
                disabled={saving}
                onClick={() => {
                  if (
                    dirty &&
                    !window.confirm(
                      "Recarregar o cadastro vai descartar suas alterações não salvas. Continuar?",
                    )
                  )
                    return;
                  void load();
                }}
              >
                Recarregar cadastro
              </Button>
            </div>
          </form>
        )}
        {!loading && !draft && <Button onClick={() => void load()}>Tentar novamente</Button>}
      </div>
    </Modal>
  );
}
