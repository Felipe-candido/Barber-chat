import type { Metadata } from "next";
import Link from "next/link";
import { EmailConfirmationForm } from "@/components/auth/email-confirmation-form";
import { OnboardingShell } from "@/components/onboarding/onboarding-shell";
import { buttonClasses } from "@/components/ui/button";
import styles from "@/components/onboarding/onboarding.module.css";

export const metadata: Metadata = { title: "Confirmar e-mail" };

export default function Page() {
  return (
    <OnboardingShell step={2}>
      <div>
        <h1>Confirme seu e-mail</h1>
        <p className={styles.description}>
          O link expirou ou não chegou? Solicite uma nova confirmação para continuar seu cadastro.
        </p>
      </div>
      <EmailConfirmationForm />
      <Link href="/login" className={buttonClasses("secondary")}>
        Já confirmei, fazer login
      </Link>
    </OnboardingShell>
  );
}
