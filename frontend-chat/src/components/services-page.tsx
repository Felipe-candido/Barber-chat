"use client";
import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  ArrowUpRight,
  Clock3,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  Scissors,
  Search,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { publicBookingPath } from "@/lib/api/config";
import { useSelectedShop, useSelectedShopAccess } from "./shop-panel";
import { createService, priceToCents } from "@/lib/api/services";
import { errorMessage } from "@/lib/api/client";
import { useServices } from "@/hooks/use-services";
import { money } from "@/lib/model";
import { useFeedback } from "./feedback-provider";
import { EmptyState, Modal } from "./ui";

export function ServicesPage() {
  const shop = useSelectedShop();
  const owner = useSelectedShopAccess().role === "owner";
  const { services, loading, error, errorStatus, reload } = useServices(shop.slug, "admin");
  const [query, setQuery] = useState("");
  const [creating, setCreating] = useState(false);
  const filtered = services.filter((s) =>
    s.name.toLocaleLowerCase("pt-BR").includes(query.toLocaleLowerCase("pt-BR")),
  );
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">O CUIDADO LEVA A SUA ASSINATURA</div>
          <h1>
            Seu catálogo de serviços<span className="heading-dot">.</span>
          </h1>
          <p>Serviços ativos de {shop.name}.</p>
        </div>
        {owner && (
          <button
            className="button button-primary"
            disabled={errorStatus === 401 || errorStatus === 403}
            onClick={() => setCreating(true)}
          >
            <Plus size={18} />
            Adicionar serviço
          </button>
        )}
      </div>
      <div className="catalog-banner">
        <div className="catalog-banner-mark">
          <Scissors size={36} strokeWidth={1.2} />
        </div>
        <div>
          <span className="eyebrow">FEITO PARA CUIDAR</span>
          <h2>Um bom atendimento começa na escolha.</h2>
          <p>
            Os serviços abaixo estão disponíveis para agendar pelo chat ou pela agenda
            administrativa.
          </p>
        </div>
        <Link href={publicBookingPath(shop.slug)} className="button button-secondary">
          Ver como cliente
          <ArrowUpRight size={16} />
        </Link>
      </div>
      <div className="inline-hint integration-note">
        A listagem atual retorna somente serviços ativos. Edição, exclusão, categorias e ativação
        aguardam suporte do backend. Cada leitura e criação valida seu vínculo com esta barbearia.
      </div>
      <div className="catalog-toolbar">
        <button
          className="button button-secondary"
          disabled={loading}
          onClick={() => void reload()}
        >
          <RefreshCw size={15} />
          Atualizar lista
        </button>
        <label className="search-field">
          <Search size={16} />
          <input
            aria-label="Buscar serviço"
            placeholder="Buscar serviço"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
      </div>
      {loading ? (
        <div className="loading-screen" role="status">
          <LoaderCircle size={26} />
          <span>Carregando serviços da API…</span>
        </div>
      ) : error ? (
        <div className="api-error" role="alert">
          <AlertCircle size={22} />
          <h2>Não foi possível carregar o catálogo</h2>
          <p>{error}</p>
          <button className="button button-secondary" onClick={() => void reload()}>
            Tentar novamente
          </button>
          {(errorStatus === 401 || errorStatus === 403) && (
            <Link
              className="button button-secondary"
              href={errorStatus === 401 ? "/login" : "/admin"}
            >
              {errorStatus === 401 ? "Entrar novamente" : "Escolher outra barbearia"}
            </Link>
          )}
        </div>
      ) : (
        <>
          <div className="catalog-count">
            <span>{filtered.length} serviços ativos</span>
            <span>Dados consultados na API</span>
          </div>
          {filtered.length ? (
            <div className="service-grid">
              {filtered.map((s) => (
                <article className="service-card" key={s.id}>
                  <div className="service-card-top">
                    <span className="service-symbol">
                      <Scissors size={25} />
                    </span>
                    <span className="service-state">
                      <i />
                      {s.active ? "Ativo" : "Inativo"}
                    </span>
                  </div>
                  <span className="service-category">SERVIÇO</span>
                  <h2>{s.name}</h2>
                  <p>{s.description || "Sem descrição."}</p>
                  <div className="service-card-price">
                    <strong>{money(s.price_cents)}</strong>
                    <span>
                      <Clock3 size={15} />
                      {s.duration_minutes} min
                    </span>
                  </div>
                  <div className="service-card-actions">
                    <button disabled title="A API ainda não oferece edição">
                      <Pencil size={15} />
                      Editar serviço
                    </button>
                    <button
                      disabled
                      className="delete-service"
                      title="A API ainda não oferece exclusão"
                      aria-label={"Excluir " + s.name}
                    >
                      <Trash2 size={16} />
                    </button>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <EmptyState
              title={query ? "Nenhum resultado" : "Nenhum serviço ativo"}
              text={
                query
                  ? "Tente buscar por outro nome."
                  : "A API retornou uma lista vazia para esta barbearia."
              }
            />
          )}
        </>
      )}
      {owner && creating && (
        <ServiceForm
          slug={shop.slug}
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false);
            void reload();
          }}
        />
      )}
    </>
  );
}
function ServiceForm({
  slug,
  onClose,
  onCreated,
}: {
  slug: string;
  onClose: () => void;
  onCreated: () => void;
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
      onClose={() => {
        if (!inFlight.current) onClose();
      }}
      title="Adicionar serviço"
      subtitle="O serviço será criado ativo, com preço em reais."
    >
      <form
        className="form-stack"
        onSubmit={async (e) => {
          e.preventDefault();
          if (inFlight.current) return;
          const values = new FormData(e.currentTarget);
          setError("");
          try {
            const input = {
              name: String(values.get("name")),
              description: String(values.get("description")),
              duration_minutes: Number(values.get("duration")),
              price_cents: priceToCents(String(values.get("price"))),
            };
            inFlight.current = true;
            setSaving(true);
            const controller = new AbortController();
            request.current = controller;
            const saved = await createService(slug, input, controller.signal);
            if (controller.signal.aborted) return;
            notify("Serviço “" + saved.name + "” criado na API.");
            onCreated();
          } catch (error) {
            if (!request.current?.signal.aborted) setError(errorMessage(error));
          } finally {
            inFlight.current = false;
            if (!request.current?.signal.aborted) setSaving(false);
          }
        }}
      >
        <fieldset
          disabled={saving}
          className="form-stack"
          style={{ border: 0, padding: 0, margin: 0 }}
        >
          <label>
            Nome do serviço
            <input name="name" required placeholder="Ex.: Corte clássico" />
          </label>
          <label>
            Descrição <span className="optional">opcional</span>
            <textarea name="description" rows={3} placeholder="Descreva o serviço" />
          </label>
          <div className="field-row">
            <label>
              Preço (R$)
              <input name="price" type="text" inputMode="decimal" required placeholder="45,00" />
            </label>
            <label>
              Duração (minutos)
              <input
                name="duration"
                type="number"
                min={1}
                max={2147483647}
                step={1}
                required
                defaultValue={30}
              />
            </label>
          </div>
        </fieldset>
        {error && (
          <div className="form-error" role="alert">
            <AlertCircle size={17} />
            {error}
          </div>
        )}
        <div className="modal-actions">
          <button
            type="button"
            className="button button-secondary"
            disabled={saving}
            onClick={onClose}
          >
            Voltar
          </button>
          <button className="button button-primary" disabled={saving}>
            {saving ? (
              <>
                <LoaderCircle size={16} />
                Salvando…
              </>
            ) : (
              "Adicionar serviço"
            )}
          </button>
        </div>
      </form>
    </Modal>
  );
}
