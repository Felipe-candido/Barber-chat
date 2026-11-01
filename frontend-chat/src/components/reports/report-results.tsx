"use client";
import { useCallback, useEffect, useState } from "react";
import { ChartNoAxesCombined } from "lucide-react";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  getActivityHistory,
  getActivityReport,
  reportMoney,
  type ReportFilters,
  type ReportHistoryItem,
  type ReportOverview,
  type ReportScope,
  type HistoryStatuses,
} from "@/lib/api/reports";
import { useCommercePage } from "@/hooks/use-commerce-page";
import { Alert, Badge, Button, SelectField } from "../ui";
import { ActivityChart } from "./activity-chart";
import styles from "./reports.module.css";

export function ReportResults({
  scope,
  owner,
  filters,
  seed,
  revision,
  onOverview,
}: {
  scope: ReportScope;
  owner: boolean;
  filters: ReportFilters;
  seed?: ReportOverview;
  revision: number;
  onOverview: (o: ReportOverview) => void;
}) {
  const [overview, setOverview] = useState<ReportOverview | null>(
      revision === 0 ? seed || null : null,
    ),
    [loading, setLoading] = useState(revision !== 0 || !seed),
    [error, setError] = useState(""),
    [denied, setDenied] = useState(false),
    [statuses, setStatuses] = useState<HistoryStatuses>({}),
    [currency, setCurrency] = useState("BRL"),
    [historyZone, setHistoryZone] = useState(seed?.period.timezone || "");
  useEffect(() => {
    const c = new AbortController();
    setDenied(false);
    setError("");
    if (seed && revision === 0) {
      onOverview(seed);
      return () => c.abort();
    }
    setOverview(null);
    setLoading(true);
    void getActivityReport(scope, filters, c.signal)
      .then((out) => {
        if (!c.signal.aborted) {
          setOverview(out);
          onOverview(out);
        }
      })
      .catch((cause) => {
        if (!c.signal.aborted) {
          setError(errorMessage(cause));
          if (cause instanceof ApiError && [401, 403].includes(cause.status)) setDenied(true);
        }
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [scope, filters, seed, revision, onOverview]);
  const load = useCallback(
    async (cursor: string | undefined, signal: AbortSignal) => {
      void revision;
      try {
        const out = await getActivityHistory(scope, filters, statuses, cursor, signal);
        if (!signal.aborted) setHistoryZone(out.period.timezone);
        return out;
      } catch (cause) {
        if (!signal.aborted && cause instanceof ApiError && [401, 403].includes(cause.status))
          setDenied(true);
        throw cause;
      }
    },
    [scope, filters, statuses, revision],
  );
  const history = useCommercePage(load);
  const currencies = overview?.values.map((v) => v.currency) || [],
    selected = currencies.includes(currency) ? currency : currencies[0] || "BRL";
  if (denied)
    return (
      <Alert tone="error">
        Seu acesso mudou. Atualize o relatório ou entre novamente para continuar.
      </Alert>
    );
  return (
    <>
      {loading && (
        <p role="status" className={styles.loading}>
          Carregando indicadores…
        </p>
      )}
      {error && (
        <Alert tone="error">
          Não foi possível carregar o resumo. {error} Use “Atualizar relatório” para tentar
          novamente.
        </Alert>
      )}
      {overview && (
        <>
          <p className={styles.muted}>
            Datas e horários em {overview.period.timezone}. Consulta de{" "}
            {new Intl.DateTimeFormat("pt-BR", {
              timeZone: overview.period.timezone,
              dateStyle: "short",
              timeStyle: "short",
            }).format(new Date(overview.observed_at))}
            .
          </p>
          <div className={styles.metrics} aria-label="Valores registrados no período">
            {(filters.source === "all"
              ? [
                  ["Valor registrado", "combined_value_cents"],
                  ["Em serviços", "completed_service_value_cents"],
                  ["Em produtos", "product_sale_value_cents"],
                ]
              : filters.source === "services"
                ? [["Em serviços", "completed_service_value_cents"]]
                : [["Em produtos", "product_sale_value_cents"]]
            ).map(([label, key], i) => (
              <dl key={key} className={i === 0 ? styles.featuredMetric : styles.metric}>
                <dt>{label}</dt>
                <dd>
                  {overview.values.length
                    ? overview.values.map((v) => (
                        <span key={v.currency}>
                          {reportMoney(v[key as "combined_value_cents"], v.currency)}
                        </span>
                      ))
                    : reportMoney(0)}
                </dd>
              </dl>
            ))}
          </div>
          <div className={styles.counts} aria-label="Quantidades no período">
            {filters.source !== "products" && (
              <dl>
                <dt>Atendimentos concluídos</dt>
                <dd>{overview.counts.completed.toLocaleString("pt-BR")}</dd>
              </dl>
            )}
            {filters.source !== "services" && (
              <>
                <dl>
                  <dt>Vendas registradas</dt>
                  <dd>
                    {overview.values
                      .reduce((n, v) => n + v.product_sale_count, 0)
                      .toLocaleString("pt-BR")}
                  </dd>
                </dl>
                <dl>
                  <dt>Unidades vendidas</dt>
                  <dd>
                    {overview.values
                      .reduce((n, v) => n + v.product_unit_count, 0)
                      .toLocaleString("pt-BR")}
                  </dd>
                </dl>
              </>
            )}
            {filters.source !== "products" && (
              <dl>
                <dt>Previsão em serviços</dt>
                <dd>
                  {overview.values.length
                    ? overview.values.map((v) => (
                        <span key={v.currency}>
                          {reportMoney(v.scheduled_service_value_cents, v.currency)}
                        </span>
                      ))
                    : reportMoney(0)}
                </dd>
              </dl>
            )}
          </div>
          <p className={styles.muted}>
            Valores registrados, sem controle de recebimentos ou lucro.{" "}
            {filters.source !== "products" && (
              <>
                Dos concluídos, {overview.counts.completed_automatically} foram concluídos
                automaticamente. Ausências e cancelamentos ficam fora do valor dos serviços.
              </>
            )}
          </p>
          {filters.source !== "products" && (
            <p className={styles.muted}>
              Agendados ou confirmados: {overview.counts.scheduled + overview.counts.confirmed}.
              Ausências: {overview.counts.no_show}. Cancelados: {overview.counts.cancelled}.
            </p>
          )}
          {currencies.length > 1 && (
            <SelectField
              label="Moeda do gráfico e agrupamentos"
              value={selected}
              onChange={(e) => setCurrency(e.target.value)}
            >
              {currencies.map((c) => (
                <option key={c}>{c}</option>
              ))}
            </SelectField>
          )}
          <ActivityChart report={overview} currency={selected} />
          <details className={styles.panel}>
            <summary>Abrir valores por {owner ? "barbeiro, " : ""}serviço e produto</summary>
            <div className={styles.breakdowns}>
              {(owner
                ? ([
                    ["Por barbeiro", overview.by_professional],
                    ["Por serviço", overview.by_service],
                    ["Por produto", overview.by_product],
                  ] as const)
                : ([
                    ["Por serviço", overview.by_service],
                    ["Por produto", overview.by_product],
                  ] as const)
              ).map(([title, groups]) => (
                <section key={title}>
                  <h3>{title}</h3>
                  {groups.length ? (
                    <ul>
                      {[...groups]
                        .sort(
                          (a, b) =>
                            (b.values.find((v) => v.currency === selected)?.combined_value_cents ||
                              0) -
                            (a.values.find((v) => v.currency === selected)?.combined_value_cents ||
                              0),
                        )
                        .map((g) => (
                          <li key={g.id || "unassigned"}>
                            <span>{g.label || "Sem barbeiro"}</span>
                            <strong>
                              {reportMoney(
                                g.values.find((v) => v.currency === selected)
                                  ?.combined_value_cents || 0,
                                selected,
                              )}
                            </strong>
                          </li>
                        ))}
                    </ul>
                  ) : (
                    <p className={styles.muted}>Sem registros.</p>
                  )}
                </section>
              ))}
            </div>
            <p className={styles.muted}>
              Uma venda com vários produtos aparece em mais de um grupo. Os valores consideram
              somente os itens que passaram pelos filtros.
            </p>
          </details>
        </>
      )}
      <section className={styles.panel} aria-labelledby="report-history-title">
        <div className={styles.sectionHeader}>
          <div>
            <h2 id="report-history-title">Histórico de atividades</h2>
            <p className={styles.muted}>
              Uma linha por atendimento ou item vendido.
              {historyZone && ` Horários em ${historyZone}.`}
            </p>
          </div>
          <ChartNoAxesCombined size={22} aria-hidden="true" />
        </div>
        <div className={styles.historyFilters}>
          {filters.source !== "products" && (
            <SelectField
              label="Situação dos atendimentos"
              value={statuses.appointment_status || ""}
              onChange={(e) =>
                setStatuses((s) => ({
                  ...s,
                  appointment_status: e.target.value as HistoryStatuses["appointment_status"],
                }))
              }
            >
              <option value="">Todas</option>
              <option value="completed">Concluídos</option>
              <option value="scheduled">Agendados</option>
              <option value="confirmed">Confirmados</option>
              <option value="no_show">Ausências</option>
              <option value="cancelled">Cancelados</option>
            </SelectField>
          )}
          {filters.source !== "services" && (
            <SelectField
              label="Situação das vendas"
              value={statuses.sale_status || ""}
              onChange={(e) =>
                setStatuses((s) => ({
                  ...s,
                  sale_status: e.target.value as HistoryStatuses["sale_status"],
                }))
              }
            >
              <option value="">Todas</option>
              <option value="recorded">Registradas</option>
              <option value="cancelled">Anuladas</option>
            </SelectField>
          )}
        </div>
        <p className={styles.muted}>
          As situações filtram apenas este histórico. Os indicadores acima continuam considerando os
          filtros do relatório.
        </p>
        {history.error && (
          <Alert tone="error">
            {history.error}
            <div className={styles.retry}>
              <Button variant="secondary" onClick={history.refresh}>
                Recarregar histórico
              </Button>
            </div>
          </Alert>
        )}
        {!history.error && (
          <ol className={styles.history}>
            {history.items.map((item) => (
              <HistoryItem
                key={item.id}
                item={item}
                zone={overview?.period.timezone || historyZone}
              />
            ))}
          </ol>
        )}
        {history.loading && <p role="status">Carregando histórico…</p>}
        {!history.loading && !history.error && !history.items.length && (
          <div className={styles.empty}>
            <ChartNoAxesCombined size={28} aria-hidden="true" />
            <h3>Nenhuma atividade encontrada</h3>
            <p>Altere o período ou os filtros para consultar outros registros.</p>
          </div>
        )}
        {!!history.items.length && !history.error && (
          <p className={styles.muted}>
            {history.items.length} registros carregados.
            {history.next && " O histórico ainda é parcial."} Os totais do resumo consideram todo o
            período.
          </p>
        )}
        {history.next && !history.error && (
          <Button variant="secondary" disabled={history.loading} onClick={history.more}>
            Carregar mais atividades
          </Button>
        )}
      </section>
    </>
  );
}
function HistoryItem({ item: r, zone }: { item: ReportHistoryItem; zone?: string }) {
  const label =
    r.source === "products"
      ? r.sale_status === "cancelled"
        ? "Venda anulada"
        : "Venda registrada"
      : r.appointment_status === "completed"
        ? r.appointment_status_source === "automatic"
          ? "Concluído automaticamente"
          : "Concluído"
        : (
            {
              scheduled: "Agendado",
              confirmed: "Confirmado",
              cancelled: "Cancelado",
              no_show: "Ausência",
            } as Record<string, string>
          )[r.appointment_status || ""];
  const excluded =
    r.source === "products"
      ? r.sale_status === "cancelled"
      : ["cancelled", "no_show"].includes(r.appointment_status || "");
  return (
    <li className={styles.historyItem}>
      <div>
        <div className={styles.rowHeading}>
          <strong>{r.source === "products" ? `${r.quantity}× ${r.label}` : r.label}</strong>
          <Badge
            tone={
              excluded
                ? "neutral"
                : r.appointment_status === "completed" || r.sale_status === "recorded"
                  ? "success"
                  : "info"
            }
          >
            {label}
          </Badge>
        </div>
        <p>
          {r.customer_name || "Venda avulsa"}
          {r.customer_phone && <span className={styles.phone}>{r.customer_phone}</span>}
        </p>
        <p className={styles.muted}>
          {r.professional_name || "Sem barbeiro"} ·{" "}
          {new Intl.DateTimeFormat("pt-BR", {
            timeZone: zone || "UTC",
            dateStyle: "short",
            timeStyle: "short",
          }).format(new Date(r.occurred_at))}
        </p>
      </div>
      <div className={styles.historyValue}>
        {excluded ? (
          <>
            <s>{reportMoney(r.value_cents, r.currency)}</s>
            <strong>{reportMoney(0, r.currency)} no total</strong>
          </>
        ) : (
          <>
            <strong>{reportMoney(r.value_cents, r.currency)}</strong>
            {r.source === "services" &&
              ["scheduled", "confirmed"].includes(r.appointment_status || "") && (
                <span className={styles.muted}>Previsão</span>
              )}
          </>
        )}
      </div>
    </li>
  );
}
