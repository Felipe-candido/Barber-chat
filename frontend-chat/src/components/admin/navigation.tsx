import Link from "next/link";
import {
  Building2,
  CalendarDays,
  Package,
  ReceiptText,
  Scissors,
  Settings,
  Users,
  UserRoundCog,
} from "lucide-react";
import type { AccessibleShop, ShopAccess, StaffIdentity } from "@/lib/api/identity";
import { adminShopPath } from "@/lib/api/config";
import { Brand } from "../ui";
import styles from "../admin-shell.module.css";

export function adminDestinations(slug: string, role: ShopAccess["role"]) {
  const root = adminShopPath(slug);
  return [
    { href: root, label: "Agenda", icon: CalendarDays },
    { href: root + "/servicos", label: "Serviços", icon: Scissors },
    { href: root + "/produtos", label: "Produtos", icon: Package },
    { href: root + "/vendas", label: "Vendas", icon: ReceiptText },
    ...(role === "owner"
      ? [
          { href: root + "/barbeiros", label: "Barbeiros", icon: Users },
          { href: root + "/contas", label: "Contas de acesso", icon: UserRoundCog },
          { href: root + "/configuracoes", label: "Configurações", icon: Settings },
        ]
      : []),
  ];
}

export function AdminNavigation({
  shop,
  identity,
  access,
  pathname,
  onNavigate,
}: {
  shop: AccessibleShop;
  identity: StaffIdentity;
  access: ShopAccess;
  pathname: string;
  onNavigate?: () => void;
}) {
  const initials = identity.display_name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => Array.from(part)[0])
    .join("")
    .toLocaleUpperCase("pt-BR");
  return (
    <>
      <Link
        href={adminShopPath(shop.slug)}
        aria-label={shop.name + " — agenda"}
        className={styles.brandLink}
        onClick={onNavigate}
      >
        <Brand />
      </Link>
      <div className={styles.unit}>
        <span>Barbearia selecionada</span>
        <strong>{shop.name}</strong>
        <Link href="/admin" onClick={onNavigate}>
          <Building2 size={16} aria-hidden="true" />
          Trocar barbearia
        </Link>
      </div>
      <nav aria-label="Navegação principal" className={styles.navigation}>
        {adminDestinations(shop.slug, access.role).map(({ href, label, icon: Icon }) => (
          <Link
            key={href}
            href={href}
            onClick={onNavigate}
            aria-current={pathname === href ? "page" : undefined}
            className={styles.navItem}
          >
            <Icon size={20} aria-hidden="true" />
            {label}
          </Link>
        ))}
      </nav>
      <section className={styles.bookingLink} aria-label="Compartilhar agendamento">
        <h2>Agendamento online</h2>
        {access.role === "owner" ? (
          <>
            <p>Confira a publicação antes de compartilhar com seus clientes.</p>
            <Link
              href={adminShopPath(shop.slug) + "/configuracoes#publication"}
              onClick={onNavigate}
            >
              Gerenciar link de agendamento
            </Link>
          </>
        ) : (
          <p>O proprietário gerencia a publicação e o link para seus clientes.</p>
        )}
      </section>
      <div className={styles.profile}>
        <span className={styles.avatar} aria-hidden="true">
          {initials}
        </span>
        <div>
          <strong>{identity.display_name}</strong>
          <span>{access.role === "owner" ? "Proprietário" : "Barbeiro"}</span>
        </div>
      </div>
    </>
  );
}
