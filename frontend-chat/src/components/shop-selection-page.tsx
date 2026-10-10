"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowRight, LogOut, RefreshCw, Scissors } from "lucide-react";
import { useAdminAccess } from "./admin-access-provider";
import { adminShopPath } from "@/lib/api/config";
import { errorMessage } from "@/lib/api/client";
import { useFeedback } from "./feedback-provider";

export function ShopSelectionPage() {
  const { state, refresh, logout } = useAdminAccess();
  const router = useRouter();
  const { notify } = useFeedback();
  if (state.status !== "ready") return null;
  return (
    <main className="shop-selection-page">
      <header className="shop-selection-header">
        <span className="eyebrow">BARBER-CHAT · SEU ESPAÇO DE GESTÃO</span>
        <button
          className="button button-secondary"
          onClick={async () => {
            try {
              await logout();
              router.replace("/login");
            } catch (error) {
              notify(errorMessage(error));
            }
          }}
        >
          <LogOut size={16} />
          Sair da conta
        </button>
      </header>
      <div className="page-heading">
        <div>
          <h1>
            Escolha sua barbearia<span className="heading-dot">.</span>
          </h1>
          <p>Olá, {state.identity.display_name}. Em qual unidade você quer trabalhar?</p>
        </div>
        <button className="button button-secondary" onClick={() => void refresh().catch(() => {})}>
          <RefreshCw size={16} />
          Atualizar vínculos
        </button>
      </div>
      {state.shops.length ? (
        <div className="service-grid shop-selection-grid">
          {state.shops.map((shop) => (
            <article key={shop.shop_id} className="service-card">
              <span className="service-symbol">
                <Scissors size={25} />
              </span>
              <h2>{shop.name}</h2>
              <p>Endereço: {shop.slug}</p>
              <Link className="button button-primary" href={adminShopPath(shop.slug) + "/servicos"}>
                Entrar em {shop.name}
                <ArrowRight size={16} />
              </Link>
            </article>
          ))}
        </div>
      ) : (
        <section className="api-error" role="status">
          <Scissors size={28} />
          <h2>Nenhuma barbearia vinculada</h2>
          <p>
            Seu usuário foi reconhecido, mas não possui vínculo ativo com uma barbearia ativa. Peça
            ao responsável para cadastrar o vínculo no banco e depois atualize esta lista.
          </p>
        </section>
      )}
      <p className="inline-hint integration-note">
        Somente suas unidades ativas aparecem aqui. Usuários, barbearias e vínculos continuam sendo
        criados manualmente; esta tela não cria cadastros.
      </p>
    </main>
  );
}
