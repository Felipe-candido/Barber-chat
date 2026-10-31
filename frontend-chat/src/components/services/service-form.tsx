"use client";

import { useEffect, useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import { createService, priceToCents } from "@/lib/api/services";
import { useFeedback } from "../feedback-provider";
import { Alert, Button, Modal, TextField, TextareaField } from "../ui";
import styles from "../admin/management.module.css";

export function ServiceForm({
  slug,
  onClose,
  onCreated,
}: {
  slug: string;
  onClose: () => void;
  onCreated: () => void;
}) {
  const { notify } = useFeedback();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [uncertain, setUncertain] = useState(false);
  const inFlight = useRef(false);
  const request = useRef<AbortController | null>(null);
  useEffect(() => () => request.current?.abort(), []);
  return (
    <Modal
      open
      onClose={() => {
        if (!inFlight.current) onClose();
      }}
      title="Adicionar serviço"
      subtitle="O serviço será criado ativo e ficará disponível para vincular aos barbeiros."
    >
      <form
        className={styles.form}
        onSubmit={async (event) => {
          event.preventDefault();
          if (inFlight.current || uncertain) return;
          const values = new FormData(event.currentTarget);
          setError("");
          try {
            const input = {
              name: String(values.get("name")),
              description: String(values.get("description")),
              duration_minutes: Number(values.get("duration")),
              price_cents: priceToCents(String(values.get("price"))),
            };
            inFlight.current = true;
            setSaving(true);
            const controller = new AbortController();
            request.current = controller;
            const saved = await createService(slug, input, controller.signal);
            if (controller.signal.aborted) return;
            notify("Serviço “" + saved.name + "” criado na API.");
            onCreated();
          } catch (cause) {
            if (!request.current?.signal.aborted) {
              setError(errorMessage(cause));
              // A failed response is not proof that a non-idempotent POST was rolled back.
              setUncertain(
                inFlight.current &&
                  (!(cause instanceof ApiError) ||
                    (cause.code !== "invalid_service" &&
                      (cause.status === 0 || cause.status >= 500))),
              );
            }
          } finally {
            inFlight.current = false;
            if (!request.current?.signal.aborted) setSaving(false);
          }
        }}
      >
        <fieldset disabled={saving || uncertain} className={styles.fields}>
          <TextField
            name="name"
            label="Nome do serviço"
            required
            maxLength={100}
            placeholder="Ex.: Corte clássico"
          />
          <TextareaField
            name="description"
            label="Descrição (opcional)"
            rows={3}
            placeholder="Descreva o que está incluído"
          />
          <TextField
            name="price"
            label="Preço (R$)"
            hint="Informe o valor em reais, por exemplo 45,00. Use 0 para gratuito."
            inputMode="decimal"
            required
            placeholder="45,00"
          />
          <TextField
            name="duration"
            label="Duração (minutos)"
            hint="Tempo do serviço. Os horários disponíveis seguem a grade configurada do barbeiro."
            type="number"
            min={1}
            max={2147483647}
            step={1}
            required
            defaultValue={30}
          />
        </fieldset>
        {error && <Alert tone="error">{error}</Alert>}
        {uncertain && (
          <Alert tone="warning">
            O resultado pode ter sido salvo. Feche esta janela e atualize o catálogo antes de
            iniciar outro cadastro.
          </Alert>
        )}
        <div className={styles.actions}>
          <Button variant="secondary" disabled={saving} onClick={onClose}>
            Voltar
          </Button>
          <Button type="submit" loading={saving} disabled={uncertain}>
            {saving ? "Salvando…" : "Adicionar serviço"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
