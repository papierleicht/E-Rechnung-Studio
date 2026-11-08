// Optional areas of the menu. Customers without an online shop or a product
// list do not need to see them; the settings show or hide them, and the
// setup decides for the shop.

export type MenuSection = "shops" | "products";

export const optionalSections: readonly { id: MenuSection; label: string; hint: string }[] = [
  {
    id: "shops",
    label: "Shop-Bestellungen",
    hint: "Bestellungen aus deinem Onlineshop übernehmen",
  },
  {
    id: "products",
    label: "Artikel",
    hint: "Artikel und Leistungen für wiederkehrende Rechnungspositionen",
  },
];

export type SectionChoices = { [id: string]: boolean | undefined } | null | undefined;

/**
 * Core areas always show. For optional ones an explicit choice wins; without
 * one the shop area shows only with a connected shop.
 */
export function sectionVisible(
  id: string,
  choices: SectionChoices,
  context: { shopConnected: boolean },
): boolean {
  if (!optionalSections.some((section) => section.id === id)) return true;
  const choice = choices?.[id];
  if (typeof choice === "boolean") return choice;
  if (id === "shops") return context.shopConnected;
  return true;
}

/** The choice of the setup: the shop area shows only when a shop was chosen. */
export function setupSections(choices: SectionChoices, shop: string): Record<string, boolean> {
  const next: Record<string, boolean> = {};
  for (const [id, value] of Object.entries(choices ?? {})) {
    if (typeof value === "boolean") next[id] = value;
  }
  next.shops = Boolean(shop);
  return next;
}
