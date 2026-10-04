"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AlertCircle, LoaderCircle } from "lucide-react";
import { useAdminAccess } from "./admin-access-provider";
import { errorMessage } from "@/lib/api/client";

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
      <main className="loading-screen access-screen" role="status">
        <LoaderCircle size={26} />
        <span>Validando seu acesso…</span>
      </main>
    );
  }
  return (
    <main className="access-screen">
      <section className="api-error" role="alert">
        <AlertCircle size={24} />
        <h1>Não foi possível liberar o painel</h1>
        <p>{state.error.message}</p>
        <p>
          As contas e os vínculos ainda são cadastrados manualmente. Uma conta no Supabase, sozinha,
          não libera o painel.
        </p>
        {logoutError && <p>{logoutError}</p>}
        <div className="access-actions">
          <button
            className="button button-secondary"
            onClick={() => void refresh().catch(() => {})}
          >
            Tentar novamente
          </button>
          <button
            className="button button-secondary"
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
          </button>
          <Link href="/login" className="button button-secondary">
            Voltar ao login
          </Link>
        </div>
      </section>
    </main>
  );
}
