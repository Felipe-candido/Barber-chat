import type { ReactNode } from "react";
import type { FeedbackTone } from "./alert";
import styles from "./foundation.module.css";

export function Badge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: FeedbackTone | "neutral";
}) {
  return <span className={styles.badge + " " + styles[tone]}>{children}</span>;
}
