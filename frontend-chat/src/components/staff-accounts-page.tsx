"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
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

export function StaffAccountsPage() {
  const access = useSelectedShopAccess();
  if (access.role !== "owner")
    return (
      <section className="api-error">
        <h1>Acesso restrito ao proprietário</h1>
        <p>Você pode consultar e criar seus agendamentos pela agenda.</p>
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
  const [recovery, setRecovery] = useState<Record<string, string>>({});
  const [replacementEmail, setReplacementEmail] = useState<Record<string, string>>({});
  const [replacementConfirmed, setReplacementConfirmed] = useState<Record<string, boolean>>({});
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
    (p) => p.active && !accounts.some((a) => a.professional_id === p.id),
  );
  async function mutate(operation: () => Promise<unknown>, message: string) {
    if (inFlight.current) return;
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
    <section className="professionals-page">
      <header className="page-heading">
        <div>
          <p className="eyebrow">ACESSO DA EQUIPE</p>
          <h1>Contas dos barbeiros</h1>
          <p>Um login individual, vinculado a um barbeiro já cadastrado.</p>
        </div>
        <button
          className="button button-secondary"
          disabled={loading || saving}
          onClick={() => void reload()}
        >
          Atualizar contas
        </button>
      </header>
      <p className="booking-note">
        O barbeiro vê a própria agenda e cria agendamentos para si. Serviços, preços e equipe
        continuam sob gestão do proprietário.
      </p>
      {error && (
        <p className="form-error" role="alert">
          {error} Confira a lista antes de repetir qualquer envio.
        </p>
      )}
      {success && (
        <p className="professional-success" role="status">
          {success}
        </p>
      )}
      {loadError && (
        <p className="form-error" role="alert">
          {loadError}
        </p>
      )}
      <div className="professionals-layout">
        <section className="professional-card">
          <h2>Acessos cadastrados</h2>
          {loading && <p role="status">Carregando contas…</p>}
          {!loading && !loadError && !accounts.length && (
            <p>Nenhuma conta de barbeiro criada nesta unidade.</p>
          )}
          <ul className="professional-list">
            {accounts.map((a) => (
              <li key={a.id}>
                <strong>
                  {professionals.find((p) => p.id === a.professional_id)?.name ?? "Barbeiro"}
                </strong>
                <span>{a.email}</span>
                <span>
                  {a.state === "linked"
                    ? a.active
                      ? "Acesso ativo"
                      : "Acesso inativo"
                    : "Vínculo precisa ser conferido"}
                </span>
                {a.state === "linked" && (
                  <button
                    className="button button-secondary"
                    disabled={saving || loading || !!loadError}
                    onClick={() =>
                      void mutate(
                        () => setStaffAccountActive(shop.slug, a.id, !a.active),
                        a.active
                          ? "Acesso desativado nesta barbearia."
                          : "Acesso ativado nesta barbearia.",
                      )
                    }
                  >
                    {a.active ? "Desativar acesso" : "Ativar acesso"}
                  </button>
                )}
                {a.state !== "linked" && (
                  <details>
                    <summary>Conferir conta existente no Supabase</summary>
                    <p>
                      Confira se o e-mail existe em Authentication → Users. Cole o UUID dessa conta
                      para concluir o vínculo. O backend confere se o e-mail coincide. Se ela ainda
                      não existir, crie ou convide no Supabase antes de vincular aqui.
                    </p>
                    <label className="booking-field">
                      UUID da conta Supabase
                      <input
                        value={recovery[a.id] ?? ""}
                        onChange={(e) =>
                          setRecovery((v) => ({ ...v, [a.id]: e.target.value.trim() }))
                        }
                        disabled={saving}
                      />
                    </label>
                    <button
                      className="button button-secondary"
                      disabled={saving || loading || !recovery[a.id] || !!loadError}
                      onClick={() =>
                        void mutate(
                          () => linkStaffAccount(shop.slug, a.id, recovery[a.id]),
                          "Conta conferida e vinculada ao barbeiro.",
                        )
                      }
                    >
                      Concluir vínculo
                    </button>
                  </details>
                )}
                {!a.active && (
                  <details>
                    <summary>Substituir conta</summary>
                    <form
                      onSubmit={(e) => {
                        e.preventDefault();
                        const newEmail = replacementEmail[a.id]?.trim().toLowerCase();
                        if (
                          !newEmail ||
                          newEmail === a.email.toLowerCase() ||
                          !replacementConfirmed[a.id]
                        )
                          return;
                        void mutate(
                          () => replaceStaffAccount(shop.slug, a.id, newEmail),
                          "Conta substituída. O Supabase recebeu a solicitação de convite para o novo e-mail.",
                        );
                      }}
                    >
                      <p>
                        O acesso antigo será desvinculado somente desta barbearia. O barbeiro e seus
                        agendamentos serão preservados. A conta antiga no Supabase não será apagada.
                        Se o convite falhar, confira o novo vínculo na lista.
                      </p>
                      <label className="booking-field">
                        Novo e-mail de acesso
                        <input
                          type="email"
                          required
                          maxLength={254}
                          autoComplete="off"
                          value={replacementEmail[a.id] ?? ""}
                          disabled={saving || loading || !!loadError}
                          onChange={(e) => {
                            setReplacementEmail((v) => ({ ...v, [a.id]: e.target.value }));
                            setReplacementConfirmed((v) => ({ ...v, [a.id]: false }));
                          }}
                        />
                      </label>
                      <label>
                        <input
                          type="checkbox"
                          required
                          checked={replacementConfirmed[a.id] ?? false}
                          disabled={saving || loading || !!loadError}
                          onChange={(e) =>
                            setReplacementConfirmed((v) => ({ ...v, [a.id]: e.target.checked }))
                          }
                        />{" "}
                        Confirmo a remoção do vínculo antigo nesta barbearia.
                      </label>
                      <button
                        className="button button-secondary"
                        disabled={
                          saving ||
                          loading ||
                          !!loadError ||
                          !replacementConfirmed[a.id] ||
                          !replacementEmail[a.id]?.trim() ||
                          replacementEmail[a.id].trim().toLowerCase() === a.email.toLowerCase()
                        }
                      >
                        Substituir e enviar convite
                      </button>
                    </form>
                  </details>
                )}
              </li>
            ))}
          </ul>
        </section>
        <form
          className="professional-card"
          onSubmit={(e) => {
            e.preventDefault();
            void mutate(
              () => inviteStaffAccount(shop.slug, professionalID, email.trim()),
              "Conta vinculada. O Supabase recebeu a solicitação de convite; peça ao barbeiro para conferir o e-mail e definir a senha.",
            );
          }}
        >
          <h2>Criar conta de acesso</h2>
          <p>O cadastro do barbeiro e o login são separados.</p>
          <label className="booking-field">
            Barbeiro vinculado
            <select
              required
              value={professionalID}
              onChange={(e) => setProfessionalID(e.target.value)}
              disabled={saving || loading || !!loadError}
            >
              <option value="">Selecione um barbeiro</option>
              {available.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          <label className="booking-field">
            E-mail do barbeiro
            <input
              type="email"
              autoComplete="email"
              required
              maxLength={254}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              disabled={saving || loading}
            />
          </label>
          <p className="booking-note">
            Ele receberá um link para definir a própria senha. O proprietário não informa nem recebe
            a senha.
          </p>
          <button
            className="button button-primary"
            disabled={
              saving ||
              loading ||
              !!loadError ||
              !available.some((p) => p.id === professionalID) ||
              !email.trim()
            }
          >
            {saving ? "Processando…" : "Criar conta e enviar convite"}
          </button>
          {!loading && !loadError && !available.length && (
            <p>
              Todos os barbeiros ativos já têm conta ou vínculo pendente. Para corrigir um e-mail,
              desative o acesso e use “Substituir conta” na lista. Para um novo barbeiro, faça o
              cadastro primeiro.
            </p>
          )}
          <Link href={adminShopPath(shop.slug) + "/barbeiros"}>Ir para cadastro de barbeiros</Link>
        </form>
      </div>
    </section>
  );
}
