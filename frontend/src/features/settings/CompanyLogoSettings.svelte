<script lang="ts">
  import Icon from "../../components/Icon.svelte";
  import { PrepareCompanyLogo } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { Company } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { serviceErrorMessage } from "../../lib/errors";
  let { company = $bindable(), disabled }: { company: Company; disabled: boolean } = $props();
  let input: HTMLInputElement;
  let preparing = $state(false),
    error = $state(""),
    warning = $state("");
  // The stored logo is already normalized; preview it at its printed size.
  let size = $state<{ width: number; height: number } | null>(null);
  const source = $derived(company.logo ? `data:image/png;base64,${company.logo}` : "");

  function readBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result).replace(/^data:[^,]*,/, ""));
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(file);
    });
  }
  async function choose(event: Event) {
    const file = (event.currentTarget as HTMLInputElement).files?.[0];
    input.value = "";
    if (!file) return;
    error = warning = "";
    if (file.size > 10 * 1024 * 1024) {
      error = "Das Logo darf höchstens 10 MB groß sein.";
      return;
    }
    preparing = true;
    try {
      const prepared = await PrepareCompanyLogo(await readBase64(file));
      company.logo = prepared.logo;
      warning = prepared.warning;
    } catch (e) {
      error = serviceErrorMessage(
        e,
        "Das Logo ließ sich nicht übernehmen. Probiere eine andere PNG- oder JPG-Datei.",
      );
    } finally {
      preparing = false;
    }
  }
  function remove() {
    company.logo = "";
    error = warning = "";
  }
  // Same fit as the invoice renderer: 45 × 20 mm box, aspect ratio preserved.
  function measure(event: Event) {
    const image = event.currentTarget as HTMLImageElement;
    let width = 45,
      height = (45 * image.naturalHeight) / image.naturalWidth;
    if (height > 20) {
      height = 20;
      width = (20 * image.naturalWidth) / image.naturalHeight;
    }
    size = { width, height };
  }
</script>

<section class="logo-settings" aria-labelledby="logo-title">
  <div>
    <h2 id="logo-title">Logo auf der Rechnung</h2>
    <p class="form-help">
      PNG oder JPG. Transparente und weiße Ränder schneiden wir automatisch ab. Das Logo wird auf
      höchstens 45 × 20 mm verkleinert und oben rechts über der Rechnungsnummer gedruckt. Bereits
      ausgestellte Rechnungen ändern sich dadurch nicht.
    </p>
  </div>
  <div class="logo-row">
    <div class="logo-paper" aria-live="polite">
      {#if source}
        <img
          src={source}
          alt="Aktuelles Rechnungslogo"
          onload={measure}
          style:width={size ? `${size.width}mm` : undefined}
          style:height={size ? `${size.height}mm` : undefined}
        />
      {:else}
        <span>Kein Logo</span>
      {/if}
    </div>
    <div class="logo-actions">
      <button
        type="button"
        class="secondary"
        disabled={disabled || preparing}
        onclick={() => input.click()}
        ><Icon name={source ? "folder" : "plus"} size={16} />
        {preparing
          ? "Logo wird vorbereitet …"
          : source
            ? "Logo ersetzen …"
            : "Logo auswählen …"}</button
      >
      {#if source}
        <button
          type="button"
          class="text danger-text"
          disabled={disabled || preparing}
          onclick={remove}><Icon name="trash" size={16} /> Logo entfernen</button
        >
      {/if}
      <input
        bind:this={input}
        class="hidden-file"
        type="file"
        accept="image/png,image/jpeg,.png,.jpg,.jpeg"
        data-field="company.logo"
        onchange={choose}
      />
    </div>
  </div>
  {#if error}<p class="logo-message error" role="alert">{error}</p>{/if}
  {#if warning}<p class="logo-message" role="status">{warning}</p>{/if}
  {#if source}<p class="form-help">
      Klicke oben rechts auf „Speichern“, um die Änderung zu übernehmen.
    </p>{/if}
</section>

<style>
  .logo-settings {
    display: grid;
    gap: 12px;
    margin: 0 0 24px;
  }
  .logo-settings h2 {
    margin: 0 0 4px;
  }
  .logo-settings p {
    margin: 0;
  }
  .logo-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
  }
  /* A slice of white paper so transparent logos are judged as printed. */
  .logo-paper {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    box-sizing: border-box;
    width: calc(45mm + 24px);
    min-height: calc(20mm + 24px);
    padding: 12px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--paper-white);
  }
  .logo-paper img {
    display: block;
    max-width: 45mm;
    max-height: 20mm;
  }
  .logo-paper span {
    margin: 0 auto;
    color: var(--paper-muted);
    font-size: 12px;
  }
  .logo-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .logo-message {
    overflow-wrap: anywhere;
    padding: 8px 12px;
    border: 1px solid var(--theme-warning-border);
    border-radius: 8px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    font-size: 13px;
  }
  .logo-message.error {
    border-color: var(--theme-error-border);
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
</style>
