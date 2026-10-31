"use client";
import { useEffect, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import type { AdminAppointment } from "@/lib/api/agenda";
import {
  changeAppointmentStatus,
  getAppointmentStatus,
  isAppointmentStatusInput,
  type AppointmentStatusDetail,
  type AppointmentStatusInput,
} from "@/lib/api/appointment-status";
import { useCommerceCommand } from "@/hooks/use-commerce-command";
import { useSelectedShopAccess } from "../shop-panel";
import { useFeedback } from "../feedback-provider";
import { Alert, Button, SelectField, TextareaField } from "../ui";
import { CommandFeedback, commerceTime, useCommerceScope } from "../commerce/shared";
import { AppointmentDetails } from "./appointment-details";
import { statusLabels } from "./appointment-status";
import styles from "../commerce/commerce.module.css";

export function AppointmentLifecyclePanel({
  id,
  timezone,
  onSaved,
  onBusyChange,
  onSell,
}: {
  id: string;
  timezone: string;
  onSaved: () => void;
  onBusyChange: (busy: boolean) => void;
  onSell: (item: AdminAppointment) => void;
}) {
  const scope = useCommerceScope(),
    owner = useSelectedShopAccess().role === "owner",
    { notify } = useFeedback();
  const [data, setData] = useState<AppointmentStatusDetail | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [revision, setRevision] = useState(0),
    [last, setLast] = useState<AppointmentStatusInput | null>(null);
  const command = useCommerceCommand(
    scope,
    "appointment:status:" + id,
    isAppointmentStatusInput,
    (body, key, signal) => changeAppointmentStatus(scope, id, body, key, signal),
  );
  const draft = command.attempt ?? last;
  useEffect(() => {
    onBusyChange(command.busy);
    return () => onBusyChange(false);
  }, [command.busy, onBusyChange]);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    setData(null);
    void getAppointmentStatus(scope, id, c.signal)
      .then((out) => {
        if (!c.signal.aborted) setData(out);
      })
      .catch((cause) => {
        if (!c.signal.aborted) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [scope, id, revision]);
  async function save(body?: AppointmentStatusInput) {
    if (body) setLast(body);
    const out = await command.submit(body);
    if (out) {
      setData(out);
      setLast(null);
      notify(
        out.replayed
          ? "Alteração recuperada. Confira a situação atual."
          : "Situação do atendimento atualizada.",
      );
      onSaved();
    }
  }
  return (
    <div className={styles.stack}>
      <div className={styles.actions}>
        <Button
          variant="quiet"
          disabled={loading || command.busy}
          onClick={() => setRevision((v) => v + 1)}
        >
          Atualizar detalhes
        </Button>
      </div>
      {loading && <p role="status">Consultando atendimento…</p>}
      {error && <Alert tone="error">{error}</Alert>}
      {data && (
        <AppointmentDetails
          item={data.appointment}
          timezone={timezone}
          onSell={command.locked ? undefined : () => onSell(data.appointment)}
        />
      )}
      <CommandFeedback command={command} onRetry={() => void save()} />
      {data && (
        <>
          <form
            className={styles.form}
            onSubmit={(event) => {
              event.preventDefault();
              if (command.locked || loading) return;
              const values = new FormData(event.currentTarget);
              const input = {
                status: String(values.get("status")),
                expected_version: data.appointment.version,
                reason: String(values.get("reason")).trim(),
              };
              if (!isAppointmentStatusInput(input)) {
                setError(
                  "Escolha uma situação e informe um motivo de até 500 caracteres em uma única linha.",
                );
                return;
              }
              setError("");
              void save(input);
            }}
          >
            <h3>Corrigir situação do atendimento</h3>
            <p className={styles.muted}>
              {owner
                ? "Como proprietário, você pode gerenciar qualquer atendimento desta barbearia."
                : "Você pode gerenciar os atendimentos do seu próprio barbeiro."}
            </p>
            <fieldset
              key={command.attempt ? "recovered" : data.appointment.version}
              className={styles.fields}
              disabled={command.locked || loading}
            >
              <div className={styles.full}>
                <SelectField
                  name="status"
                  label="Nova situação"
                  required
                  defaultValue={draft?.status ?? ""}
                >
                  <option value="" disabled>
                    Escolha o que aconteceu
                  </option>
                  {(["no_show", "cancelled", "completed"] as const)
                    .filter(
                      (status) =>
                        status !== data.appointment.status || command.attempt?.status === status,
                    )
                    .map((status) => (
                      <option key={status} value={status}>
                        {status === "no_show"
                          ? "Cliente não compareceu"
                          : status === "cancelled"
                            ? "Cancelar atendimento"
                            : "Marcar como realizado"}
                      </option>
                    ))}
                </SelectField>
              </div>
              <div className={styles.full}>
                <TextareaField
                  name="reason"
                  label="Motivo da alteração"
                  hint="Informe o motivo em uma única linha. Ele ficará no histórico."
                  required
                  maxLength={500}
                  rows={2}
                  defaultValue={draft?.reason ?? ""}
                />
              </div>
            </fieldset>
            <p className={styles.muted}>
              Ausência pode ser registrada após o início; realizado, após o término reservado. Essa
              alteração não registra pagamento nem anula vendas de produtos.
            </p>
            <div className={styles.actions}>
              <Button type="submit" disabled={command.locked || loading} loading={command.busy}>
                Salvar situação
              </Button>
            </div>
          </form>
          <details className={styles.panel}>
            <summary>Histórico da situação</summary>
            <ul className={styles.history}>
              {data.history.map((e) => (
                <li key={e.version}>
                  <strong>
                    {statusLabels[e.previous_status]} → {statusLabels[e.status]}
                  </strong>
                  <p>
                    {e.source === "automatic"
                      ? "Horário reservado encerrado."
                      : e.source === "public"
                        ? "Cancelado pelo código do agendamento."
                        : e.reason}
                  </p>
                  <p className={styles.muted}>
                    {commerceTime(e.occurred_at, timezone)} ·{" "}
                    {e.source === "automatic"
                      ? "Automático"
                      : e.source === "public"
                        ? "Código do agendamento"
                        : e.actor_id === scope.userID
                          ? "Você"
                          : "Equipe da barbearia"}
                  </p>
                </li>
              ))}
            </ul>
            {!data.history.length && (
              <p>Nenhuma alteração registrada após a atualização do sistema.</p>
            )}
            {data.history_truncated && (
              <p className={styles.muted}>Exibindo as 100 alterações mais recentes.</p>
            )}
          </details>
        </>
      )}
    </div>
  );
}
