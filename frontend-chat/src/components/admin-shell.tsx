"use client";
import { useCallback, useId, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { LogOut, Menu } from "lucide-react";
import type { AccessibleShop, StaffIdentity, ShopAccess } from "@/lib/api/identity";
import { errorMessage } from "@/lib/api/client";
import { useAdminAccess } from "./admin-access-provider";
import { useFeedback } from "./feedback-provider";
import { AdminNavigation, adminDestinations } from "./admin/navigation";
import { NavigationDrawer } from "./admin/navigation-drawer";
import styles from "./admin-shell.module.css";

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
  const drawerId = useId();
  const contentId = useId();
  const closeMenu = useCallback(() => setOpen(false), []);
  const section =
    adminDestinations(shop.slug, access.role).find((item) => item.href === pathname)?.label ??
    "Painel";
  return (
    <div className={styles.layout}>
      <a href={"#" + contentId} className={styles.skipLink}>
        Ir para o conteúdo
      </a>
      <aside className={styles.desktopSidebar} aria-label="Navegação do painel">
        <AdminNavigation shop={shop} identity={identity} access={access} pathname={pathname} />
      </aside>
      <NavigationDrawer open={open} onClose={closeMenu} id={drawerId}>
        <AdminNavigation
          shop={shop}
          identity={identity}
          access={access}
          pathname={pathname}
          onNavigate={closeMenu}
        />
      </NavigationDrawer>
      <div className={styles.workspace}>
        <header className={styles.topbar + " topbar"}>
          <div className={styles.location}>
            <button
              type="button"
              className={styles.menuButton + " icon-button"}
              aria-label="Abrir menu"
              aria-controls={drawerId}
              aria-expanded={open}
              onClick={() => setOpen(true)}
            >
              <Menu size={22} aria-hidden="true" />
            </button>
            <div>
              <strong>{shop.name}</strong>
              <span>{section}</span>
            </div>
          </div>
          <div className={styles.account}>
            <span className={styles.role}>
              {access.role === "owner" ? "Proprietário" : "Barbeiro"}
            </span>
            <button
              type="button"
              className="icon-button"
              aria-label="Sair da conta"
              onClick={async () => {
                try {
                  await logout();
                  router.replace("/login");
                } catch (error) {
                  notify(errorMessage(error), { tone: "error" });
                }
              }}
            >
              <LogOut size={20} aria-hidden="true" />
            </button>
          </div>
        </header>
        <main id={contentId} tabIndex={-1} className={styles.main}>
          {children}
        </main>
        <footer className={styles.footer}>
          <span>{shop.name}</span>
          <span>Barber-chat</span>
        </footer>
      </div>
    </div>
  );
}
