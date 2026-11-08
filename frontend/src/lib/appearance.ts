export type Appearance = "system" | "light" | "dark";

export function normalizeAppearance(value?: string): Appearance {
  return value === "light" || value === "dark" ? value : "system";
}

export function resolveAppearance(
  value: string | undefined,
  systemDark: boolean,
): "light" | "dark" {
  const appearance = normalizeAppearance(value);
  return appearance === "system" ? (systemDark ? "dark" : "light") : appearance;
}
