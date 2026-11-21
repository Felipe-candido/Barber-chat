"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useSelectedShop, useSelectedShopAccess } from "../shop-panel";
import { useFeedback } from "../feedback-provider";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  deleteTimeOff,
  listTimeOff,
  saveTimeOff,
  timeOffLabel,
  type TimeOffAttempt,
  type TimeOffBlock,
  type TimeOffData,
} from "@/lib/api/time-off";
import { Alert, Button, Modal, SelectField, TextField } from "../ui";
import styles from "../admin/management.module.css";

export function TimeOffDialog({
  onClose,
  onChanged,
}: {
  onClose: () => void;
  onChanged: () => void;
}) {
  const shop = useSelectedShop(),
    access = useSelectedShopAccess(),
    { notify } = useFeedback();
  const own = access.role === "barber" ? access.professional_id : null;
  const [data, setData] = useState<TimeOffData | null>(null),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false);
  const [error, setError] = useState(""),
    [professional, setProfessional] = useState(own ?? ""),
    [from, setFrom] = useState(""),
    [through, setThrough] = useState("");
  const [attempt, setAttempt] = useState<TimeOffAttempt | null>(null),
    [removing, setRemoving] = useState<TimeOffBlock | null>(null);
  const inFlight = useRef(false),
    request = useRef<AbortController | null>(null);
  const load = useCallback(async () => {
    request.current?.abort();
    const c = new AbortController();
    request.current = c;
    setLoading(true);
    setError("");
    setData(null);
    try {
      const next = await listTimeOff(shop.slug, own, c.signal);
      if (c.signal.aborted) return;
      setData(next);
      setProfessional((current) =>
        next.professionals.some((p) => p.id === current)
          ? current
          : (next.professionals[0]?.id ?? ""),
      );
      setFrom((current) => current || next.today);
      setThrough((current) => current || next.today);
    } catch (cause) {
      if (!c.signal.aborted) setError(errorMessage(cause));
    } finally {
      if (!c.signal.aborted) setLoading(false);
    }
  }, [shop.slug, own]);
  useEffect(() => {
    void load();
    return () => request.current?.abort();
  }, [load]);
  async function save() {
    if (inFlight.current || !data || loading) return;
    const sent = attempt ?? {
      id: crypto.randomUUID(),
      professional,
      from_date: from,
      through_date: through,
    };
    inFlight.current = true;
    setBusy(true);
    setError("");
    const c = new AbortController();
    request.current = c;
    try {
      await saveTimeOff(shop.slug, sent, c.signal);
      if (c.signal.aborted) return;
      setAttempt(null);
      notify("Período bloqueado. Você não receberá novos agendamentos nesses dias.");
      onChanged();
      await load();
    } catch (cause) {
      if (c.signal.aborted) return;
      setError(errorMessage(cause));
      const uncertain =
        !(cause instanceof ApiError) ||
        cause.code === "invalid_response" ||
        cause.status === 0 ||
        cause.status >= 500;
      setAttempt(uncertain ? sent : null);
    } finally {
      inFlight.current = false;
      if (!c.signal.aborted || request.current !== c) setBusy(false);
    }
  }
  async function remove(b: TimeOffBlock) {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    const c = new AbortController();
    request.current = c;
    try {
      await deleteTimeOff(shop.slug, b, c.signal);
      if (c.signal.aborted) return;
      setRemoving(null);
      notify("Folga removida. Os horários voltam a seguir o expediente habitual.");
      onChanged();
      await load();
    } catch (cause) {
      if (!c.signal.aborted) setError(errorMessage(cause));
    } finally {
      inFlight.current = false;
      if (!c.signal.aborted || request.current !== c) setBusy(false);
    }
  }
  return (
    <Modal
      open
      title="Folgas e férias"
      subtitle="Escolha os dias em que não haverá atendimento, sem mudar o expediente da semana."
      onClose={() => {
        if (!inFlight.current) onClose();
      }}
    >
      <div className={styles.form}>
        {loading && <p role="status">Carregando folgas…</p>}
        {error && <Alert tone="error">{error}</Alert>}
        {!loading && !data && <Button onClick={() => void load()}>Tentar novamente</Button>}
        {data && (
          <>
            <p className={styles.hint}>
              As datas seguem o horário da barbearia. O primeiro e o último dia ficam bloqueados por
              inteiro. Para uma folga de um dia, repita a mesma data nos dois campos.
            </p>
            <form
              className={styles.form}
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              <fieldset
                className={styles.fields}
                disabled={busy || loading || !!attempt || !!removing || !data.professionals.length}
              >
                {access.role === "owner" ? (
                  <SelectField
                    label="Barbeiro"
                    value={professional}
                    required
                    onChange={(e) => setProfessional(e.target.value)}
                  >
                    {data.professionals.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </SelectField>
                ) : (
                  <p>
                    Suas folgas: <strong>{data.professionals[0]?.name}</strong>
                  </p>
                )}
                <TextField
                  label="Primeiro dia sem atendimento"
                  type="date"
                  required
                  min={data.today}
                  max={data.max_date}
                  value={from}
                  onChange={(e) => {
                    setFrom(e.target.value);
                    if (through < e.target.value) setThrough(e.target.value);
                  }}
                />
                <TextField
                  label="Último dia sem atendimento"
                  type="date"
                  required
                  min={from || data.today}
                  max={data.max_date}
                  hint="Você pode bloquear até um ano por vez, dentro dos próximos dois anos."
                  value={through}
                  onChange={(e) => setThrough(e.target.value)}
                />
              </fieldset>
              {!data.professionals.length && (
                <Alert>Cadastre um barbeiro na tela Equipe antes de adicionar folgas.</Alert>
              )}
              {attempt && (
                <Alert tone="warning">
                  Ainda precisamos confirmar se a folga foi salva. Seus dados foram mantidos. Use
                  Repetir confirmação para conferir sem criar outro bloqueio.
                </Alert>
              )}
              <div className={styles.actions}>
                <Button
                  type="submit"
                  loading={busy}
                  disabled={loading || !professional || !data.professionals.length || !!removing}
                >
                  {attempt ? "Repetir confirmação" : "Bloquear período"}
                </Button>
                <Button
                  variant="secondary"
                  disabled={busy}
                  onClick={() => {
                    setAttempt(null);
                    setRemoving(null);
                    void load();
                  }}
                >
                  Atualizar folgas
                </Button>
              </div>
            </form>
            <section className={styles.panel} aria-label="Períodos sem atendimento">
              <h3>Próximas folgas e férias</h3>
              {!data.blocks.length ? (
                <p>Nenhuma folga programada. O expediente habitual continua valendo.</p>
              ) : (
                <ul className={styles.list}>
                  {data.blocks.map((b) => (
                    <li key={b.id}>
                      <strong>{b.professional_name}</strong>
                      <p>{timeOffLabel(b, data.shop_timezone)}</p>
                      {removing?.id === b.id ? (
                        <>
                          <p>
                            Remover esta folga? Os horários voltam a ficar disponíveis conforme o
                            expediente.
                          </p>
                          <div className={styles.actions}>
                            <Button
                              variant="secondary"
                              disabled={busy}
                              onClick={() => setRemoving(null)}
                            >
                              Manter folga
                            </Button>
                            <Button variant="danger" loading={busy} onClick={() => void remove(b)}>
                              Liberar dias
                            </Button>
                          </div>
                        </>
                      ) : (
                        <Button
                          variant="secondary"
                          disabled={busy || !!attempt}
                          onClick={() => setRemoving(b)}
                        >
                          Remover período
                        </Button>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </>
        )}
        <p className={styles.hint}>
          Se houver atendimentos no período, remaneje ou cancele esses agendamentos antes de
          bloquear os dias. Nenhum atendimento será cancelado automaticamente.
        </p>
      </div>
    </Modal>
  );
}
