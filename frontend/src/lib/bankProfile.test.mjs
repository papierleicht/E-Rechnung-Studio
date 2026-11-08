import test from "node:test";
import assert from "node:assert/strict";
import { bankFromCompanyProfile } from "./bankProfile.ts";

test("prepares the FinTS connection from the primary company account", () => {
  assert.deepEqual(
    bankFromCompanyProfile({
      iban: "DE89 3704 0044 0532 0130 00",
      bankName: "Meine Geschäftsbank",
    }),
    {
      iban: "DE89370400440532013000",
      bankCode: "37040044",
      label: "Meine Geschäftsbank",
    },
  );
});

test("uses the bank directory name and rejects an invalid IBAN for FinTS defaults", () => {
  assert.equal(
    bankFromCompanyProfile({ iban: "DE89370400440532013000", bankName: "" }).label,
    "Commerzbank",
  );
  assert.deepEqual(bankFromCompanyProfile({ iban: "DE00", bankName: "Profilbank" }), {
    iban: "DE00",
    bankCode: "",
    label: "Profilbank",
  });
});
