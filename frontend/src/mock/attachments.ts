import type { ReceivedAttachment } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";

const year = new Date().getFullYear();

/** Sample supporting documents (BG-24) of the demo receipts, by receipt ID. */
export const receivedAttachments: Record<string, ReceivedAttachment[]> = {
  // The final invoice names the down payment invoice and brings the proof of work.
  "rcv-planbuero": [
    {
      index: 0,
      kind: "file",
      name: "Stundennachweis Planungsphase",
      reference: "SN-" + year + "-0042",
      filename: "Stundennachweis-Planungsphase.pdf",
      mimeType: "application/pdf",
      size: 48_213,
      link: "",
      openable: true,
    },
    {
      index: 1,
      kind: "reference",
      name: "Anzahlungsrechnung",
      reference: "PB-" + year + "-0031",
      filename: "",
      mimeType: "",
      size: 0,
      link: "",
      openable: false,
    },
    {
      index: 2,
      kind: "link",
      name: "Leistungsverzeichnis",
      reference: "LV-12",
      filename: "",
      mimeType: "",
      size: 0,
      link: "https://planbuero-winter.example/projekte/lv-12",
      openable: false,
    },
    {
      index: 3,
      kind: "file",
      name: "Aufmaß",
      reference: "AM-3",
      filename: "aufmass.xlsx",
      mimeType: "application/zip",
      size: 20_480,
      link: "",
      openable: false,
    },
  ],
};
