"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { RefreshCw, UserRoundKey } from "lucide-react";
import { useSelectedShop, useSelectedShopAccess } from "./shop-panel";
import { listStaffProfessionals, type StaffProfessional } from "@/lib/api/professionals";
import {
  inviteStaffAccount,
  linkStaffAccount,
  listStaffAccounts,
  replaceStaffAccount,
  setStaffAccountActive,
  type StaffAccount,
} from "@/lib/api/staff-accounts";
import { errorMessage } from "@/lib/api/client";
import { adminShopPath } from "@/lib/api/config";
import { AccountItem } from "./accounts/account-item";
import { Alert, Button, PageHeader, SelectField, TextField } from "./ui";
import styles from "./admin/management.module.css";

export function StaffAccountsPage() {
  const access = useSelectedShopAccess();
  if (access.role !== "owner")
    return (
      <section className={styles.page}>
        <PageHeader
          title="Acesso restrito ao proprietário"
          description="Você pode consultar e criar seus agendamentos pela agenda."
        />
      </section>
    );
  return <OwnerAccounts />;
}

function OwnerAccounts() {
  const shop = useSelectedShop();
  const [professionals, setProfessionals] = useState<StaffProfessional[]>([]);
  const [accounts, setAccounts] = useState<StaffAccount[]>([]);
  const [professionalID, setProfessionalID] = useState("");
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const inFlight = useRef(false);
  const request = useRef<AbortController | null>(null);
  const reload = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setLoadError("");
    try {
      const [team, accesses] = await Promise.all([
        listStaffProfessionals(shop.slug, controller.signal),
        listStaffAccounts(shop.slug, controller.signal),
      ]);
      if (!controller.signal.aborted) {
        setProfessionals(team);
        setAccounts(accesses);
      }
    } catch (cause) {
      if (!controller.signal.aborted) setLoadError(errorMessage(cause));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [shop.slug]);
  useEffect(() => {
    void reload();
    return () => request.current?.abort();
  }, [reload]);
  const available = professionals.filter(
    (professional) =>
      professional.active &&
      !accounts.some((account) => account.professional_id === professional.id),
  );
  const disabled = saving || loading || !!loadError;
  async function mutate(operation: () => Promise<unknown>, message: string) {
    if (inFlight.current || loading || loadError) return;
    inFlight.current = true;
    setSaving(true);
    setError("");
    setSuccess("");
    try {
      await operation();
      setSuccess(message);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      await reload();
      inFlight.current = false;
      setSaving(false);
    }
  }
  return (
    <section className={styles.page}>
      <PageHeader
        title="Contas dos barbeiros"
        description={`Logins individuais para a equipe de ${shop.name}.`}
        actions={
          <Button variant="quiet" disabled={loading || saving} onClick={() => void reload()}>
            <RefreshCw size={16} aria-hidden="true" />
            Atualizar contas
          </Button>
        }
      />
      <p className={styles.hint}>
        O barbeiro vê a própria agenda e cria agendamentos para si. Serviços, preços e equipe
        continuam sob gestão do proprietário.
      </p>
      {error && (
        <Alert tone="error">{error} Confira a lista antes de repetir qualquer envio.</Alert>
      )}
      {success && <Alert tone="success">{success}</Alert>}
      {loadError && (
        <Alert tone="error">
          {loadError} Atualize as contas para conferir os vínculos antes de fazer alterações.
        </Alert>
      )}
      <div className={styles.layout}>
        <section className={styles.panel} aria-labelledby="staff-accounts-title">
          <h2 id="staff-accounts-title">Acessos cadastrados</h2>
          {loading && (
            <p className={styles.loading} role="status">
              Carregando contas…
            </p>
          )}
          {!loading && !loadError && !accounts.length && (
            <div className={styles.empty}>
              <UserRoundKey size={28} aria-hidden="true" />
              <h3>Nenhuma conta de barbeiro</h3>
              <p>Crie um acesso individual para um barbeiro já cadastrado nesta unidade.</p>
            </div>
          )}
          {/* Keep recovery inputs mounted during a refresh; never expose stale controls after a failed read. */}
          <ul className={styles.list} hidden={!!loadError || !accounts.length}>
            {accounts.map((account) => (
              <AccountItem
                key={account.id}
                account={account}
                professionalName={
                  professionals.find((professional) => professional.id === account.professional_id)
                    ?.name ?? "Barbeiro"
                }
                disabled={disabled}
                onStatus={() =>
                  void mutate(
                    () => setStaffAccountActive(shop.slug, account.id, !account.active),
                    account.active
                      ? "Acesso desativado nesta barbearia."
                      : "Acesso ativado nesta barbearia.",
                  )
                }
                onLink={(userID) =>
                  void mutate(
                    () => linkStaffAccount(shop.slug, account.id, userID),
                    "Conta conferida e vinculada ao barbeiro.",
                  )
                }
                onReplace={(newEmail) =>
                  void mutate(
                    () => replaceStaffAccount(shop.slug, account.id, newEmail),
                    "Conta substituída. O Supabase recebeu a solicitação de convite para o novo e-mail.",
                  )
                }
              />
            ))}
          </ul>
        </section>
        <section className={styles.panel} aria-labelledby="staff-invite-title">
          <div>
            <h2 id="staff-invite-title">Criar conta de acesso</h2>
            <p className={styles.hint}>
              Vincule o login a um barbeiro. Esse processo não cria outro profissional.
            </p>
          </div>
          <form
            className={styles.form}
            onSubmit={(event) => {
              event.preventDefault();
              if (
                disabled ||
                !available.some((professional) => professional.id === professionalID) ||
                !email.trim()
              )
                return;
              void mutate(
                () => inviteStaffAccount(shop.slug, professionalID, email.trim()),
                "Conta vinculada. O Supabase recebeu a solicitação de convite; peça ao barbeiro para conferir o e-mail e definir a senha.",
              );
            }}
          >
            <fieldset className={styles.fields} disabled={disabled || !available.length}>
              <SelectField
                label="Barbeiro vinculado"
                required
                value={professionalID}
                onChange={(event) => setProfessionalID(event.target.value)}
              >
                <option value="">Selecione um barbeiro</option>
                {available.map((professional) => (
                  <option key={professional.id} value={professional.id}>
                    {professional.name}
                  </option>
                ))}
              </SelectField>
              <TextField
                label="E-mail do barbeiro"
                type="email"
                autoComplete="email"
                required
                maxLength={254}
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                hint="Confira o endereço antes de enviar o convite."
              />
            </fieldset>
            <p className={styles.hint}>
              Ele receberá um link para definir a própria senha. O proprietário não informa nem
              recebe a senha.
            </p>
            <Button
              type="submit"
              loading={saving}
              disabled={
                disabled ||
                !available.some((professional) => professional.id === professionalID) ||
                !email.trim()
              }
            >
              {saving ? "Processando…" : "Criar conta e enviar convite"}
            </Button>
          </form>
          {!loading && !loadError && !available.length && (
            <p className={styles.hint}>
              Todos os barbeiros ativos já têm conta ou vínculo pendente. Para corrigir um e-mail,
              desative o acesso e use “Substituir conta” na lista. Para um novo barbeiro, faça o
              cadastro primeiro.
            </p>
          )}
          <Link href={adminShopPath(shop.slug) + "/barbeiros"}>Ir para cadastro de barbeiros</Link>
        </section>
      </div>
    </section>
  );
}
