<script lang="ts">
  import FormDialog from "../../components/FormDialog.svelte";
  import { errorDetail } from "../../lib/errors";

  let {
    what,
    onsend,
    onclose,
  }: {
    /** "Nachricht" or "Eintrag". */
    what: string;
    onsend: (reason: string) => Promise<void>;
    onclose: () => void;
  } = $props();
  const reasons = [
    "Beleidigend oder respektlos",
    "Spam oder Werbung am falschen Ort",
    "Daten von Dritten oder Zugangsdaten",
    "Betrug oder unseriöses Angebot",
    "Rechtswidriger Inhalt",
    "Etwas anderes",
  ];
  let reason = $state(reasons[0]);
  let note = $state("");
  let busy = $state(false);
  let error = $state("");

  async function send() {
    busy = true;
    error = "";
    try {
      await onsend(note.trim() ? `${reason}: ${note.trim()}` : reason);
    } catch (err) {
      error = errorDetail(err);
    } finally {
      busy = false;
    }
  }
</script>

<FormDialog
  title={`${what} melden`}
  description="Das papierleicht-Team sieht sich die Meldung an. Nach drei Meldungen wird der Beitrag ausgeblendet, bis wir entschieden haben."
  {busy}
  {error}
  {onclose}
  onsubmit={send}
>
  <div class="form-grid">
    <label
      >Grund<select bind:value={reason}>
        {#each reasons as r}<option value={r}>{r}</option>{/each}
      </select></label
    >
    <label
      >Ergänzung (freiwillig)<textarea
        rows="3"
        maxlength="300"
        bind:value={note}
        placeholder="Was ist dir aufgefallen?"></textarea></label
    >
  </div>
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button type="submit" class="primary" disabled={busy}
      >{busy ? "Wird gemeldet …" : "Melden"}</button
    >
  {/snippet}
</FormDialog>
