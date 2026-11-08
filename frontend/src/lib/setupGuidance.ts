/** Explanations describe a chosen treatment; they never determine eligibility. */
export const setupTaxGuidance: Record<
  string,
  { meaning: string; example: string; effect: string }
> = {
  standard: {
    meaning:
      "Du weist die Umsatzsteuer zusätzlich zum Nettobetrag aus. Welcher Steuersatz gilt, hängt von deiner Leistung ab.",
    example:
      "Rechenbeispiel bei 19 %: 100 € netto + 19 € Umsatzsteuer = 119 € Rechnungsbetrag. Das Beispiel legt deinen Steuersatz nicht fest.",
    effect:
      "Neue Rechnungen starten mit Umsatzsteuerausweis. Prüfe den Steuersatz bei jeder Position.",
  },
  "small-business": {
    meaning:
      "Du wendest die Kleinunternehmerregelung nach § 19 UStG an. Das ist eine steuerliche Regelung – nicht einfach ein anderes Wort für ein kleines Unternehmen oder ein Kleingewerbe.",
    example:
      "Bei einem vereinbarten Preis von 100 € weist du keine 19 € Umsatzsteuer zusätzlich aus. Auf der Rechnung steht ein Hinweis auf die Kleinunternehmerregelung.",
    effect:
      "Die App weist keine Umsatzsteuer gesondert aus und ergänzt den Hinweis nach § 19. Wähle diese Option nur, wenn du die Regelung tatsächlich anwendest.",
  },
  "zero-rated": {
    meaning:
      "Für deine konkrete Leistung gilt ein gesetzlicher Steuersatz von 0 %. Das ist nicht dasselbe wie die Kleinunternehmerregelung oder eine Steuerbefreiung.",
    example:
      "Das gilt nur für Umsätze mit entsprechender gesetzlicher Grundlage – nicht pauschal, weil du keine Steuer berechnen möchtest.",
    effect:
      "Die Rechnung verwendet die Steuerkategorie Nullsteuersatz. Ob du sie nutzen darfst, musst du für jeden Umsatz prüfen.",
  },
  "reverse-charge": {
    meaning:
      "Hier schuldet der Leistungsempfänger die Umsatzsteuer, nicht du als Rechnungssteller.",
    example:
      "Das kann bei bestimmten gesetzlich geregelten Leistungen gelten. Dass dein Kunde ein Unternehmen ist, reicht dafür nicht.",
    effect:
      "Keine gesonderte Umsatzsteuer; die App ergänzt einen Hinweis auf die Steuerschuldnerschaft des Leistungsempfängers.",
  },
  "intra-community": {
    meaning:
      "Es geht um eine innergemeinschaftliche Warenlieferung, wenn die dafür geltenden Voraussetzungen erfüllt sind.",
    example:
      "Du lieferst Waren an ein Unternehmen in einem anderen EU-Land. Ein Kunde in der EU reicht allein nicht aus; Dienstleistungen musst du gesondert beurteilen.",
    effect:
      "Die App verwendet den Steuerfall innergemeinschaftliche Lieferung. Prüfe die USt-IdNr. und die weiteren Voraussetzungen.",
  },
  export: {
    meaning:
      "Es geht um eine steuerfreie Ausfuhrlieferung von Waren in ein Drittland, wenn die dafür geltenden Voraussetzungen erfüllt sind.",
    example:
      "Dass dein Kunde außerhalb der EU sitzt, macht eine Leistung nicht automatisch zur steuerfreien Ausfuhrlieferung.",
    effect:
      "Die Rechnung erhält den Steuerfall Ausfuhrlieferung. Die erforderlichen Nachweise musst du selbst bereithalten – die App ersetzt sie nicht.",
  },
  exempt: {
    meaning:
      "Deine Leistung ist aufgrund einer anderen gesetzlichen Regelung von der Umsatzsteuer befreit.",
    example:
      "Es gibt zum Beispiel Befreiungen für bestimmte Heilbehandlungen. Nicht jede Tätigkeit in einer Branche ist automatisch befreit.",
    effect:
      "Keine gesonderte Umsatzsteuer. Ergänze den Befreiungshinweis, der zu deiner Leistung passt.",
  },
  "out-of-scope": {
    meaning:
      "Der Umsatz ist nicht steuerbar, etwa weil er nicht unter die betreffende inländische Umsatzbesteuerung fällt.",
    example:
      "Nicht steuerbar und steuerfrei sind unterschiedliche Sachverhalte. Kläre anhand deiner konkreten Leistung, was zutrifft.",
    effect: "Die App verwendet die Kategorie nicht steuerbarer Umsatz und den passenden Hinweis.",
  },
  "margin-goods": {
    meaning: "Für bestimmte Gebrauchtgegenstände wendest du die Differenzbesteuerung an.",
    example:
      "Der Handel mit gebrauchten Waren allein genügt nicht; auch die Voraussetzungen beim Einkauf und Verkauf sind entscheidend.",
    effect:
      "Kein gesonderter Umsatzsteuerausweis. Die App ergänzt den Sonderregelungshinweis, berechnet die interne Margensteuer aber nicht.",
  },
  "margin-art": {
    meaning: "Für die betreffenden Kunstgegenstände wendest du die Differenzbesteuerung an.",
    example:
      "Wähle das nur, wenn du geklärt hast, dass es für deinen konkreten Handel gilt – nicht pauschal für jede künstlerische Tätigkeit.",
    effect:
      "Die App ergänzt den Sonderregelungshinweis für Kunstgegenstände, ohne gesonderten Umsatzsteuerausweis. Die interne Margensteuer berechnet sie nicht.",
  },
  "margin-collectibles": {
    meaning: "Für Antiquitäten oder Sammlungsstücke wendest du die Differenzbesteuerung an.",
    example:
      "Entscheidend sind die gesetzlichen Voraussetzungen des konkreten Umsatzes, nicht nur die Bezeichnung des Artikels.",
    effect:
      "Die App ergänzt den passenden Sonderregelungshinweis, ohne gesonderten Umsatzsteuerausweis. Die interne Margensteuer berechnet sie nicht.",
  },
};

export const setupPaymentGuidance: Record<string, string> = {
  "58": "Deine Kunden überweisen per SEPA auf deine IBAN. Dafür hinterlegst du das Konto, auf dem das Geld eingehen soll.",
  "30": "Für Überweisungen außerhalb des SEPA-Verfahrens. Stimme die nötigen Bankangaben mit deinem Kunden ab.",
  "10": "Du bekommst das Geld bar. Eine Bankverbindung brauchst du dafür nicht.",
  "97": "Du vereinbarst die Zahlungsweise individuell. Trage die konkreten Bedingungen auf der jeweiligen Rechnung ein.",
};
