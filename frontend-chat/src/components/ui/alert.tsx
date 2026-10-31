import { CircleAlert, CircleCheck, Info, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import styles from "./foundation.module.css";

export type FeedbackTone = "info" | "success" | "warning" | "error";
export const feedbackIcons = {
  info: Info,
  success: CircleCheck,
  warning: TriangleAlert,
  error: CircleAlert,
};

export function Alert({
  children,
  tone = "info",
  className = "",
  id,
}: {
  children: ReactNode;
  tone?: FeedbackTone;
  className?: string;
  id?: string;
}) {
  const Icon = feedbackIcons[tone];
  return (
    <div
      id={id}
      className={`${styles.alert} ${styles[tone]} ${className}`}
      role={tone === "error" ? "alert" : "status"}
    >
      <Icon size={20} aria-hidden="true" />
      <div>{children}</div>
    </div>
  );
}
