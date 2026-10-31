"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import { adminShopPath } from "@/lib/api/config";
import { createProfessional, timeToMinutes, type StaffProfessional } from "@/lib/api/professionals";
import { useServices } from "@/hooks/use-services";
import { Alert, Button, TextField } from "../ui";
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

export function ProfessionalForm({
  slug,
  catalog,
  onCreated,
  checking,
}: {
  slug: string;
  catalog: ReturnType<typeof useServices>;
  onCreated: (professional: StaffProfessional) => Promise<void>;
  checking: boolean;
}) {
  const [name, setName] = useState("");
  const [serviceIds, setServiceIds] = useState<string[]>([]);
  const [weekdays, setWeekdays] = useState<number[]>([1, 2, 3, 4, 5, 6]);
  const [opening, setOpening] = useState("09:30");
  const [closing, setClosing] = useState("18:30");
  const [interval, setInterval] = useState("60");
  const [buffer, setBuffer] = useState("0");
  const [saving, setSaving] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const inFlight = useRef(false);
  const services = catalog.services.filter(
    (service) => service.active && service.duration_minutes <= 1440,
  );

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
    } catch (cause) {
      setError(errorMessage(cause));
      return;
    }
    inFlight.current = true;
    setSaving(true);
    try {
      const created = await createProfessional(slug, {
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
      setName("");
      setServiceIds([]);
      setSuccess(
        "Barbeiro cadastrado com os serviços e o expediente. A agenda já está disponível no chat público.",
      );
      await onCreated(created);
    } catch (cause) {
      setError(errorMessage(cause));
      setUncertain(!(cause instanceof ApiError) || cause.status === 0 || cause.status >= 500);
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  }

  return (
    <section className={styles.panel} aria-labelledby="professional-form-title">
      <div>
        <h2 id="professional-form-title">Cadastrar barbeiro</h2>
        <p className={styles.hint}>
          Defina os serviços e o expediente inicial. O login é criado separadamente.
        </p>
      </div>
      {success && <Alert tone="success">{success}</Alert>}
      {catalog.loading ? (
        <p className={styles.loading} role="status">
          Carregando serviços…
        </p>
      ) : catalog.error ? (
        <>
          <Alert tone="error">{catalog.error}</Alert>
          <Button variant="secondary" onClick={() => void catalog.reload()}>
            Recarregar serviços
          </Button>
        </>
      ) : !services.length ? (
        <p className={styles.hint}>
          Cadastre primeiro um <Link href={adminShopPath(slug) + "/servicos"}>serviço ativo</Link>{" "}
          com duração de até um dia.
        </p>
      ) : (
        <form
          className={styles.form}
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <fieldset disabled={saving || uncertain} className={styles.fields}>
            <TextField
              label="Nome do barbeiro"
              required
              maxLength={100}
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
              placeholder="Nome exibido no agendamento"
            />
            <fieldset className={styles.choices}>
              <legend>Serviços que atende</legend>
              {services.map((service) => (
                <label className={styles.choice} key={service.id}>
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
                  <span>
                    {service.name} · {service.duration_minutes} min
                  </span>
                </label>
              ))}
            </fieldset>
            <fieldset className={styles.choices}>
              <legend>Dias de atendimento</legend>
              <div className={styles.days}>
                {days.map((day, index) => (
                  <label className={styles.choice} key={day}>
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
                    <span>{day}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <fieldset className={styles.group}>
              <legend>Expediente e horários</legend>
              <TextField
                label="Abertura"
                type="time"
                required
                value={opening}
                onChange={(event) => setOpening(event.target.value)}
              />
              <TextField
                label="Fechamento"
                type="time"
                required
                value={closing}
                onChange={(event) => setClosing(event.target.value)}
                hint="Depois da abertura, no mesmo dia. Os horários seguem o fuso da barbearia."
              />
              <TextField
                label="Intervalo da grade (minutos)"
                type="number"
                min={1}
                max={1440}
                step={1}
                required
                value={interval}
                onChange={(event) => setInterval(event.target.value)}
                hint="Com abertura às 09:30 e intervalo de 60, os inícios são 09:30, 10:30, 11:30…"
              />
              <TextField
                label="Margem adicional (minutos)"
                type="number"
                min={0}
                max={1440}
                step={1}
                required
                value={buffer}
                onChange={(event) => setBuffer(event.target.value)}
                hint="Tempo acrescentado à duração do serviço antes de arredondar para a grade. Use 0 se não precisar."
              />
            </fieldset>
            <details className={styles.details}>
              <summary>Como a duração ocupa a agenda?</summary>
              <div>
                <p className={styles.hint}>
                  Na grade de 60 minutos, um serviço de 40 ocupa um horário; um de 80 ocupa dois.
                  Esta tela cria um período para cada dia selecionado. Não altera expedientes
                  existentes, pausas ou folgas.
                </p>
              </div>
            </details>
          </fieldset>
          {error && <Alert tone="error">{error}</Alert>}
          {uncertain && (
            <Alert tone="warning">
              <div className={styles.feedback}>
                <p>
                  O resultado pode ter sido salvo. Atualize e confira a equipe antes de fazer outro
                  cadastro. Não repita automaticamente.
                </p>
                <Button
                  variant="secondary"
                  disabled={checking || saving}
                  onClick={() => {
                    setUncertain(false);
                    setName("");
                    setServiceIds([]);
                    setError("");
                  }}
                >
                  Já conferi a lista; encerrar esta tentativa
                </Button>
              </div>
            </Alert>
          )}
          <Button type="submit" loading={saving} disabled={uncertain}>
            {saving ? "Cadastrando…" : "Cadastrar barbeiro"}
          </Button>
        </form>
      )}
    </section>
  );
}
