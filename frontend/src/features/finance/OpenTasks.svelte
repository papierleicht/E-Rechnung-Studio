<script lang="ts">
  import type { FinanceIssue } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import EntriesDialog from "./EntriesDialog.svelte";

  // What is still open in the user's data, independent of the chosen period:
  // the list does not empty itself when a month without problems is selected.
  // Each row offers the step that resolves it where the app knows one.
  let {
    issues,
    loading = false,
    onassign,
    onsettings,
  }: {
    issues: FinanceIssue[] | null;
    loading?: boolean;
    onassign?: () => void;
    onsettings?: () => void;
  } = $props();

  let shown = $state<FinanceIssue | null>(null);
  const count = $derived(issues?.length ?? 0);

  function action(issue: FinanceIssue): { label: string; run: () => void } | null {
    switch (issue.code) {
      case "unassigned_bookings":
        return onassign ? { label: "Zuordnen", run: onassign } : null;
      case "missing_balance":
      case "stale_balance":
        return onsettings ? { label: "Kontostand eintragen", run: onsettings } : null;
      case "missing_reserve_rate":
        return onsettings ? { label: "Satz festlegen", run: onsettings } : null;
      default:
        return null;
    }
  }
</script>

{#if issues && count === 0}
  <p class="all-done" role="status">
    Keine offenen Punkte. Alle Zahlen auf dieser Seite sind vollständig.
  </p>
{:else if issues}
  <section class="open-tasks" aria-labelledby="open-tasks-title">
    <header>
      <h2 id="open-tasks-title">Zu erledigen</h2>
      <span class="count">{count} {count === 1 ? "offener Punkt" : "offene Punkte"}</span>
    </header>
    <p class="hint">
      Bis sie erledigt sind, sind die betroffenen Zahlen unten als „unvollständig“ markiert. Die
      Liste gilt für alle Zeiträume.
    </p>
    <ul>
      {#each issues as issue (issue.code)}
        {@const step = action(issue)}
        {@const entries = issue.entries ?? []}
        <li>
          <span class="message"
            >{issue.message}{#if entries.length}<span class="count-badge">{entries.length}</span
              >{/if}</span
          >
          <span class="task-actions">
            {#if entries.length}
              <button type="button" class="secondary" onclick={() => (shown = issue)}
                >Ansehen</button
              >
            {/if}
            {#if step}
              <button type="button" class="primary" onclick={step.run}>{step.label}</button>
            {/if}
          </span>
        </li>
      {/each}
    </ul>
  </section>
{:else if loading}
  <p class="hint">Offene Punkte werden geprüft …</p>
{/if}

{#if shown}
  <EntriesDialog
    title="Offener Punkt"
    description={shown.message}
    entries={shown.entries ?? []}
    onclose={() => (shown = null)}
  />
{/if}

<style>
  .all-done {
    margin: 0;
    padding: 10px 14px;
    border-radius: 10px;
    border: 1px solid var(--theme-success-border);
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    font-size: 13px;
  }
  .open-tasks {
    padding: var(--ui-card-padding);
    border-radius: 12px;
    border: 1px solid var(--theme-warning-border);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    min-width: 0;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: 4px 12px;
  }
  /* Size and weight come from the app's section title. */
  h2 {
    margin: 0;
    color: inherit;
  }
  .count {
    font-size: 12px;
    font-weight: 600;
  }
  .hint {
    margin: 6px 0 0;
    font-size: 12px;
    line-height: 1.5;
    color: inherit;
  }
  ul {
    list-style: none;
    margin: 12px 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
  }
  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px 16px;
    padding: 10px 0;
    border-top: 1px solid color-mix(in srgb, currentColor 20%, transparent);
    font-size: 13px;
  }
  .message {
    flex: 1 1 260px;
    min-width: 0;
    overflow-wrap: anywhere;
    line-height: 1.45;
  }
  /* Not .badge: the app styles that class globally as a status pill. */
  .count-badge {
    display: inline-block;
    min-width: 18px;
    text-align: center;
    margin-left: 6px;
    padding: 0 7px;
    border-radius: 999px;
    /* A tint of the text colour shows on the warning ground in both themes. */
    background: color-mix(in srgb, currentColor 18%, transparent);
    font-size: 11px;
    font-weight: 600;
  }
  .task-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
</style>
