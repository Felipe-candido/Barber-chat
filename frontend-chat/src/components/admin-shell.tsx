"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  CalendarDays,
  Scissors,
  MessageCircle,
  ArrowUpRight,
  Copy,
  PanelLeftClose,
  Menu,
} from "lucide-react";
import { useState } from "react";
import { publicBookingPath, configuredShopSlug } from "@/lib/api/config";
import { Brand } from "./ui";
import { useFeedback } from "./feedback-provider";

export function AdminShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { notify } = useFeedback();
  const [open, setOpen] = useState(false);
  async function copyLink() {
    if (!configuredShopSlug) {
      notify("Configure NEXT_PUBLIC_SHOP_SLUG antes de compartilhar o link.");
      return;
    }
    try {
      await navigator.clipboard.writeText(window.location.origin + publicBookingPath);
      notify("Link de agendamento copiado!");
    } catch {
      notify("Acesse o chat e copie o endereço na barra do navegador.");
    }
  }
  return (
    <div className="admin-layout">
      {open && (
        <button
          className="sidebar-backdrop"
          aria-label="Fechar navegação"
          onClick={() => setOpen(false)}
        />
      )}
      <aside className={"sidebar " + (open ? "sidebar-open" : "")}>
        <Link href="/admin" className="brand-link" aria-label="Palma Barbearia — agenda">
          <Brand light />
        </Link>
        <button
          className="mobile-close icon-button"
          aria-label="Fechar menu"
          onClick={() => setOpen(false)}
        >
          <PanelLeftClose size={20} />
        </button>
        <div className="sidebar-caption">SEU ESPAÇO DE GESTÃO</div>
        <nav aria-label="Navegação principal">
          <Link
            className={"nav-item " + (pathname === "/admin" ? "active" : "")}
            href="/admin"
            onClick={() => setOpen(false)}
          >
            <CalendarDays size={20} />
            Agenda
            <span className="nav-dot" />
          </Link>
          <Link
            className={"nav-item " + (pathname === "/admin/servicos" ? "active" : "")}
            href="/admin/servicos"
            onClick={() => setOpen(false)}
          >
            <Scissors size={20} />
            Serviços
          </Link>
        </nav>
        <div className="sidebar-booking">
          <span className="sidebar-booking-icon">
            <MessageCircle size={22} />
          </span>
          <h3>
            Menos mensagens.
            <br />
            Mais tempo para cuidar.
          </h3>
          <p>Seu cliente consulta os serviços. Agendamentos em breve.</p>
          <Link href={publicBookingPath}>
            Abrir catálogo público
            <ArrowUpRight size={16} />
          </Link>
          <button onClick={copyLink}>
            <Copy size={14} />
            Copiar link do chat
          </button>
        </div>
        <div className="sidebar-bottom">
          <div className="demo-note">
            <span />
            Catálogo conectado à API
          </div>
          <div className="profile">
            <span className="avatar">BC</span>
            <div>
              <strong>Barber-chat</strong>
              <small>Autenticação pendente</small>
            </div>
            <span className="profile-dot" />
          </div>
        </div>
      </aside>
      <div className="admin-workspace">
        <header className="topbar">
          <div className="flex items-center gap-3">
            <button
              className="mobile-menu icon-button"
              aria-label="Abrir menu"
              onClick={() => setOpen(true)}
            >
              <Menu size={21} />
            </button>
            <span>
              PALMA <span className="breadcrumb-separator">/</span>{" "}
              <strong>{pathname.includes("servicos") ? "Serviços" : "Agenda"}</strong>
            </span>
          </div>
          <div className="topbar-right">
            <span className="demo-pill">
              <span />
              Integração parcial
            </span>
            <Link href="/login" className="avatar avatar-small" aria-label="Abrir tela de acesso">
              BC
            </Link>
          </div>
        </header>
        <main className="main-content">{children}</main>
        <footer className="admin-footer">
          <span>PALMA BARBEARIA</span>
          <span>Cuidado em cada detalhe.</span>
        </footer>
      </div>
    </div>
  );
}
