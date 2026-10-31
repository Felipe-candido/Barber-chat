import type { KeyboardEvent } from "react";

// Native dialog supplies modal semantics; this also keeps keyboard cycling inside
// when focus starts on a programmatically focused heading or a field becomes disabled.
export function containDialogFocus(event: KeyboardEvent<HTMLDialogElement>) {
  if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey) return;
  const items = Array.from(
    event.currentTarget.querySelectorAll<HTMLElement>(
      "a[href], button, input, select, textarea, summary, [tabindex]",
    ),
  ).filter(
    (element) =>
      element.tabIndex >= 0 &&
      !element.matches(":disabled") &&
      !element.closest("[inert]") &&
      element.getClientRects().length > 0,
  );
  const first = items[0];
  const last = items[items.length - 1];
  if (!first) {
    event.preventDefault();
    return;
  }
  const active = document.activeElement;
  if (event.shiftKey && (active === first || !items.includes(active as HTMLElement))) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && (active === last || !items.includes(active as HTMLElement))) {
    event.preventDefault();
    first.focus();
  }
}
