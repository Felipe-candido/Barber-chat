"use client";

import { useState } from "react";
import type { StaffAccount } from "@/lib/api/staff-accounts";
import { Badge, Button, TextField } from "../ui";
import styles from "../admin/management.module.css";

export function AccountItem({
  account,
  professionalName,
  disabled,
  onStatus,
  onLink,
  onReplace,
}: {
  account: StaffAccount;
  professionalName: string;
  disabled: boolean;
  onStatus: () => void;
  onLink: (userID: string) => void;
  onReplace: (email: string) => void;
}) {
  const [recovery, setRecovery] = useState("");
  const [email, setEmail] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const linked = account.state === "linked";
  const newEmail = email.trim().toLowerCase();
  return (
    <li>
      <div className={styles.itemHeader}>
        <div>
          <strong>{professionalName}</strong>
          <p className={styles.email}>{account.email}</p>
        </div>
        <Badge tone={linked ? (account.active ? "success" : "neutral") : "warning"}>
          {linked
            ? account.active
              ? "Acesso ativo"
              : "Acesso inativo"
            : "Acesso precisa ser conferido"}
        </Badge>
      </div>
      {linked && (
        <div className={styles.actions}>
          <Button variant="secondary" disabled={disabled} onClick={onStatus}>
            {account.active ? "Desativar acesso" : "Ativar acesso"}
          </Button>
        </div>
      )}
      {!linked && (
        <details className={styles.details}>
          <summary>Recuperar acesso com ajuda do suporte</summary>
          <div>
            <p className={styles.hint}>
              Peça ao suporte o código de recuperação da conta deste barbeiro. Com ele, você pode
              concluir a liberação do acesso sem enviar outro convite.
            </p>
            <TextField
              label="Código de recuperação da conta"
              value={recovery}
              onChange={(event) => setRecovery(event.target.value.trim())}
              disabled={disabled}
              autoComplete="off"
              spellCheck={false}
            />
            <Button
              variant="secondary"
              disabled={disabled || !recovery}
              onClick={() => onLink(recovery)}
            >
              Concluir acesso
            </Button>
          </div>
        </details>
      )}
      {!account.active && (
        <details className={styles.details}>
          <summary>Substituir conta</summary>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (disabled || !confirmed || !newEmail || newEmail === account.email.toLowerCase())
                return;
              onReplace(newEmail);
            }}
          >
            <p className={styles.hint}>
              O acesso antigo será desvinculado somente desta barbearia. O barbeiro e seus
              agendamentos serão preservados. Se o convite não chegar, confira o acesso na lista
              antes de enviar outro.
            </p>
            <TextField
              label="Novo e-mail de acesso"
              type="email"
              required
              maxLength={254}
              autoComplete="off"
              value={email}
              disabled={disabled}
              onChange={(event) => {
                setEmail(event.target.value);
                setConfirmed(false);
              }}
            />
            <label className={styles.choice}>
              <input
                type="checkbox"
                required
                checked={confirmed}
                disabled={disabled}
                onChange={(event) => setConfirmed(event.target.checked)}
              />
              <span>Confirmo a remoção do vínculo antigo nesta barbearia.</span>
            </label>
            <Button
              variant="secondary"
              type="submit"
              disabled={
                disabled || !confirmed || !newEmail || newEmail === account.email.toLowerCase()
              }
            >
              Substituir e enviar convite
            </Button>
          </form>
        </details>
      )}
    </li>
  );
}
