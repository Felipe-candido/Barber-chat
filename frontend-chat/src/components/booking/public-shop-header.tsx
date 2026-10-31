import { ShopBrand } from "../ui";
import styles from "./booking.module.css";

// Presentation only: tenant selection stays in the route and API calls.
// Logo, cover and accent configuration can be added here when the API supports them.
export function PublicShopHeader({ name, slug }: { name?: string; slug: string }) {
  return (
    <header className={styles.header}>
      <div>
        <ShopBrand name={name} />
        <p>{name ? "Agendamento online" : `Link da barbearia: ${slug}`}</p>
      </div>
      <span className={styles.headerNote}>Sem criar uma conta</span>
    </header>
  );
}
