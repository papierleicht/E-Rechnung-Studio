<script lang="ts">
  import Icon from "./Icon.svelte";
  import type { ValidationResult } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { issueClassLabels } from "../lib/received";
  let {
    result,
    pending = false,
    received = false,
    harmless = false,
    onresolve,
  }: {
    result?: ValidationResult;
    pending?: boolean;
    received?: boolean;
    /** The findings of a received document concern no Pflichtangabe. */
    harmless?: boolean;
    onresolve?: (field: string) => void;
  } = $props();
  const titles: Record<string, string> = {
    valid: "Die technische Prüfung ist bestanden",
    warning: "Das solltest du dir ansehen",
    invalid: "Hier musst du etwas korrigieren",
    incomplete: "Diese Angaben fehlen noch",
    error: "Die Prüfung konnte nicht abgeschlossen werden",
    unsupported: "Dieses Rechnungsformat wird noch nicht unterstützt",
  };
  const receivedTitles: Record<string, string> = {
    invalid: "Die Rechnungsdaten enthalten Fehler",
    incomplete: "Die Rechnungsdaten sind unvollständig",
    unsupported: "Keine bestätigte E-Rechnung",
  };
</script>

<section
  class="guidance-report"
  class:passed={result?.state === "valid" && !pending}
  class:failed={!pending && ["invalid", "error"].includes(result?.state || "")}
  class:pending={pending || !result}
  aria-label="Prüfergebnis"
  aria-busy={pending}
>
  <header>
    <strong role="status"
      >{pending
        ? "Deine Änderungen werden geprüft …"
        : (received && harmless && result?.state === "invalid" && "Die Prüfung meldet Hinweise") ||
          (received && receivedTitles[result?.state || ""]) ||
          titles[result?.state || ""] ||
          "Die Rechnung wurde noch nicht geprüft"}</strong
    >
    {#if !pending && result?.issues?.length}<span
        >{result.issues.length} {result.issues.length === 1 ? "Hinweis" : "Hinweise"}</span
      >{/if}
  </header>
  {#if result?.state === "valid" && !pending}<p>
      Die unterstützten Format- und Rechenregeln sind erfüllt. Kontrolliere trotzdem Empfänger,
      Leistungen und Beträge {received ? "vor der Zahlung" : "vor dem Ausstellen"}.
    </p>{/if}
  {#if result?.issues?.length}
    <ul class:checking={pending}>
      {#each result.issues as issue}
        <li>
          <div class="guidance-text">
            <p>
              {#if received && issue.class && issueClassLabels[issue.class]}<strong
                  >{issueClassLabels[issue.class]}:</strong
                >{" "}{/if}{issue.message}
            </p>
            {#if issue.details || issue.location}
              <details class="chevron-toggle">
                <summary>Prüfdetails · {issue.ruleId}</summary>
                <p>Originalmeldung des Prüfmoduls:</p>
                <pre>{issue.details || issue.message}</pre>
                {#if issue.location}<pre>{issue.location}</pre>{/if}
              </details>
            {/if}
          </div>
          {#if onresolve && issue.field}<button
              class="text"
              onclick={() => onresolve?.(issue.field!)}
              >{issue.field.startsWith("company.") ? "Unternehmensdaten öffnen" : "Zur Angabe"}
              <Icon name="next" size={16} /></button
            >{/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>
