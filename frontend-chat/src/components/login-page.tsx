"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  CalendarDays,
  Check,
  Eye,
  EyeOff,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  Mail,
  Scissors,
  ShieldCheck,
} from "lucide-react";
import { Brand, Modal } from "./ui";
import { useFeedback } from "./feedback-provider";
import { signInWithPassword } from "@/lib/supabase/auth";

export function LoginPage() {
  const router = useRouter();
  const { notify } = useFeedback();
  const [visible, setVisible] = useState(false);
  const [help, setHelp] = useState<"recovery" | "invitation" | null>(null);
  const [recoveryPreview, setRecoveryPreview] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [loginError, setLoginError] = useState("");
  function closeHelp() {
    setHelp(null);
    setRecoveryPreview(false);
  }
  return (
    <div className="login-page">
      <header className="login-nav">
        <Link href="/login" aria-label="Palma Barbearia — acesso">
          <Brand />
        </Link>
        <Link href="/chat" className="login-public-link">
          Quero agendar um horário
          <ArrowUpRight size={16} />
        </Link>
      </header>
      <main className="login-layout">
        <section className="login-story" aria-label="Gestão da barbearia">
          <span className="login-story-label">
            <span />
            SEU ESPAÇO PARA CRESCER
          </span>
          <div className="login-story-copy">
            <span className="login-scissors">
              <Scissors size={29} strokeWidth={1.3} />
            </span>
            <h1>
              Seu talento
              <br />
              cuida do estilo.
              <br />
              <em>
                A gente cuida
                <br />
                da organização.
              </em>
            </h1>
            <p>
              Agenda, serviços e atendimento.
              <br />
              Tudo em um só lugar, no seu ritmo.
            </p>
          </div>
          <div className="login-preview" aria-hidden="true">
            <div className="login-preview-top">
              <span>
                <CalendarDays size={15} />
                Um dia bem organizado
              </span>
              <span className="login-preview-dots">•••</span>
            </div>
            <div className="login-preview-row">
              <span className="preview-time">09:00</span>
              <span className="preview-service-icon">
                <Scissors size={16} />
              </span>
              <div>
                <strong>Corte clássico</strong>
                <small>Cuidado em cada detalhe</small>
              </div>
              <Check size={15} />
            </div>
            <div className="login-preview-row">
              <span className="preview-time">10:00</span>
              <span className="preview-service-icon">
                <Scissors size={16} />
              </span>
              <div>
                <strong>Cabelo + barba</strong>
                <small>Um momento para renovar</small>
              </div>
              <Check size={15} />
            </div>
          </div>
          <div className="login-story-footer">
            <ShieldCheck size={17} />
            <span>
              Um espaço para sua barbearia.
              <br />
              <strong>Um acesso para cada pessoa da equipe.</strong>
            </span>
          </div>
          <div className="login-orbits" aria-hidden="true">
            <i />
            <i />
            <i />
          </div>
        </section>
        <section className="login-form-panel" aria-label="Acesso administrativo">
          <div className="login-form-content">
            <span className="login-form-symbol">
              <KeyRound size={22} strokeWidth={1.5} />
            </span>
            <div className="eyebrow">PAINEL DA BARBEARIA</div>
            <h2>
              Bem-vindo de volta<span>.</span>
            </h2>
            <p className="login-intro">
              Seu próximo bom dia começa por aqui.
              <br />
              Entre para cuidar da sua barbearia.
            </p>
            <div className="login-demo-label">
              <span />
              Acesso protegido pelo Supabase Auth
            </div>
            <form
              className="login-form"
              onSubmit={async (e) => {
                e.preventDefault();
                if (submitting) return;
                const form = e.currentTarget;
                const values = new FormData(form);
                setLoginError("");
                setSubmitting(true);
                try {
                  await signInWithPassword(
                    String(values.get("email")),
                    String(values.get("password")),
                  );
                  form.reset();
                  setVisible(false);
                  notify("Acesso confirmado pelo Supabase.");
                  router.replace("/admin");
                } catch (error) {
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
              <label htmlFor="login-email">E-mail</label>
              <div className="login-input">
                <Mail size={17} />
                <input
                  id="login-email"
                  name="email"
                  type="email"
                  autoComplete="username"
                  placeholder="voce@barbearia.com"
                  required
                  maxLength={254}
                  disabled={submitting}
                />
              </div>
              <div className="login-password-label">
                <label htmlFor="login-password">Senha</label>
                <button type="button" onClick={() => setHelp("recovery")}>
                  Esqueci minha senha
                </button>
              </div>
              <div className="login-input">
                <LockKeyhole size={17} />
                <input
                  id="login-password"
                  name="password"
                  type={visible ? "text" : "password"}
                  autoComplete="current-password"
                  placeholder="Sua senha"
                  required
                  maxLength={128}
                  disabled={submitting}
                />
                <button
                  type="button"
                  onClick={() => setVisible(!visible)}
                  aria-label={visible ? "Ocultar senha" : "Mostrar senha"}
                  aria-pressed={visible}
                  disabled={submitting}
                >
                  {visible ? <EyeOff size={18} /> : <Eye size={18} />}
                </button>
              </div>
              {loginError && (
                <div className="form-error login-form-error" role="alert">
                  <AlertCircle size={17} />
                  {loginError}
                </div>
              )}
              <button className="button button-primary login-submit" disabled={submitting}>
                {submitting ? "Entrando…" : "Entrar no painel"}
                {submitting ? <LoaderCircle size={17} /> : <ArrowRight size={17} />}
              </button>
            </form>
            <div className="login-divider">
              <span />
              ou conheça o espaço
              <span />
            </div>
            <Link href="/admin" className="button button-secondary login-explore">
              Explorar interface
              <ArrowUpRight size={15} />
            </Link>
            <p className="login-invitation">
              Ainda não recebeu seu acesso?
              <br />
              <button onClick={() => setHelp("invitation")}>
                Saiba como funciona o convite
                <ArrowRight size={13} />
              </button>
            </p>
          </div>
          <p className="login-demo-footer">
            <LockKeyhole size={13} />
            Sua senha é enviada diretamente ao Supabase e nunca à API Go.
            <br />A sessão fornece um token curto para as chamadas administrativas.
          </p>
        </section>
      </main>
      <footer className="login-footer">
        <span>PALMA BARBEARIA</span>
        <span>Seu estilo. Nosso cuidado.</span>
      </footer>
      {help && (
        <Modal
          open
          onClose={closeHelp}
          title={
            help === "invitation"
              ? "Seu acesso começa com um convite"
              : recoveryPreview
                ? "Prévia de recuperação"
                : "Recuperar seu acesso"
          }
          subtitle={
            help === "invitation"
              ? "Uma conta pessoal, vinculada à sua barbearia."
              : "Vamos ajudar você a voltar ao seu espaço."
          }
        >
          {help === "invitation" ? (
            <div className="login-help">
              <span className="login-help-icon">
                <Mail size={24} />
              </span>
              <p>
                O acesso administrativo será liberado por convite. A administração cadastra a
                barbearia e convida o proprietário pelo e-mail dele.
              </p>
              <p>
                O proprietário define a própria senha e, depois, poderá convidar a equipe com as
                permissões adequadas. Não é necessário compartilhar uma senha.
              </p>
              <div className="inline-hint">
                Nesta demonstração, nenhum convite é criado. Você pode conhecer o painel sem uma
                conta.
              </div>
              <Link href="/admin" className="button button-primary" onClick={closeHelp}>
                Explorar interface
                <ArrowRight size={16} />
              </Link>
            </div>
          ) : recoveryPreview ? (
            <div className="login-help">
              <span className="login-help-icon">
                <Mail size={24} />
              </span>
              <p>
                Na versão integrada, enviaremos as instruções de recuperação se o e-mail tiver uma
                conta.
              </p>
              <div className="inline-hint" role="status">
                Demonstração: nenhum e-mail foi enviado e nenhum dado foi salvo.
              </div>
              <button className="button button-primary" onClick={closeHelp}>
                Voltar ao login
                <ArrowLeft size={16} />
              </button>
            </div>
          ) : (
            <form
              className="form-stack"
              onSubmit={(e) => {
                e.preventDefault();
                e.currentTarget.reset();
                setRecoveryPreview(true);
              }}
            >
              <p className="dialog-copy">
                Informe o e-mail usado no seu acesso. Na versão integrada, você poderá criar uma
                nova senha pelo link de recuperação.
              </p>
              <label>
                E-mail da conta
                <input
                  name="recovery-email"
                  type="email"
                  autoComplete="email"
                  placeholder="voce@barbearia.com"
                  required
                  maxLength={254}
                />
              </label>
              <p className="inline-hint">Prévia do fluxo. Nenhuma mensagem será enviada.</p>
              <div className="modal-actions">
                <button className="button button-secondary" type="button" onClick={closeHelp}>
                  Voltar
                </button>
                <button className="button button-primary">
                  Simular recuperação
                  <ArrowRight size={16} />
                </button>
              </div>
            </form>
          )}
        </Modal>
      )}
    </div>
  );
}
