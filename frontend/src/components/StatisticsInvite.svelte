<script lang="ts">
  import HandHeartIcon from "@lucide/svelte/icons/hand-heart";

  /** The one-time question after the update that introduced the statistics.
      Both answers count as answered; the choice can be changed in the settings. */
  let {
    busy = false,
    onanswer,
  }: {
    busy?: boolean;
    onanswer: (helping: boolean) => void;
  } = $props();
</script>

<section class="statistics-invite" aria-labelledby="statistics-invite-title">
  <span class="statistics-invite-icon" aria-hidden="true"><HandHeartIcon size={20} /></span>
  <div class="statistics-invite-body">
    <h2 id="statistics-invite-title">Hilfst du mit, E-Rechnung-Studio besser zu machen?</h2>
    <p>
      Wenn du zustimmst, zählt die App, welche Bereiche du öffnest und welche Funktionen du nutzt,
      und schickt einmal am Tag nur diese Summen, ohne Kennung. Rechnungen, Kunden, Beträge und
      Namen verlassen dein Gerät nie. So sehen wir, was gebraucht wird und wo die App hakt.
    </p>
    <p class="statistics-invite-note">
      Neu ist außerdem: Bei der Update-Suche zählt die App anonym mit, wie viele Installationen
      aktiv sind. Beides kannst du jederzeit unter Einstellungen → Allgemein → Statistik ändern.
    </p>
    <div class="statistics-invite-actions">
      <button type="button" class="secondary" disabled={busy} onclick={() => onanswer(true)}
        >Ja, mithelfen</button
      >
      <button type="button" class="secondary" disabled={busy} onclick={() => onanswer(false)}
        >Nein, danke</button
      >
    </div>
  </div>
</section>

<style>
  .statistics-invite {
    display: flex;
    gap: 12px;
    margin: 0 0 20px;
    padding: 18px 20px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
    min-width: 0;
  }
  .statistics-invite-icon {
    display: inline-flex;
    flex-shrink: 0;
    padding-top: 2px;
    color: var(--theme-accent);
  }
  .statistics-invite-body {
    min-width: 0;
  }
  .statistics-invite h2 {
    margin: 0 0 6px;
    font-size: var(--ui-section-title-size);
  }
  .statistics-invite p {
    margin: 0 0 8px;
    font-size: 13px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .statistics-invite-note {
    color: var(--theme-muted);
    font-size: 12px;
  }
  .statistics-invite-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 12px;
  }
</style>
