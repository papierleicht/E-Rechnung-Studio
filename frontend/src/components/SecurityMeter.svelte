<script lang="ts">
  import { SECURITY_STEPS } from "../lib/securityLevel";

  let { level, large = false }: { level: number; large?: boolean } = $props();
</script>

<!-- Level meter: one rising bar per step, the steps reached are filled. -->
<svg class="plan-meter" class:large viewBox="0 0 18 14" aria-hidden="true">
  {#each { length: SECURITY_STEPS } as _, index}
    <rect
      class:filled={index < level}
      x={index * 6.5}
      y={14 - (6 + index * 4)}
      width="5"
      height={6 + index * 4}
      rx="1.2"
    />
  {/each}
</svg>
