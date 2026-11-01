"use client";
import { useCallback, useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { errorMessage } from "@/lib/api/client";
import {
  getActivityReport,
  reportFilterError,
  reportMonth,
  type ReportFilters,
  type ReportGroup,
  type ReportOverview,
  type ReportScope,
  type ReportSource,
} from "@/lib/api/reports";
import { useCommerceScope } from "../commerce/shared";
import { useSelectedShopAccess } from "../shop-panel";
import { Alert, Button, PageHeader, SelectField, TextField } from "../ui";
import { ReportResults } from "./report-results";
import styles from "./reports.module.css";

type Choices = { professionals: ReportGroup[]; services: ReportGroup[]; products: ReportGroup[] };
const choicesFrom = (o: ReportOverview): Choices => ({
  professionals: o.by_professional.filter((g) => g.id),
  services: o.by_service,
  products: o.by_product,
});
export function ReportsPage() {
  const base = useCommerceScope(),
    access = useSelectedShopAccess();
  const scope = useMemo<ReportScope>(
    () => ({ ...base, professionalID: access.role === "barber" ? access.professional_id : null }),
    [base, access.role, access.professional_id],
  );
  return (
    <ReportStart
      key={`${scope.userID}:${scope.shopID}:${access.role}:${scope.professionalID}`}
      scope={scope}
      owner={access.role === "owner"}
    />
  );
}
function ReportStart({ scope, owner }: { scope: ReportScope; owner: boolean }) {
  const [initial, setInitial] = useState<{
      filters: ReportFilters;
      seed?: ReportOverview;
      choices: Choices;
    } | null>(null),
    [error, setError] = useState(""),
    [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    setError("");
    // Bootstrap timezone through the same authorized read, including for barber
    // accounts that cannot access owner settings. Never display the probe month.
    const probe: ReportFilters = { ...reportMonth(new Date().toISOString(), "UTC"), source: "all" };
    void getActivityReport(scope, probe, c.signal)
      .then((out) => {
        if (c.signal.aborted) return;
        const filters: ReportFilters = {
          ...reportMonth(out.observed_at, out.period.timezone),
          source: "all",
        };
        setInitial({
          filters,
          choices: choicesFrom(out),
          seed: filters.from === probe.from && filters.through === probe.through ? out : undefined,
        });
      })
      .catch((cause) => {
        if (!c.signal.aborted) setError(errorMessage(cause));
      });
    return () => c.abort();
  }, [scope, attempt]);
  if (initial) return <ReportContent scope={scope} owner={owner} initial={initial} />;
  return (
    <section className={styles.page}>
      <PageHeader
        title="Relatórios"
        description="Atendimentos e vendas, com os valores separados por origem."
      />
      {error ? (
        <>
          <Alert tone="error">{error}</Alert>
          <Button variant="secondary" onClick={() => setAttempt((a) => a + 1)}>
            Tentar novamente
          </Button>
        </>
      ) : (
        <p role="status">Carregando os relatórios da barbearia…</p>
      )}
    </section>
  );
}
function ReportContent({
  scope,
  owner,
  initial,
}: {
  scope: ReportScope;
  owner: boolean;
  initial: { filters: ReportFilters; seed?: ReportOverview; choices: Choices };
}) {
  const [filters, setFilters] = useState(initial.filters),
    [draft, setDraft] = useState(initial.filters),
    [formError, setFormError] = useState(""),
    [choices, setChoices] = useState(initial.choices),
    [revision, setRevision] = useState(0);
  const options = useCallback(
    (out: ReportOverview) =>
      setChoices((old) => {
        const next = choicesFrom(out);
        const merge = (a: ReportGroup[], b: ReportGroup[]) =>
          [...new Map([...a, ...b].map((g) => [g.id, g])).values()].sort((a, b) =>
            a.label.localeCompare(b.label, "pt-BR"),
          );
        return {
          professionals: merge(old.professionals, next.professionals),
          services: merge(old.services, next.services),
          products: merge(old.products, next.products),
        };
      }),
    [],
  );
  const changeSource = (source: ReportSource) =>
    setDraft((d) => ({
      ...d,
      source,
      service_id: source === "products" ? undefined : d.service_id,
      product_id: source === "services" ? undefined : d.product_id,
    }));
  return (
    <section className={styles.page}>
      <PageHeader
        title="Relatórios"
        description={
          owner
            ? "Acompanhe os atendimentos e as vendas da barbearia."
            : "Acompanhe os atendimentos e as vendas atribuídos a você."
        }
        actions={
          <Button variant="secondary" onClick={() => setRevision((r) => r + 1)}>
            <RefreshCw size={16} aria-hidden="true" />
            Atualizar relatório
          </Button>
        }
      />
      <form
        className={styles.filters}
        aria-label="Filtros dos relatórios"
        onSubmit={(e) => {
          e.preventDefault();
          const next = {
            ...draft,
            customer_name: draft.customer_name?.trim(),
            phone: draft.phone?.trim(),
            phone_country:
              draft.phone?.trim() && !draft.phone.trim().startsWith("+") ? "BR" : undefined,
          };
          const error = reportFilterError(next);
          setFormError(error);
          if (!error) {
            setFilters(next);
            setRevision((r) => r + 1);
          }
        }}
      >
        <div className={styles.mainFilters}>
          <TextField
            label="De"
            type="date"
            required
            min="1900-01-01"
            max={draft.through || "9998-12-31"}
            value={draft.from}
            onChange={(e) => setDraft((d) => ({ ...d, from: e.target.value }))}
          />
          <TextField
            label="Até"
            type="date"
            required
            min={draft.from || "1900-01-01"}
            max="9998-12-31"
            value={draft.through}
            onChange={(e) => setDraft((d) => ({ ...d, through: e.target.value }))}
          />
          <SelectField
            label="Origem"
            value={draft.source}
            onChange={(e) => changeSource(e.target.value as ReportSource)}
          >
            <option value="all">Atendimentos e produtos</option>
            <option value="services">Atendimentos</option>
            <option value="products">Produtos</option>
          </SelectField>
          {owner && (
            <SelectField
              label="Barbeiro"
              value={draft.professional_id || ""}
              onChange={(e) =>
                setDraft((d) => ({ ...d, professional_id: e.target.value || undefined }))
              }
            >
              <option value="">Todos os barbeiros</option>
              {choices.professionals.map((g) => (
                <option key={g.id} value={g.id!}>
                  {g.label}
                </option>
              ))}
            </SelectField>
          )}
          <Button type="submit">Aplicar filtros</Button>
        </div>
        <details className={styles.disclosure}>
          <summary>Mais filtros</summary>
          <div className={styles.extraFilters}>
            <TextField
              label="Nome do cliente"
              maxLength={201}
              autoComplete="off"
              value={draft.customer_name || ""}
              onChange={(e) => setDraft((d) => ({ ...d, customer_name: e.target.value }))}
            />
            <TextField
              label="Telefone do cliente"
              type="tel"
              autoComplete="off"
              maxLength={40}
              hint="Com DDD. Para outro país, inclua +código."
              value={draft.phone || ""}
              onChange={(e) => setDraft((d) => ({ ...d, phone: e.target.value }))}
            />
            {draft.source !== "products" && (
              <SelectField
                label="Serviço"
                value={draft.service_id || ""}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, service_id: e.target.value || undefined }))
                }
              >
                <option value="">Todos os serviços</option>
                {choices.services.map((g) => (
                  <option key={g.id} value={g.id!}>
                    {g.label}
                  </option>
                ))}
              </SelectField>
            )}
            {draft.source !== "services" && (
              <SelectField
                label="Produto"
                value={draft.product_id || ""}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, product_id: e.target.value || undefined }))
                }
              >
                <option value="">Todos os produtos</option>
                {choices.products.map((g) => (
                  <option key={g.id} value={g.id!}>
                    {g.label}
                  </option>
                ))}
              </SelectField>
            )}
          </div>
          <p className={styles.muted}>
            As opções incluem registros históricos. Em “Atendimentos e produtos”, serviço filtra
            apenas atendimentos e produto filtra apenas itens vendidos. Vendas avulsas sem contato
            não entram na busca por nome ou telefone.
          </p>
        </details>
        <div className={styles.filterFooter}>
          <span className={styles.muted}>Consulte até 366 dias por vez.</span>
          <Button
            variant="quiet"
            onClick={() => {
              setDraft(initial.filters);
              setFilters(initial.filters);
              setRevision((r) => r + 1);
              setFormError("");
            }}
          >
            Limpar filtros
          </Button>
        </div>
        {formError && <Alert tone="error">{formError}</Alert>}
      </form>
      <ReportResults
        key={JSON.stringify(filters)}
        scope={scope}
        owner={owner}
        filters={filters}
        seed={
          JSON.stringify(filters) === JSON.stringify(initial.filters) ? initial.seed : undefined
        }
        revision={revision}
        onOverview={options}
      />
    </section>
  );
}
