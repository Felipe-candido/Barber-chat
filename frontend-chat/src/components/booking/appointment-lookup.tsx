"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AppointmentReceipt } from "../appointment-receipt";
import { Button, TextField } from "../ui";
import styles from "./booking.module.css";

export function AppointmentLookup({
  slug,
  onShopName,
  onLockedChange,
}: {
  slug: string;
  onShopName?: (name: string) => void;
  onLockedChange?: (locked: boolean) => void;
}) {
  const [code, setCode] = useState("");
  const [submitted, setSubmitted] = useState<{ code: string; attempt: number } | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const [locked, setLocked] = useState(false);
  const handleLocked = useCallback(
    (value: boolean) => {
      setLocked(value);
      onLockedChange?.(value);
    },
    [onLockedChange],
  );
  useEffect(() => {
    heading.current?.focus();
  }, []);
  return (
    <section className={styles.lookup}>
      <h2 ref={heading} tabIndex={-1}>
        Consultar meu agendamento
      </h2>
      <p className={styles.hint}>
        Use o código recebido ao concluir sua reserva nesta barbearia. Ele permite consultar as
        informações e cancelar antes do início.
      </p>
      <form
        className={styles.form}
        onSubmit={(event) => {
          event.preventDefault();
          if (locked) return;
          setSubmitted({ code: code.trim().toUpperCase(), attempt: (submitted?.attempt ?? 0) + 1 });
        }}
      >
        <TextField
          label="Código recebido"
          required
          maxLength={80}
          value={code}
          onChange={(event) => setCode(event.target.value)}
          autoComplete="off"
          autoCapitalize="characters"
          spellCheck={false}
          disabled={locked}
          hint="Cole o código completo, incluindo os hífens."
        />
        <Button type="submit" disabled={locked}>
          Consultar agendamento
        </Button>
      </form>
      {submitted && (
        <AppointmentReceipt
          key={submitted.attempt}
          slug={slug}
          code={submitted.code}
          onShopName={onShopName}
          onLockedChange={handleLocked}
        />
      )}
    </section>
  );
}
