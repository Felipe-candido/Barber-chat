export function dateKey(date: Date) {
  return [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, "0"),
    String(date.getDate()).padStart(2, "0"),
  ].join("-");
}
export function parseDate(value: string) {
  return new Date(value + "T12:00:00");
}
export function addDays(date: Date, count: number) {
  const next = new Date(date);
  next.setDate(next.getDate() + count);
  return next;
}
export function startOfWeek(date: Date) {
  return addDays(date, -((date.getDay() + 6) % 7));
}
export function formatDate(date: Date | string, options: Intl.DateTimeFormatOptions) {
  const value = typeof date === "string" ? parseDate(date) : date;
  return Number.isNaN(value.getTime())
    ? "Selecione uma data"
    : new Intl.DateTimeFormat("pt-BR", options).format(value);
}
export function money(cents: number) {
  return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);
}
