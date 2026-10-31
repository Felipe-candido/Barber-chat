import Link from "next/link";
import { AccessPage } from "@/components/ui/access-page";
import { buttonClasses } from "@/components/ui/button";
import styles from "@/components/ui/access-page.module.css";
export default function Page() {
  return (
    <AccessPage
      title="Abra o link da sua barbearia"
      description="Cada barbearia tem seu próprio link de agendamento."
    >
      <p className={styles.text}>
        Peça à barbearia o endereço do catálogo, no formato /b/slug-da-barbearia. Este endereço
        genérico não seleciona uma unidade.
      </p>
      <p className={styles.text}>
        No link da unidade, você pode escolher um serviço e reservar um horário. Se já agendou, use
        seu código para consultar ou cancelar a reserva.
      </p>
      <Link className={buttonClasses("secondary")} href="/login">
        Acesso da equipe
      </Link>
    </AccessPage>
  );
}
