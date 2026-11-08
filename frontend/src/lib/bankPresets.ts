import { germanBanks } from "./bankDirectory.generated.ts";
import { lookupGermanBank, normalizeIBAN } from "./bankLookup.ts";

export interface BankPreset {
  id: string;
  name: string;
  bankCodes: string[];
  endpoint: string;
  loginLabel: string;
  hint: string;
  /** Explains a login part that many customers do not know by name. */
  loginHelp?: { question: string; answer: string };
  source: string;
  defaultTAN?: string;
  customerIDFromLogin?: boolean;
  loginBankCode?: string;
  branchBankCodes?: boolean;
}

// Login help shared by the institutes of one banking group, as each group
// documents it (checked 2026-11-02).
const sparkasseHelp = {
  question: "Was ist der Anmeldename oder die Legitimations-ID?",
  answer:
    "Beides sind Benutzernamen für dein Online-Banking bei der Sparkasse; du brauchst nur einen davon. Du findest sie im Brief zu deinem Online-Banking. Dazu kommt deine Online-Banking-PIN. Hast du noch die Start-PIN aus dem Brief, melde dich zuerst einmal im Browser an und ändere sie. Freigaben laufen über S-pushTAN oder chipTAN.",
};
const vrHelp = {
  question: "Was ist der VR-NetKey?",
  answer:
    "Der VR-NetKey ist dein Benutzername fürs Online-Banking. Er hat 5 bis 11 Ziffern und kam per Post von deiner Bank; im Online-Banking steht er unter „Datenschutz & Sicherheit“. Statt des VR-NetKeys kannst du deinen selbst gewählten Alias eingeben. Eine extra Freischaltung brauchst du nicht, Freigaben laufen z. B. über VR SecureGo plus.",
};

// Individually researched configuration, checked 2026-09-22 with anonymous
// FinTS 300 probes; the evidence stays in the maintainers' notes. No
// account-access promise.
export const bankPresets: BankPreset[] = [
  {
    id: "consors",
    name: "Consorsbank",
    bankCodes: ["76030080"],
    endpoint: "https://brokerage-hbci.consorsbank.de/hbci",
    loginLabel: "Kontonummer + Berechtigtennummer",
    hint: "Gib deine Kontonummer mit der dreistelligen Berechtigtennummer ein. Für Verfahren 900 erzeugst du die neunstellige TAN in der Consorsbank-App.",
    loginHelp: {
      question: "Was ist die Berechtigtennummer?",
      answer:
        "Sie ist dreistellig und zeigt, wer auf das Konto zugreift: Der erste Kontoinhaber hat 001, weitere Berechtigte haben 002, 003 und so weiter. Du findest sie in deinen Unterlagen zur Kontoeröffnung. Gib deine Kontonummer (nicht die IBAN) und direkt dahinter die Berechtigtennummer ein, ohne Leerzeichen: Aus 123456789 und 001 wird 123456789001.",
    },
    defaultTAN: "900",
    source:
      "https://www.consorsbank.de/web/Wissen/FAQ/kontokarten/HBCI-Schnittstelle-Konto-und-Zahlungsverkehrsprogramm",
  },
  {
    id: "ing",
    name: "ING",
    bankCodes: ["50010517"],
    endpoint: "https://fints.ing.de/fints/",
    loginLabel: "Benutzername oder Zugangsnummer",
    hint: "Nur lesender Abruf; Girokonto-Umsätze bis 90 Tage. Melde dich mindestens alle 90 Tage im ING-Banking an und gib den Zugriff frei. Diese Verbindung ist für die klassischen ING-Konten dokumentiert.",
    loginHelp: {
      question: "Was trage ich bei der ING ein?",
      answer:
        "Hast du dir bei der ING einen Benutzernamen gegeben, nimm ihn und dein Passwort. Sonst nimm deine Zugangsnummer (die letzten 10 Stellen deiner IBAN) und deine Internetbanking-PIN. Eine TAN fragt die ING hier nicht ab; den Zugriff gibst du beim Anmelden im ING-Banking frei.",
    },
    source: "https://www.ing.de/hilfe/log-in/fints/",
  },
  {
    id: "dkb",
    name: "DKB",
    bankCodes: ["12030000"],
    endpoint: "https://fints.dkb.de/fints",
    loginLabel: "Anmeldename",
    hint: "Anmeldename und Passwort aus dem DKB-Banking. Die Freigabe läuft je nach Zugang über die DKB-App oder chipTAN.",
    loginHelp: {
      question: "Wo finde ich meinen DKB-Anmeldenamen?",
      answer:
        "Hast du ihn nie geändert, ist er meist deine Kontonummer (die letzten 10 Stellen der IBAN) mit Zusatz, z. B. 1234567890_p. Er stand im Brief zur Kontoeröffnung; vergessen, lässt du ihn dir auf dkb.de per E-Mail schicken. Dazu kommt dein Banking-Passwort. Nutzt du chipTAN, hast du dafür einen eigenen Anmeldenamen.",
    },
    source: "https://www.dkb.de/geschaeftskunden/electronic-banking",
  },
  {
    id: "comdirect",
    name: "comdirect",
    bankCodes: ["20041111", "20041133", "20041144", "20041155"],
    endpoint: "https://fints.comdirect.de/fints/",
    loginLabel: "Zugangsnummer",
    hint: "Gib deine Zugangsnummer und PIN aus dem comdirect-Banking ein. Die Bankleitzahl muss zu deinem Konto passen.",
    loginHelp: {
      question: "Wo finde ich meine Zugangsnummer?",
      answer:
        "Es ist die 8-stellige Nummer, mit der du dich auch im comdirect-Online-Banking anmeldest; du hast sie nach der Kontoeröffnung von comdirect bekommen. Dazu kommt deine 6-stellige PIN. Die Freigabe läuft über photoTAN oder mobileTAN.",
    },
    source: "https://www.comdirect.de/cms/sicherheit-software-banking-fints.html",
  },
  {
    id: "deutsche-bank",
    name: "Deutsche Bank",
    bankCodes: ["50070010", "76070024"],
    endpoint: "https://fints.deutsche-bank.de/",
    loginLabel: "Deutsche Bank ID / Filial- und Kundennummer",
    hint: "Melde dich nach der Banking-Umstellung zuerst im Web an. Die Kunden-ID wird bei Bedarf aus deiner Benutzerkennung übernommen.",
    loginHelp: {
      question: "Was ist meine Deutsche Bank ID?",
      answer:
        "Hast du keine eigene ID vergeben, ist es deine Filial- und Kontonummer ohne Leerzeichen und ohne Unterkonto, z. B. 7001234567. Deine IDs siehst du im Online-Banking unter Profil > ID Verwaltung. Dazu kommt dein Online-Banking-Passwort. Freigaben laufen über photoTAN oder BestSign.",
    },
    customerIDFromLogin: true,
    branchBankCodes: true,
    source: "https://www.deutsche-bank.de/pk/konto-und-karte/services/hbci-fints.html",
  },
  {
    id: "postbank",
    name: "Postbank",
    bankCodes: ["10010010"],
    endpoint: "https://hbci.postbank.de/banking/hbci.do",
    loginLabel: "Postbank ID",
    hint: "Gib deine Postbank ID ohne früheren #Profilnamen ein. Die Kunden-ID wird aus der Postbank ID übernommen; die Freigabe läuft über BestSign.",
    loginHelp: {
      question: "Was trage ich als Postbank ID ein?",
      answer:
        "Deine Postbank ID fürs Online-Banking und dein Passwort. Steht in der ID ein # mit Profilnamen, lass das # und alles danach weg. Hast du mehrere Postbank IDs, richte jede einzeln ein. Eine Postbank ID nur für das Service-Portal reicht nicht. Freigaben laufen über BestSign, als App oder Gerät.",
    },
    customerIDFromLogin: true,
    branchBankCodes: true,
    source: "https://www.postbank.de/geschaeftskunden/services/fints.html",
  },
  {
    id: "commerzbank",
    name: "Commerzbank",
    bankCodes: ["37040044"],
    endpoint: "https://fints.commerzbank.de/fints",
    loginLabel: "Teilnehmernummer",
    branchBankCodes: true,
    hint: "FinTS/HBCI muss für deinen Zugang freigeschaltet sein. Nimm die BLZ deines Kontos und die dafür vorgesehenen PIN-/TAN-Zugangsdaten.",
    loginHelp: {
      question: "Wo finde ich meine Teilnehmernummer?",
      answer:
        "In der Commerzbank-App unter Profil, im Online-Banking mit Klick auf deinen Namen oder im Brief zur Kontoeröffnung. Dazu kommt deine Online-Banking-PIN, freigeben tust du mit photoTAN. Den Zugang für Finanzsoftware (HBCI mit PIN/TAN) beantragst du vorher bei der Commerzbank.",
    },
    source: "https://www.commerzbank.de/service/netzwerkfehler-im-hbci-was-kann-ich-tun/",
  },
  {
    id: "hvb",
    name: "HypoVereinsbank",
    bankCodes: ["70020270"],
    endpoint: "https://hbci-01.hypovereinsbank.de/bank/hbci",
    loginLabel: "Direct Banking Nummer",
    loginBankCode: "70020270",
    hint: "Gib deine Direct-Banking-Nummer und PIN ein. Für FinTS gilt die zentrale Anmelde-BLZ 70020270, auch wenn deine IBAN eine andere Filial-BLZ enthält.",
    loginHelp: {
      question: "Wo finde ich meine Direct Banking Nummer?",
      answer:
        "Sie ist 10-stellig, enthält dein Geburtsdatum und kam bei der Registrierung fürs Online-Banking. Dazu kommt dein Online-Banking-Passwort. Die TAN gibst du in der HVB Mobile Banking App (appTAN) oder mit photoTAN frei. Kommt nur der Kontostand, aber keine Umsätze, ist FinTS für deinen Zugang noch nicht freigeschaltet.",
    },
    source: "https://www.hypovereinsbank.de/hvb/services/online-banking/tan-verfahren",
  },
  {
    id: "targobank",
    name: "TARGOBANK",
    bankCodes: ["30020900"],
    endpoint: "https://fints.targobank.de/travicretail_fints30pintanbroker/FinTs30PinTanHttpGate",
    loginLabel: "Benutzername",
    hint: "Gib Benutzernamen und Passwort aus deinem Online-Banking ein. Ob der Abruf funktioniert, hängt von deinem freigeschalteten Zugang ab.",
    loginHelp: {
      question: "Wo finde ich meinen Benutzernamen?",
      answer:
        "Die TARGOBANK vergibt ihn als 14-stellige Ziffernfolge. Kennst du ihn nicht, hilft „Zugangsdaten vergessen“ auf der Login-Seite. Dazu kommt dein Online-Banking-Passwort. Du musst für easyTAN (TARGOBANK-App) oder photoTAN registriert sein. Spar-, Festgeldkonten und Depots lassen sich nicht abrufen.",
    },
    source: "https://www.targobank.de/de/service/multi-banking/index.html",
  },
  {
    id: "haspa",
    name: "Hamburger Sparkasse",
    bankCodes: ["20050550"],
    endpoint: "https://banking-hh7.s-fints-pt-hh.de/fints30",
    loginLabel: "Anmeldename oder Legitimations-ID",
    hint: "Gib deinen Online-Banking-Anmeldenamen oder deine Legitimations-ID und deine PIN ein.",
    loginHelp: sparkasseHelp,
    source:
      "https://www.haspa.de/content/dam/myif/haspa/work/dokumente/pdf/allgemein/anleitung-zur-konfiguration-der-finanz-software.pdf?n=true",
  },
  {
    id: "berliner-sparkasse",
    name: "Berliner Sparkasse",
    bankCodes: ["10050000"],
    endpoint: "https://banking-be3.s-fints-pt-be.de/fints30",
    loginLabel: "Anmeldename oder Legitimations-ID",
    hint: "Gib deinen Online-Banking-Anmeldenamen oder deine Legitimations-ID und deine PIN ein.",
    loginHelp: sparkasseHelp,
    source:
      "https://www.berliner-sparkasse.de/content/dam/myif/berliner-sk/work/dokumente/pdf/allgemein/faq-und-details-zu-chiptan-usb.pdf",
  },
  {
    id: "kreissparkasse-koeln",
    name: "Kreissparkasse Köln",
    bankCodes: ["37050299"],
    endpoint: "https://banking-rl6.s-fints-pt-rl.de/fints30",
    loginLabel: "Anmeldename oder Legitimations-ID",
    hint: "Diese Verbindung gilt für die Kreissparkasse Köln. Die Sparkasse KölnBonn ist ein anderes Institut.",
    loginHelp: sparkasseHelp,
    source:
      "https://www.ksk-koeln.de/fi/home/produkte/banking-und-software/weitere-services/formatabkuendigung.html",
  },
  {
    id: "gls",
    name: "GLS Gemeinschaftsbank",
    bankCodes: ["43060967"],
    endpoint: "https://fints1.atruvia.de/cgi-bin/hbciservlet",
    loginLabel: "VR-NetKey oder Alias",
    hint: "Gib deinen VR-NetKey oder Alias und deine Online-Banking-PIN ein.",
    loginHelp: {
      question: "Was ist der VR-NetKey?",
      answer:
        "Dein Benutzername fürs Online-Banking. Du findest ihn im Brief der GLS Bank oder im Online-Banking: oben rechts auf deinen Namen klicken, dann „Datenschutz & Sicherheit“. Statt des VR-NetKeys kannst du deinen Alias eingeben. Als PIN nimmst du das Passwort, das du im Online-Banking festgelegt hast. Eine extra Freischaltung brauchst du nicht.",
    },
    source:
      "https://www.gls.de/konten-karten/zahlungsverkehr/sepa-zahlungsverkehr/daten-zum-onlinebanking/",
  },
  {
    id: "berliner-volksbank",
    name: "Berliner Volksbank",
    bankCodes: ["10090000"],
    endpoint: "https://fints2.atruvia.de/cgi-bin/hbciservlet",
    loginLabel: "VR-NetKey oder Alias",
    hint: "Gib deinen VR-NetKey oder Alias und deine Online-Banking-PIN ein.",
    loginHelp: vrHelp,
    source: "https://www.berliner-volksbank.de/firmenkunden/zahlungsverkehr/fints-und-ebics.html",
  },
  {
    id: "frankfurter-volksbank",
    name: "Frankfurter Volksbank Rhein-Main",
    bankCodes: ["50190000"],
    endpoint: "https://fints2.atruvia.de/cgi-bin/hbciservlet",
    loginLabel: "VR-NetKey oder Alias",
    hint: "Gib deinen VR-NetKey oder Alias und deine Online-Banking-PIN ein. Diese Vorauswahl gilt für BLZ 50190000.",
    loginHelp: vrHelp,
    source: "https://www.frankfurter-volksbank.de/",
  },
  {
    id: "sparda-hamburg",
    name: "Sparda-Bank Hamburg",
    bankCodes: ["20690500"],
    endpoint: "https://fints2.atruvia.de/cgi-bin/hbciservlet",
    loginLabel: "Sparda-NetKey oder Alias",
    hint: "Gib deinen aktuellen Sparda-NetKey oder Alias und deine Online-Banking-PIN ein.",
    loginHelp: {
      question: "Was ist der Sparda-NetKey?",
      answer:
        "Dein Benutzername fürs Online-Banking der Sparda-Bank. Hast du einen Alias angelegt, kannst du stattdessen den Alias eingeben. Dazu kommt deine Online-PIN. Eine extra Freischaltung brauchst du nicht; bestätigen kannst du z. B. mit SecureGo plus oder Sm@rt-TAN.",
    },
    source:
      "https://www.sparda-bank-hamburg.de/service-und-kontakt/banking-und-apps/finanzsoftware.html",
  },
  {
    id: "sparda-baden-wuerttemberg",
    name: "Sparda-Bank Baden-Württemberg",
    bankCodes: ["60090800"],
    endpoint: "https://fints2.atruvia.de/cgi-bin/hbciservlet",
    loginLabel: "Sparda-NetKey oder Alias",
    hint: "Gib deinen aktuellen Sparda-NetKey oder Alias ein. Die Verbindung berücksichtigt die Umstellung von Juli 2025.",
    loginHelp: {
      question: "Was ist der Sparda-NetKey?",
      answer:
        "Seit dem 28. Juli 2025 setzt er sich so zusammen: „Sparda“, deine Kundennummer und dein Geburtsdatum (TTMMJJJJ). Im Online-Banking kannst du dir stattdessen einen Alias anlegen. Dazu kommt deine Online-PIN. Die SpardaSecureApp gibt es nicht mehr; du bestätigst mit SpardaSecureGo+ oder Sm@rtTAN plus.",
    },
    source:
      "https://hub.sparda-bw.de/storyblok/asset/x/451f95e076/english_future-banking-sparda-bank-bw.pdf",
  },
  {
    id: "n26",
    name: "N26",
    bankCodes: [],
    endpoint: "",
    loginLabel: "Benutzerkennung",
    hint: "Für N26 ist in papierleicht noch kein direkter FinTS-Zugang geprüft. N26 bietet Kontoaktivitätsberichte als CSV und PDF an.",
    source:
      "https://support.n26.com/de-de/konto-und-personliche-informationen/kontoauszuge-und-kontobestatigungen/kontoauszug-herunterladen",
  },
  {
    id: "c24",
    name: "C24 Bank",
    bankCodes: ["50024024"],
    endpoint: "",
    loginLabel: "Benutzerkennung",
    hint: "Für C24 ist in papierleicht noch kein direkter FinTS-Zugang geprüft. Im C24-Online-Banking kannst du Umsätze als CSV oder Excel exportieren.",
    source:
      "https://hilfe.c24.de/hc/de/articles/8082169487250-Wie-kann-ich-meine-Transaktionen-exportieren",
  },
  {
    id: "santander",
    name: "Santander",
    bankCodes: [],
    endpoint: "",
    loginLabel: "Benutzerkennung",
    hint: "Für Santander ist in papierleicht noch kein direkter FinTS-Zugang geprüft. Deine Kontoauszüge findest du in der PostBox deiner Bank.",
    source:
      "https://www.santander.de/privatkunden/service-und-information/informationen/faq/online-banking/mysantander-online-banking/",
  },
  {
    id: "trade-republic",
    name: "Trade Republic",
    bankCodes: [],
    endpoint: "",
    loginLabel: "Benutzerkennung",
    hint: "Für Trade Republic ist in papierleicht noch kein direkter FinTS-Zugang geprüft. Eine IBAN der Partnerbank bedeutet nicht automatisch, dass du dort Zugang bekommst.",
    source: "https://support.traderepublic.com/de-de/267",
  },
];

export function bankPresetByID(id: string): BankPreset | undefined {
  return bankPresets.find((bank) => bank.id === id);
}

export function bankPresetForCode(bankCode: string): BankPreset | undefined {
  if (!/^\d{8}$/.test(bankCode)) return undefined;
  const exact = bankPresets.find((bank) => bank.bankCodes.includes(bankCode));
  if (exact) return exact;
  // Resolve branch BLZs only from the bundled Bundesbank record, never a
  // user-entered bank name/BIC or a blanket Sparkasse/Volksbank brand match.
  const entry = germanBanks[bankCode];
  if (!entry) return undefined;
  const [name, bic] = entry;
  if (bic.startsWith("NTSBDEB1") && name.startsWith("N26")) return bankPresetByID("n26");
  if (bic.startsWith("TRBKDEBB")) return bankPresetByID("trade-republic");
  if (name === "Postbank Ndl der Deutsche Bank" && bic.startsWith("PBNKDEFF"))
    return bankPresetByID("postbank");
  if (name === "Commerzbank - GF comdirect" && bic.startsWith("COBADEHD"))
    return bankPresetByID("comdirect");
  if (name === "Deutsche Bank" && bic.startsWith("DEUTDE")) return bankPresetByID("deutsche-bank");
  if (
    name.startsWith("Commerzbank") &&
    (bic.startsWith("COBADE") || bic.startsWith("DRESDE")) &&
    !bic.startsWith("COBADEHD")
  )
    return bankPresetByID("commerzbank");
  if (name === "UniCredit Bank - HypoVereinsbank" && bic.startsWith("HYVEDE"))
    return bankPresetByID("hvb");
  return undefined;
}

export function presetBankCode(preset: BankPreset, profileCode = ""): string {
  if (preset.loginBankCode) return preset.loginBankCode;
  if (bankPresetForCode(profileCode)?.id === preset.id) return profileCode;
  return !preset.branchBankCodes && preset.bankCodes.length === 1 ? preset.bankCodes[0] : "";
}

export function bankPresetForIBAN(iban: string): BankPreset | undefined {
  return lookupGermanBank(iban) ? bankPresetForCode(normalizeIBAN(iban).slice(4, 12)) : undefined;
}

export function presetMatchesEndpoint(preset: BankPreset | undefined, endpoint: string): boolean {
  // Preserve intentional manual URLs. Even a trailing slash can matter.
  return Boolean(preset && preset.endpoint === endpoint);
}

export function bankRoutingMismatch(bankCode: string, endpoint: string, presetID = ""): boolean {
  if (!/^\d{8}$/.test(bankCode)) return false;
  const expected = bankPresetForCode(bankCode);
  const selected = bankPresetByID(presetID);
  if (selected?.endpoint && selected.id !== expected?.id) return true;
  // Atruvia endpoints are shared by multiple institutions. Compare the route,
  // never the first preset that happens to have the same server URL.
  const knownEndpoint = bankPresets.some((bank) => bank.endpoint && bank.endpoint === endpoint);
  return Boolean(knownEndpoint && expected && expected.endpoint !== endpoint);
}
