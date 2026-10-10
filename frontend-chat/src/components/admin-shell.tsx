"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  CalendarDays,
  Scissors,
  MessageCircle,
  ArrowUpRight,
  Copy,
  PanelLeftClose,
  Menu,
  LogOut,
  Building2,
  Users,
  UserRoundCog,
} from "lucide-react";
import { useState } from "react";
import { publicBookingPath, adminShopPath } from "@/lib/api/config";
import type { AccessibleShop, StaffIdentity, ShopAccess } from "@/lib/api/identity";
import { errorMessage } from "@/lib/api/client";
import { useAdminAccess } from "./admin-access-provider";
import { Brand } from "./ui";
import { useFeedback } from "./feedback-provider";

export function AdminShell({
  children,
  shop,
  identity,
  access,
}: {
  children: React.ReactNode;
  shop: AccessibleShop;
  identity: StaffIdentity;
  access: ShopAccess;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const { logout } = useAdminAccess();
  const { notify } = useFeedback();
  const [open, setOpen] = useState(false);
  const panelPath = adminShopPath(shop.slug);
  const bookingPath = publicBookingPath(shop.slug);
  const initials = identity.display_name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => Array.from(part)[0])
    .join("")
    .toLocaleUpperCase("pt-BR");
  async function copyLink() {
    try {
      await navigator.clipboard.writeText(window.location.origin + bookingPath);
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
        <Link href={panelPath} className="brand-link" aria-label={shop.name + " — agenda"}>
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
            className={"nav-item " + (pathname === panelPath ? "active" : "")}
            href={panelPath}
            onClick={() => setOpen(false)}
          >
            <CalendarDays size={20} />
            Agenda
            <span className="nav-dot" />
          </Link>
          <Link
            className={"nav-item " + (pathname === panelPath + "/servicos" ? "active" : "")}
            href={panelPath + "/servicos"}
            onClick={() => setOpen(false)}
          >
            <Scissors size={20} />
            Serviços
          </Link>
          <Link className="nav-item" href="/admin" onClick={() => setOpen(false)}>
            <Building2 size={20} />
            Trocar barbearia
          </Link>
          {access.role === "owner" && (
            <Link
              className={"nav-item " + (pathname === panelPath + "/barbeiros" ? "active" : "")}
              href={panelPath + "/barbeiros"}
              onClick={() => setOpen(false)}
            >
              <Users size={20} />
              Barbeiros
            </Link>
          )}
          {access.role === "owner" && (
            <Link
              className={"nav-item " + (pathname === panelPath + "/contas" ? "active" : "")}
              href={panelPath + "/contas"}
              onClick={() => setOpen(false)}
            >
              <UserRoundCog size={20} />
              Contas de acesso
            </Link>
          )}
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
          <p>Seu cliente escolhe o serviço, o barbeiro e reserva um horário pelo chat.</p>
          <Link href={bookingPath}>
            Abrir chat de agendamento
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
            <span className="avatar">{initials}</span>
            <div>
              <strong>{identity.display_name}</strong>
              <small>{shop.name}</small>
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
            <span className="shop-breadcrumb">
              {shop.name} <span className="breadcrumb-separator">/</span>{" "}
              <strong>
                {pathname.includes("servicos")
                  ? "Serviços"
                  : pathname.includes("barbeiros")
                    ? "Barbeiros"
                    : pathname.includes("contas")
                      ? "Contas de acesso"
                      : "Agenda"}
              </strong>
            </span>
          </div>
          <div className="topbar-right">
            <span className="demo-pill">
              <span />
              {access.role === "owner" ? "Proprietário" : "Barbeiro"}
            </span>
            <button
              className="icon-button"
              aria-label="Sair da conta"
              onClick={async () => {
                try {
                  await logout();
                  router.replace("/login");
                } catch (error) {
                  notify(errorMessage(error));
                }
              }}
            >
              <LogOut size={20} />
            </button>
          </div>
        </header>
        <main className="main-content">{children}</main>
        <footer className="admin-footer">
          <span>{shop.name}</span>
          <span>Cuidado em cada detalhe.</span>
        </footer>
      </div>
    </div>
  );
}
