<script lang="ts">
  import type { Issue } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/taxforms/models";
  import { dateLabel } from "../../lib/format";

  let { issues }: { issues: Issue[] | null | undefined } = $props();
</script>

<!-- Step 1 of every tax task: what is missing before the values hold. -->
{#if issues?.length}
  <div class="tax-issues" role="note">
    <strong>Bevor du die Werte überträgst</strong>
    <ul>
      {#each issues as issue (issue.code)}<li>
          {issue.message}
          {#if issue.entries?.length}<details class="link-toggle">
              <summary>{issue.entries.length} betroffen</summary>
              <ul class="tax-entries">
                {#each issue.entries as e}<li>
                    {e.date ? dateLabel(e.date) + " · " : ""}{e.label}{e.note ? ` (${e.note})` : ""}
                  </li>{/each}
              </ul>
            </details>{/if}
        </li>{/each}
    </ul>
  </div>
{:else}
  <p class="tax-ok">
    Die App hat keine offenen Punkte gefunden. Belege, die noch nicht in der App sind, kann sie
    nicht kennen.
  </p>
{/if}
