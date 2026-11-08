<script lang="ts">
  import LayoutDashboardIcon from "@lucide/svelte/icons/layout-dashboard";
  import SharedIcon from "../brand/Icon.svelte";
  import type { IconName } from "../lib/icons";
  let { name, size = 18 }: { name: IconName; size?: number } = $props();
</script>

{#if name === "dashboard"}
  <LayoutDashboardIcon
    class="app-icon"
    {size}
    color="currentColor"
    stroke-width="1.65"
    aria-hidden="true"
    focusable="false"
  />
{:else}
  <SharedIcon {name} {size} />
{/if}
