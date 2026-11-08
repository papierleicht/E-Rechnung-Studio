<script lang="ts">
  import { untrack } from "svelte";

  let {
    endpoint = $bindable(""),
    scheme = $bindable(""),
    email = "",
    field = "",
    followEmail = false,
    onchange = () => {},
  }: {
    endpoint: string;
    scheme: string;
    email?: string;
    field?: string;
    followEmail?: boolean;
    onchange?: () => void;
  } = $props();

  // The sender's electronic address follows the contact e-mail until another
  // address or address type is chosen; only e-mail changes trigger this.
  let previousEmail = untrack(() => email);
  $effect(() => {
    const next = email.trim();
    untrack(() => {
      const following = !endpoint.trim() || endpoint.trim() === previousEmail.trim();
      if (followEmail && next && following && (scheme === "" || scheme === "EM")) {
        endpoint = next;
        scheme = "EM";
      }
      previousEmail = next;
    });
  });
</script>

<div class="electronic-address">
  <label
    >Elektronische Adresse<input
      data-field={field ? field + ".endpoint" : undefined}
      bind:value={endpoint}
      {onchange}
      placeholder="z. B. E-Mail-Adresse oder Leitweg-ID"
    /></label
  >
  <label
    >Adressart<select
      data-field={field ? field + ".endpointScheme" : undefined}
      bind:value={scheme}
      {onchange}
    >
      <option value="">Auswählen …</option>
      <option value="EM">E-Mail</option>
      <option value="0204">Leitweg-ID</option>
      <option value="0088">GLN</option>
      <option value="9930">Deutsche USt-IdNr.</option>
      {#if scheme && !["EM", "0204", "0088", "9930"].includes(scheme)}<option value={scheme}
          >{scheme}</option
        >{/if}
    </select></label
  >
  {#if email && (endpoint !== email || scheme !== "EM")}<button
      type="button"
      class="text address-use-email"
      onclick={() => {
        endpoint = email;
        scheme = "EM";
        onchange();
      }}>Kontakt-E-Mail übernehmen</button
    >{/if}
</div>
