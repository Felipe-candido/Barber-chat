import Link from "next/link";
export default function Page() {
  return (
    <main className="access-screen">
      <section className="api-error">
        <h1>Abra o link da sua barbearia</h1>
        <p>
          Peça à barbearia o endereço do catálogo, no formato /b/slug-da-barbearia. Este endereço
          genérico não seleciona uma unidade.
        </p>
        <p>
          Os serviços já podem ser consultados; o agendamento de horários ainda não está disponível.
        </p>
        <Link className="button button-secondary" href="/login">
          Acesso da equipe
        </Link>
      </section>
    </main>
  );
}
