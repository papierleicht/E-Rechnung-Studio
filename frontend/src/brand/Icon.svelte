<script lang="ts">
  import { icons, type IconName } from "./icons";
  let { name, size = 18 }: { name: IconName; size?: number } = $props();
  const Glyph = $derived(icons[name]);
</script>

<Glyph
  class="app-icon"
  {size}
  color="currentColor"
  stroke-width="1.65"
  aria-hidden="true"
  focusable="false"
/>
