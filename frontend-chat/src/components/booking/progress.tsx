import { Check } from "lucide-react";
import styles from "./booking.module.css";

export type BookingStage = "service" | "schedule" | "contact" | "review" | "complete";
const stages = [
  { key: "service", label: "Serviço" },
  { key: "schedule", label: "Horário" },
  { key: "contact", label: "Seus dados" },
  { key: "review", label: "Confirmação" },
] as const;

export function BookingProgress({ stage }: { stage: BookingStage }) {
  const active =
    stage === "complete" ? stages.length : stages.findIndex((item) => item.key === stage);
  return (
    <ol className={styles.progress} aria-label="Etapas do agendamento">
      {stages.map((item, index) => (
        <li
          key={item.key}
          aria-current={active === index ? "step" : undefined}
          data-complete={index < active || undefined}
        >
          <span aria-hidden="true">{index < active ? <Check size={14} /> : index + 1}</span>
          <span>
            {item.label}
            <span className={styles.srOnly}>
              {index < active ? " concluída" : active === index ? ", etapa atual" : ""}
            </span>
          </span>
        </li>
      ))}
    </ol>
  );
}
