"use client";

import { useState } from "react";
import { Download } from "lucide-react";
import { Button, Modal } from "../ui";
import { usePwa } from "./pwa-provider";
import styles from "./install-app.module.css";

export function InstallAppButton({ className = "" }: { className?: string }) {
  const { ready, installed, ios, installing, install } = usePwa();
  const [instructions, setInstructions] = useState(false);
  if (!ready || installed) return null;

  return (
    <>
      <Button
        variant="secondary"
        className={className}
        loading={installing}
        onClick={async () => {
          if ((await install()) === "unavailable") setInstructions(true);
        }}
      >
        <Download size={18} aria-hidden="true" />
        Baixar app
      </Button>
      <Modal
        open={instructions}
        onClose={() => setInstructions(false)}
        title="Instalar Na Regua"
        subtitle="Acesse pelo ícone na tela inicial, sem procurar o link novamente."
      >
        <div className={styles.instructions}>
          {ios ? (
            <>
              <p>No iPhone ou iPad:</p>
              <ol>
                <li>Abra o menu de compartilhamento do navegador.</li>
                <li>
                  Toque em <strong>Adicionar à Tela de Início</strong> e confirme.
                </li>
              </ol>
              <p>Se a opção não aparecer, abra este endereço no Safari e tente novamente.</p>
            </>
          ) : (
            <>
              <p>
                No Chrome ou Edge, abra o menu do navegador e procure <strong>Instalar app</strong>{" "}
                ou <strong>Instalar Na Regua</strong>. No computador, a opção também pode aparecer
                na barra de endereço.
              </p>
              <p>
                Se ela não estiver disponível, tente em um navegador compatível e fora de uma aba
                anônima.
              </p>
            </>
          )}
          <p>
            A instalação é gratuita. Você precisa de internet para consultar a agenda e confirmar
            alterações.
          </p>
          <Button onClick={() => setInstructions(false)}>Entendi</Button>
        </div>
      </Modal>
    </>
  );
}
