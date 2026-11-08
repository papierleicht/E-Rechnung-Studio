<script lang="ts">
  import Tabs from "../../components/Tabs.svelte";
  let {
    active,
    disabled = false,
    onchange,
  }: {
    active: "invoices" | "quotes" | "recurring";
    disabled?: boolean;
    onchange: (tab: "invoices" | "quotes" | "recurring") => void;
  } = $props();
  const tabs = [
    { id: "invoices", label: "Rechnungen" },
    { id: "quotes", label: "Angebote" },
    { id: "recurring", label: "Dauerrechnungen" },
  ] as const;
</script>

<!-- Quotes belong to the customer invoicing workflow, not to the main navigation. -->
<Tabs {tabs} {active} {disabled} {onchange} label="Kundenbelege" />
