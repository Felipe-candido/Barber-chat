"use client";
import Image from "next/image";
import { useEffect, useId, useRef } from "react";
import { Scissors, X } from "lucide-react";

export function Brand({ compact = false, light = false }: { compact?: boolean; light?: boolean }) {
  return (
    <div className={"brand " + (light ? "brand-light" : "")}>
      <Image
        src="/brand/palma-logo.png"
        width={64}
        height={64}
        alt="Logo Palma Barbearia"
        priority
        className="brand-logo"
      />
      {!compact && (
        <div>
          <strong>
            PALMA<span>BARBEARIA</span>
          </strong>
        </div>
      )}
    </div>
  );
}
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
  useEffect(() => {
    const dialog = ref.current;
    if (open && !dialog?.open) dialog?.showModal();
    if (!open && dialog?.open) dialog?.close();
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
      aria-labelledby={titleId}
      className={"modal " + (wide ? "modal-wide" : "")}
      onCancel={onClose}
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
          {subtitle && <p>{subtitle}</p>}
        </div>
        <button className="icon-button" onClick={onClose} aria-label="Fechar janela">
          <X size={20} />
        </button>
      </div>
      {open && children}
    </dialog>
  );
}
export function LoadingScreen() {
  return (
    <div className="loading-screen">
      <Scissors size={30} />
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
      <span>{icon ?? <Scissors size={26} />}</span>
      <h3>{title}</h3>
      <p>{text}</p>
      {children}
    </div>
  );
}
