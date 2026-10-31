"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { LoaderCircle } from "lucide-react";
import { useAdminAccess } from "./admin-access-provider";
import { errorMessage } from "@/lib/api/client";
import { AccessPage } from "./ui/access-page";
import { Button, buttonClasses } from "./ui";
import styles from "./ui/access-page.module.css";

export function AdminAccessGuard({ children }: { children: React.ReactNode }) {
  const { state, refresh, logout } = useAdminAccess();
  const router = useRouter();
  const [logoutError, setLogoutError] = useState("");
  useEffect(() => {
    if (state.status === "idle") void refresh().catch(() => {});
    if (state.status === "error" && state.error.status === 401) router.replace("/login");
  }, [state, refresh, router]);

  if (state.status === "ready") return children;
  if (state.status !== "error" || state.error.status === 401) {
    return (
      <AccessPage title="Preparando seu painel">
        <p className={styles.loading} role="status">
          <LoaderCircle size={24} aria-hidden="true" />
          Validando seu acesso…
        </p>
      </AccessPage>
    );
  }
  return (
    <AccessPage title="Não foi possível liberar o painel" description={state.error.message} error>
      <p className={styles.text}>
        Uma conta no Supabase, sozinha, não libera o painel. Peça ao proprietário para conferir seu
        cadastro e o vínculo ativo com esta barbearia.
      </p>
      {logoutError && <p className={styles.text}>{logoutError}</p>}
      <div className={styles.actions}>
        <Button variant="primary" onClick={() => void refresh().catch(() => {})}>
          Tentar novamente
        </Button>
        <Button
          variant="secondary"
          onClick={async () => {
            try {
              await logout();
              router.replace("/login");
            } catch (error) {
              setLogoutError(errorMessage(error));
            }
          }}
        >
          Sair da conta
        </Button>
        <Link href="/login" className={buttonClasses("quiet")}>
          Voltar ao login
        </Link>
        <Link href="/cadastro/barbearia" className={buttonClasses("quiet")}>
          Cadastrar minha barbearia
        </Link>
      </div>
    </AccessPage>
  );
}
