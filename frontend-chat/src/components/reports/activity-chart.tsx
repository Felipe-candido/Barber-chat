"use client";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { reportMoney, type ReportOverview } from "@/lib/api/reports";
import styles from "./reports.module.css";

export function ActivityChart({ report, currency }: { report: ReportOverview; currency: string }) {
  const host = useRef<HTMLDivElement>(null),
    title = useId();
  const [width, setWidth] = useState(680);
  useEffect(() => {
    const observer = new ResizeObserver(([entry]) =>
      setWidth(Math.max(260, Math.round(entry.contentRect.width))),
    );
    if (host.current) observer.observe(host.current);
    return () => observer.disconnect();
  }, []);
  const { points, grouping } = useMemo(() => {
    const { from_date: from, through_date: through } = report.period;
    const days =
      Math.round(
        (Date.parse(through + "T12:00:00Z") - Date.parse(from + "T12:00:00Z")) / 86400000,
      ) + 1;
    const grouping = days <= 31 ? "dia" : days <= 92 ? "semana" : "mês";
    const source = new Map(
      report.by_day.map((day) => [day.local_date, day.values.find((v) => v.currency === currency)]),
    );
    const bins = new Map<
      string,
      { date: string; end: string; services: number; products: number }
    >();
    for (let i = 0; i < days; i++) {
      const date = new Date(from + "T12:00:00Z");
      date.setUTCDate(date.getUTCDate() + i);
      const key = date.toISOString().slice(0, 10),
        group =
          grouping === "dia"
            ? key
            : grouping === "semana"
              ? String(Math.floor(i / 7))
              : key.slice(0, 7);
      const point = bins.get(group) || { date: key, end: key, services: 0, products: 0 },
        value = source.get(key);
      point.end = key;
      point.services += value?.completed_service_value_cents || 0;
      point.products += value?.product_sale_value_cents || 0;
      bins.set(group, point);
    }
    return { points: [...bins.values()], grouping };
  }, [report, currency]);
  const height = 240,
    left = 76,
    right = 18,
    top = 20,
    bottom = 38;
  const maximum = Math.max(1, ...points.flatMap((p) => [p.services, p.products]));
  const step = 10 ** Math.floor(Math.log10(maximum / 4 || 1)),
    ceiling = Math.ceil(maximum / (step * 4)) * step * 4;
  const x = (i: number) =>
      left + (points.length === 1 ? 0.5 : i / (points.length - 1)) * (width - left - right),
    y = (value: number) => top + (1 - value / ceiling) * (height - top - bottom);
  const label = (date: string) =>
    new Intl.DateTimeFormat("pt-BR", { timeZone: "UTC", day: "2-digit", month: "2-digit" }).format(
      new Date(date + "T12:00:00Z"),
    );
  const tickCount = Math.min(points.length, Math.max(2, Math.floor((width - left - right) / 95)));
  const ticks = new Set(
    Array.from({ length: tickCount }, (_, i) =>
      tickCount === 1 ? 0 : Math.round((i * (points.length - 1)) / (tickCount - 1)),
    ),
  );
  const hasValue = points.some((p) => p.services || p.products);
  return (
    <section className={styles.panel} aria-labelledby={title}>
      <div className={styles.sectionHeader}>
        <div>
          <h2 id={title}>Evolução dos valores</h2>
          <p className={styles.muted}>
            Por {grouping}, em {currency}. Dias sem registros entram com zero.
          </p>
        </div>
        <div className={styles.legend}>
          <span>
            <i className={styles.serviceLine} />
            Serviços
          </span>
          <span>
            <i className={styles.productLine} />
            Produtos
          </span>
        </div>
      </div>
      {!hasValue && <p className={styles.muted}>Nenhum valor registrado neste período.</p>}
      <div ref={host} className={styles.chart}>
        <svg
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label={`Valores de serviços e produtos por ${grouping}, em ${currency}`}
        >
          <title>
            Serviços concluídos e produtos vendidos. A tabela abaixo contém os valores exatos.
          </title>
          {[0, 1, 2, 3, 4].map((i) => {
            const value = (ceiling * i) / 4;
            return (
              <g key={i}>
                <line
                  x1={left}
                  x2={width - right}
                  y1={y(value)}
                  y2={y(value)}
                  className={styles.gridLine}
                />
                <text x={left - 10} y={y(value) + 4} textAnchor="end" className={styles.axis}>
                  {new Intl.NumberFormat("pt-BR", {
                    style: "currency",
                    currency,
                    notation: "compact",
                    maximumFractionDigits: 1,
                  }).format(value / 100)}
                </text>
              </g>
            );
          })}
          <polyline
            points={points.map((p, i) => `${x(i)},${y(p.services)}`).join(" ")}
            className={styles.services}
          />
          <polyline
            points={points.map((p, i) => `${x(i)},${y(p.products)}`).join(" ")}
            className={styles.products}
          />
          {points.length === 1 && (
            <>
              <circle cx={x(0)} cy={y(points[0].services)} r="4" className={styles.serviceDot} />
              <rect
                x={x(0) - 4}
                y={y(points[0].products) - 4}
                width="8"
                height="8"
                className={styles.productDot}
              />
            </>
          )}
          {points.map((p, i) =>
            ticks.has(i) ? (
              <text
                key={p.date}
                x={x(i)}
                y={height - 10}
                textAnchor={i === 0 ? "start" : i === points.length - 1 ? "end" : "middle"}
                className={styles.axis}
              >
                {label(p.date)}
              </text>
            ) : null,
          )}
        </svg>
      </div>
      <details className={styles.disclosure}>
        <summary>Ver valores do gráfico em tabela</summary>
        <div
          className={styles.tableScroll}
          tabIndex={0}
          role="region"
          aria-label="Tabela dos valores por período"
        >
          <table>
            <caption>
              Valores agrupados por {grouping}, em {currency}
            </caption>
            <thead>
              <tr>
                <th scope="col">Período</th>
                <th scope="col">Serviços</th>
                <th scope="col">Produtos</th>
              </tr>
            </thead>
            <tbody>
              {points.map((p) => (
                <tr key={p.date}>
                  <th scope="row">
                    {label(p.date)}
                    {p.end !== p.date && ` a ${label(p.end)}`}
                  </th>
                  <td>{reportMoney(p.services, currency)}</td>
                  <td>{reportMoney(p.products, currency)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </section>
  );
}
