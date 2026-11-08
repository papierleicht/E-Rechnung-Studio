import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import test from "node:test";

const source = new URL("../", import.meta.url);
const css =
  readFileSync(new URL("brand/colors.css", source), "utf8") +
  readFileSync(new URL("styles/theme.css", source), "utf8");
const [lightCSS, darkCSS] = css.split(':root[data-theme="dark"]');
const tokens = (text) =>
  Object.fromEntries(
    [...text.matchAll(/(--theme-[\w-]+):\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]),
  );
const light = tokens(lightCSS);
const dark = tokens(darkCSS.slice(0, darkCSS.indexOf("}")));

test("all UI palette roles have a light and dark definition", () => {
  assert.deepEqual(Object.keys(light).sort(), Object.keys(dark).sort());
});

test("component colours come from the central palette and reference defined roles", () => {
  const defined = new Set([...css.matchAll(/(--[\w-]+):/g)].map((m) => m[1]));
  for (const file of readdirSync(source, { recursive: true })
    .map((file) => file.replaceAll("\\", "/"))
    .filter((file) => /\.(css|svelte)$/.test(file))) {
    const text = readFileSync(new URL(file, source), "utf8");
    const styles = file.endsWith(".svelte")
      ? [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join("\n")
      : text;
    if (file !== "brand/colors.css") {
      assert.doesNotMatch(
        styles,
        /#[\da-f]{3,8}\b|\brgba?\(|\bhsla?\(|:\s*(?:white|black)\b/i,
        file,
      );
    }
    for (const match of styles.matchAll(
      /var\((--(?:brand|palette|dark|theme|paper|report)-[\w-]+)/g,
    )) {
      assert.ok(defined.has(match[1]), `${file}: undefined ${match[1]}`);
    }
  }
});

const primitives = Object.fromEntries(
  [...lightCSS.matchAll(/(--(?:brand|palette|dark)-[\w-]+):\s*([^;]+);/g)].map((m) => [
    m[1],
    m[2].trim(),
  ]),
);

function luminance(hex) {
  hex = hex.replace(/var\((--(?:brand|palette|dark)-[\w-]+)\)/g, (_, name) => {
    assert.ok(primitives[name], `undefined palette colour: ${name}`);
    return primitives[name];
  });
  const rgb = hex
    .slice(1)
    .match(/../g)
    .slice(0, 3)
    .map((c) => parseInt(c, 16) / 255)
    .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
}

test("normal UI text and status text retain readable contrast in both modes", () => {
  const pairs = [
    ["text", "canvas"],
    ["text", "surface"],
    ["muted", "surface"],
    ["muted", "surface-muted"],
    ["text", "input"],
    ["placeholder", "input"],
    ["text", "selected"],
    ["on-primary", "primary"],
    ["on-primary", "primary-hover"],
    ["accent", "surface"],
    ["accent", "canvas"],
    ["text", "hover"],
    ["muted", "sidebar"],
    ["success-text", "success-bg"],
    ["warning-text", "warning-bg"],
    ["error-text", "error-bg"],
    ["info-text", "info-bg"],
    ["toast-text", "toast-bg"],
  ];
  for (const [mode, palette] of [
    ["light", light],
    ["dark", dark],
  ]) {
    for (const [fg, bg] of pairs) {
      const a = luminance(palette[`--theme-${fg}`]),
        b = luminance(palette[`--theme-${bg}`]);
      const ratio = (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
      assert.ok(ratio >= 4.5, `${mode}: ${fg} / ${bg} = ${ratio.toFixed(2)}:1`);
    }
  }
});

// Bound the design system: new UI states should reuse roles, not add hex colours.
test("palette stays compact and semantic roles contain no literal colours", () => {
  assert.equal(Object.keys(primitives).length, 23);
  for (const block of [lightCSS, darkCSS]) {
    for (const [, name, value] of block.matchAll(/(--(?:theme|paper)-[\w-]+):\s*([^;]+);/g)) {
      assert.doesNotMatch(value, /#[\da-f]+/i, name);
    }
  }
});
