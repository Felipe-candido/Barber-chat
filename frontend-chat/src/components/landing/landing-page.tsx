import Image from "next/image";
import Link from "next/link";
import { Check } from "lucide-react";
import { PRICE_LABEL, PRODUCT_NAME, TRIAL_DAYS, whatsAppHref } from "./landing-config";
import styles from "./landing.module.css";
import { InstallAppButton } from "../pwa/install-app-button";

const SIGNUP = "/cadastro";

function faq() {
  return [
    {
      q: "Meu cliente precisa baixar app?",
      a: "Não. Ele abre o link no navegador e agenda sem criar conta.",
    },
    {
      q: "Funciona com mais de um barbeiro?",
      a: "Sim. Cada barbeiro tem acesso e agenda próprios.",
    },
    {
      q: "O WhatsApp sai de qual número?",
      a: "Do número da sua barbearia, conectado por QR code. Só recebe mensagem o cliente que autorizou.",
    },
    {
      q: "E se o cliente quiser desmarcar?",
      a: "Ele cancela pelo código do agendamento e o horário volta a ficar livre.",
    },
    {
      q: "Dois clientes podem marcar o mesmo horário?",
      a: "Não. O sistema bloqueia a reserva dupla.",
    },
    {
      q: "Preciso pagar para começar?",
      a: TRIAL_DAYS
        ? `Não. Criar a conta e configurar a barbearia é grátis. Ao ativar o link de agendamento, os primeiros ${TRIAL_DAYS} dias são grátis e a mensalidade só é cobrada depois. Se cancelar antes, não paga nada.`
        : "Não. Criar a conta e configurar a barbearia é grátis. Você assina o plano mensal quando quiser publicar o link e receber agendamentos.",
    },
    {
      q: "Quanto custa?",
      a: TRIAL_DAYS
        ? `${PRICE_LABEL}, depois de ${TRIAL_DAYS} dias grátis. Um plano só, com todos os recursos.`
        : `${PRICE_LABEL}. Um plano só, com todos os recursos. Configurar é grátis.`,
    },
  ];
}

function Tick() {
  return <Check size={20} strokeWidth={2.25} aria-hidden="true" className={styles.tick} />;
}

export function LandingPage({ channel }: { channel: string }) {
  const whatsApp = whatsAppHref(channel);
  const cta = TRIAL_DAYS ? `Começar ${TRIAL_DAYS} dias grátis` : "Criar conta grátis";
  return (
    <div className={styles.page}>
      <a className={styles.skip} href="#conteudo">
        Pular para o conteúdo
      </a>
      <header className={styles.header}>
        <div className={`${styles.wrap} ${styles.headerRow}`}>
          <Link href="/" className={styles.logo}>
            {PRODUCT_NAME}
          </Link>
          <nav aria-label="Seções" className={styles.nav}>
            <a href="#como">Como funciona</a>
            <a href="#recursos">Recursos</a>
            <a href="#preco">Preço</a>
            <a href="#duvidas">Dúvidas</a>
          </nav>
          <div className={styles.headerActions}>
            <Link href="/login" className={styles.navLink}>
              Entrar
            </Link>
            <Link href={SIGNUP} className={`${styles.btn} ${styles.btnBrass} ${styles.btnSmall}`}>
              Criar conta
            </Link>
          </div>
        </div>
      </header>

      <main id="conteudo">
        <section className={styles.hero}>
          <div className={`${styles.wrap} ${styles.heroRow}`}>
            <div className={styles.heroText}>
              <p className={styles.eyebrowLight}>Agenda online para barbearias</p>
              <h1 className={styles.heroTitle}>
                Sua barbearia <em>na régua.</em>
              </h1>
              <p className={styles.heroLead}>
                O cliente agenda pelo link, recebe a confirmação e o lembrete no WhatsApp, e você vê
                a semana inteira no celular.
              </p>
              <div className={styles.actions}>
                <Link href={SIGNUP} className={`${styles.btn} ${styles.btnBrass}`}>
                  {cta}
                </Link>
                <a href="#como" className={`${styles.btn} ${styles.btnGhost}`}>
                  Ver como funciona
                </a>
              </div>
              <p className={styles.heroNote}>
                {TRIAL_DAYS
                  ? `${TRIAL_DAYS} dias grátis, depois ${PRICE_LABEL}. Configure sem pagar e ative quando quiser.`
                  : `Configure sem pagar. Assine por ${PRICE_LABEL} quando quiser publicar o seu link.`}
              </p>
            </div>
            <div className={styles.heroArt}>
              <Image
                className={styles.heroDesktop}
                src="/landing/d-agenda-semana.webp"
                alt="Agenda da semana no computador, com os atendimentos de cada barbeiro"
                width={1600}
                height={1000}
                priority
                sizes="(max-width: 900px) 92vw, 640px"
              />
              <div className={`${styles.phone} ${styles.heroPhone}`}>
                <Image
                  src="/landing/m-agenda-semana.webp"
                  alt="Agenda da semana no celular"
                  width={720}
                  height={1558}
                  priority
                  sizes="220px"
                />
              </div>
            </div>
          </div>
        </section>

        <section aria-label="O que muda na rotina" className={styles.pains}>
          <div className={`${styles.wrap} ${styles.painGrid}`}>
            <div>
              <h2 className={styles.cardTitle}>Chega de responder com a máquina na mão</h2>
              <p className={styles.muted}>O cliente vê os horários livres e marca sozinho.</p>
            </div>
            <div>
              <h2 className={styles.cardTitle}>Cliente lembrado antes do horário</h2>
              <p className={styles.muted}>Confirmação e lembrete saem do WhatsApp da barbearia.</p>
            </div>
            <div>
              <h2 className={styles.cardTitle}>O mês fechado sem caderno</h2>
              <p className={styles.muted}>Faturamento por dia, barbeiro, serviço e produto.</p>
            </div>
          </div>
        </section>

        <section id="como" className={styles.section}>
          <div className={styles.wrap}>
            <p className={styles.eyebrow}>Como funciona</p>
            <h2 className={styles.sectionTitle}>Três passos e a agenda anda sozinha.</h2>
            <ol className={styles.steps}>
              <li>
                <div className={styles.stepArt}>
                  <Image
                    src="/landing/m-publico-servicos.webp"
                    alt="Serviços com preço e duração"
                    width={720}
                    height={1558}
                    sizes="240px"
                  />
                </div>
                <h3 className={styles.cardTitle}>
                  <span className={styles.num}>1.</span> Crie a conta e cadastre a barbearia
                </h3>
                <p className={styles.muted}>
                  Serviços, preços e barbeiros, uma vez só. Cada barbeiro ganha acesso próprio.
                </p>
              </li>
              <li>
                <div className={styles.stepArt}>
                  <Image
                    src="/landing/m-publico-horarios.webp"
                    alt="Cliente escolhendo um horário livre"
                    width={720}
                    height={1558}
                    sizes="240px"
                  />
                </div>
                <h3 className={styles.cardTitle}>
                  <span className={styles.num}>2.</span> Publique o link na bio e no WhatsApp
                </h3>
                <p className={styles.muted}>
                  O cliente escolhe serviço, barbeiro e horário pelo navegador. Sem baixar app e sem
                  criar conta.
                </p>
              </li>
              <li>
                <div className={styles.stepArt}>
                  <Image
                    src="/landing/m-publico-comprovante.webp"
                    alt="Comprovante com o código do agendamento"
                    width={720}
                    height={1558}
                    sizes="240px"
                  />
                </div>
                <h3 className={styles.cardTitle}>
                  <span className={styles.num}>3.</span> Receba o agendamento confirmado
                </h3>
                <p className={styles.muted}>
                  O horário entra na sua agenda e o cliente recebe o código para consultar ou
                  cancelar.
                </p>
              </li>
            </ol>
          </div>
        </section>

        <section className={styles.dark}>
          <div className={`${styles.wrap} ${styles.split}`}>
            <div>
              <p className={styles.eyebrowLight}>WhatsApp</p>
              <h2 className={styles.sectionTitle}>
                A confirmação sai do número <em>da sua barbearia.</em>
              </h2>
              <p className={styles.leadLight}>
                Você conecta o WhatsApp da barbearia lendo um QR code. O cliente que autorizou
                recebe a confirmação ao marcar e um lembrete antes do horário, com serviço,
                barbeiro, data, valor e endereço.
              </p>
              <ul className={styles.ticks}>
                <li>
                  <Tick />
                  Só recebe mensagem quem deu consentimento
                </li>
                <li>
                  <Tick />
                  Você acompanha se a confirmação foi entregue e lida
                </li>
                <li>
                  <Tick />O cliente cancela pelo código e o horário volta a ficar livre
                </li>
              </ul>
            </div>
            <Image
              className={styles.shot}
              src="/landing/m-whatsapp-lembrete-card.webp"
              alt="Atendimento com a confirmação lida e o lembrete pronto para envio"
              width={960}
              height={1034}
              sizes="(max-width: 900px) 92vw, 480px"
            />
          </div>
        </section>

        <section id="recursos" className={styles.section}>
          <div className={styles.wrap}>
            <p className={styles.eyebrow}>Recursos</p>
            <h2 className={styles.sectionTitle}>Tudo que a barbearia usa no dia a dia.</h2>
            <ul className={styles.features}>
              <li>
                <h3 className={styles.cardTitle}>Agenda por semana, dia e mês</h3>
                <p className={styles.muted}>
                  No celular ou no computador, com os horários de cada barbeiro.
                </p>
              </li>
              <li>
                <h3 className={styles.cardTitle}>Link de agendamento</h3>
                <p className={styles.muted}>
                  Sem app e sem cadastro para o cliente. Horário duplo é bloqueado.
                </p>
              </li>
              <li>
                <h3 className={styles.cardTitle}>Confirmação e lembrete</h3>
                <p className={styles.muted}>Pelo WhatsApp da barbearia, para quem autorizou.</p>
              </li>
              <li>
                <h3 className={styles.cardTitle}>Acesso por barbeiro</h3>
                <p className={styles.muted}>Cada profissional entra com o próprio login.</p>
              </li>
              <li>
                <h3 className={styles.cardTitle}>Produtos e estoque mínimo</h3>
                <p className={styles.muted}>Registre vendas no balcão e saiba quando repor.</p>
              </li>
              <li>
                <h3 className={styles.cardTitle}>Relatórios</h3>
                <p className={styles.muted}>Faturamento por dia, barbeiro, serviço e produto.</p>
              </li>
            </ul>
            <figure className={styles.figure}>
              <Image
                className={styles.wide}
                src="/landing/d-relatorios.webp"
                alt="Relatório do mês com faturamento de serviços e produtos"
                width={1600}
                height={1000}
                sizes="(max-width: 1240px) 92vw, 1152px"
              />
              <figcaption className={styles.caption}>
                Tela com dados de uma barbearia de demonstração.
              </figcaption>
            </figure>
          </div>
        </section>

        <section id="preco" className={styles.brassSection}>
          <div className={`${styles.wrap} ${styles.split}`}>
            <div>
              <p className={styles.eyebrow}>Preço</p>
              <h2 className={styles.sectionTitle}>
                {TRIAL_DAYS
                  ? `${TRIAL_DAYS} dias grátis para testar.`
                  : "Configure grátis. Assine para publicar."}
              </h2>
              <p className={styles.lead}>
                {TRIAL_DAYS
                  ? `Crie a conta e cadastre serviços e barbeiros sem pagar nada. Quando ativar o link de agendamento, você usa ${TRIAL_DAYS} dias grátis com clientes de verdade. Depois, ${PRICE_LABEL}.`
                  : `Crie a conta, cadastre serviços e barbeiros e veja a agenda funcionando sem pagar nada. Quando estiver pronto para receber clientes pelo link, você assina por ${PRICE_LABEL}.`}
              </p>
            </div>
            <div className={styles.priceCard}>
              <p className={styles.priceMain}>{PRICE_LABEL}</p>
              <p className={styles.priceSub}>
                {TRIAL_DAYS
                  ? `Os primeiros ${TRIAL_DAYS} dias são grátis. Cancele quando quiser.`
                  : "Configuração grátis, sem cartão para começar"}
              </p>
              <ul className={styles.ticks}>
                <li>
                  <Tick />
                  Agenda, link de agendamento e WhatsApp
                </li>
                <li>
                  <Tick />
                  Produtos, estoque e relatórios
                </li>
                <li>
                  <Tick />
                  Acesso próprio para cada barbeiro
                </li>
              </ul>
              <Link href={SIGNUP} className={`${styles.btn} ${styles.btnBrass} ${styles.btnBlock}`}>
                {cta}
              </Link>
              {whatsApp && (
                <a
                  className={styles.priceAlt}
                  href={whatsApp}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Prefere conversar antes? Chamar no WhatsApp
                </a>
              )}
            </div>
          </div>
        </section>

        <section id="duvidas" className={styles.section}>
          <div className={styles.narrow}>
            <p className={styles.eyebrow}>Dúvidas</p>
            <h2 className={styles.sectionTitle}>Antes de começar</h2>
            <div className={styles.faq}>
              {faq().map((item) => (
                <details key={item.q}>
                  <summary>{item.q}</summary>
                  <p>{item.a}</p>
                </details>
              ))}
            </div>
          </div>
        </section>

        <section className={styles.dark}>
          <div className={`${styles.wrap} ${styles.finalRow}`}>
            <h2 className={styles.sectionTitle}>
              Bota a sua barbearia <em>na régua.</em>
            </h2>
            <div className={styles.actions}>
              <Link href={SIGNUP} className={`${styles.btn} ${styles.btnBrass}`}>
                {cta}
              </Link>
              {whatsApp && (
                <a
                  className={`${styles.btn} ${styles.btnGhost}`}
                  href={whatsApp}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Chamar no WhatsApp
                </a>
              )}
            </div>
          </div>
        </section>
      </main>

      <footer className={styles.footer}>
        <div className={`${styles.wrap} ${styles.footerRow}`}>
          <span className={styles.logo}>{PRODUCT_NAME}</span>
          <span>Agendamento e gestão de barbearias</span>
          <Link href="/login">Entrar</Link>
          <InstallAppButton />
        </div>
      </footer>
    </div>
  );
}
