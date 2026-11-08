<script lang="ts">
  import { untrack } from "svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import { errorDetail } from "../../lib/errors";
  import { plan } from "../../lib/plan";
  import type { MarketListing } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";

  let {
    listing,
    onsend,
    onclose,
  }: {
    listing: MarketListing;
    onsend: (message: string) => Promise<void>;
    onclose: () => void;
  } = $props();
  const kind = $derived(listing.kind === "offer" ? "Angebot" : "Gesuch");
  let message = $state(untrack(() => `Hallo ${listing.author},\n\n`));
  let share = $state(false);
  let busy = $state(false);
  let error = $state("");

  async function send() {
    if (!share) {
      error = "Bitte erlaube, dass wir deine E-Mail-Adresse für die Antwort weitergeben.";
      return;
    }
    busy = true;
    error = "";
    try {
      await onsend(message.trim());
    } catch (err) {
      error = errorDetail(err);
    } finally {
      busy = false;
    }
  }
</script>

<FormDialog
  title="Anfrage senden"
  description={`An ${listing.author} zu „${listing.title}“. Die Nachricht geht per E-Mail, nicht in den Chat.`}
  {busy}
  {error}
  {onclose}
  onsubmit={send}
>
  <div class="form-grid contact-fields">
    <label
      >Deine Nachricht<textarea
        rows="7"
        required
        minlength="10"
        maxlength="2000"
        bind:value={message}
        placeholder={`Was möchtest du zu diesem ${kind} wissen oder anbieten?`}></textarea></label
    >
    <label
      ><input type="checkbox" bind:checked={share} /><span
        >Meine E-Mail-Adresse {#if $plan.email}<strong>{$plan.email}</strong>{/if} darf an
        {listing.author} gehen, damit die Antwort mich erreicht.</span
      ></label
    >
    <p class="form-help">
      {listing.author} bekommt deine Nachricht per E-Mail und antwortet direkt an dich. Die Adresse von
      {listing.author} erfährst du mit der Antwort. Wir speichern den Text nicht.
    </p>
  </div>
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button type="submit" class="primary" disabled={busy}
      >{busy ? "Wird gesendet …" : "Anfrage senden"}</button
    >
  {/snippet}
</FormDialog>

<style>
  .form-grid.contact-fields {
    grid-template-columns: minmax(0, 1fr);
  }
  .contact-fields .form-help {
    margin: 0;
  }
</style>
