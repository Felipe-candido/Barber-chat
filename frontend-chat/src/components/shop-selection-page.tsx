"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowRight, LogOut, RefreshCw, Scissors } from "lucide-react";
import { useAdminAccess } from "./admin-access-provider";
import { adminShopPath } from "@/lib/api/config";
import { errorMessage } from "@/lib/api/client";
import { useFeedback } from "./feedback-provider";
import { Brand, Button, EmptyState, PageHeader, buttonClasses } from "./ui";
import styles from "./shop-selection-page.module.css";

export function ShopSelectionPage() {
  const { state, refresh, logout } = useAdminAccess();
  const router = useRouter();
  const { notify } = useFeedback();
  if (state.status !== "ready") return null;
  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <Brand />
        <Button
          variant="quiet"
          onClick={async () => {
            try {
              await logout();
              router.replace("/login");
            } catch (error) {
              notify(errorMessage(error), { tone: "error" });
            }
          }}
        >
          <LogOut size={18} aria-hidden="true" />
          Sair da conta
        </Button>
      </header>
      <PageHeader
        title="Escolha sua barbearia"
        description={`Olá, ${state.identity.display_name}. Em qual unidade você quer trabalhar?`}
        actions={
          <Button variant="quiet" onClick={() => void refresh().catch(() => {})}>
            <RefreshCw size={18} aria-hidden="true" />
            Atualizar vínculos
          </Button>
        }
      />
      {state.shops.length ? (
        <div className={styles.grid}>
          {state.shops.map((shop) => (
            <article key={shop.shop_id} className={styles.shop}>
              <span className={styles.shopIcon}>
                <Scissors size={24} aria-hidden="true" />
              </span>
              <h2>{shop.name}</h2>
              <p>
                Link de agendamento: <span>/b/{shop.slug}</span>
              </p>
              <Link
                className={buttonClasses("secondary", styles.enter)}
                href={adminShopPath(shop.slug) + "/servicos"}
              >
                Entrar em {shop.name}
                <ArrowRight size={18} aria-hidden="true" />
              </Link>
            </article>
          ))}
        </div>
      ) : (
        <EmptyState
          title="Nenhuma barbearia vinculada"
          text="Sua conta foi reconhecida, mas não possui acesso ativo a uma barbearia. Peça ao proprietário ou ao responsável pelo sistema para conferir seu vínculo e atualize a lista."
        />
      )}
      <p className={styles.note}>
        Somente as barbearias com acesso ativo para sua conta aparecem aqui.
      </p>
      <Link href="/cadastro/barbearia" className={buttonClasses("quiet")}>
        Cadastrar minha barbearia
      </Link>
    </main>
  );
}
