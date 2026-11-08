import test from "node:test";
import assert from "node:assert/strict";
import {
  bankPresets,
  bankPresetByID,
  bankPresetForCode,
  bankPresetForIBAN,
  presetBankCode,
  presetMatchesEndpoint,
  bankRoutingMismatch,
} from "./bankPresets.ts";
import { connectionDefaultsFromProfile } from "./bankProfile.ts";

function iban(code) {
  const bban = code + "0000001234";
  const check = 98n - (BigInt(bban + "131400") % 97n);
  return `DE${String(check).padStart(2, "0")}${bban}`;
}

test("profile defaults fill the Consors endpoint and TAN without deriving personal credentials", () => {
  assert.deepEqual(
    connectionDefaultsFromProfile({ iban: iban("76030080"), bankName: "Mein Konto" }),
    {
      label: "Mein Konto",
      bankCode: "76030080",
      endpoint: "https://brokerage-hbci.consorsbank.de/hbci",
      securityFunction: "900",
    },
  );
  assert.equal(bankPresetForIBAN(iban("76030080"))?.id, "consors");
  assert.equal(bankPresetForIBAN("DE00" + iban("76030080").slice(4)), undefined);
  assert.equal(
    connectionDefaultsFromProfile({ iban: "DE00", bankName: "Consorsbank" }).endpoint,
    "",
  );
});

test("comdirect branches never inherit the Commerzbank endpoint", () => {
  for (const suffix of ["11", "33", "44", "55", "66", "77", "88", "99"])
    assert.equal(bankPresetForCode("200411" + suffix)?.id, "comdirect");
  assert.equal(bankPresetForCode("37040044")?.id, "commerzbank");
  assert.equal(bankPresetForCode("10040000")?.id, "commerzbank");
  assert.equal(bankPresetForCode("60080000")?.id, "commerzbank");
});

test("Postbank, Deutsche Bank and DSL remain distinct", () => {
  assert.equal(bankPresetForCode("50010060")?.id, "postbank");
  assert.equal(bankPresetForCode("10070024")?.id, "deutsche-bank");
  assert.equal(bankPresetForCode("10070324"), undefined);
  assert.equal(bankPresetForCode("38010700"), undefined);
  assert.equal(
    connectionDefaultsFromProfile({ iban: iban("50010060"), bankName: "" }).bankCode,
    "50010060",
  );
  assert.equal(presetBankCode(bankPresetByID("postbank")), "");
});

test("regional institutions require an exact mapping", () => {
  assert.equal(bankPresetForCode("20050550")?.id, "haspa");
  assert.equal(bankPresetForCode("10050000")?.id, "berliner-sparkasse");
  assert.equal(bankPresetForCode("37050299")?.id, "kreissparkasse-koeln");
  assert.equal(bankPresetForCode("37050198"), undefined); // Sparkasse KölnBonn
  assert.equal(
    bankPresetForCode("43060967")?.endpoint,
    "https://fints1.atruvia.de/cgi-bin/hbciservlet",
  );
  assert.equal(
    bankPresetForCode("10090000")?.endpoint,
    "https://fints2.atruvia.de/cgi-bin/hbciservlet",
  );
  assert.equal(bankPresetForCode("12345678"), undefined);
  assert.equal(bankPresetForCode(""), undefined);
});

test("HVB uses the central login BLZ while retaining the profile account", () => {
  const accountIBAN = iban("20030000");
  assert.equal(bankPresetForIBAN(accountIBAN)?.id, "hvb");
  const defaults = connectionDefaultsFromProfile({ iban: accountIBAN, bankName: "HVB Hamburg" });
  assert.equal(defaults.bankCode, "70020270");
  assert.equal(defaults.label, "HVB Hamburg");
});

test("unverified banks expose no invented endpoint or credentials", () => {
  for (const id of ["n26", "c24", "santander", "trade-republic"])
    assert.equal(bankPresetByID(id)?.endpoint, "");
  assert.equal(bankPresetForCode("50024024")?.id, "c24");
  assert.equal(bankPresetForCode("31010833"), undefined); // Current directory: Openbank Deutschland, not a MySantander preset.
  assert.equal(
    connectionDefaultsFromProfile({ iban: iban("50024024"), bankName: "C24" }).endpoint,
    "",
  );
});

test("custom saved endpoints and the comdirect trailing slash are preserved", () => {
  const bank = bankPresetByID("comdirect");
  assert.ok(presetMatchesEndpoint(bank, "https://fints.comdirect.de/fints/"));
  assert.equal(presetMatchesEndpoint(bank, "https://fints.comdirect.de/fints"), false);
  assert.equal(presetMatchesEndpoint(bank, "https://custom.example/fints"), false);
  assert.equal(presetBankCode(bank), "");
  assert.equal(presetBankCode(bank, "20041155"), "20041155");
});

test("routing validation accepts shared Atruvia servers and manual unknown banks", () => {
  for (const id of [
    "berliner-volksbank",
    "frankfurter-volksbank",
    "sparda-hamburg",
    "sparda-baden-wuerttemberg",
  ]) {
    const bank = bankPresetByID(id);
    assert.equal(bankRoutingMismatch(bank.bankCodes[0], bank.endpoint, id), false);
  }
  assert.equal(bankRoutingMismatch("12030000", bankPresetByID("consors").endpoint), true);
  assert.equal(bankRoutingMismatch("37050198", bankPresetByID("haspa").endpoint, "haspa"), true);
  assert.equal(bankRoutingMismatch("12345678", "https://manual.example/fints"), false);
  assert.equal(bankRoutingMismatch("12030000", "https://custom.example/fints"), false);
});

test("presets use unique identifiers and HTTPS endpoints", () => {
  assert.equal(new Set(bankPresets.map((bank) => bank.id)).size, bankPresets.length);
  for (const bank of bankPresets) {
    if (bank.endpoint) {
      const url = new URL(bank.endpoint);
      assert.equal(url.protocol, "https:");
      assert.equal(url.username + url.password, "");
    }
  }
});

test("every bank with a FinTS address explains its login in a few sentences", () => {
  for (const bank of bankPresets.filter((item) => item.endpoint)) {
    assert.ok(bank.loginHelp?.question.endsWith("?"), bank.id);
    assert.ok(bank.loginHelp.answer.length <= 400, `${bank.id}: ${bank.loginHelp.answer.length}`);
  }
});
