<script lang="ts">
  import { parseGuide, plain, type Inline } from "../../lib/guide";
  let { text, checked = "" }: { text: string; checked?: string } = $props();
  const blocks = $derived(parseGuide(text));
  // Ticked items of the checklist; they only help along and are not saved.
  let done = $state<Record<string, boolean>>({});
</script>

{#snippet line(parts: Inline[])}{#each parts as part}{#if part.strong}<strong>{part.text}</strong
      >{:else if part.code}<code>{part.text}</code>{:else}{part.text}{/if}{/each}{/snippet}

<div class="guide">
  {#each blocks as block, index}
    {#if block.kind === "heading"}
      {#if block.level === 2}<h3>{block.text}</h3>{:else}<h4>{block.text}</h4>{/if}
    {:else if block.kind === "paragraph"}
      <p>{@render line(block.parts)}</p>
    {:else if block.kind === "list" && block.ordered}
      <ol>
        {#each block.items as item}<li>{@render line(item)}</li>{/each}
      </ol>
    {:else if block.kind === "list"}
      <ul>
        {#each block.items as item}<li>{@render line(item)}</li>{/each}
      </ul>
    {:else if block.kind === "checklist"}
      <ul class="guide-checklist">
        {#each block.items as item}
          {@const key = `${index}:${plain(item)}`}
          <li>
            <label
              ><input
                type="checkbox"
                checked={!!done[key]}
                onchange={(event) => (done = { ...done, [key]: event.currentTarget.checked })}
              /><span>{@render line(item)}</span></label
            >
          </li>
        {/each}
      </ul>
    {/if}
  {/each}
  {#if checked}<p class="guide-checked">Stand: {checked}</p>{/if}
</div>

<style>
  .guide {
    min-width: 0;
    font-size: 13px;
    line-height: 1.6;
  }
  .guide h3 {
    margin: 22px 0 8px;
    font-size: 14px;
  }
  .guide h3:first-child {
    margin-top: 0;
  }
  .guide h4 {
    margin: 16px 0 6px;
    font-size: 13px;
  }
  .guide p,
  .guide ol,
  .guide ul {
    margin: 0 0 10px;
    overflow-wrap: anywhere;
  }
  .guide ol,
  .guide ul {
    padding-left: 22px;
  }
  .guide li + li {
    margin-top: 5px;
  }
  .guide code {
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--theme-surface-muted);
    font-size: 12px;
  }
  .guide-checklist {
    list-style: none;
    padding-left: 0 !important;
  }
  .guide-checklist label {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    cursor: pointer;
  }
  .guide-checklist input {
    flex-shrink: 0;
    margin-top: 4px;
  }
  .guide-checked {
    margin-top: 18px !important;
    color: var(--theme-muted);
    font-size: 12px;
  }
</style>
