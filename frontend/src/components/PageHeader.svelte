<script lang="ts">
  import type { Snippet } from "svelte";
  import Icon from "./Icon.svelte";
  let {
    back,
    onback,
    title,
    description = "",
    badge = "",
    badgeClass = "",
    disabled = false,
    actions,
  }: {
    /** Where the back link leads, e.g. "Alle Kunden". */
    back: string;
    onback: () => void;
    title: string;
    /** One muted line below the title, e.g. what to do on this page. */
    description?: string;
    badge?: string;
    badgeClass?: string;
    disabled?: boolean;
    /** Save state and buttons on the right. */
    actions?: Snippet;
  } = $props();
</script>

<!-- The head of every sub-page in the window's top row: back to where you came
     from, title and status, actions on the right. The description follows below. -->
<header class="topbar page-header">
  <button class="text page-back" {disabled} onclick={onback}
    ><Icon name="back" size={16} /> {back}</button
  >
  <h1>
    <span class="page-title">{title}</span>{#if badge}<span class="badge {badgeClass}">{badge}</span
      >{/if}
  </h1>
  {#if actions}<div class="actions">{@render actions()}</div>{/if}
</header>
{#if description}<p class="page-intro page-description">{description}</p>{/if}

<style>
  /* Titles can hold file names without spaces: they shorten instead of
     widening the row, and only the title grows, so the actions stay right. */
  .page-back {
    flex: none;
    padding: 0;
    white-space: nowrap;
  }
  h1 {
    flex: 1 1 auto;
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .page-title {
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  h1 .badge {
    flex: none;
  }
  .page-description {
    overflow-wrap: anywhere;
  }
</style>
