import type { ShopRegistration } from "./api/registration";
import { validateShopAddress } from "./shop-address";
export { brazilianStates } from "./shop-address";

export type RegistrationErrors = Partial<Record<string, string>>;

export function shopSlug(name: string) {
  return name
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 100)
    .replace(/-$/, "");
}

export function validateShopRegistration(input: ShopRegistration): RegistrationErrors {
  const errors: RegistrationErrors = {};
  const text = (field: string, value: string, max: number, optional = false) => {
    if (
      (!optional && !value.trim()) ||
      [...value.trim()].length > max ||
      /[\u0000-\u001f\u007f-\u009f]/.test(value)
    )
      errors[field] = optional
        ? `Use até ${max} caracteres, sem quebras de linha.`
        : `Preencha este campo com até ${max} caracteres, sem quebras de linha.`;
  };
  text("display_name", input.display_name, 100);
  text("name", input.name, 150);
  if (
    input.slug.length < 3 ||
    input.slug.length > 100 ||
    !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(input.slug)
  )
    errors.slug = "Use de 3 a 100 letras minúsculas, números e hífens entre palavras.";
  Object.assign(errors, validateShopAddress(input.address));
  try {
    new Intl.DateTimeFormat("pt-BR", { timeZone: input.timezone });
  } catch {
    errors.timezone = "Selecione um fuso horário válido.";
  }
  return errors;
}

export type RegistrationAttempt = { userID: string; key: string; input: ShopRegistration };
const storageKey = "barberchat-shop-registration-v1";

// Only a submitted business draft is kept in this tab. A reload must not turn an
// uncertain POST into a new request. Never store credentials or another token.
export function saveRegistrationAttempt(attempt: RegistrationAttempt) {
  sessionStorage.setItem(storageKey, JSON.stringify(attempt));
}
export function clearRegistrationAttempt() {
  sessionStorage.removeItem(storageKey);
}
export function loadRegistrationAttempt(userID: string): RegistrationAttempt | null {
  const raw = sessionStorage.getItem(storageKey);
  if (!raw) return null;
  try {
    const attempt = JSON.parse(raw) as RegistrationAttempt;
    if (attempt.userID !== userID) {
      clearRegistrationAttempt();
      return null;
    }
    if (
      !/^[0-9a-f]{64}$/.test(attempt.key) ||
      Object.keys(validateShopRegistration(attempt.input)).length
    )
      throw new Error("Invalid attempt");
    return attempt;
  } catch {
    throw new Error(
      "Há uma tentativa de cadastro que não pôde ser recuperada nesta aba. Confira suas barbearias no painel antes de iniciar outra tentativa.",
    );
  }
}
