<script lang="ts">
  import type { Snippet } from "svelte";
  import Icon from "./Icon.svelte";
  let {
    title,
    subtitle = "",
    meta = "",
    amount = "",
    status,
    tooltip,
    disabled = false,
    onclick,
  }: {
    title: string;
    subtitle?: string;
    /** The narrow column after the title, usually a date. */
    meta?: string;
    amount?: string;
    /** One element for the status column: a badge or a payment state. */
    status?: Snippet;
    tooltip?: string;
    disabled?: boolean;
    onclick: (event: MouseEvent & { currentTarget: HTMLButtonElement }) => void;
  } = $props();
</script>

<!-- One row of a list, with the columns and classes of the invoice list, so every
     list in the app reads the same. Place rows in a <div class="invoice-rows">. -->
<button class="invoice-entry" {onclick} {disabled} title={tooltip}>
  <span class="invoice-entry-title">
    <strong class="entry-clamp" {title}>{title}</strong>
    {#if subtitle}<small class="entry-clamp" title={subtitle}>{subtitle}</small>{/if}
  </span>
  <span class="invoice-entry-date">{meta}</span>
  <strong class="invoice-entry-amount">{amount}</strong>
  {#if status}{@render status()}{:else}<span></span>{/if}
  <Icon name="next" size={16} />
</button>

<style>
  /* Long names and purposes stop after two lines, so rows keep an even height;
     the tooltip has the full text. */
  .entry-clamp {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
  }
</style>
