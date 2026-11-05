"use client";

import { useEffect, useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  contactPhone,
  createContact,
  listContacts,
  type Contact,
  type ContactInput,
  type ContactScope,
} from "@/lib/api/contacts";
import { Alert, Button, Modal, TextField } from "../ui";
import styles from "./contacts.module.css";

type Mode = "editing" | "uncertain" | "conflict" | "found" | "missing" | "blocked";
export function ContactForm({
  scope,
  onClose,
  onSaved,
  onAccessDenied,
}: {
  scope: ContactScope;
  onClose: () => void;
  onSaved: (contact: Contact, created: boolean) => void;
  onAccessDenied: () => void;
}) {
  const [draft, setDraft] = useState<ContactInput>({
    first_name: "",
    last_name: "",
    phone: "",
    phone_country: "BR",
  });
  const [mode, setMode] = useState<Mode>("editing"),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [existing, setExisting] = useState<Contact | null>(null);
  const [fields, setFields] = useState<Record<string, string>>({});
  const running = useRef(false),
    alive = useRef(true),
    request = useRef<AbortController | null>(null);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      request.current?.abort();
    };
  }, []);
  function close() {
    if (running.current) return;
    if (
      mode === "uncertain" &&
      !window.confirm(
        "O cadastro pode ter sido concluído. Ao sair, confira o telefone na lista antes de cadastrar novamente. Deseja sair?",
      )
    )
      return;
    onClose();
  }
  function change(key: keyof ContactInput, value: string) {
    setDraft((d) => ({ ...d, [key]: value }));
    setFields((f) => ({ ...f, [key]: "" }));
    setError("");
  }
  async function submit() {
    if (running.current || mode !== "editing") return;
    const next: Record<string, string> = {};
    for (const key of ["first_name", "last_name"] as const) {
      if (
        (key === "first_name" && !draft[key].trim()) ||
        [...draft[key].trim()].length > 100 ||
        /[\u0000-\u001f\u007f-\u009f]/.test(draft[key])
      )
        next[key] =
          key === "first_name"
            ? "Informe um nome com até 100 caracteres."
            : "Use até 100 caracteres no sobrenome.";
    }
    if (!draft.phone.trim() || draft.phone.length > 40)
      next.phone = "Informe o telefone completo com DDD.";
    setFields(next);
    if (Object.keys(next).length) return;
    running.current = true;
    setBusy(true);
    setError("");
    const c = new AbortController();
    request.current = c;
    try {
      const saved = await createContact(scope, draft, c.signal);
      if (alive.current && !c.signal.aborted) onSaved(saved, true);
    } catch (cause) {
      if (!alive.current || c.signal.aborted) return;
      setError(errorMessage(cause));
      if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
        setMode("blocked");
        onAccessDenied();
      } else if (cause instanceof ApiError && cause.code === "contact_exists") setMode("conflict");
      else if (
        !(cause instanceof ApiError) ||
        cause.status === 0 ||
        cause.status >= 500 ||
        cause.code === "invalid_response"
      )
        setMode("uncertain");
    } finally {
      running.current = false;
      if (alive.current) setBusy(false);
    }
  }
  async function check() {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setError("");
    const c = new AbortController();
    request.current = c;
    try {
      const page = await listContacts(
        scope,
        {
          phone: draft.phone,
          phone_country: draft.phone.trim().startsWith("+") ? undefined : draft.phone_country,
          limit: 1,
        },
        c.signal,
      );
      if (!alive.current || c.signal.aborted) return;
      setExisting(page.items[0] ?? null);
      setMode(page.items.length ? "found" : "missing");
    } catch (cause) {
      if (!alive.current || c.signal.aborted) return;
      setError(errorMessage(cause));
      if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
        setMode("blocked");
        onAccessDenied();
      }
    } finally {
      running.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return (
    <Modal
      open
      onClose={close}
      title="Novo contato"
      subtitle="Nome e telefone para organizar os contatos da barbearia."
    >
      {error && <Alert tone="error">{error}</Alert>}
      {mode !== "blocked" && (
        <form
          className={styles.form}
          aria-label="Cadastrar contato"
          aria-busy={busy}
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <fieldset className={styles.fields} disabled={busy || mode !== "editing"}>
            <TextField
              label="Nome"
              name="first_name"
              autoComplete="given-name"
              required
              maxLength={100}
              value={draft.first_name}
              error={fields.first_name}
              onChange={(e) => change("first_name", e.target.value)}
            />
            <TextField
              label="Sobrenome (opcional)"
              name="last_name"
              autoComplete="family-name"
              maxLength={100}
              value={draft.last_name}
              error={fields.last_name}
              onChange={(e) => change("last_name", e.target.value)}
            />
            <TextField
              label="Telefone com DDD"
              name="phone"
              type="tel"
              autoComplete="tel"
              required
              maxLength={40}
              value={draft.phone}
              error={fields.phone}
              onChange={(e) => change("phone", e.target.value)}
              hint="Ex.: (11) 91234-5678. Para outro país, inclua + e o código internacional."
            />
          </fieldset>
          {mode === "uncertain" && (
            <Alert tone="warning">
              O cadastro pode ter sido concluído. Confira o contato pelo telefone antes de tentar
              novamente.
            </Alert>
          )}
          {mode === "conflict" && (
            <p>
              Este telefone já está cadastrado. Você pode conferir o contato existente ou corrigir
              os dados do novo cadastro.
            </p>
          )}
          {mode === "found" && existing && (
            <div className={styles.found}>
              <h3>Contato encontrado</h3>
              <p>{existing.name}</p>
              <p>{contactPhone(existing.phone)}</p>
              <p>Confira os dados: esse registro pode já existir antes da sua tentativa.</p>
            </div>
          )}
          {mode === "missing" && (
            <Alert>
              Nenhum contato foi encontrado com esse telefone. Você pode conferir novamente ou
              revisar os dados antes de tentar cadastrar.
            </Alert>
          )}
          <div className={styles.actions}>
            <Button variant="quiet" disabled={busy} onClick={close}>
              Fechar
            </Button>
            {mode === "editing" ? (
              <Button type="submit" loading={busy}>
                Cadastrar contato
              </Button>
            ) : (
              <>
                {mode !== "found" && (
                  <Button variant="secondary" loading={busy} onClick={() => void check()}>
                    Conferir contato pelo telefone
                  </Button>
                )}
                {(mode === "conflict" || mode === "missing") && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      setMode("editing");
                      setError("");
                    }}
                  >
                    Revisar cadastro
                  </Button>
                )}
                {mode === "found" && existing && (
                  <Button onClick={() => onSaved(existing, false)}>Concluir</Button>
                )}
              </>
            )}
          </div>
          <p className={styles.note}>
            Cadastrar um contato não cria agendamento nem autoriza mensagens de WhatsApp.
          </p>
        </form>
      )}
    </Modal>
  );
}
