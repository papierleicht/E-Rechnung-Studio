<script lang="ts">
  import FormDialog from "../../components/FormDialog.svelte";
  import ReleaseNotes from "./ReleaseNotes.svelte";
  import type { VersionNotes } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  let {
    versions,
    title,
    onclose,
  }: {
    versions: VersionNotes[];
    /** Defaults to "Neu in Version …" for one version. */
    title?: string;
    onclose: () => void;
  } = $props();
  const heading = $derived(
    title ?? (versions.length === 1 ? `Neu in Version ${versions[0].version}` : "Was ist neu?"),
  );
</script>

<FormDialog title={heading} {onclose}>
  <div class="notes-list">
    {#each versions as version (version.version)}
      <section>
        {#if versions.length > 1 || title}<h3 class="notes-version">
            Version {version.version}
          </h3>{/if}
        <ReleaseNotes notes={version.notes} />
      </section>
    {/each}
  </div>
  {#snippet footer()}
    <button type="button" class="primary" onclick={onclose}>Verstanden</button>
  {/snippet}
</FormDialog>

<style>
  .notes-list {
    display: grid;
    gap: 18px;
    max-height: 420px;
    overflow: auto;
  }
  .notes-version {
    margin: 0 0 8px;
    padding-bottom: 4px;
    border-bottom: 1px solid var(--theme-border);
    font-size: 14px;
  }
</style>
