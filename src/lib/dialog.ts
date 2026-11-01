/** Keep keyboard focus inside a modal and return it to the trigger on close. */
export function focusDialog(node: HTMLElement, close: () => void) {
  const trigger = document.activeElement as HTMLElement | null;
  const selector =
    'button:not(:disabled), input, select, textarea, a[href], [tabindex="0"]';
  requestAnimationFrame(() =>
    (node.querySelector<HTMLElement>(selector) ?? node).focus(),
  );
  function keydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
    }
    if (event.key !== "Tab") return;
    const items = [...node.querySelectorAll<HTMLElement>(selector)];
    const first = items[0],
      last = items.at(-1);
    if (first && !node.contains(document.activeElement)) {
      event.preventDefault();
      first.focus();
      return;
    }
    if (!first) {
      event.preventDefault();
      node.focus();
      return;
    }
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last?.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
  document.addEventListener("keydown", keydown);
  return {
    destroy() {
      document.removeEventListener("keydown", keydown);
      trigger?.focus();
    },
  };
}
