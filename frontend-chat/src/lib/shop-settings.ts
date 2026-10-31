import type { SettingsDraft, ShopSettings } from "./api/shop-settings";
import { emptyShopAddress, validateShopAddress } from "./shop-address";

export type SettingsField = "name" | "phone" | "description" | keyof SettingsDraft["address"];
export type SettingsErrors = Partial<Record<SettingsField, string>>;
export const settingsLabels: Record<SettingsField, string> = {
  name: "Nome da barbearia",
  phone: "Telefone comercial",
  description: "Descrição",
  postal_code: "CEP",
  street: "Rua ou avenida",
  number: "Número",
  complement: "Complemento",
  district: "Bairro",
  city: "Cidade",
  state: "Estado",
};

export function normalizeBusinessPhone(value: string) {
  const raw = value.trim();
  if (!raw || /[^0-9+ ()-]/.test(raw)) return raw;
  const digits = raw.replace(/\D/g, "");
  if (/^55\d{10,11}$/.test(digits)) return "+" + digits;
  if (!raw.startsWith("+") && /^\d{10,11}$/.test(digits)) return "+55" + digits;
  return raw;
}

export function settingsDraft(settings: ShopSettings): SettingsDraft {
  return {
    name: settings.name,
    phone: settings.phone,
    description: settings.description,
    address: settings.address ? { ...settings.address } : emptyShopAddress(),
  };
}

export function canonicalSettings(draft: SettingsDraft): SettingsDraft {
  const a = draft.address;
  return {
    name: draft.name.trim(),
    phone: normalizeBusinessPhone(draft.phone),
    description: draft.description.replace(/\r\n/g, "\n").trim(),
    address: {
      postal_code: a.postal_code.replace(/-/g, "").trim(),
      street: a.street.trim(),
      number: a.number.trim(),
      complement: a.complement.trim(),
      district: a.district.trim(),
      city: a.city.trim(),
      state: a.state.trim().toUpperCase(),
    },
  };
}

export function sameSettings(left: SettingsDraft, right: SettingsDraft) {
  return JSON.stringify(canonicalSettings(left)) === JSON.stringify(canonicalSettings(right));
}

export function settingsValue(draft: SettingsDraft, field: SettingsField) {
  return field === "name" || field === "phone" || field === "description"
    ? draft[field]
    : draft.address[field];
}
export function changedSettings(current: SettingsDraft, proposed: SettingsDraft) {
  const a = canonicalSettings(current),
    b = canonicalSettings(proposed);
  return (Object.keys(settingsLabels) as SettingsField[]).filter(
    (field) => settingsValue(a, field) !== settingsValue(b, field),
  );
}

// Preserve remote changes in untouched fields. Local changes are proposed only
// for explicit human review, never resent automatically with a newer version.
export function mergeSettings(
  base: SettingsDraft,
  remote: SettingsDraft,
  local: SettingsDraft,
): SettingsDraft {
  const result = structuredClone(remote);
  for (const field of changedSettings(base, local)) {
    if (field === "name" || field === "phone" || field === "description")
      result[field] = local[field];
    else result.address[field] = local.address[field];
  }
  return result;
}

export function validateSettingsDraft(draft: SettingsDraft): SettingsErrors {
  const errors: SettingsErrors = { ...validateShopAddress(draft.address) };
  if (
    !draft.name.trim() ||
    [...draft.name.trim()].length > 150 ||
    /[\u0000-\u001f\u007f-\u009f]/.test(draft.name)
  )
    errors.name = "Informe um nome de até 150 caracteres, sem quebras de linha.";
  if (
    [...draft.description].length > 1000 ||
    /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/.test(draft.description)
  )
    errors.description = "Use até 1000 caracteres. Quebras de linha são permitidas.";
  const phone = draft.phone.trim();
  if (phone && (phone.length > 40 || !/^\+55[1-9][0-9]{9,10}$/.test(normalizeBusinessPhone(phone))))
    errors.phone = "Informe um telefone brasileiro com DDD, sem ramal, ou deixe vazio.";
  return errors;
}
