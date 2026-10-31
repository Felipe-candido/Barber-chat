"use client";

import { useEffect } from "react";

export function useUnsavedSettings(dirty: boolean, pending: boolean) {
  useEffect(() => {
    if (!dirty && !pending) return;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    const navigate = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0 || !(event.target instanceof Element))
        return;
      const anchor = event.target.closest<HTMLAnchorElement>("a[href]");
      if (!anchor || anchor.target === "_blank" || anchor.hasAttribute("download")) return;
      const target = new URL(anchor.href, window.location.href);
      if (target.pathname === window.location.pathname && target.search === window.location.search)
        return;
      const message = pending
        ? "O salvamento pode estar em andamento ou ainda não foi conferido. Ao voltar, consulte os dados antes de salvar novamente. Deseja sair desta tela?"
        : "Há alterações não salvas. Deseja sair desta tela e descartar o rascunho?";
      if (!window.confirm(message)) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    window.addEventListener("beforeunload", beforeUnload);
    document.addEventListener("click", navigate, true);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      document.removeEventListener("click", navigate, true);
    };
  }, [dirty, pending]);
}
