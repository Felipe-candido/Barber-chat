"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type ComponentProps } from "react";
import { useRouter } from "next/navigation";
import { ApiError, errorMessage } from "@/lib/api/client";
import { registerShop, type RegisteredShop, type ShopRegistration } from "@/lib/api/registration";
import {
  brazilianStates,
  clearRegistrationAttempt,
  loadRegistrationAttempt,
  saveRegistrationAttempt,
  shopSlug,
  validateShopRegistration,
  type RegistrationAttempt,
  type RegistrationErrors,
} from "@/lib/onboarding";
import { useAdminAccess } from "../admin-access-provider";
import { Alert, Button, SelectField, TextField, buttonClasses } from "../ui";
import { OnboardingShell } from "./onboarding-shell";
import { ErrorSummary } from "./error-summary";
import { useRegistrationSession } from "./use-registration-session";
import styles from "./onboarding.module.css";

const emptyInput: ShopRegistration = {
  display_name: "",
  name: "",
  slug: "",
  timezone: "America/Sao_Paulo",
  address: {
    postal_code: "",
    street: "",
    number: "",
    complement: "",
    district: "",
    city: "",
    state: "",
  },
};
const labels: Record<string, string> = {
  display_name: "Seu nome",
  name: "Nome da barbearia",
  slug: "Endereço do link",
  timezone: "Fuso horário",
  postal_code: "CEP",
  street: "Rua ou avenida",
  number: "Número",
  complement: "Complemento (opcional)",
  district: "Bairro",
  city: "Cidade",
  state: "Estado",
};

export function ShopRegistrationPage() {
  const { user, loading, error: sessionError, retry } = useRegistrationSession();
  const { refresh, logout } = useAdminAccess();
  const router = useRouter();
  const [input, setInput] = useState<ShopRegistration>(emptyInput);
  const [errors, setErrors] = useState<RegistrationErrors>({});
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState<RegistrationAttempt | null>(null);
  const [created, setCreated] = useState<RegisteredShop | null>(null);
  const [busy, setBusy] = useState(false);
  const [preparedFor, setPreparedFor] = useState<string | null>(null);
  const [blocked, setBlocked] = useState(false);
  const slugEdited = useRef(false);
  const inFlight = useRef(false);
  const summaryRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    setPreparedFor(null);
    setBlocked(false);
    if (!user) return;
    try {
      const saved = loadRegistrationAttempt(user.id);
      setAttempt(saved);
      setInput(
        saved?.input ?? {
          ...emptyInput,
          display_name:
            typeof user.user_metadata?.display_name === "string"
              ? user.user_metadata.display_name.slice(0, 100)
              : "",
        },
      );
      slugEdited.current = !!saved;
      setCreated(null);
      setError("");
      setErrors({});
    } catch (error) {
      setInput(emptyInput);
      setAttempt(null);
      setCreated(null);
      setError(errorMessage(error));
      setBlocked(true);
    }
    setPreparedFor(user.id);
  }, [user]);
  const prepared = !!user && preparedFor === user.id;
  async function submit() {
    if (inFlight.current || !user || !prepared || blocked) return;
    const validation = validateShopRegistration(input);
    setErrors(validation);
    if (Object.keys(validation).length) {
      requestAnimationFrame(() => summaryRef.current?.focus());
      return;
    }
    let pending = attempt;
    if (!pending) {
      const key = Array.from(crypto.getRandomValues(new Uint8Array(32)), (value) =>
        value.toString(16).padStart(2, "0"),
      ).join("");
      pending = { userID: user.id, key, input };
      try {
        saveRegistrationAttempt(pending);
      } catch {
        setError(
          "Permita o armazenamento desta aba para preservar sua tentativa com segurança. Nada foi enviado à API.",
        );
        return;
      }
      setAttempt(pending);
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await registerShop(pending.input, pending.key, pending.userID);
      setCreated(result);
      try {
        clearRegistrationAttempt();
      } catch {
        /* A confirmed response is still success. */
      }
      setAttempt(null);
    } catch (error) {
      if (
        error instanceof ApiError &&
        (error.status === 422 || error.code === "shop_slug_taken" || error.code === "invalid_body")
      ) {
        // These responses guarantee no registration commit; editing is safe.
        try {
          clearRegistrationAttempt();
          setAttempt(null);
        } catch {
          setBlocked(true);
        }
      }
      setError(
        error instanceof ApiError && error.code === "idempotency_conflict"
          ? "Esta tentativa já foi usada com outros dados. Confira suas barbearias no painel antes de iniciar outro cadastro."
          : errorMessage(error),
      );
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  async function openPanel() {
    if (!created || inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      await refresh();
      router.replace(`/admin/${encodeURIComponent(created.slug)}/servicos`);
    } catch {
      setError(
        "Sua barbearia foi criada, mas o painel não pôde ser carregado. Tente abrir o painel novamente; não precisa refazer o cadastro.",
      );
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  function field(name: string, props: Partial<ComponentProps<typeof TextField>> = {}) {
    const addressField = name in input.address;
    const value = addressField
      ? input.address[name as keyof typeof input.address]
      : input[name as "display_name" | "name" | "slug"];
    return (
      <TextField
        key={name}
        id={`registration-${name}`}
        label={labels[name]}
        value={value}
        error={errors[name]}
        required={name !== "complement"}
        {...props}
        onBlur={() =>
          setErrors((current) => ({ ...current, [name]: validateShopRegistration(input)[name] }))
        }
        onChange={(event) => {
          const value = event.target.value;
          setErrors((current) => ({ ...current, [name]: undefined }));
          if (addressField)
            setInput((current) => ({ ...current, address: { ...current.address, [name]: value } }));
          else {
            if (name === "slug") slugEdited.current = true;
            setInput((current) => ({
              ...current,
              [name]: value,
              ...(name === "name" && !slugEdited.current ? { slug: shopSlug(value) } : {}),
            }));
          }
        }}
      />
    );
  }
  const confirmed = !!user?.email_confirmed_at && !user.is_anonymous;
  const locked = busy || !!attempt || blocked;
  return (
    <OnboardingShell step={3}>
      <div>
        <h1>{created ? "Sua barbearia está criada" : "Agora, sua barbearia"}</h1>
        <p className={styles.description}>
          {created
            ? "Você já pode preparar os serviços e a equipe no painel."
            : "Informe o essencial. Todos os campos são obrigatórios, exceto o complemento. Os demais detalhes ficarão para Configurações."}
        </p>
      </div>
      {error && <Alert tone="error">{error}</Alert>}
      {sessionError && <Alert tone="error">{sessionError}</Alert>}
      {loading ? (
        <p role="status">Confirmando sua sessão…</p>
      ) : !user ? (
        <>
          {!sessionError && (
            <Alert>
              Entre com sua conta confirmada para cadastrar a barbearia. Se acabou de criar a conta,
              confira o link recebido por e-mail.
            </Alert>
          )}
          <Link href="/login" className={buttonClasses()}>
            Entrar com minha conta
          </Link>
          <Link href="/cadastro" className={buttonClasses("secondary")}>
            Criar conta
          </Link>
          <Button variant="quiet" onClick={retry}>
            Verificar sessão novamente
          </Button>
        </>
      ) : !confirmed ? (
        <>
          <Alert tone="warning">
            Confirme seu e-mail antes de continuar. Depois faça login novamente para atualizar sua
            sessão.
          </Alert>
          <Link href="/login" className={buttonClasses()}>
            Voltar ao login
          </Link>
        </>
      ) : created ? (
        <>
          <Alert tone="success">
            {created.name} foi cadastrada com seu endereço e seu acesso de proprietário.
          </Alert>
          <Alert>
            O link de agendamento continua privado. Nenhuma assinatura foi iniciada e nenhum
            pagamento foi feito. A publicação será liberada na etapa de integração com a Stripe.
          </Alert>
          <Button loading={busy} onClick={() => void openPanel()}>
            Abrir painel e cadastrar serviços
          </Button>
        </>
      ) : prepared ? (
        <>
          <p className={styles.description}>Conta: {user.email}</p>
          {attempt && (
            <Alert tone="warning">
              Há um envio aguardando confirmação. Os dados foram preservados somente nesta aba. Use
              “Recuperar cadastro” para repetir a mesma tentativa sem duplicar a barbearia.
            </Alert>
          )}
          <form
            className={styles.form}
            noValidate
            aria-busy={busy}
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <ErrorSummary errors={errors} labels={labels} summaryRef={summaryRef} />
            <fieldset className={styles.fields} disabled={locked}>
              {field("display_name", { maxLength: 100, autoComplete: "name" })}
              {field("name", { maxLength: 150, autoComplete: "organization" })}
              {field("slug", {
                maxLength: 100,
                autoCapitalize: "none",
                spellCheck: false,
                hint: `Seu futuro link: /b/${input.slug || "sua-barbearia"}. Ele só ficará disponível após a publicação e o pagamento da assinatura.`,
              })}
              <SelectField
                id="registration-timezone"
                label="Fuso horário"
                value={input.timezone}
                hint="Os horários da agenda usarão este fuso. Confira se ele corresponde à sua cidade."
                onChange={(event) => setInput({ ...input, timezone: event.target.value })}
                required
              >
                <option value="America/Sao_Paulo">Brasília — UTC−3</option>
                <option value="America/Manaus">Manaus — UTC−4</option>
                <option value="America/Rio_Branco">Rio Branco — UTC−5</option>
                <option value="America/Noronha">Fernando de Noronha — UTC−2</option>
              </SelectField>
            </fieldset>
            <fieldset className={`${styles.fields} ${styles.address}`} disabled={locked}>
              <legend>Endereço da barbearia</legend>
              {field("postal_code", {
                maxLength: 9,
                autoComplete: "postal-code",
                inputMode: "numeric",
                placeholder: "01310-100",
              })}
              {field("street", { maxLength: 150, autoComplete: "address-line1" })}
              {field("number", { maxLength: 20, hint: "Se o imóvel não tem número, informe S/N." })}
              {field("complement", {
                maxLength: 100,
                autoComplete: "address-line2",
                hint: "Exemplo: sala 2, loja 5 ou fundos.",
              })}
              {field("district", { maxLength: 100 })}
              {field("city", { maxLength: 100, autoComplete: "address-level2" })}
              <SelectField
                id="registration-state"
                label="Estado"
                autoComplete="address-level1"
                required
                value={input.address.state}
                error={errors.state}
                onBlur={() =>
                  setErrors((current) => ({
                    ...current,
                    state: validateShopRegistration(input).state,
                  }))
                }
                onChange={(event) => {
                  setInput({ ...input, address: { ...input.address, state: event.target.value } });
                  setErrors((current) => ({ ...current, state: undefined }));
                }}
              >
                <option value="">Selecione o estado</option>
                {brazilianStates.map((state) => (
                  <option key={state} value={state}>
                    {state}
                  </option>
                ))}
              </SelectField>
            </fieldset>
            <Button type="submit" disabled={blocked} loading={busy}>
              {busy
                ? "Confirmando cadastro…"
                : attempt
                  ? "Recuperar cadastro"
                  : "Criar minha barbearia"}
            </Button>
          </form>
          <div className={styles.actions}>
            <Link href="/admin" className={buttonClasses("quiet")}>
              Minhas barbearias
            </Link>
            <Button
              variant="quiet"
              disabled={busy}
              onClick={async () => {
                if (
                  attempt &&
                  !window.confirm(
                    "Há uma tentativa pendente. Antes de cadastrar novamente, confira suas barbearias no painel. Deseja sair desta conta?",
                  )
                )
                  return;
                try {
                  await logout();
                  clearRegistrationAttempt();
                  router.replace("/login");
                } catch (error) {
                  setError(errorMessage(error));
                }
              }}
            >
              Sair desta conta
            </Button>
          </div>
        </>
      ) : (
        <p role="status">Preparando cadastro…</p>
      )}
    </OnboardingShell>
  );
}
