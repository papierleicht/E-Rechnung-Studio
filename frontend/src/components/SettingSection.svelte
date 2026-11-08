<script lang="ts">
  import type { Snippet } from "svelte";
  let {
    title,
    summary = "",
    open = $bindable(false),
    action = "Ändern",
    onopen,
    field,
    children,
  }: {
    title: string;
    /** What is set right now, in a few words, so nobody has to open the section to see it. */
    summary?: string;
    open?: boolean;
    /** Button label while closed, e.g. "Ändern" or "Ansehen". */
    action?: string;
    onopen?: () => void;
    /** data-field of the body, so a field link can find and open the section. */
    field?: string;
    children: Snippet;
  } = $props();
  const bodyId = $props.id();
</script>

<!-- A settings area that stays closed until needed: its current value and a
     visible button instead of a disclosure triangle. -->
<section class="setting-section">
  <div class="setting-head">
    <div class="setting-text">
      <h3>{title}</h3>
      {#if summary}<p>{summary}</p>{/if}
    </div>
    <button
      type="button"
      class="secondary"
      aria-expanded={open}
      aria-controls={bodyId}
      onclick={() => {
        open = !open;
        if (open) onopen?.();
      }}>{open ? "Schließen" : action}</button
    >
  </div>
  <!-- The body stays a <details> with a hidden summary: dialogs and field links that
       reveal an invalid or requested field open it like any disclosure. -->
  <details
    id={bodyId}
    class="setting-body"
    data-field={field}
    bind:open
    oninvalidcapture={() => (open = true)}
  >
    <summary>{title}</summary>
    {@render children()}
  </details>
</section>

<style>
  .setting-section {
    margin-top: 16px;
    padding-top: 16px;
    border-top: 1px solid var(--theme-border);
  }
  .setting-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 16px;
  }
  .setting-text {
    flex: 1 1 16rem;
    min-width: 0;
  }
  h3 {
    margin: 0;
    font-size: 14px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  p {
    margin: 3px 0 0;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .setting-head button {
    margin-left: auto;
  }
  .setting-body[open] {
    margin-top: 12px;
  }
  .setting-body > summary {
    display: none;
  }
</style>
