<script lang="ts">
  import FormDialog from "../../components/FormDialog.svelte";
  let {
    name,
    bookings,
    csv = false,
    busy = false,
    error = "",
    onclose,
    onconfirm,
  }: {
    name: string;
    /** How many bookings go with the account. */
    bookings: number;
    /** An account known only from CSV imports, not from the bank. */
    csv?: boolean;
    busy?: boolean;
    error?: string;
    onclose: () => void;
    onconfirm: () => void;
  } = $props();
</script>

<FormDialog
  title="Konto entfernen"
  description={name}
  {busy}
  {error}
  {onclose}
  onsubmit={onconfirm}
>
  <p>
    Das Konto und {bookings === 1 ? "sein Umsatz" : `seine ${bookings} Umsätze`} werden aus dieser App
    entfernt. {csv
      ? "Hast du die CSV-Datei noch, kannst du sie später erneut einlesen."
      : "Bei deiner Bank ändert sich nichts. Du kannst das Konto später wieder verbinden und die Umsätze neu abrufen."}
  </p>
  <p>
    Umsätze, die schon einer Rechnung oder Ausgabe zugeordnet sind, verhindern das Entfernen. Hebe
    diese Zuordnungen vorher im Reiter „Zugeordnet“ auf. Das Entfernen wird protokolliert.
  </p>
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button type="submit" class="secondary danger-text" disabled={busy}>Konto entfernen</button>
  {/snippet}
</FormDialog>

<style>
  p {
    margin: 0 0 12px;
    font-size: 13px;
    line-height: 1.5;
  }
</style>
