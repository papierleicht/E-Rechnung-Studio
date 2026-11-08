export type SettingsDecision = "save" | "discard" | "cancel";

/** Keep the current page and edits until persistence has actually succeeded. */
export async function resolveSettingsNavigation(
  decision: SettingsDecision,
  actions: { save: () => Promise<boolean>; discard: () => void; navigate: () => void },
): Promise<boolean> {
  if (decision === "cancel") return false;
  if (decision === "save" && !(await actions.save())) return false;
  if (decision === "discard") actions.discard();
  actions.navigate();
  return true;
}
