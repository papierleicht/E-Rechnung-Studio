<script lang="ts">
  /** The one statistics choice, in the settings and in the setup:
      "Bei der Verbesserung helfen" (off until chosen). What the update check
      counts without it is described in the privacy policy. */
  let {
    improvement = $bindable(),
  }: {
    improvement: boolean;
  } = $props();
</script>

<div class="statistics-choices">
  <label class="statistics-choice">
    <input type="checkbox" bind:checked={improvement} />
    <span>
      <strong>Bei der Verbesserung helfen</strong>
      <span class="statistics-help">
        Die App zählt ohne Kennung, welche Bereiche und Funktionen du nutzt, und schickt einmal am
        Tag nur diese Summen. Deine Rechnungen, Kunden und Beträge verlassen dein Gerät nie.
      </span>
    </span>
  </label>
</div>

<style>
  .statistics-choices {
    display: grid;
    gap: 14px;
  }
  .statistics-choice {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    min-width: 0;
    color: var(--theme-text);
    font-size: 13px;
    font-weight: 400;
    letter-spacing: normal;
    text-transform: none;
    cursor: pointer;
  }
  .statistics-choice input {
    flex-shrink: 0;
    margin-top: 2px;
  }
  .statistics-choice > span {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .statistics-choice strong {
    font-weight: 650;
  }
  .statistics-help {
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
</style>
