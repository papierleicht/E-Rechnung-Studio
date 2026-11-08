<script lang="ts">
  import DatevExportDialog from "./DatevExportDialog.svelte";
  import TaxFilingPage from "./TaxFilingPage.svelte";
  import FileOutputIcon from "@lucide/svelte/icons/file-output";

  // What the tax office expects: the year by kind of tax, the filing tasks with
  // their deadlines and the export for the tax firm.
  let { initialTask = "" }: { initialTask?: string } = $props();

  let year = $state(new Date().getFullYear());
  let datevOpen = $state(false);
</script>

<header class="topbar">
  <div>
    <h1>Steuern</h1>
  </div>
  <button
    type="button"
    class="secondary"
    title="Für die Steuerkanzlei: alle Buchungen des gewählten Jahres als DATEV-Datei"
    onclick={() => (datevOpen = true)}
    ><FileOutputIcon size={16} aria-hidden="true" /> DATEV-Export</button
  >
</header>
{#if datevOpen}
  <DatevExportDialog
    from={`${year}-01-01`}
    to={`${year}-12-31`}
    onclose={() => (datevOpen = false)}
  />
{/if}

<TaxFilingPage {initialTask} bind:year />
