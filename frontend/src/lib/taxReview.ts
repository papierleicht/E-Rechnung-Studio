import type { InvoiceDraft } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import { taxChoices, taxReasons } from "./tax.ts";

export function taxReviewItems(draft: InvoiceDraft) {
  return (draft.lines ?? []).flatMap((line, index) => {
    const kind =
      draft.taxTreatment === "mixed" ? line.taxTreatment || "standard" : draft.taxTreatment;
    if (kind !== "standard") return [];
    const raw = line.taxRate.trim().replace(",", ".");
    if (!/^[+]?\d+(\.\d+)?$/.test(raw)) return [];
    const rate = Number(raw);
    if (rate === 0)
      return [
        {
          index,
          rate,
          message:
            "Du hast 0 % Umsatzsteuer eingetragen. Dafür kann es verschiedene Gründe geben – wähle hier den passenden für diese Position.",
        },
      ];
    if ([7, 19].includes(rate)) return [];
    const message = [5.5, 7.8].includes(rate)
      ? "Dieser Steuersatz ist für bestimmte Betriebe in der Land- und Forstwirtschaft vorgesehen (§ 24 UStG). Prüfe, ob er für deine Leistung und deren Datum gilt. Der passende Durchschnittssatz muss auf der Rechnung stehen."
      : [5, 16].includes(rate)
        ? "5 % und 16 % galten allgemein von Juli bis Dezember 2020. Geht es um eine Leistung aus dieser Zeit? Bei einer Korrektur solltest du auch die ursprüngliche Rechnung prüfen."
        : "Dieser Steuersatz ist in Deutschland unüblich. Prüfe, ob er zu deiner Leistung passt. Bei älteren Leistungen oder Leistungen im Ausland können andere Steuersätze gelten.";
    return [{ index, rate, message }];
  });
}

export const taxQuestions = [
  {
    id: "small-business",
    question: "Ich rechne als Kleinunternehmer ab",
    explanation:
      "Du nutzt die Kleinunternehmerregelung und weist deshalb keine Umsatzsteuer aus. Prüfe, ob die Regelung für dich gilt und ob du nicht auf sie verzichtet hast.",
    source: "https://www.gesetze-im-internet.de/ustg_1980/__19.html",
  },
  {
    id: "zero-rated",
    question: "Ich liefere oder installiere eine Solaranlage",
    explanation:
      "Für bestimmte Solaranlagen, wichtige Bauteile, Speicher und deren Installation sind 0 % gesetzlich vorgesehen. Prüfe die Voraussetzungen für deine Anlage. Andere Arbeiten rund um die Anlage und der Verkauf von Strom sind nicht automatisch begünstigt.",
    source:
      "https://www.bundesfinanzministerium.de/Content/DE/FAQ/foerderung-photovoltaikanlagen.html",
  },
  {
    id: "exempt",
    question: "Meine Leistung ist von der Umsatzsteuer befreit",
    explanation:
      "Das kann zum Beispiel für bestimmte Heilbehandlungen, Unterricht oder Vermietungen gelten. Nicht jede Leistung in diesen Bereichen ist steuerfrei. Nenne die gesetzliche Regelung, die für deine Leistung gilt.",
    source: "https://www.gesetze-im-internet.de/ustg_1980/__4.html",
  },
  {
    id: "standard",
    question: "Ich muss Umsatzsteuer berechnen",
    explanation:
      "Meist gelten 19 % oder 7 %. Trage den Steuersatz ein, der zu deiner Leistung passt. Der Rechnungsbetrag wird danach neu berechnet.",
    source: "https://docs.peppol.eu/poacc/billing/3.0/rules/ubl-tc434/BR-S-05/",
  },
  {
    id: "reverse-charge",
    question: "Mein Kunde muss die Umsatzsteuer abführen",
    explanation:
      "In bestimmten Fällen führt dein Kunde die Steuer an das Finanzamt ab. Das nennt sich „Reverse Charge“. Es kommt etwa bei bestimmten Bauleistungen oder Dienstleistungen für Unternehmen im Ausland vor. Ein Kunde im Ausland allein reicht dafür nicht aus.",
    source: "https://www.gesetze-im-internet.de/ustg_1980/__13b.html",
  },
  {
    id: "intra-community",
    question: "Ich liefere Waren in ein anderes EU-Land",
    explanation:
      "Eine Warenlieferung in ein anderes EU-Land kann steuerfrei sein. Prüfe die Voraussetzungen, die nötige Umsatzsteuer-ID deines Kunden und die Liefernachweise. Für Dienstleistungen gelten andere Regeln.",
    source: "https://www.gesetze-im-internet.de/ustg_1980/__6a.html",
  },
  {
    id: "export",
    question: "Ich liefere Waren in ein Land außerhalb der EU",
    explanation:
      "Eine solche Warenlieferung kann steuerfrei sein. Du brauchst einen Nachweis, dass die Ware ausgeführt wurde. Die Adresse deines Kunden genügt nicht. Diese Regel gilt nicht für Dienstleistungen.",
    source: "https://www.gesetze-im-internet.de/ustg_1980/__6.html",
  },
  {
    id: "out-of-scope",
    question: "Die Zahlung fällt nicht unter die Umsatzsteuer",
    explanation:
      "Das kann zum Beispiel bei echtem Schadensersatz ohne Gegenleistung zutreffen. Bei Leistungen im Ausland musst du zuerst prüfen, ob dein Kunde oder du dort Steuer zahlen musst. Solche Zahlungen musst du hier getrennt von anderen Steuerfällen abrechnen.",
    source: "https://www.gesetze-im-internet.de/ustg_1980/__3a.html",
  },
  {
    id: "margin-goods",
    question: "Ich nutze die Sonderregelung für gebrauchte Waren",
    explanation:
      "Bei der sogenannten Differenzbesteuerung weist du keine Umsatzsteuer gesondert aus. Die Steuer auf deinen Gewinnanteil berechnet die App nicht. Für Kunst oder Sammlerstücke wähle den passenden Fall in den Rechnungseinstellungen.",
    source: "https://www.gesetze-im-internet.de/ustg_1980/__25a.html",
  },
] as const;

/** Apply only an explicit decision; preserve the effective treatment of other positions. */
export function applyTaxDecision(
  draft: InvoiceDraft,
  index: number,
  treatment: string,
  reason = "",
  rate = "0",
): InvoiceDraft {
  const choice = taxChoices.find((item) => item.id === treatment);
  if (!choice || !draft.lines?.[index] || draft.status !== "draft")
    throw new Error("Der Steuerfall lässt sich nicht übernehmen.");
  if (draft.adjustments?.length)
    throw new Error(
      "Prüfe Steuerfall und Rabatte / Zuschläge gemeinsam in den Rechnungseinstellungen.",
    );
  if (treatment === "exempt" && !reason.trim())
    throw new Error("Gib den konkreten Befreiungsgrund an.");
  if (
    treatment === "standard" &&
    (!/^\d+(\.\d+)?$/.test(rate.trim().replace(",", ".")) || Number(rate.replace(",", ".")) <= 0)
  )
    throw new Error("Gib einen Steuersatz größer als 0 ein, zum Beispiel 19 oder 7.");
  const lines = draft.lines.map((line, i) => {
    const originalKind =
      draft.taxTreatment === "mixed" ? line.taxTreatment || "standard" : draft.taxTreatment;
    const originalReason =
      draft.taxTreatment === "mixed" ? line.exemptionReason : draft.taxExemptionReason;
    return i === index
      ? {
          ...line,
          taxTreatment: treatment,
          taxCategory: choice.category,
          taxRate: treatment === "standard" ? rate.trim() : "0",
          exemptionReason: reason.trim() || taxReasons[treatment] || "",
          exemptionCode: "",
        }
      : {
          ...line,
          taxTreatment: originalKind,
          exemptionReason: originalReason,
          exemptionCode: draft.taxTreatment === "mixed" ? line.exemptionCode : "",
        };
  });
  const outside = lines.some(
    (line) =>
      line.taxTreatment === "out-of-scope" ||
      (line.taxTreatment === "standard" && line.taxCategory === "O"),
  );
  if (
    outside &&
    lines.some(
      (line) =>
        line.taxTreatment !== "out-of-scope" &&
        !(line.taxTreatment === "standard" && line.taxCategory === "O"),
    )
  )
    throw new Error(
      "Erfasse diese Zahlung in einer eigenen Rechnung. Sie lässt sich nicht mit anderen Steuerfällen kombinieren.",
    );
  return { ...draft, lines, taxTreatment: "mixed", taxExemptionReason: "", taxSource: "manual" };
}

const otherEUCountries = new Set(
  "AT BE BG CY CZ DK EE ES FI FR GR HR HU IE IT LT LU LV MT NL PL PT RO SE SI SK".split(" "),
);

/** A customer abroad with a VAT ID: worth a look, whether it applies stays the user's call. */
export function reverseChargeHint(draft: InvoiceDraft): boolean {
  return (
    draft.status === "draft" &&
    draft.taxTreatment === "standard" &&
    otherEUCountries.has((draft.customer.address.country ?? "").trim().toUpperCase()) &&
    Boolean(draft.customer.vatId?.trim())
  );
}
