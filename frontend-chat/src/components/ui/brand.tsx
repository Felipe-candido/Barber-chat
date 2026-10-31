import { CalendarDays, Scissors } from "lucide-react";
import styles from "./foundation.module.css";

export function ProductBrand({
  compact = false,
  light = false,
}: {
  compact?: boolean;
  light?: boolean;
}) {
  return (
    <span className={`${styles.brand} ${light ? styles.brandLight : ""}`}>
      <span className={styles.brandMark} aria-hidden="true">
        <CalendarDays size={22} />
      </span>
      {compact ? <span className={styles.srOnly}>Barber-chat</span> : <strong>Barber-chat</strong>}
    </span>
  );
}

// A neutral public identity until the API supplies the establishment's name.
// This is presentation only: it never selects or authorizes a shop.
export function ShopBrand({ name }: { name?: string }) {
  return (
    <span className={styles.brand}>
      <span className={styles.shopMark} aria-hidden="true">
        <Scissors size={22} />
      </span>
      <strong>{name || "Agendamento online"}</strong>
    </span>
  );
}
