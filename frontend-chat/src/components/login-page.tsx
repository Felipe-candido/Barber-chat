"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowRight, ArrowUpRight, CalendarDays, Eye, EyeOff, Scissors, Users } from "lucide-react";
import { Alert, Brand, Button, Modal, TextField, buttonClasses } from "./ui";
import { useFeedback } from "./feedback-provider";
import { signInWithPassword } from "@/lib/supabase/auth";
import { ApiError } from "@/lib/api/client";
import { useAdminAccess } from "./admin-access-provider";
import styles from "./login-page.module.css";

export function LoginPage() {
  const router = useRouter();
  const { notify } = useFeedback();
  const { refresh } = useAdminAccess();
  const [visible, setVisible] = useState(false);
  const [help, setHelp] = useState<"recovery" | "invitation" | null>(null);
  const [recoveryPreview, setRecoveryPreview] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [loginError, setLoginError] = useState("");
  const [canCompleteSignup, setCanCompleteSignup] = useState(false);
  function closeHelp() {
    setHelp(null);
    setRecoveryPreview(false);
  }
  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Link href="/login" aria-label="Barber-chat — acesso">
          <Brand />
        </Link>
        <Link href="/chat" className={styles.publicLink}>
          Quero agendar um horário <ArrowUpRight size={18} aria-hidden="true" />
        </Link>
      </header>
      <main className={styles.layout}>
        <section className={styles.introduction} aria-labelledby="login-introduction">
          <h2 id="login-introduction">
            Uma rotina organizada.
            <br />
            Mais tempo para atender.
          </h2>
          <p>
            Agenda, serviços e equipe no mesmo lugar. Entre na sua conta e escolha a barbearia que
            quer gerenciar.
          </p>
          <ul className={styles.features}>
            <li>
              <CalendarDays size={20} aria-hidden="true" />
              <span>Acompanhe os horários da agenda</span>
            </li>
            <li>
              <Scissors size={20} aria-hidden="true" />
              <span>Organize os serviços da barbearia</span>
            </li>
            <li>
              <Users size={20} aria-hidden="true" />
              <span>Cada pessoa com seu próprio acesso</span>
            </li>
          </ul>
        </section>
        <section className={styles.panel} aria-labelledby="login-title">
          <div>
            <h1 id="login-title">Bem-vindo de volta</h1>
            <p className={styles.description}>Entre com o e-mail e a senha do seu acesso.</p>
          </div>
          <form
            className={styles.form}
            aria-busy={submitting}
            onSubmit={async (e) => {
              e.preventDefault();
              if (submitting) return;
              const form = e.currentTarget;
              const values = new FormData(form);
              setLoginError("");
              setCanCompleteSignup(false);
              setSubmitting(true);
              try {
                await signInWithPassword(
                  String(values.get("email")),
                  String(values.get("password")),
                );
                form.reset();
                setVisible(false);
                await refresh();
                notify("Identidade validada. Escolha sua barbearia.");
                router.replace("/admin");
              } catch (error) {
                setCanCompleteSignup(error instanceof ApiError && error.status === 403);
                setLoginError(
                  error instanceof Error
                    ? error.message
                    : "Não foi possível entrar. Tente novamente.",
                );
              } finally {
                setSubmitting(false);
              }
            }}
          >
            <TextField
              id="login-email"
              label="E-mail"
              name="email"
              type="email"
              autoComplete="username"
              placeholder="voce@barbearia.com"
              required
              maxLength={254}
              disabled={submitting}
            />
            <TextField
              id="login-password"
              label="Senha"
              name="password"
              type={visible ? "text" : "password"}
              autoComplete="current-password"
              required
              maxLength={128}
              disabled={submitting}
              endAdornment={
                <button
                  type="button"
                  className="icon-button"
                  onClick={() => setVisible(!visible)}
                  aria-label={visible ? "Ocultar senha" : "Mostrar senha"}
                  aria-pressed={visible}
                  disabled={submitting}
                >
                  {visible ? (
                    <EyeOff size={18} aria-hidden="true" />
                  ) : (
                    <Eye size={18} aria-hidden="true" />
                  )}
                </button>
              }
            />
            <button type="button" className={styles.textAction} onClick={() => setHelp("recovery")}>
              Esqueci minha senha
            </button>
            {loginError && (
              <Alert tone="error" className="login-form-error">
                {loginError}
              </Alert>
            )}
            <Button type="submit" loading={submitting} className={styles.fullWidth}>
              {submitting ? "Entrando…" : "Entrar no painel"}
              {!submitting && <ArrowRight size={18} aria-hidden="true" />}
            </Button>
          </form>
          {canCompleteSignup && (
            <Link
              href="/cadastro/barbearia"
              className={buttonClasses("secondary", styles.fullWidth)}
            >
              Concluir cadastro da minha barbearia
            </Link>
          )}
          <p className={styles.help}>
            É proprietário e está começando?{" "}
            <Link href="/cadastro" className={styles.textAction}>
              Criar minha conta
            </Link>
          </p>
          <Link href="/admin" className={buttonClasses("secondary", styles.fullWidth)}>
            Continuar com sessão ativa
          </Link>
          <p className={styles.help}>
            Ainda não recebeu seu acesso?{" "}
            <button
              type="button"
              className={styles.textAction}
              onClick={() => setHelp("invitation")}
            >
              Como obter acesso
            </button>
          </p>
        </section>
      </main>
      <footer className={styles.footer}>Barber-chat · Agendamento e gestão de barbearias</footer>
      {help && (
        <Modal
          open
          onClose={closeHelp}
          title={
            help === "invitation"
              ? "Como seu acesso é liberado"
              : recoveryPreview
                ? "Prévia de recuperação"
                : "Recuperar seu acesso"
          }
          subtitle={
            help === "invitation"
              ? "Uma conta pessoal, vinculada à sua barbearia."
              : "A recuperação por e-mail ainda não está disponível nesta tela."
          }
        >
          {help === "invitation" ? (
            <div className={styles.form}>
              <p>
                Se você é barbeiro, peça ao proprietário para criar seu acesso na área de contas da
                barbearia. Você receberá um convite para definir sua senha.
              </p>
              <p>
                Se você é proprietário, crie sua conta e cadastre sua barbearia. O acesso da equipe
                continua sendo criado por você no painel.
              </p>
              <Alert>
                Use seu próprio e-mail. Não é necessário compartilhar uma senha: cada pessoa da
                equipe possui sua conta e suas permissões.
              </Alert>
              <Button onClick={closeHelp}>Voltar ao login</Button>
            </div>
          ) : recoveryPreview ? (
            <div className={styles.form}>
              <Alert>Prévia: nenhum e-mail foi enviado e nenhum dado foi salvo.</Alert>
              <p>Para recuperar o acesso agora, fale com o responsável pelo sistema.</p>
              <Button onClick={closeHelp}>Voltar ao login</Button>
            </div>
          ) : (
            <form
              className={styles.form}
              onSubmit={(e) => {
                e.preventDefault();
                e.currentTarget.reset();
                setRecoveryPreview(true);
              }}
            >
              <Alert tone="warning">
                Esta é apenas uma prévia. Nenhuma mensagem será enviada. Para recuperar o acesso
                agora, fale com o responsável pelo sistema.
              </Alert>
              <TextField
                label="E-mail da conta"
                name="recovery-email"
                type="email"
                autoComplete="email"
                placeholder="voce@barbearia.com"
                required
                maxLength={254}
              />
              <div className="modal-actions">
                <Button variant="secondary" onClick={closeHelp}>
                  Voltar
                </Button>
                <Button type="submit">Simular recuperação</Button>
              </div>
            </form>
          )}
        </Modal>
      )}
    </div>
  );
}
