"use client";

import {
  useId,
  type ComponentProps,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
  type ReactNode,
} from "react";
import styles from "./foundation.module.css";

type TextFieldProps = ComponentProps<"input"> & {
  label: string;
  hint?: string;
  error?: string;
  endAdornment?: ReactNode;
  reserveErrorSpace?: boolean;
};

export function TextField({
  label,
  hint,
  error,
  endAdornment,
  reserveErrorSpace = false,
  id,
  className,
  "aria-describedby": describedBy,
  ...props
}: TextFieldProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const hintId = `${inputId}-hint`;
  const errorId = `${inputId}-error`;
  const description = [describedBy, hint && hintId, error && errorId].filter(Boolean).join(" ");
  return (
    <div className={styles.field}>
      <label htmlFor={inputId}>{label}</label>
      {hint && (
        <p id={hintId} className={styles.hint}>
          {hint}
        </p>
      )}
      <div className={styles.controlWrap}>
        <input
          {...props}
          id={inputId}
          className={[styles.control, endAdornment && styles.withAdornment, className]
            .filter(Boolean)
            .join(" ")}
          aria-describedby={description || undefined}
          aria-invalid={error ? true : props["aria-invalid"]}
        />
        {endAdornment && <div className={styles.adornment}>{endAdornment}</div>}
      </div>
      {(error || reserveErrorSpace) && (
        <p
          id={errorId}
          aria-hidden={!error || undefined}
          className={[styles.fieldError, reserveErrorSpace && styles.fieldErrorSpace]
            .filter(Boolean)
            .join(" ")}
        >
          {error}
        </p>
      )}
    </div>
  );
}

type FieldProps = { label: string; hint?: string; error?: string };

// Native controls retain browser validation and keyboard behavior.
export function SelectField({
  label,
  hint,
  error,
  id,
  className,
  children,
  "aria-describedby": describedBy,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement> & FieldProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const description = [describedBy, hint && `${inputId}-hint`, error && `${inputId}-error`]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={styles.field}>
      <label htmlFor={inputId}>{label}</label>
      {hint && (
        <p id={`${inputId}-hint`} className={styles.hint}>
          {hint}
        </p>
      )}
      <select
        {...props}
        id={inputId}
        className={[styles.control, className].filter(Boolean).join(" ")}
        aria-describedby={description || undefined}
        aria-invalid={error ? true : props["aria-invalid"]}
      >
        {children}
      </select>
      {error && (
        <p id={`${inputId}-error`} className={styles.fieldError}>
          {error}
        </p>
      )}
    </div>
  );
}

export function TextareaField({
  label,
  hint,
  error,
  id,
  className,
  "aria-describedby": describedBy,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement> & FieldProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const description = [describedBy, hint && `${inputId}-hint`, error && `${inputId}-error`]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={styles.field}>
      <label htmlFor={inputId}>{label}</label>
      {hint && (
        <p id={`${inputId}-hint`} className={styles.hint}>
          {hint}
        </p>
      )}
      <textarea
        {...props}
        id={inputId}
        className={[styles.control, styles.textarea, className].filter(Boolean).join(" ")}
        aria-describedby={description || undefined}
        aria-invalid={error ? true : props["aria-invalid"]}
      />
      {error && (
        <p id={`${inputId}-error`} className={styles.fieldError}>
          {error}
        </p>
      )}
    </div>
  );
}
