import type { Sale } from "@/lib/api/sales";
import { money } from "@/lib/model";
import { Alert, Badge } from "../ui";
import { commerceTime } from "./shared";
import styles from "./commerce.module.css";

export function SaleReceipt({ sale, timezone }: { sale: Sale; timezone?: string }) {
  return (
    <section className={styles.receipt} aria-label="Comprovante da venda">
      <div className={styles.rowTop}>
        <h3>Comprovante da venda</h3>
        <Badge tone={sale.status === "recorded" ? "success" : "neutral"}>
          {sale.status === "recorded" ? "Registrada" : "Anulada"}
        </Badge>
      </div>
      <dl>
        <div>
          <dt>Registro</dt>
          <dd>{sale.id}</dd>
        </div>
        <div>
          <dt>Data</dt>
          <dd>
            {commerceTime(sale.sold_at, timezone)}
            {!timezone && " (horário do dispositivo)"}
          </dd>
        </div>
        <div>
          <dt>Barbeiro</dt>
          <dd>{sale.professional_name || "Sem barbeiro"}</dd>
        </div>
        {sale.appointment_id && (
          <div>
            <dt>Agendamento vinculado</dt>
            <dd>{sale.appointment_id}</dd>
          </div>
        )}
      </dl>
      <ul className={styles.history}>
        {sale.items.map((i) => (
          <li key={i.id}>
            <div className={styles.rowTop}>
              <strong>{i.product_name}</strong>
              <strong>{money(i.line_total_cents)}</strong>
            </div>
            <p className={styles.muted}>
              {i.quantity} un. × {money(i.unit_price_cents)}
            </p>
          </li>
        ))}
      </ul>
      <div className={styles.total}>
        <span>Total registrado</span>
        <strong>{money(sale.total_cents)}</strong>
      </div>
      {sale.cancellation && (
        <Alert tone="warning">
          <p>
            <strong>Anulada em {commerceTime(sale.cancellation.cancelled_at, timezone)}</strong>
          </p>
          <p>{sale.cancellation.reason}</p>
          <p>
            {sale.cancellation.restock
              ? "As quantidades foram devolvidas ao estoque."
              : "As quantidades continuam retiradas do estoque."}
          </p>
        </Alert>
      )}
      <p className={styles.muted}>
        Este registro não comprova recebimento de pagamento. O estado de um agendamento vinculado
        permanece separado.
      </p>
    </section>
  );
}
