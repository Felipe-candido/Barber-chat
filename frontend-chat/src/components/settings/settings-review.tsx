import type { SettingsDraft, ShopSettings } from "@/lib/api/shop-settings";
import { changedSettings, settingsDraft, settingsLabels, settingsValue } from "@/lib/shop-settings";
import { Button } from "../ui";
import styles from "./shop-settings.module.css";

export function SettingsReview({
  current,
  proposed,
  onKeep,
  onDiscard,
}: {
  current: ShopSettings;
  proposed: SettingsDraft;
  onKeep: () => void;
  onDiscard: () => void;
}) {
  const remote = settingsDraft(current);
  const changes = changedSettings(remote, proposed);
  return (
    <section className={styles.review} aria-labelledby="settings-review-title">
      <h2 id="settings-review-title">Confira antes de continuar</h2>
      <p>
        A versão atual da barbearia foi consultada. As mudanças feitas na API em campos que você não
        editou serão preservadas. Nenhum salvamento foi reenviado.
      </p>
      {changes.length ? (
        <table className={styles.comparison}>
          <caption>Diferenças para revisar</caption>
          <thead>
            <tr>
              <th scope="col">Campo</th>
              <th scope="col">Na API</th>
              <th scope="col">Seu rascunho</th>
            </tr>
          </thead>
          <tbody>
            {changes.map((field) => (
              <tr key={field}>
                <th scope="row">{settingsLabels[field]}</th>
                <td>{settingsValue(remote, field) || "Não preenchido"}</td>
                <td>{settingsValue(proposed, field) || "Deixar vazio"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p>Não há diferenças entre o rascunho e os dados atuais.</p>
      )}
      <div className={styles.actions}>
        {changes.length > 0 && <Button onClick={onKeep}>Revisar minhas alterações</Button>}
        <Button variant="secondary" onClick={onDiscard}>
          Usar dados atuais da API
        </Button>
      </div>
      <p>
        “Revisar minhas alterações” prepara o formulário com a versão atual. Você ainda precisará
        revisar e clicar em “Salvar alterações”.
      </p>
    </section>
  );
}
