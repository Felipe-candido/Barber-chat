"use client";
import { useState } from "react";
import {
  Clock3,
  Plus,
  Search,
  Scissors,
  Pencil,
  Trash2,
  Sparkles,
  ArrowUpRight,
  CircleCheck,
  Layers3,
} from "lucide-react";
import Link from "next/link";
import { Category, Service, money } from "@/lib/model";
import { useDemo } from "./demo-provider";
import { EmptyState, LoadingScreen, Modal } from "./ui";

const categories: Category[] = ["Cabelo", "Barba", "Combos", "Cuidados"];
export function ServicesPage() {
  const { data, ready, deleteService } = useDemo();
  const [category, setCategory] = useState("Todos");
  const [query, setQuery] = useState("");
  const [editing, setEditing] = useState<Service | "new" | null>(null);
  const [deleting, setDeleting] = useState<Service | null>(null);
  if (!ready) return <LoadingScreen />;
  const services = data.services.filter(
    (s) =>
      (category === "Todos" || s.category === category) &&
      s.name.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">O CUIDADO LEVA A SUA ASSINATURA</div>
          <h1>
            Seu catálogo de serviços<span className="heading-dot">.</span>
          </h1>
          <p>Organize o que você faz de melhor. Seu cliente cuida da escolha.</p>
        </div>
        <button className="button button-primary" onClick={() => setEditing("new")}>
          <Plus size={18} />
          Adicionar serviço
        </button>
      </div>
      <div className="catalog-banner">
        <div className="catalog-banner-mark">
          <Scissors size={36} strokeWidth={1.2} />
        </div>
        <div>
          <span className="eyebrow">FEITO PARA CUIDAR</span>
          <h2>Um bom atendimento começa na escolha.</h2>
          <p>Os serviços ativos ficam disponíveis no seu chat de agendamento.</p>
        </div>
        <Link href="/chat" className="button button-secondary">
          Ver como cliente
          <ArrowUpRight size={16} />
        </Link>
      </div>
      <div className="catalog-toolbar">
        <div className="category-tabs" aria-label="Categorias de serviços">
          {["Todos", ...categories].map((c) => (
            <button key={c} onClick={() => setCategory(c)} aria-pressed={category === c}>
              {c}
              {c === "Todos" && <span>{data.services.length}</span>}
            </button>
          ))}
        </div>
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
      <div className="catalog-count">
        <span>{services.length} serviços no catálogo</span>
        <span>
          <CircleCheck size={14} />
          {data.services.filter((s) => s.active).length} disponíveis para agendar
        </span>
      </div>
      {services.length ? (
        <div className="service-grid">
          {services.map((s) => (
            <article className={"service-card " + (!s.active ? "service-inactive" : "")} key={s.id}>
              <div className="service-card-top">
                <span className={"service-symbol category-" + s.category.toLowerCase()}>
                  {s.category === "Cuidados" ? (
                    <Sparkles size={25} strokeWidth={1.5} />
                  ) : s.category === "Combos" ? (
                    <Layers3 size={25} strokeWidth={1.5} />
                  ) : (
                    <Scissors size={25} strokeWidth={1.5} />
                  )}
                </span>
                <span className={"service-state " + (s.active ? "" : "inactive")}>
                  <i />
                  {s.active ? "Ativo" : "Inativo"}
                </span>
              </div>
              <span className="service-category">{s.category}</span>
              <h2>{s.name}</h2>
              <p>{s.description}</p>
              <div className="service-card-price">
                <strong>{money(s.price)}</strong>
                <span>
                  <Clock3 size={15} />
                  {s.duration} min
                </span>
              </div>
              <div className="service-card-actions">
                <button onClick={() => setEditing(s)}>
                  <Pencil size={15} />
                  Editar serviço
                </button>
                <button
                  aria-label={"Excluir " + s.name}
                  className="delete-service"
                  onClick={() => setDeleting(s)}
                >
                  <Trash2 size={16} />
                </button>
              </div>
            </article>
          ))}
        </div>
      ) : (
        <EmptyState
          icon={<Scissors size={26} />}
          title="Nenhum serviço por aqui"
          text={
            query || category !== "Todos"
              ? "Tente outra busca ou categoria."
              : "Adicione seu primeiro serviço para começar a agendar."
          }
        >
          <button className="button button-primary" onClick={() => setEditing("new")}>
            <Plus size={16} />
            Adicionar serviço
          </button>
        </EmptyState>
      )}
      <p className="catalog-footnote">
        <span className="mini-dot" />
        Você pode desativar um serviço temporariamente sem excluir seu cadastro.
      </p>
      {editing && (
        <ServiceForm
          service={editing === "new" ? undefined : editing}
          onClose={() => setEditing(null)}
        />
      )}
      {deleting && (
        <Modal
          open
          onClose={() => setDeleting(null)}
          title="Excluir serviço?"
          subtitle="Confira antes de continuar."
        >
          <p className="dialog-copy">
            O serviço <strong>{deleting.name}</strong> deixará de aparecer no catálogo e no chat.
            Agendamentos existentes manterão suas informações.
          </p>
          <div className="modal-actions">
            <button className="button button-secondary" onClick={() => setDeleting(null)}>
              Manter serviço
            </button>
            <button
              className="button button-danger"
              onClick={() => {
                deleteService(deleting.id);
                setDeleting(null);
              }}
            >
              Excluir serviço
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}
function ServiceForm({ service, onClose }: { service?: Service; onClose: () => void }) {
  const { saveService, notify } = useDemo();
  const [active, setActive] = useState(service?.active ?? true);
  const [error, setError] = useState("");
  return (
    <Modal
      open
      onClose={onClose}
      title={service ? "Editar serviço" : "Adicionar serviço"}
      subtitle="Os detalhes fazem toda a diferença."
    >
      <form
        className="form-stack"
        onSubmit={(e) => {
          e.preventDefault();
          const values = new FormData(e.currentTarget);
          const price = Math.round(Number(String(values.get("price")).replace(",", ".")) * 100);
          const duration = Number(values.get("duration"));
          const name = String(values.get("name")).trim();
          if (
            !name ||
            !Number.isFinite(price) ||
            price < 0 ||
            !Number.isInteger(duration) ||
            duration < 5 ||
            duration > 600
          ) {
            setError("Revise o nome, o preço e a duração do serviço.");
            return;
          }
          saveService({
            id: service?.id ?? crypto.randomUUID(),
            name,
            description: String(values.get("description")).trim(),
            price,
            duration,
            category: values.get("category") as Category,
            active,
          });
          notify(service ? "Serviço atualizado!" : "Serviço adicionado ao catálogo!");
          onClose();
        }}
      >
        <label>
          Nome do serviço
          <input
            name="name"
            required
            maxLength={80}
            defaultValue={service?.name}
            placeholder="Ex.: Corte clássico"
          />
        </label>
        <label>
          Descrição <span className="optional">opcional</span>
          <textarea
            name="description"
            rows={2}
            maxLength={240}
            defaultValue={service?.description}
            placeholder="Conte o que torna esse cuidado especial"
          />
        </label>
        <label>
          Categoria
          <select name="category" defaultValue={service?.category ?? "Cabelo"}>
            {categories.map((c) => (
              <option key={c}>{c}</option>
            ))}
          </select>
        </label>
        <div className="field-row">
          <label>
            Preço (R$)
            <input
              name="price"
              type="number"
              inputMode="decimal"
              min="0"
              max="99999"
              step="0.01"
              required
              defaultValue={service ? service.price / 100 : ""}
              placeholder="45,00"
            />
          </label>
          <label>
            Duração (minutos)
            <input
              name="duration"
              type="number"
              min="5"
              max="600"
              step="5"
              required
              defaultValue={service?.duration ?? 30}
            />
          </label>
        </div>
        <div className="toggle-field">
          <div>
            <strong>Disponível para agendamento</strong>
            <p>Exibir este serviço no chat dos clientes.</p>
          </div>
          <button
            type="button"
            className={"switch " + (active ? "on" : "")}
            role="switch"
            aria-checked={active}
            aria-label="Disponível para agendamento"
            onClick={() => setActive(!active)}
          >
            <span />
          </button>
        </div>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        <div className="modal-actions">
          <button type="button" className="button button-secondary" onClick={onClose}>
            Voltar
          </button>
          <button className="button button-primary">
            {service ? "Salvar alterações" : "Adicionar serviço"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
