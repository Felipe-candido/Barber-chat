"use client";
import { useEffect, useId, useRef } from "react";
import { Scissors, X } from "lucide-react";
import { containDialogFocus } from "./ui/dialog-focus";

// Compatibility entry point: existing screens keep their imports during migration.
export { ProductBrand as Brand, ProductBrand, ShopBrand } from "./ui/brand";
export { Button, buttonClasses } from "./ui/button";
export { TextField, SelectField, TextareaField } from "./ui/form-field";
export { Alert } from "./ui/alert";
export { PageHeader } from "./ui/page-header";
export { Badge } from "./ui/badge";
export function Modal({
  open,
  onClose,
  title,
  subtitle,
  children,
  wide = false,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  subtitle?: string;
  children: React.ReactNode;
  wide?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const subtitleId = useId();
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (!open) {
      if (dialog.open) dialog.close();
      return;
    }
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (dialog.open && !dialog.matches(":modal")) dialog.close();
    if (!dialog.open) dialog.showModal();
    return () => {
      // Closing before removal also restores focus when the caller unmounts the modal.
      if (dialog.open) dialog.close();
      if (trigger?.isConnected) trigger.focus();
    };
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const prior = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prior;
    };
  }, [open]);
  return (
    <dialog
      ref={ref}
      aria-modal={open || undefined}
      onKeyDown={containDialogFocus}
      aria-labelledby={titleId}
      aria-describedby={subtitle ? subtitleId : undefined}
      className={"modal " + (wide ? "modal-wide" : "")}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          const r = e.currentTarget.getBoundingClientRect();
          if (
            e.clientX < r.left ||
            e.clientX > r.right ||
            e.clientY < r.top ||
            e.clientY > r.bottom
          )
            onClose();
        }
      }}
    >
      <div className="modal-header">
        <div>
          <h2 id={titleId}>{title}</h2>
          {subtitle && <p id={subtitleId}>{subtitle}</p>}
        </div>
        <button type="button" className="icon-button" onClick={onClose} aria-label="Fechar janela">
          <X size={20} aria-hidden="true" />
        </button>
      </div>
      {open && children}
    </dialog>
  );
}
export function LoadingScreen() {
  return (
    <div className="loading-screen" role="status">
      <Scissors size={30} aria-hidden="true" />
      <span>Preparando seu espaço…</span>
    </div>
  );
}
export function EmptyState({
  title,
  text,
  children,
  icon,
}: {
  title: string;
  text: string;
  children?: React.ReactNode;
  icon?: React.ReactNode;
}) {
  return (
    <div className="empty-state">
      <span aria-hidden="true">{icon ?? <Scissors size={26} />}</span>
      <h3>{title}</h3>
      <p>{text}</p>
      {children}
    </div>
  );
}
