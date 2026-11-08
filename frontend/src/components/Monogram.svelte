<script lang="ts">
  // Neutral letter tile that stands in for a provider or bank logo. The app
  // bundles no third-party marks; the name is the only identification.
  import { monogramFor, monogramTone } from "../lib/monogram";
  let { name, size = 40 }: { name: string; size?: number } = $props();
</script>

<span
  class="monogram"
  class:long={monogramFor(name).length > 2}
  data-tone={monogramTone(name)}
  style:width={`${size}px`}
  style:height={`${size}px`}
  style:font-size={`${Math.round(size * 0.38)}px`}
  aria-hidden="true">{monogramFor(name)}</span
>

<style>
  .monogram {
    display: inline-grid;
    place-items: center;
    flex-shrink: 0;
    border-radius: 22%;
    border: 1px solid var(--theme-border);
    background: var(--theme-surface-muted);
    color: var(--theme-text);
    font-weight: 700;
    letter-spacing: 0.02em;
    line-height: 1;
    user-select: none;
  }
  .monogram.long {
    font-size: 0.8em;
    letter-spacing: 0;
  }
  /* Accent as a border, not as text: sage on the muted surface only reaches
     4.5:1 in light mode, too close for 11px bold letters. */
  .monogram[data-tone="1"] {
    border-color: var(--theme-accent);
    background: var(--theme-surface);
  }
  .monogram[data-tone="2"] {
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .monogram[data-tone="3"] {
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
  }
</style>
