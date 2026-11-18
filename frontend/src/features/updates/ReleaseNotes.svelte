<script lang="ts">
  import { parseReleaseNotes } from "../../lib/releaseNotes";
  let { notes }: { notes: string } = $props();
  const blocks = $derived(parseReleaseNotes(notes));
</script>

<div class="release-notes">
  {#each blocks as block}
    {#if block.kind === "heading"}
      <h3>{block.text}</h3>
    {:else if block.kind === "list"}
      <ul>
        {#each block.items as item}
          <li>
            {#each item as span}{#if span.strong}<strong>{span.text}</strong
                >{:else}{span.text}{/if}{/each}
          </li>
        {/each}
      </ul>
    {:else}
      <p>
        {#each block.spans as span}{#if span.strong}<strong>{span.text}</strong
            >{:else}{span.text}{/if}{/each}
      </p>
    {/if}
  {:else}
    <p class="release-notes-empty">Zu dieser Version gibt es keine Hinweise.</p>
  {/each}
</div>

<style>
  .release-notes {
    font-size: 13px;
    line-height: 1.55;
    overflow-wrap: anywhere;
  }
  .release-notes h3 {
    margin: 14px 0 6px;
    font-size: 13px;
    font-weight: 700;
  }
  .release-notes h3:first-child {
    margin-top: 0;
  }
  .release-notes ul {
    margin: 0;
    padding-left: 18px;
  }
  .release-notes li + li {
    margin-top: 4px;
  }
  .release-notes p {
    margin: 0 0 8px;
  }
  .release-notes-empty {
    color: var(--theme-muted);
  }
</style>
