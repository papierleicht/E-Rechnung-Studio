type Constraints = {
  type?: string;
  min?: string;
  max?: string;
  minLength?: number;
  maxLength?: number;
};

/** Do not rely on the OS/WebView language for native validation bubbles. */
export function germanValidationMessage(
  validity: Partial<ValidityState>,
  field: Constraints,
): string {
  if (validity.valueMissing) {
    if (field.type === "checkbox") return "Bitte setze hier ein Häkchen.";
    if (field.type === "radio" || field.type?.startsWith("select")) return "Wähle eine Option aus.";
    if (field.type === "file") return "Wähle eine Datei aus.";
    return "Fülle dieses Feld aus.";
  }
  if (validity.typeMismatch)
    return field.type === "email"
      ? "Gib eine gültige E-Mail-Adresse ein."
      : "Gib eine gültige Internetadresse ein.";
  if (validity.badInput) return "Gib einen gültigen Wert ein.";
  if (validity.patternMismatch) return "Verwende das vorgegebene Format.";
  if (validity.tooShort) return `Gib mindestens ${field.minLength} Zeichen ein.`;
  if (validity.tooLong) return `Gib höchstens ${field.maxLength} Zeichen ein.`;
  if (validity.rangeUnderflow) return `Gib einen Wert ab ${field.min} ein.`;
  if (validity.rangeOverflow) return `Gib einen Wert bis ${field.max} ein.`;
  if (validity.stepMismatch) return "Gib einen Wert im erlaubten Schrittmaß ein.";
  return "";
}

export function installGermanFormValidation(root: Document) {
  const localized = new WeakSet<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>();
  const control = (target: EventTarget | null) =>
    target instanceof HTMLInputElement ||
    target instanceof HTMLSelectElement ||
    target instanceof HTMLTextAreaElement
      ? target
      : null;
  const clear = (event: Event) => {
    const field = control(event.target);
    if (field && localized.has(field)) {
      field.setCustomValidity("");
      localized.delete(field);
    }
  };
  const invalid = (event: Event) => {
    const field = control(event.target);
    if (!field) return;
    clear(event);
    // Leave explicit application validation messages intact.
    if (field.validity.customError) return;
    const message = germanValidationMessage(field.validity, field);
    if (message) {
      field.setCustomValidity(message);
      localized.add(field);
    }
  };
  root.addEventListener("invalid", invalid, true);
  root.addEventListener("input", clear, true);
  root.addEventListener("change", clear, true);
  return () => {
    root.removeEventListener("invalid", invalid, true);
    root.removeEventListener("input", clear, true);
    root.removeEventListener("change", clear, true);
  };
}
