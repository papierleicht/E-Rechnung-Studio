<script lang="ts">
  import { KEY_GROUPS, formatRecoveryKey, keyGroupsEntered } from "../lib/backup";

  /** Bound value, always formatted as XXXX-XXXX-…; `expected` compares it live. */
  let {
    value = $bindable(""),
    label,
    expected = "",
    disabled = false,
  }: { value?: string; label: string; expected?: string; disabled?: boolean } = $props();

  const groups = $derived(keyGroupsEntered(value));
  const complete = $derived(groups === KEY_GROUPS);
  const wrong = $derived(complete && Boolean(expected) && value !== expected);
</script>

<label class="key-input">
  <span>{label}</span>
  <textarea
    class="key-field"
    rows="2"
    autocomplete="off"
    autocapitalize="characters"
    spellcheck="false"
    placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
    {disabled}
    {value}
    oninput={(event) => {
      const field = event.currentTarget;
      value = formatRecoveryKey(field.value);
      field.value = value;
    }}
    onkeydown={(event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        event.currentTarget.form?.requestSubmit();
      }
    }}></textarea>
  <span class="key-progress" class:wrong class:right={complete && !wrong} aria-live="polite">
    {#if wrong}
      Das passt noch nicht. Vergleiche die Gruppen mit deiner Notiz.
    {:else if complete}
      Vollständig
    {:else}
      {groups} von {KEY_GROUPS} Gruppen
    {/if}
  </span>
</label>

<style>
  .key-input {
    display: grid;
    gap: 6px;
    margin-top: 12px;
  }
  .key-input textarea.key-field {
    box-sizing: content-box;
    width: 37ch;
    max-width: calc(100% - 28px);
    height: 3.4em;
    padding: 12px 14px;
    border: 1px solid var(--theme-input-border);
    border-radius: 8px;
    background: var(--theme-input);
    color: var(--theme-text);
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
    font-size: 18px;
    line-height: 1.7;
    resize: none;
  }
  .key-input textarea.key-field::placeholder {
    color: var(--theme-placeholder);
  }
  .key-input .key-progress {
    color: var(--theme-muted);
  }
  .key-input .key-progress.right {
    color: var(--theme-success-text);
  }
  .key-input .key-progress.wrong {
    color: var(--theme-error-text);
  }
</style>
