import type { ReactNode } from "react";
import { CircleAlert } from "lucide-react";
import { ProductBrand } from "./brand";
import styles from "./access-page.module.css";

// Shared presentation only: callers still own authentication and authorization.
export function AccessPage({
  title,
  description,
  error = false,
  children,
}: {
  title: string;
  description?: string;
  error?: boolean;
  children: ReactNode;
}) {
  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <ProductBrand />
      </header>
      <section className={styles.panel} aria-label={title} role={error ? "alert" : undefined}>
        <div className={styles.heading}>
          {error && <CircleAlert className={styles.errorIcon} size={24} aria-hidden="true" />}
          <h1>{title}</h1>
          {description && <p>{description}</p>}
        </div>
        {children}
      </section>
      <footer className={styles.footer}>Barber-chat · Agendamento e gestão de barbearias</footer>
    </main>
  );
}
