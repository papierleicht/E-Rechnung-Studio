<script lang="ts">
  import type { SmallBusinessStatus } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { amountLabel, dateLabel } from "../../lib/format";

  let { status }: { status: SmallBusinessStatus } = $props();
  const year = $derived(status.year);
  const warning = $derived(["next_year", "exceeded", "previous_exceeded"].includes(status.state));
  const message = $derived.by(() => {
    switch (status.state) {
      case "previous_exceeded":
        return `Nach den Zahlen in der App lag dein Umsatz ${year - 1} über 25.000 €. Dann gilt die Kleinunternehmerregelung für ${year} in der Regel nicht mehr: Du weist Umsatzsteuer aus und gibst Voranmeldungen ab. Trifft das zu, ändere deine Unternehmensdaten.`;
      case "exceeded":
        return `Nach den Zahlen in der App liegt dein Umsatz ${year} über 100.000 €.${status.crossed ? ` Die Grenze wird mit der Zahlung vom ${dateLabel(status.crossed.date)} überschritten (${status.crossed.label}).` : ""} Ab dem Umsatz, der die Grenze überschreitet, endet die Kleinunternehmerregelung in der Regel sofort: Für diesen und alle späteren Umsätze fällt Umsatzsteuer an. Trifft das zu, berichtige betroffene Rechnungen und ändere deine Unternehmensdaten. Kläre das am besten mit deiner Steuerberatung.`;
      case "next_year":
        return `Nach den Zahlen in der App liegt dein Umsatz ${year} über 25.000 €. Dann endet die Kleinunternehmerregelung in der Regel zum 1. Januar ${year + 1}. Für ${year} gilt die Grenze von 100.000 € – außer du hast dein Unternehmen ${year} gegründet: Dann gilt schon in diesem Jahr die Grenze von 25.000 €.`;
      case "near":
        return `Du näherst dich 25.000 €. Liegt dein Umsatz ${year} darüber, endet die Kleinunternehmerregelung in der Regel zum 1. Januar ${year + 1}.`;
    }
    return "Nach den Zahlen in der App liegst du unter den Grenzen der Kleinunternehmerregelung.";
  });
</script>

<section class="tax-card" aria-labelledby="small-business-title">
  <h2 id="small-business-title">Kleinunternehmerregelung</h2>
  <dl class="limits">
    <div>
      <dt>Umsatz {year}</dt>
      <dd><strong>{amountLabel(status.turnover)}</strong> <span>von 100.000 €</span></dd>
    </div>
    <div>
      <dt>Umsatz {year - 1}</dt>
      <dd>
        {#if status.previousTurnover}<strong>{amountLabel(status.previousTurnover)}</strong>
          <span>von 25.000 €</span>{:else}<span>keine Daten in der App – prüfe selbst</span>{/if}
      </dd>
    </div>
  </dl>
  <p class:tax-issues={warning} class:tax-ok={!warning} role={warning ? "alert" : undefined}>
    {message}
  </p>
  <div class="detail-list">
    <details>
      <summary
        ><span class="detail-title">Wie die App zählt</span><span class="detail-state"
          >Zahlungseingänge, Grenzen, Ausnahmen</span
        ></summary
      >
      <p class="tax-muted">
        Gezählt werden Zahlungseingänge für steuerbare Umsätze; Leistungen für Unternehmen im
        Ausland zählen nicht mit. Einnahmen ohne Rechnung in der App rechnest du selbst hinzu. Im
        Jahr der Gründung gilt bereits die Grenze von 25.000 €.
      </p>
    </details>
  </div>
  <h3>Umsatzsteuer trotz Kleinunternehmerregelung</h3>
  <p>
    Gilt für eine Leistung die Steuerschuld des Empfängers – das kann etwa Werbung bei Google oder
    Meta, Software von Anbietern im Ausland oder Bauleistungen betreffen –, schuldest du die
    Umsatzsteuer auch als Kleinunternehmer (§ 13b UStG). Ob das zutrifft, prüft die App nicht.
    Erfasse solche Rechnungen mit „Steuerschuld bei dir“; die Voranmeldung erscheint dann in deinen
    Aufgaben.
  </p>
</section>

<style>
  .limits {
    display: flex;
    flex-wrap: wrap;
    gap: 12px 32px;
    margin: 8px 0 12px;
  }
  .limits dt {
    color: var(--theme-muted);
    font-size: 13px;
  }
  .limits dd {
    margin: 2px 0 0;
  }
  .limits dd strong {
    font-size: 18px;
  }
  .limits dd span {
    color: var(--theme-muted);
  }
  .detail-list {
    margin: 0 0 12px;
  }
  details p {
    margin-top: 6px;
  }
  h3 {
    margin-top: 4px;
  }
</style>
