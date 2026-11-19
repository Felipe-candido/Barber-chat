"use client";

import { useEffect, useRef, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import { deleteService, type Service } from "@/lib/api/services";
import { useFeedback } from "../feedback-provider";
import { Alert, Button, Modal } from "../ui";
import styles from "../admin/management.module.css";

export function DeleteServiceDialog({
  slug,
  service,
  onClose,
  onDeleted,
}: {
  slug: string;
  service: Service;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const { notify } = useFeedback();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const inFlight = useRef(false);
  const request = useRef<AbortController | null>(null);
  useEffect(() => () => request.current?.abort(), []);

  return (
    <Modal
      open
      title="Excluir serviço?"
      onClose={() => {
        if (!inFlight.current) onClose();
      }}
    >
      <div className={styles.form}>
        <p>
          “{service.name}” deixará de aparecer para novos agendamentos. Os atendimentos já marcados
          e o histórico serão mantidos.
        </p>
        {error && <Alert tone="error">{error}</Alert>}
        <div className={styles.actions}>
          <Button variant="secondary" disabled={saving} onClick={onClose} autoFocus>
            Manter serviço
          </Button>
          <Button
            variant="danger"
            loading={saving}
            onClick={async () => {
              if (inFlight.current) return;
              inFlight.current = true;
              setSaving(true);
              setError("");
              const controller = new AbortController();
              request.current = controller;
              try {
                await deleteService(slug, service.id, controller.signal);
                if (controller.signal.aborted) return;
                notify("Serviço “" + service.name + "” excluído.");
                onDeleted();
              } catch (cause) {
                if (!controller.signal.aborted) setError(errorMessage(cause));
              } finally {
                inFlight.current = false;
                if (!controller.signal.aborted) setSaving(false);
              }
            }}
          >
            {saving ? "Excluindo…" : "Excluir serviço"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
