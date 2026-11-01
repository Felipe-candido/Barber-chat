import Link from "next/link";
import { LayoutGrid } from "lucide-react";
import type { ShopAccess } from "@/lib/api/identity";
import { adminDestinations } from "./navigation";
import styles from "../admin-shell.module.css";

// Phones and tablets: the four daily sections stay one tap away; everything else lives in "Mais".
export function BottomNavigation({
  slug,
  role,
  pathname,
  menuId,
  menuOpen,
  onOpenMenu,
}: {
  slug: string;
  role: ShopAccess["role"];
  pathname: string;
  menuId: string;
  menuOpen: boolean;
  onOpenMenu: () => void;
}) {
  const primary = adminDestinations(slug, role).slice(0, 4);
  const elsewhere = !primary.some((item) => item.href === pathname);
  return (
    <nav aria-label="Navegação rápida" className={styles.tabBar} data-tab-bar>
      {primary.map(({ href, label, icon: Icon }) => (
        <Link
          key={href}
          href={href}
          className={styles.tab}
          aria-current={pathname === href ? "page" : undefined}
        >
          <span className={styles.tabIcon}>
            <Icon size={21} aria-hidden="true" />
          </span>
          <span className={styles.tabLabel}>{label}</span>
        </Link>
      ))}
      <button
        type="button"
        className={styles.tab}
        aria-controls={menuId}
        aria-expanded={menuOpen}
        data-active={elsewhere || menuOpen || undefined}
        onClick={onOpenMenu}
      >
        <span className={styles.tabIcon}>
          <LayoutGrid size={21} aria-hidden="true" />
        </span>
        <span className={styles.tabLabel}>Mais</span>
      </button>
    </nav>
  );
}
