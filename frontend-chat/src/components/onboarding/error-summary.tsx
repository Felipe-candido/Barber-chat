import type { RefObject } from "react";
import type { RegistrationErrors } from "@/lib/onboarding";
import styles from "./onboarding.module.css";

export function ErrorSummary({
  errors,
  labels,
  summaryRef,
}: {
  errors: RegistrationErrors;
  labels: Record<string, string>;
  summaryRef: RefObject<HTMLDivElement | null>;
}) {
  if (!Object.values(errors).some(Boolean)) return null;
  return (
    <div className={styles.summary} tabIndex={-1} ref={summaryRef} role="alert">
      <p>Confira os campos abaixo antes de continuar.</p>
      <ul>
        {Object.entries(errors)
          .filter(([, error]) => error)
          .map(([field, error]) => (
            <li key={field}>
              <a
                href={`#registration-${field}`}
                onClick={(event) => {
                  event.preventDefault();
                  document.getElementById(`registration-${field}`)?.focus();
                }}
              >
                {labels[field] ?? field}: {error}
              </a>
            </li>
          ))}
      </ul>
    </div>
  );
}
