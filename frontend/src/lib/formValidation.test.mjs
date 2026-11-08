import assert from "node:assert/strict";
import test from "node:test";
import { germanValidationMessage, installGermanFormValidation } from "./formValidation.ts";

test("localized errors clear after editing and listeners are removed on disposal", () => {
  class Field {
    message = "";
    missing = true;
    type = "text";
    get validity() {
      return { valueMissing: this.missing, customError: !!this.message };
    }
    setCustomValidity(message) {
      this.message = message;
    }
  }
  const names = ["HTMLInputElement", "HTMLSelectElement", "HTMLTextAreaElement"];
  const previous = names.map((name) => globalThis[name]);
  names.forEach((name) => {
    globalThis[name] = Field;
  });
  try {
    const handlers = new Map();
    const dispose = installGermanFormValidation({
      addEventListener: (name, handler) => handlers.set(name, handler),
      removeEventListener: (name) => handlers.delete(name),
    });
    const field = new Field();
    handlers.get("invalid")({ target: field });
    assert.equal(field.message, "Fülle dieses Feld aus.");
    field.missing = false;
    handlers.get("input")({ target: field });
    assert.equal(field.message, "");
    field.message = "Eigene Validierung";
    handlers.get("invalid")({ target: field });
    assert.equal(field.message, "Eigene Validierung");
    dispose();
    assert.equal(handlers.size, 0);
  } finally {
    names.forEach((name, index) => {
      if (previous[index] === undefined) delete globalThis[name];
      else globalThis[name] = previous[index];
    });
  }
});

test("native form errors have German messages independent of WebView language", () => {
  const cases = [
    [{ valueMissing: true }, {}, "Fülle dieses Feld aus."],
    [{ valueMissing: true }, { type: "checkbox" }, "Bitte setze hier ein Häkchen."],
    [{ valueMissing: true }, { type: "select-one" }, "Wähle eine Option aus."],
    [{ typeMismatch: true }, { type: "email" }, "Gib eine gültige E-Mail-Adresse ein."],
    [{ typeMismatch: true }, { type: "url" }, "Gib eine gültige Internetadresse ein."],
    [{ rangeUnderflow: true }, { min: "0" }, "Gib einen Wert ab 0 ein."],
    [{ rangeOverflow: true }, { max: "100" }, "Gib einen Wert bis 100 ein."],
    [{ tooShort: true }, { minLength: 2 }, "Gib mindestens 2 Zeichen ein."],
    [{ tooLong: true }, { maxLength: 20 }, "Gib höchstens 20 Zeichen ein."],
    [{ patternMismatch: true }, {}, "Verwende das vorgegebene Format."],
    [{ stepMismatch: true }, {}, "Gib einen Wert im erlaubten Schrittmaß ein."],
    [{ badInput: true }, {}, "Gib einen gültigen Wert ein."],
  ];
  for (const [validity, field, message] of cases)
    assert.equal(germanValidationMessage(validity, field), message);
  assert.equal(germanValidationMessage({ valid: true }, {}), "");
  assert.equal(germanValidationMessage({ customError: true }, {}), "");
});
