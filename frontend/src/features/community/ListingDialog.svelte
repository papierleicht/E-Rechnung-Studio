<script lang="ts">
  import { untrack } from "svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import { marketCategories } from "../../lib/community";
  import { errorDetail } from "../../lib/errors";
  import type { MarketDraft } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";

  let {
    postalCode = "",
    city = "",
    oncreate,
    onclose,
  }: {
    postalCode?: string;
    city?: string;
    oncreate: (draft: MarketDraft) => Promise<void>;
    onclose: () => void;
  } = $props();
  let draft: MarketDraft = $state(
    untrack(() => ({
      kind: "offer",
      category: "",
      title: "",
      body: "",
      postalCode: /^[0-9]{5}$/.test(postalCode) ? postalCode : "",
      city,
      remote: false,
      contact: "",
    })),
  );
  let busy = $state(false);
  let error = $state("");

  async function create() {
    busy = true;
    error = "";
    try {
      await oncreate({ ...draft });
    } catch (err) {
      error = errorDetail(err);
    } finally {
      busy = false;
    }
  }
</script>

<FormDialog
  title="Eintrag veröffentlichen"
  description="Alle Mitglieder sehen deinen Eintrag 30 Tage lang. Du kannst ihn jederzeit beenden oder verlängern."
  {busy}
  {error}
  {onclose}
  onsubmit={create}
>
  <div class="form-grid cols-2">
    <div class="listing-kind full" role="radiogroup" aria-label="Art des Eintrags">
      <label class:chosen={draft.kind === "offer"}
        ><input
          type="radio"
          name="kind"
          value="offer"
          aria-label="Ich biete an"
          bind:group={draft.kind}
        /><strong>Ich biete an</strong><small>Leistung, Ware oder freie Kapazität</small></label
      >
      <label class:chosen={draft.kind === "request"}
        ><input
          type="radio"
          name="kind"
          value="request"
          aria-label="Ich suche"
          bind:group={draft.kind}
        /><strong>Ich suche</strong><small>Handwerker, Dienstleister, Partner</small></label
      >
    </div>
    <label class="full"
      >Branche<select required bind:value={draft.category}>
        <option value="" disabled>Bitte wählen</option>
        {#each marketCategories as c}<option value={c.id}>{c.label}</option>{/each}
      </select></label
    >
    <label class="full"
      >Titel<input
        required
        minlength="4"
        maxlength="80"
        bind:value={draft.title}
        placeholder={draft.kind === "offer"
          ? "z. B. Webseiten für Handwerksbetriebe"
          : "z. B. Elektriker für Ladenumbau gesucht"}
      /></label
    >
    <label class="full"
      >Beschreibung<textarea
        rows="4"
        maxlength="1000"
        bind:value={draft.body}
        placeholder="Was, bis wann, in welchem Umfang? Preise und Bedingungen ehrlich nennen."
      ></textarea></label
    >
    <label
      >Postleitzahl<input
        required
        inputmode="numeric"
        pattern={"[0-9]{5}"}
        title="Fünfstellige Postleitzahl"
        maxlength="5"
        bind:value={draft.postalCode}
      /></label
    >
    <label>Ort<input maxlength="60" bind:value={draft.city} /></label>
    <label class="full"
      ><input type="checkbox" bind:checked={draft.remote} />Auch online oder deutschlandweit möglich</label
    >
    <label class="full"
      >Kontakt (freiwillig)<input
        maxlength="120"
        bind:value={draft.contact}
        placeholder="z. B. Telefon, Website oder E-Mail für Mitglieder"
      /></label
    >
    <p class="form-help full">
      Mitglieder erreichen dich über „Anfrage senden“: Die Nachricht kommt per E-Mail an die Adresse
      deines Kontos, und die sieht erst, wem du antwortest. Was du hier als Kontakt angibst, sehen
      alle Mitglieder.
    </p>
  </div>
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button type="submit" class="primary" disabled={busy}
      >{busy ? "Wird veröffentlicht …" : "Veröffentlichen"}</button
    >
  {/snippet}
</FormDialog>

<style>
  .full {
    grid-column: 1 / -1;
  }
  .listing-kind {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(12rem, 100%), 1fr));
    gap: 8px;
  }
  .listing-kind label {
    display: grid !important;
    grid-template-columns: auto minmax(0, 1fr);
    column-gap: 8px;
    align-items: center;
    padding: 10px 12px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    cursor: pointer;
  }
  .listing-kind label.chosen {
    border-color: var(--theme-accent);
    background: var(--theme-selected);
  }
  .listing-kind input {
    grid-row: span 2;
  }
  .listing-kind small {
    grid-column: 2;
    color: var(--theme-muted);
    font-size: 12px;
    font-weight: 400;
  }
</style>
