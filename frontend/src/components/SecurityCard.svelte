<script lang="ts">
  import Icon from "./Icon.svelte";
  import SecurityMeter from "./SecurityMeter.svelte";
  import { security, stages } from "../lib/security";
  import { SECURITY_STEPS, openStage, type Stage, type StageAction } from "../lib/securityLevel";

  /** Carries out a station's next step. */
  let { onaction }: { onaction: (kind: StageAction) => void } = $props();

  // Null follows the state, a click keeps the station the customer chose.
  let chosen = $state<Stage["id"] | "" | null>(null);
  const open = $derived(chosen === null ? openStage($stages) : chosen);
  const toggle = (id: Stage["id"]) => (chosen = open === id ? "" : id);
</script>

<section class="settings-group" aria-label="Dein Stand">
  <div class="journey-head">
    <SecurityMeter level={$security.level} large />
    <div>
      <h3>{$security.title}</h3>
      <p class="settings-help">Stufe {$security.level} von {SECURITY_STEPS}</p>
    </div>
  </div>
  <ol class="journey">
    {#each $stages as stage (stage.id)}
      <li class:reached={stage.reached} class:open={open === stage.id}>
        <button
          type="button"
          class="stage-head"
          aria-expanded={open === stage.id}
          onclick={() => toggle(stage.id)}
        >
          <span class="stage-mark" aria-hidden="true">
            {#if stage.reached}<Icon name="check" size={13} />{/if}
          </span>
          <span class="stage-text">
            <strong>{stage.title}</strong>
            <span>{stage.summary}</span>
          </span>
          {#if stage.action && open !== stage.id}<span class="stage-hint">{stage.action.label}</span
            >{/if}
          <span class="stage-chevron" aria-hidden="true"><Icon name="next" size={14} /></span>
        </button>
        {#if open === stage.id}
          <div class="stage-body">
            <ul>
              {#each stage.items as item (item.label)}
                <li class:done={item.done}>
                  <span class="item-kind">{item.kind === "time" ? "Spart Zeit" : "Schützt"}</span>
                  <span>{item.label}</span>
                  <span class="item-state">{item.done ? "läuft" : ""}</span>
                </li>
              {/each}
            </ul>
            {#if stage.action}
              {@const action = stage.action}
              <button class="primary" type="button" onclick={() => onaction(action.kind)}
                >{action.label}</button
              >
            {/if}
          </div>
        {/if}
      </li>
    {/each}
  </ol>
  {#if $security.level > 0}
    <button class="text manage" type="button" onclick={() => onaction("website")}
      >Abo verwalten</button
    >
  {/if}
</section>

<style>
  .journey-head {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .journey {
    margin: 14px 0 0;
    padding: 0;
    list-style: none;
  }
  .journey > li {
    position: relative;
  }
  /* The line that joins the stations into one way. */
  .journey > li:not(:last-child)::before {
    content: "";
    position: absolute;
    top: 30px;
    bottom: -10px;
    left: 9px;
    width: 2px;
    background: var(--theme-border);
  }
  .journey > li.reached:not(:last-child)::before {
    background: var(--theme-accent);
  }
  .stage-head {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 8px 8px 8px 0;
    border: 0;
    border-radius: 8px;
    background: none;
    color: var(--theme-text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .stage-head:hover {
    background: var(--theme-surface-muted);
  }
  .stage-mark {
    display: inline-flex;
    position: relative;
    flex: none;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border: 2px solid var(--theme-border);
    border-radius: 50%;
    background: var(--theme-surface);
    box-sizing: border-box;
  }
  .reached > .stage-head .stage-mark {
    border-color: var(--theme-accent);
    background: var(--theme-accent);
    color: var(--theme-on-primary);
  }
  .stage-text {
    display: grid;
    flex: 1;
    min-width: 0;
  }
  .stage-text strong {
    font-weight: 600;
  }
  .stage-text span,
  .item-kind,
  .item-state {
    color: var(--theme-muted);
  }
  .stage-hint {
    flex: none;
    color: var(--theme-accent);
    font-weight: 600;
  }
  .stage-chevron {
    display: inline-flex;
    flex: none;
    color: var(--theme-muted);
    transition: transform 140ms ease;
  }
  .open > .stage-head .stage-chevron {
    transform: rotate(90deg);
  }
  .stage-body {
    padding: 4px 8px 12px 32px;
  }
  .stage-body ul {
    display: grid;
    gap: 6px;
    margin: 0 0 12px;
    padding: 0;
    list-style: none;
  }
  .stage-body ul:last-child {
    margin-bottom: 0;
  }
  .stage-body li {
    display: grid;
    grid-template-columns: 76px minmax(0, 1fr) auto;
    gap: 10px;
    color: var(--theme-muted);
  }
  .stage-body li.done {
    color: var(--theme-text);
  }
  .item-state {
    color: var(--theme-accent);
  }
  .manage {
    margin-top: 6px;
  }
  @media (prefers-reduced-motion: reduce) {
    .stage-chevron {
      transition: none;
    }
  }
</style>
