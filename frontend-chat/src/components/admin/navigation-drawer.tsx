"use client";
import { useEffect, useId, useRef, type ReactNode } from "react";
import { X } from "lucide-react";
import { containDialogFocus } from "../ui/dialog-focus";
import styles from "../admin-shell.module.css";

export function NavigationDrawer({
  open,
  onClose,
  id,
  children,
}: {
  open: boolean;
  onClose: () => void;
  id: string;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (!open) {
      if (dialog.open) dialog.close();
      return;
    }
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    if (dialog.open && !dialog.matches(":modal")) dialog.close();
    if (!dialog.open) dialog.showModal();
    closeButton.current?.focus();
    return () => {
      if (dialog.open) dialog.close();
      document.body.style.overflow = previousOverflow;
      if (trigger?.isConnected) trigger.focus();
    };
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const query = window.matchMedia("(min-width: 1024px)");
    const closeOnDesktop = () => {
      if (query.matches) onClose();
    };
    query.addEventListener("change", closeOnDesktop);
    closeOnDesktop();
    return () => query.removeEventListener("change", closeOnDesktop);
  }, [open, onClose]);
  return (
    <dialog
      id={id}
      ref={ref}
      className={styles.drawer}
      aria-labelledby={titleId}
      aria-modal={open || undefined}
      onKeyDown={containDialogFocus}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(event) => {
        if (event.target !== event.currentTarget) return;
        const bounds = event.currentTarget.getBoundingClientRect();
        if (
          event.clientX > bounds.right ||
          event.clientX < bounds.left ||
          event.clientY > bounds.bottom ||
          event.clientY < bounds.top
        )
          onClose();
      }}
    >
      <span className={styles.grabber} aria-hidden="true" />
      <div className={styles.drawerHeading}>
        <h2 id={titleId}>Menu do painel</h2>
        <button
          ref={closeButton}
          type="button"
          className="icon-button"
          aria-label="Fechar menu"
          onClick={onClose}
        >
          <X size={20} aria-hidden="true" />
        </button>
      </div>
      <div className={styles.drawerContent}>{children}</div>
    </dialog>
  );
}
