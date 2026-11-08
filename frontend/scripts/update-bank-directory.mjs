import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const downloadPage =
  "https://www.bundesbank.de/de/aufgaben/unbarer-zahlungsverkehr/serviceangebot/bankleitzahlen/download-bankleitzahlen-602592";

function isoDate(germanDate) {
  const [day, month, year] = germanDate.split(".");
  return `${year}-${month}-${day}`;
}

async function downloadCurrentDirectory() {
  const pageResponse = await fetch(downloadPage);
  if (!pageResponse.ok) throw new Error(`Bundesbank download page: HTTP ${pageResponse.status}`);
  const page = await pageResponse.text();
  const match = page.match(
    /href="([^"]*blz-aktuell-csv-data\.csv)"[\s\S]{0,600}?gültig vom (\d{2}\.\d{2}\.\d{4}) bis (\d{2}\.\d{2}\.\d{4})/,
  );
  if (!match) throw new Error("Current Bundesbank CSV link or validity period not found");
  const csvURL = new URL(match[1].replaceAll("&amp;", "&"), downloadPage);
  if (csvURL.hostname !== "www.bundesbank.de") throw new Error("Unexpected download host");
  const csvResponse = await fetch(csvURL);
  if (!csvResponse.ok) throw new Error(`Bundesbank CSV: HTTP ${csvResponse.status}`);
  return {
    source: new Uint8Array(await csvResponse.arrayBuffer()),
    validFrom: isoDate(match[2]),
    validUntil: isoDate(match[3]),
  };
}

function parseRow(line) {
  const fields = [];
  let field = "";
  let quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    const character = line[index];
    if (character === '"') {
      if (quoted && line[index + 1] === '"') {
        field += '"';
        index += 1;
      } else {
        quoted = !quoted;
      }
    } else if (character === ";" && !quoted) {
      fields.push(field);
      field = "";
    } else {
      field += character;
    }
  }
  fields.push(field);
  return fields;
}

const args = process.argv.slice(2);
const downloads = args[0] === "--download";
const output = args[1] ?? "src/lib/bankDirectory.generated.ts";
const downloaded = downloads ? await downloadCurrentDirectory() : undefined;
const sourceBytes = downloaded?.source ?? readFileSync(resolve(args[0] ?? "blz-aktuell.csv"));
const validFrom = downloaded?.validFrom ?? args[2] ?? "unknown";
const validUntil = downloaded?.validUntil ?? args[3] ?? "unknown";
const source = new TextDecoder("windows-1252").decode(sourceBytes);
const records = source
  .trim()
  .split(/\r?\n/)
  .slice(1)
  .map(parseRow)
  .filter((row) => row[1] === "1" && row[10] !== "D")
  .map((row) => [row[0], [row[2], row[7]]])
  .sort(([left], [right]) => left.localeCompare(right));

const uniqueBankCodes = new Set(records.map(([bankCode]) => bankCode));
if (records.length < 3_000 || uniqueBankCodes.size !== records.length) {
  throw new Error(
    `Implausible Bundesbank directory: ${records.length} records, ${uniqueBankCodes.size} unique bank codes`,
  );
}

const generated = `// Generated from the Deutsche Bundesbank bank sort code directory:
// https://www.bundesbank.de/de/aufgaben/unbarer-zahlungsverkehr/serviceangebot/bankleitzahlen/download-bankleitzahlen-602592
// Valid from ${validFrom} through ${validUntil}. Do not edit manually.
export const germanBanks: Readonly<Record<string, readonly [name: string, bic: string]>> = Object.fromEntries(${JSON.stringify(
  records,
)}) as Record<string, readonly [string, string]>;
`;

writeFileSync(resolve(output), generated, "utf8");
console.log(`Wrote ${records.length} bank records to ${output}`);
