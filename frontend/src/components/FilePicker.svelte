<script lang="ts">
  // A file field in the app's own words: the native control labels itself in
  // the system language and never shows a file that arrived by drag and drop.
  let {
    accept,
    disabled = false,
    filename = "",
    empty = "Keine Datei ausgewählt",
    onpick,
  }: {
    accept: string;
    disabled?: boolean;
    /** The file taken so far, however it arrived. */
    filename?: string;
    /** Shown while no file is taken. */
    empty?: string;
    onpick: (file: File) => void;
  } = $props();
  let input: HTMLInputElement;
  function changed(event: Event) {
    const element = event.currentTarget as HTMLInputElement;
    const file = element.files?.[0];
    // The same file can be picked again after it was replaced by a drop.
    element.value = "";
    if (file) onpick(file);
  }
</script>

<span class="file-picker">
  <button type="button" class="secondary" {disabled} onclick={() => input.click()}
    >Datei auswählen</button
  >
  <span class="file-picker-name" class:empty={!filename}>{filename || empty}</span>
  <input bind:this={input} class="hidden-file" type="file" {accept} {disabled} onchange={changed} />
</span>

<style>
  .file-picker {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .file-picker button {
    flex: none;
    padding: 8px 12px;
    font-size: 13px;
  }
  .file-picker-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--theme-text);
    font-size: 13px;
    font-weight: 400;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .file-picker-name.empty {
    color: var(--theme-muted);
  }
</style>
