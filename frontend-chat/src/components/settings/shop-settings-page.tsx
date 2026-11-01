"use client";

import Link from "next/link";
import { useRef, useState, type ComponentProps } from "react";
import { LoaderCircle, RefreshCw, Save } from "lucide-react";
import { useShopSettings } from "@/hooks/use-shop-settings";
import { useShopCommercial } from "@/hooks/use-shop-commercial";
import { useSubscriptionCheckout } from "@/hooks/use-subscription-checkout";
import { useBillingPortal } from "@/hooks/use-billing-portal";
import { useUnsavedSettings } from "@/hooks/use-unsaved-settings";
import { brazilianStates } from "@/lib/shop-address";
import {
  settingsLabels,
  validateSettingsDraft,
  type SettingsErrors,
  type SettingsField,
} from "@/lib/shop-settings";
import { useAdminAccess } from "../admin-access-provider";
import { useSelectedShop, useSelectedShopAccess } from "../shop-panel";
import {
  Alert,
  Button,
  Modal,
  PageHeader,
  SelectField,
  TextareaField,
  TextField,
  buttonClasses,
} from "../ui";
import { SettingsReview } from "./settings-review";
import { ShopCommercialCard } from "./shop-commercial-card";
import styles from "./shop-settings.module.css";

export function ShopSettingsPage() {
  const shop = useSelectedShop();
  const access = useSelectedShopAccess();
  const { state } = useAdminAccess();
  if (access.role !== "owner")
    return (
      <section className={styles.page}>
        <PageHeader title="Configurações" />
        <Alert tone="warning">
          Somente o proprietário pode consultar e editar as configurações da barbearia.
        </Alert>
        <Link
          href={`/admin/${encodeURIComponent(shop.slug)}`}
          className={buttonClasses("secondary")}
        >
          Voltar para a agenda
        </Link>
      </section>
    );
  if (state.status !== "ready") return null;
  return (
    <OwnerSettings
      key={`${shop.shop_id}:${state.identity.user_id}`}
      shopID={shop.shop_id}
      slug={shop.slug}
      userID={state.identity.user_id}
    />
  );
}

function OwnerSettings({ shopID, slug, userID }: { shopID: string; slug: string; userID: string }) {
  const { syncShopName } = useAdminAccess();
  const editor = useShopSettings(slug, shopID, userID, syncShopName);
  const commercial = useShopCommercial(slug, shopID, userID, editor.settings?.version);
  const checkout = useSubscriptionCheckout(slug, shopID, userID);
  const portal = useBillingPortal(slug, shopID, userID);
  const [errors, setErrors] = useState<SettingsErrors>({});
  const [confirm, setConfirm] = useState<"reset" | "reload" | null>(null);
  const summaryRef = useRef<HTMLDivElement>(null);
  const pending = ["saving", "uncertain", "conflict", "checking", "review"].includes(editor.mode);
  useUnsavedSettings(
    editor.dirty,
    pending || commercial.pending || checkout.pending || portal.pending,
  );
  const locked = editor.mode !== "ready";

  function reload() {
    if (editor.dirty) setConfirm("reload");
    else {
      setErrors({});
      void editor.reload();
    }
  }
  function submit() {
    if (!editor.draft || locked) return;
    const validation = validateSettingsDraft(editor.draft);
    setErrors(validation);
    if (Object.keys(validation).length) {
      requestAnimationFrame(() => summaryRef.current?.focus());
      return;
    }
    void editor.save();
  }
  function field(name: SettingsField, props: Partial<ComponentProps<typeof TextField>> = {}) {
    const draft = editor.draft!;
    const addressField = name !== "name" && name !== "phone" && name !== "description";
    return (
      <TextField
        id={`settings-${name}`}
        label={settingsLabels[name]}
        {...props}
        value={addressField ? draft.address[name] : draft[name]}
        error={errors[name]}
        onBlur={() =>
          setErrors((current) => ({ ...current, [name]: validateSettingsDraft(draft)[name] }))
        }
        onChange={(event) => {
          setErrors((current) => ({ ...current, [name]: undefined }));
          editor.setDraft(
            addressField
              ? { ...draft, address: { ...draft.address, [name]: event.target.value } }
              : { ...draft, [name]: event.target.value },
          );
        }}
      />
    );
  }

  return (
    <section className={styles.page}>
      <PageHeader
        title="Configurações"
        description="Cadastro, assinatura e agendamento online da sua barbearia."
        actions={
          <Button variant="quiet" disabled={locked} onClick={reload}>
            <RefreshCw size={16} aria-hidden="true" />
            Recarregar dados
          </Button>
        }
      />
      <div className={styles.content}>
        {editor.mode !== "blocked" && (
          <ShopCommercialCard
            commercial={commercial}
            checkout={checkout}
            portal={portal}
            configurationPending={editor.dirty || editor.mode !== "ready"}
            timezone={editor.settings?.timezone ?? "America/Sao_Paulo"}
          />
        )}
        {editor.mode === "loading" ? (
          <div className={styles.loading} role="status">
            <LoaderCircle size={22} aria-hidden="true" />
            Carregando configurações…
          </div>
        ) : editor.mode === "blocked" ? (
          <div className={styles.feedback}>
            <Alert tone="error">
              {editor.error?.message ?? "Não foi possível liberar as configurações."}
            </Alert>
            <Link
              href={editor.error?.status === 401 ? "/login" : "/admin"}
              className={buttonClasses("secondary")}
            >
              {editor.error?.status === 401 ? "Entrar novamente" : "Escolher barbearia"}
            </Link>
          </div>
        ) : editor.mode === "load-error" ? (
          <div className={styles.feedback}>
            <Alert tone="error">
              <strong>Não foi possível carregar as configurações.</strong>
              <p>{editor.error?.message}</p>
            </Alert>
            <Button variant="secondary" onClick={() => void editor.reload()}>
              Tentar novamente
            </Button>
          </div>
        ) : editor.settings && editor.draft ? (
          <div className={styles.feedback}>
            {editor.notice && (
              <Alert tone={editor.dirty ? "info" : "success"}>{editor.notice}</Alert>
            )}
            {editor.error && <Alert tone="error">{editor.error.message}</Alert>}
            {(editor.mode === "uncertain" ||
              editor.mode === "conflict" ||
              editor.mode === "checking") && (
              <div className={styles.feedback}>
                <Alert tone="warning">
                  {editor.mode === "conflict"
                    ? "Outra edição foi salva. Seu rascunho foi mantido e nenhum dado foi sobrescrito."
                    : "O resultado do salvamento ainda não foi confirmado. Seu rascunho foi mantido. Não envie novamente antes de conferir os dados na API."}
                </Alert>
                <Button
                  variant="secondary"
                  loading={editor.mode === "checking"}
                  onClick={() => void editor.checkCurrent()}
                >
                  {editor.mode === "checking" ? "Conferindo dados…" : "Conferir dados na API"}
                </Button>
              </div>
            )}
            {editor.mode === "review" && editor.review && (
              <SettingsReview
                current={editor.review.current}
                proposed={editor.review.proposed}
                onKeep={() => {
                  setErrors({});
                  editor.useReview(true);
                }}
                onDiscard={() => {
                  setErrors({});
                  editor.useReview(false);
                }}
              />
            )}
            {!editor.settings.address && (
              <Alert>
                Complete o endereço da barbearia antes de salvar as configurações. Os dados antigos
                não foram alterados automaticamente.
              </Alert>
            )}
            <form
              noValidate
              className={styles.form}
              aria-busy={editor.mode === "saving"}
              onSubmit={(event) => {
                event.preventDefault();
                submit();
              }}
            >
              {Object.values(errors).some(Boolean) && (
                <div ref={summaryRef} tabIndex={-1} className={styles.summary} role="alert">
                  <p>Confira os campos abaixo antes de salvar.</p>
                  <ul>
                    {Object.entries(errors)
                      .filter(([, error]) => error)
                      .map(([field, error]) => (
                        <li key={field}>
                          <a
                            href={`#settings-${field}`}
                            onClick={(event) => {
                              event.preventDefault();
                              document.getElementById(`settings-${field}`)?.focus();
                            }}
                          >
                            {settingsLabels[field as SettingsField]}: {error}
                          </a>
                        </li>
                      ))}
                  </ul>
                </div>
              )}
              <section className={styles.section} aria-labelledby="settings-general-title">
                <h2 id="settings-general-title">Dados da barbearia</h2>
                <p>
                  Telefone e descrição são opcionais. Deixe em branco para remover essas
                  informações.
                </p>
                <fieldset
                  disabled={locked}
                  className={styles.fields}
                  aria-labelledby="settings-general-title"
                >
                  {field("name", { required: true, maxLength: 150, autoComplete: "organization" })}
                  <div className={styles.medium}>
                    {field("phone", {
                      label: "Telefone comercial (opcional)",
                      maxLength: 40,
                      type: "tel",
                      autoComplete: "tel",
                      placeholder: "(11) 91234-5678",
                      hint: "Telefone brasileiro com DDD, sem ramal.",
                    })}
                  </div>
                  <div>
                    <TextareaField
                      id="settings-description"
                      label="Descrição (opcional)"
                      maxLength={1000}
                      rows={4}
                      value={editor.draft.description}
                      error={errors.description}
                      hint="Apresente sua barbearia em até 1000 caracteres."
                      aria-describedby="settings-description-count"
                      onBlur={() =>
                        setErrors((current) => ({
                          ...current,
                          description: validateSettingsDraft(editor.draft!).description,
                        }))
                      }
                      onChange={(event) => {
                        editor.setDraft({ ...editor.draft!, description: event.target.value });
                        setErrors((current) => ({ ...current, description: undefined }));
                      }}
                    />
                    <p className={styles.counter} id="settings-description-count">
                      {1000 - [...editor.draft.description].length} caracteres restantes
                    </p>
                  </div>
                </fieldset>
              </section>
              <section className={styles.section} aria-labelledby="settings-address-title">
                <h2 id="settings-address-title">Endereço da barbearia</h2>
                <p>Informe onde os clientes serão atendidos. Somente o complemento é opcional.</p>
                <fieldset
                  disabled={locked}
                  className={styles.fields}
                  aria-labelledby="settings-address-title"
                >
                  <div className={styles.short}>
                    {field("postal_code", {
                      required: true,
                      maxLength: 9,
                      inputMode: "numeric",
                      autoComplete: "postal-code",
                      placeholder: "01310-100",
                    })}
                  </div>
                  {field("street", {
                    required: true,
                    maxLength: 150,
                    autoComplete: "address-line1",
                  })}
                  <div className={styles.short}>
                    {field("number", {
                      required: true,
                      maxLength: 20,
                      hint: "Para imóvel sem número, informe S/N.",
                    })}
                  </div>
                  {field("complement", {
                    label: "Complemento (opcional)",
                    maxLength: 100,
                    autoComplete: "address-line2",
                  })}
                  {field("district", { required: true, maxLength: 100 })}
                  {field("city", {
                    required: true,
                    maxLength: 100,
                    autoComplete: "address-level2",
                  })}
                  <div className={styles.short}>
                    <SelectField
                      id="settings-state"
                      label="Estado"
                      required
                      autoComplete="address-level1"
                      value={editor.draft.address.state}
                      error={errors.state}
                      onBlur={() =>
                        setErrors((current) => ({
                          ...current,
                          state: validateSettingsDraft(editor.draft!).state,
                        }))
                      }
                      onChange={(event) => {
                        editor.setDraft({
                          ...editor.draft!,
                          address: { ...editor.draft!.address, state: event.target.value },
                        });
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
                  </div>
                </fieldset>
              </section>
              <div className={styles.actions}>
                <Button
                  type="submit"
                  disabled={locked || !editor.dirty}
                  loading={editor.mode === "saving"}
                >
                  <Save size={16} aria-hidden="true" />
                  {editor.mode === "saving" ? "Salvando…" : "Salvar alterações"}
                </Button>
                <Button
                  variant="secondary"
                  disabled={locked || !editor.dirty}
                  onClick={() => setConfirm("reset")}
                >
                  Descartar alterações
                </Button>
                <p>
                  {editor.mode === "saving"
                    ? "Aguarde a confirmação da API."
                    : editor.dirty
                      ? "Há alterações não salvas."
                      : "Nenhuma alteração pendente."}
                </p>
              </div>
            </form>
            <section className={styles.section} aria-labelledby="settings-readonly-title">
              <h2 id="settings-readonly-title">Endereço reservado e fuso horário</h2>
              <p>
                Estas informações não são editáveis nesta etapa. Alterar o cadastro não publica a
                barbearia nem inicia uma assinatura.
              </p>
              <dl className={styles.readonly}>
                <div>
                  <dt>Endereço reservado para agendamento</dt>
                  <dd>/b/{editor.settings.slug}</dd>
                </div>
                <div>
                  <dt>Fuso horário da agenda</dt>
                  <dd>{editor.settings.timezone}</dd>
                </div>
              </dl>
            </section>
          </div>
        ) : null}
      </div>
      {confirm && (
        <Modal
          open
          title={
            confirm === "reload" ? "Recarregar e descartar alterações?" : "Descartar alterações?"
          }
          onClose={() => setConfirm(null)}
          subtitle="As alterações ainda não salvas serão perdidas. Nenhum dado será enviado para atualização."
        >
          <div className={styles.actions}>
            <Button variant="secondary" onClick={() => setConfirm(null)}>
              Continuar editando
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                const action = confirm;
                setConfirm(null);
                setErrors({});
                if (action === "reload") void editor.reload();
                else editor.reset();
              }}
            >
              {confirm === "reload" ? "Descartar e recarregar" : "Descartar alterações"}
            </Button>
          </div>
        </Modal>
      )}
    </section>
  );
}
