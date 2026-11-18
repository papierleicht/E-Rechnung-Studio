<script lang="ts">
  /** The two statistics choices, in the settings and in the setup.
      usage: anonymous count of active installations with the update check (on by default).
      improvement: which areas and functions are used (off until chosen). */
  let {
    usage = $bindable(),
    improvement = $bindable(),
  }: {
    usage: boolean;
    improvement: boolean;
  } = $props();
</script>

<div class="statistics-choices">
  <label class="statistics-choice">
    <input type="checkbox" bind:checked={usage} />
    <span>
      <strong>Anonyme Nutzungsstatistik senden</strong>
      <span class="statistics-help">
        Bei der Update-Suche schickt die App ihre Version, das Betriebssystem und ob sie heute,
        diese Woche oder diesen Monat zum ersten Mal sucht. So sieht papierleicht, wie viele
        Installationen aktiv sind. Die App sendet dabei keine Kennung und keine deiner Daten.
      </span>
    </span>
  </label>
  <label class="statistics-choice">
    <input type="checkbox" bind:checked={improvement} />
    <span>
      <strong>Bei der Verbesserung helfen</strong>
      <span class="statistics-help">
        Die App zählt, welche Bereiche du öffnest und welche Funktionen du nutzt, zum Beispiel
        „Rechnung festschreiben“ oder „Bankabruf“. Einmal am Tag schickt sie nur diese Summen,
        ebenfalls ohne Kennung. Rechnungen, Kunden, Beträge und Namen verlassen dein Gerät nie.
        Schaltest du es aus, löscht die App die noch nicht gesendeten Zähler.
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
