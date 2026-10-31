import Link from "next/link";
import type { ReactNode } from "react";
import { Brand } from "../ui";
import styles from "./onboarding.module.css";

export function OnboardingShell({ step, children }: { step: 1 | 2 | 3; children: ReactNode }) {
  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Link href="/login" aria-label="Barber-chat — início">
          <Brand />
        </Link>
        <Link href="/login">Já tenho conta</Link>
      </header>
      <main className={styles.main}>
        <nav aria-label="Etapas do cadastro">
          <ol className={styles.steps}>
            {["Sua conta", "Confirmar e-mail", "Sua barbearia"].map((label, index) => (
              <li
                key={label}
                aria-current={step === index + 1 ? "step" : undefined}
                data-complete={step > index + 1 || undefined}
              >
                <span aria-hidden="true">{index + 1}</span>
                {label}
              </li>
            ))}
          </ol>
        </nav>
        <section className={styles.panel}>{children}</section>
        <p className={styles.note}>
          Primeiro, organize sua barbearia. A assinatura será necessária para publicar o link e
          começar a receber agendamentos.
        </p>
      </main>
      <footer className={styles.footer}>Barber-chat · Sua rotina, mais simples</footer>
    </div>
  );
}
