<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onDestroy } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    Payment,
    PaymentBalance,
    InstallmentPlan,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import PaymentStatus from "./PaymentStatus.svelte";
  import Icon from "../../components/Icon.svelte";
  import { paymentMoney as money } from "../../lib/paymentWorkflow";
  import { dateLabel, decimalInput, plainDecimal } from "../../lib/format";
  import { serviceErrorMessage } from "../../lib/errors";
  import MoreMenu, { type MoreMenuItem } from "../../components/MoreMenu.svelte";
  import Undo2Icon from "@lucide/svelte/icons/undo-2";
  import CalendarClockIcon from "@lucide/svelte/icons/calendar-clock";
  import CalendarXIcon from "@lucide/svelte/icons/calendar-x";
  let {
    invoiceID,
    direction = "outgoing",
    onchanged = async () => {},
    onbusychange = (_busy: boolean) => {},
    oncorrection,
  }: {
    invoiceID: string;
    direction?: string;
    onchanged?: () => Promise<void>;
    onbusychange?: (busy: boolean) => void;
    oncorrection?: () => void;
  } = $props();
  let balance = $state<PaymentBalance | null>(null),
    history = $state<Payment[]>([]),
    plan = $state<InstallmentPlan | null>(null);
  let mode = $state<"" | "payment" | "refund" | "plan" | "cancel">("");
  let amount = $state(""),
    date = $state(new Date().toLocaleDateString("sv-SE")),
    note = $state(""),
    refundKind = $state<"" | "repaid" | "reduction">(""),
    rate = $state(""),
    first = $state(new Date().toLocaleDateString("sv-SE"));
  let proposed = $state<InstallmentPlan | null>(null),
    revokeID = $state(""),
    revokeReason = $state(""),
    error = $state(""),
    notice = $state(""),
    busy = $state(false);
  let loading = $state(true);
  let loadedID = "",
    loadedDirection = "";
  let version = 0,
    disposed = false;
  async function load(id: string) {
    const v = ++version;
    loading = true;
    error = "";
    try {
      const [o, p] = await Promise.all([
        API.Payments(),
        direction === "outgoing" ? API.InstallmentPlan(id) : Promise.resolve(null),
      ]);
      if (disposed || v !== version) return;
      balance = o.invoices?.find((b) => b.id === id && b.direction === direction) ?? null;
      history = o.payments?.filter((p) => p.invoiceId === id && p.direction === direction) ?? [];
      plan = p;
    } catch (e) {
      if (!disposed && v === version)
        error = serviceErrorMessage(
          e,
          "Der Zahlungsstatus konnte nicht geladen werden. Versuche es gleich noch einmal.",
        );
    } finally {
      if (!disposed && v === version) loading = false;
    }
  }
  $effect(() => {
    const id = invoiceID;
    const kind = direction;
    if (id === loadedID && kind === loadedDirection) return;
    loadedID = id;
    loadedDirection = kind;
    mode = "";
    proposed = null;
    revokeID = "";
    notice = "";
    balance = null;
    history = [];
    void load(id);
  });
  onDestroy(() => {
    disposed = true;
    version++;
    onbusychange(false);
  });
  async function perform(action: () => Promise<unknown>, mutation = true) {
    if (busy) return;
    busy = true;
    onbusychange(true);
    error = "";
    notice = "";
    try {
      await action();
      if (mutation) {
        await load(invoiceID);
        await onchanged();
      }
    } catch (e) {
      error = serviceErrorMessage(
        e,
        "Deine Änderung konnte nicht gespeichert werden. Versuche es noch einmal.",
      );
    } finally {
      busy = false;
      if (!disposed) onbusychange(false);
    }
  }
  const askRefundKind = $derived(mode === "refund" && direction === "outgoing" && !balance?.credit);
  function start(next: typeof mode) {
    mode = next;
    note = "";
    proposed = null;
    refundKind = "";
    if (next === "refund")
      amount = Number(balance?.overpaid) > 0 ? decimalInput(balance?.overpaid ?? "") : "";
    else amount = decimalInput(balance?.remaining ?? "");
  }
  async function savePayment() {
    if (!balance || (askRefundKind && !refundKind)) return;
    await perform(async () => {
      const a = plainDecimal(amount);
      await API.RecordPayment({
        invoiceId: invoiceID,
        direction,
        amount: mode === "refund" ? `-${a}` : a,
        reduction: mode === "refund" && refundKind === "reduction",
        currency: balance!.currency,
        date,
        source: "manual",
        reason:
          note.trim() ||
          (mode === "refund" ? "Erstattung manuell erfasst" : "Zahlung manuell erfasst"),
      } as Payment);
      mode = "";
      notice = "Zahlung gespeichert.";
    });
  }
  const open = $derived(!!balance && balance.state !== "bezahlt" && balance.state !== "verrechnet");
  // Recording a payment is the common case; the rest sits in the "Mehr" menu.
  const moreItems = $derived.by((): MoreMenuItem[] => [
    {
      id: "refund",
      icon: Undo2Icon,
      label: "Erstattung erfassen",
      hint:
        direction === "incoming"
          ? "Geld, das du vom Lieferanten zurückbekommen hast."
          : "Geld, das du dem Kunden zurückgezahlt hast.",
      onclick: () => start("refund"),
    },
    ...(direction === "outgoing" && open
      ? [
          {
            id: "plan",
            icon: CalendarClockIcon,
            label: plan?.active ? "Ratenvereinbarung ändern" : "Ratenzahlung vereinbaren",
            hint: "Verteilt den Restbetrag auf monatliche Raten. Die Rechnung bleibt gleich.",
            onclick: () => start("plan"),
          },
        ]
      : []),
    ...(plan?.active
      ? [
          {
            id: "cancel",
            icon: CalendarXIcon,
            danger: true,
            label: "Ratenzahlung aufheben",
            hint: "Danach gilt wieder die Fälligkeit der Rechnung.",
            onclick: () => start("cancel"),
          },
        ]
      : []),
  ]);
</script>

<section class="invoice-payments" aria-label="Zahlungsstatus">
  {#if error}<p role="alert" class="error">{error}</p>{/if}
  {#if notice}<p role="status">{notice}</p>{/if}
  {#if balance}
    <div class="payment-heading">
      <div>
        <h3>{direction === "incoming" ? "Bezahlung" : "Zahlungseingang"}</h3>
        <PaymentStatus {balance} />
        <small
          >{money(balance.paid, balance.currency)}
          {direction === "incoming" ? "bezahlt" : "erhalten"} von {money(
            balance.gross,
            balance.currency,
          )}</small
        >
      </div>
      <div class="payment-actions">
        <button
          class="secondary"
          disabled={busy}
          aria-expanded={mode === "payment"}
          onclick={() => start("payment")}><Icon name="plus" size={16} /> Zahlung erfassen</button
        >
        <MoreMenu items={moreItems} disabled={busy} title="Weitere Zahlungsaktionen" />
      </div>
    </div>
    <p class="hint">Beim Abruf deines Bankkontos gleicht die App Zahlungen automatisch ab.</p>
    {#if balance.awaitingCredit}<div class="awaiting-credit">
        <p>
          {money(balance.awaitingCredit, balance.currency)} hast du als Minderung erstattet. Die App mahnt
          diesen Betrag nicht an. In die Umsatzsteuer geht die Minderung erst mit einer Korrekturgutschrift
          ein.
        </p>
        {#if oncorrection}<button class="secondary" disabled={busy} onclick={oncorrection}
            >Korrekturgutschrift erstellen</button
          >{/if}
      </div>{/if}
    {#if mode === "payment" || mode === "refund"}<form
        onsubmit={(e) => {
          e.preventDefault();
          void savePayment();
        }}
      >
        <h4>{mode === "refund" ? "Erstattung" : "Zahlung"} erfassen</h4>
        {#if askRefundKind}<fieldset class="refund-kind" disabled={busy}>
            <legend>Schuldet der Kunde den Betrag noch?</legend>
            <label
              ><input type="radio" bind:group={refundKind} value="repaid" required />
              <span
                >Ja, ich zahle nur Zuviel oder eine Fehlbuchung zurück<small
                  >Der Rechnungsbetrag bleibt geschuldet. Was danach offen ist, kann die App
                  anmahnen.</small
                ></span
              ></label
            ><label
              ><input type="radio" bind:group={refundKind} value="reduction" required />
              <span
                >Nein, der Preis sinkt, etwa durch Rückgabe oder Storno<small
                  >Die App mahnt den Betrag nicht an. Eine solche Minderung berichtigst du in der
                  Regel mit einer Korrekturgutschrift.</small
                ></span
              ></label
            >
          </fieldset>{/if}
        <div class="fields">
          <label
            >Betrag in {balance.currency}<input
              bind:value={amount}
              inputmode="decimal"
              required
              disabled={busy}
            /></label
          ><label>Datum<input type="date" bind:value={date} required disabled={busy} /></label>
        </div>
        <label>Notiz (optional)<input bind:value={note} maxlength="2000" disabled={busy} /></label
        ><button class="primary" disabled={busy || (askRefundKind && !refundKind)}>Speichern</button
        ><button type="button" class="text" disabled={busy} onclick={() => (mode = "")}
          >Abbrechen</button
        >
      </form>{/if}
    {#if mode === "plan"}<form
        onsubmit={(e) => {
          e.preventDefault();
          void perform(async () => {
            proposed = await API.PreviewInstallments(invoiceID, plainDecimal(rate), first);
          }, false);
        }}
      >
        <h4>Ratenzahlung vereinbaren</h4>
        <p>
          Der aktuelle Restbetrag von {money(balance.remaining, balance.currency)} wird auf monatliche
          Raten verteilt. Die Rechnung selbst ändert sich nicht.
        </p>
        <div class="fields">
          <label
            >Monatliche Rate in {balance.currency}<input
              bind:value={rate}
              oninput={() => (proposed = null)}
              inputmode="decimal"
              required
              disabled={busy}
            /></label
          ><label
            >Erste Fälligkeit<input
              type="date"
              bind:value={first}
              oninput={() => (proposed = null)}
              required
              disabled={busy}
            /></label
          >
        </div>
        <label
          >Notiz zur Vereinbarung (optional)<input
            bind:value={note}
            maxlength="2000"
            disabled={busy}
          /></label
        ><button class="secondary" disabled={busy}>Ratenplan anzeigen</button><button
          type="button"
          class="text"
          disabled={busy}
          onclick={() => (mode = "")}>Abbrechen</button
        >
      </form>
      {#if proposed}<div class="plan">
          <h4>Vorschau der Vereinbarung</h4>
          {#each proposed.rates ?? [] as r}<p>
              {dateLabel(r.date)} · {money(r.amount, balance.currency)}
            </p>{/each}
          <SettingSection
            title="Raten einzeln anpassen"
            action="Anpassen"
            summary={`${proposed.rates?.length ?? 0} Raten, Termine und Beträge änderbar`}
            >{#each proposed.rates ?? [] as r, i}<div class="fields">
                <label
                  >Fälligkeit Rate {i + 1}<input
                    type="date"
                    bind:value={r.date}
                    disabled={busy}
                  /></label
                ><label
                  >Betrag Rate {i + 1} in {balance.currency}<input
                    bind:value={r.amount}
                    inputmode="decimal"
                    disabled={busy}
                  /></label
                >
              </div>{/each}
          </SettingSection>
          <button
            class="primary"
            disabled={busy}
            onclick={() =>
              perform(async () => {
                await API.SaveInstallmentPlan({
                  ...proposed!,
                  rates:
                    proposed!.rates?.map((r) => ({ ...r, amount: plainDecimal(r.amount) })) ?? [],
                  note,
                });
                mode = "";
                proposed = null;
                notice =
                  "Ratenvereinbarung gespeichert. Die App berücksichtigt jetzt die vereinbarten Termine.";
              })}>Vereinbarung speichern</button
          >
        </div>{/if}{/if}
    {#if mode === "cancel"}<form
        onsubmit={(e) => {
          e.preventDefault();
          void perform(async () => {
            await API.SaveInstallmentPlan({ ...plan!, active: false, note });
            mode = "";
            notice = "Ratenvereinbarung aufgehoben.";
          });
        }}
      >
        <label
          >Grund der Aufhebung<input
            bind:value={note}
            required
            maxlength="2000"
            disabled={busy}
          /></label
        ><button class="secondary" disabled={busy}>Vereinbarung aufheben</button><button
          type="button"
          class="text"
          disabled={busy}
          onclick={() => (mode = "")}>Abbrechen</button
        >
      </form>{/if}
    {#if balance.overpaid && Number(balance.overpaid) > 0}<p>
        {money(balance.overpaid, balance.currency)} über dem Rechnungsbetrag.
      </p>{/if}
    {#if history.length}<ul class="payment-history" aria-label="Erfasste Zahlungen">
        {#each history as p}<li>
            <span class="payment-entry"
              ><strong>{money(p.amount, p.currency)}</strong> · {dateLabel(p.date)}<small
                >{p.source === "bank"
                  ? "Bankabgleich"
                  : p.source === "csv"
                    ? "Kontoimport"
                    : p.source === "shop"
                      ? "Shop"
                      : "Manuell erfasst"} · {p.reason}</small
              >{#if p.revokedAt}<small
                  >Aufgehoben: {p.revocationReason} · {dateLabel(p.revokedAt.slice(0, 10))}</small
                >{/if}</span
            >{#if !p.revokedAt}<button
                class="secondary"
                title="Zuordnung dieser Zahlung korrigieren"
                disabled={busy}
                onclick={() => {
                  revokeID = p.id;
                  revokeReason = "";
                }}>Korrigieren</button
              >{/if}
          </li>{/each}
      </ul>{:else}<p class="hint">Noch keine Zahlung erfasst.</p>{/if}
    {#if revokeID}<form
        onsubmit={(e) => {
          e.preventDefault();
          void perform(async () => {
            await API.RevokePayment(revokeID, revokeReason);
            revokeID = "";
            notice = "Zuordnung aufgehoben. Der Restbetrag ist aktualisiert.";
          });
        }}
      >
        <label
          >Grund der Korrektur<input
            bind:value={revokeReason}
            required
            maxlength="2000"
            disabled={busy}
          /></label
        ><button class="secondary" disabled={busy}>Zuordnung aufheben</button><button
          class="text"
          type="button"
          disabled={busy}
          onclick={() => (revokeID = "")}>Abbrechen</button
        >
      </form>{/if}
    {#if plan?.active}<div class="plan">
        <h4>Vereinbarte Raten</h4>
        {#each plan.rates ?? [] as r}<p>
            {dateLabel(r.date)} · {money(r.amount, plan.currency)}
          </p>{/each}{#if plan.note}<p>{plan.note}</p>{/if}
      </div>{/if}
  {:else if loading}<p class="hint" role="status">
      Zahlungsstatus wird geladen …
    </p>{:else if !error}<p class="hint">
      Für diesen Beleg konnte die App keinen Rechnungsbetrag auslesen.
    </p>{/if}
</section>

<style>
  .invoice-payments {
    padding: 20px;
    margin: 20px 0;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  .payment-heading {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 16px;
    align-items: start;
  }
  .payment-heading > div:first-child {
    min-width: 0;
  }
  h3,
  h4 {
    margin: 0 0 10px;
  }
  .payment-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin-left: auto;
  }
  small {
    display: block;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .hint {
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  label {
    display: grid;
    gap: 6px;
    margin: 14px 0;
  }
  input {
    width: 100%;
    min-width: 0;
  }
  .fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
  }
  form,
  .plan {
    margin-top: 20px;
    padding-top: 16px;
    border-top: 1px solid var(--theme-border);
  }
  .plan p {
    margin: 6px 0;
    overflow-wrap: anywhere;
  }
  .awaiting-credit {
    margin-top: 12px;
    padding: 12px 14px;
    border: 1px solid var(--theme-warning-text);
    border-radius: 8px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    font-size: 13px;
    line-height: 1.5;
  }
  .awaiting-credit p {
    margin: 0;
  }
  .refund-kind {
    margin: 14px 0;
    padding: 0;
    border: 0;
  }
  .refund-kind legend {
    padding: 0;
    font-weight: 600;
  }
  .refund-kind label {
    display: flex;
    gap: 8px;
    align-items: start;
    margin: 10px 0;
  }
  .refund-kind label span {
    min-width: 0;
  }
  .refund-kind input {
    width: auto;
    margin-top: 3px;
  }
  .payment-history {
    margin: 12px 0 0;
    padding: 0;
    list-style: none;
  }
  .payment-history li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 12px;
    padding: 10px 0;
    border-top: 1px solid var(--theme-border);
  }
  .payment-entry {
    flex: 1 1 14rem;
    min-width: 0;
  }
  form button,
  .plan button,
  .awaiting-credit button {
    margin-right: 8px;
    margin-top: 6px;
  }
  .error {
    color: var(--theme-error-text);
    overflow-wrap: anywhere;
  }
  @media (max-width: 900px) {
    .fields {
      grid-template-columns: minmax(0, 1fr);
      gap: 0;
    }
  }
</style>
