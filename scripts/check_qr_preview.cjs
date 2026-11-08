// Generate fixtures first: E_RECHNUNG_STUDIO_QR_TEST_OUTPUT="$PWD/tmp/qr-preview" go test ./internal/app -run TestPaymentQRDisplaySettings
const fs = require("fs");
const path = require("path");
const assert = require("assert/strict");
const root = path.resolve(__dirname, "..");
const fixtures = path.join(root, "tmp/qr-preview");

(async () => {
  const { createServer } =
    await import("../frontend/node_modules/vite/dist/node/index.js");
  const { svelte } =
    await import("../frontend/node_modules/@sveltejs/vite-plugin-svelte/src/index.js");
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
  const server = await createServer({
    root: path.join(root, "frontend"),
    configFile: false,
    server: { host: "127.0.0.1", port: 0 },
    plugins: [
      {
        name: "qr-test",
        enforce: "pre",
        resolveId(id, importer) {
          if (
            importer?.endsWith("InvoiceHTML.svelte") &&
            id.includes("invoiceservice")
          )
            return "\0qr-service";
          if (id === "/entry.js") return "\0qr-entry";
          if (id === "/harness.svelte") return "\0qr-harness.svelte";
        },
        load(id) {
          if (id === "\0qr-service")
            return `
          export async function InvoiceDocumentHTML(d) {
            const mode=d.disablePaymentQR?'off-'+(d.hidePaymentQRDetails?'code':'info'):(d.hidePaymentQRDetails?'code':'info');
            return fetch('/qr-'+mode+'.html').then(r=>r.text());
          }
          export async function InvoicePreviewFonts(){return fetch('/fonts.css').then(r=>r.text())}`;
          if (id === "\0qr-entry")
            return `
          import {mount} from 'svelte'; import Harness from '/harness.svelte';
          import '/src/styles/app.css'; import '/src/styles/features.css'; import '/src/styles/document.css';
          import '/src/styles/onboarding.css'; import '/src/styles/shell.css';
            import '/src/styles/document-preview.css';
            import '/src/styles/controls.css';
          import '/src/styles/theme.css'; import '/src/brand/logo.css'; import '/src/styles/responsive.css'; import '/src/styles/ui.css';
          mount(Harness,{target:document.body});`;
          if (id === "\0qr-harness.svelte")
            return `
          <script>
            import InvoiceDocument from '/src/features/invoices/InvoiceDocument.svelte';
            let draft=$state(${fs.readFileSync(path.join(fixtures, "qr-info.json"), "utf8")});
            let disabled=$state(false);
            window.qrHarness={snapshot:()=>JSON.parse(JSON.stringify(draft)),setDisabled:v=>disabled=v,issue:()=>draft.status='issued'};
          </script>
          <main class="workspace"><div class="document-stage">
            <InvoiceDocument bind:draft company={{}} {disabled} onchange={()=>window.qrChanges=(window.qrChanges||0)+1}
              oneditcompany={()=>{}} onchangecustomer={()=>{}} oneditline={()=>{}} oneditorstatechange={dirty=>window.qrDirty=dirty}/>
          </div></main>`;
        },
        configureServer(s) {
          s.middlewares.use((req, res, next) => {
            if (req.url === "/harness") {
              res.setHeader("Content-Type", "text/html");
              res.end('<script type="module" src="/entry.js"></script>');
            } else if (
              /^\/qr-[a-z-]+\.(html|pdf)$/.test(req.url) ||
              req.url === "/fonts.css"
            ) {
              res.setHeader(
                "Content-Type",
                req.url.endsWith(".pdf") ? "application/pdf" : "text/plain",
              );
              res.end(
                fs.readFileSync(path.join(fixtures, path.basename(req.url))),
              );
            } else if (["/pdf.mjs", "/pdf.worker.mjs"].includes(req.url)) {
              res.setHeader("Content-Type", "text/javascript");
              res.end(
                fs.readFileSync(
                  path.join(
                    root,
                    "frontend/node_modules/pdfjs-dist/legacy/build",
                    path.basename(req.url),
                  ),
                ),
              );
            } else next();
          });
        },
      },
      svelte(),
    ],
  });
  await server.listen();
  const browser = await chromium.launch({
    executablePath:
      process.env.EDGE_PATH ||
      "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
    headless: true,
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 1100, height: 1200 },
    });
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto(
      "http://127.0.0.1:" + server.httpServer.address().port + "/harness",
    );
    const ready = () =>
      page.locator('.html-preview[aria-busy="false"]').waitFor();
    await ready();
    const qr = page.frameLocator("iframe").locator(".payment-qr");
    const dialog = page.getByRole("dialog", { name: "QR-Code-Einstellungen" });
    const showQR = page.getByRole("checkbox", {
      name: "QR-Code und Informationen anzeigen",
      exact: true,
    });
    const showInfo = page.getByRole("checkbox", {
      name: "Zusätzliche Informationen anzeigen",
      exact: true,
    });
    await qr.locator("img").click();
    await dialog.waitFor();
    assert.equal(await showQR.isChecked(), true);
    assert.equal(await showInfo.isChecked(), true);
    await showInfo.uncheck();
    assert.equal(await page.evaluate(() => window.qrDirty), true);
    await page.screenshot({ path: path.join(fixtures, "qr-dialog.png") });
    await page.getByRole("button", { name: "Abbrechen", exact: true }).click();
    assert.equal(
      await page.evaluate(
        () => !!window.qrHarness.snapshot().hidePaymentQRDetails,
      ),
      false,
    );
    assert.equal(await page.evaluate(() => window.qrDirty), false);
    assert.equal(await page.evaluate(() => window.qrChanges || 0), 0);
    await qr.focus();
    await qr.press("Enter");
    await dialog.waitFor();
    await showInfo.uncheck();
    await page.getByRole("button", { name: "Übernehmen", exact: true }).click();
    await ready();
    assert.equal(
      await page.evaluate(
        () => window.qrHarness.snapshot().hidePaymentQRDetails,
      ),
      true,
    );
    assert.equal(await qr.locator("img").count(), 1);
    assert.equal((await qr.innerText()).trim(), "");
    await qr.locator("img").click();
    await showQR.uncheck();
    assert.equal(await showInfo.isEnabled(), false);
    await page.getByRole("button", { name: "Übernehmen", exact: true }).click();
    await ready();
    assert.equal(await qr.count(), 0);
    await page
      .getByRole("button", { name: "QR-Code-Einstellungen", exact: true })
      .click();
    assert.equal(await showQR.isChecked(), false);
    assert.equal(await showInfo.isChecked(), false);
    await showQR.check();
    await page.getByRole("button", { name: "Übernehmen", exact: true }).click();
    await ready();
    assert.equal((await qr.innerText()).trim(), "");
    await qr.click();
    await showInfo.check();
    await page.getByRole("button", { name: "Übernehmen", exact: true }).click();
    await ready();
    assert.match(await qr.innerText(), /SEPA-Überweisung/);
    assert.equal(await page.evaluate(() => window.qrChanges), 4);
    await page.evaluate(() => window.qrHarness.setDisabled(true));
    await qr.click();
    assert.equal(await dialog.count(), 0);
    await page.evaluate(() => {
      window.qrHarness.setDisabled(false);
      window.qrHarness.issue();
    });
    await ready();
    assert.equal(await qr.getAttribute("data-edit"), null);
    assert.equal(
      await page
        .getByRole("button", { name: "QR-Code-Einstellungen", exact: true })
        .count(),
      0,
    );
    console.log(
      "QR dialog: mouse/keyboard, cancel, dirty state, both switches, re-enable and editing locks passed.",
    );

    const results = await page.evaluate(async () => {
      const { prepareInvoice, paginateInvoice } =
        await import("/src/lib/htmlInvoice.ts");
      const pdfjs = await import("/pdf.mjs");
      pdfjs.GlobalWorkerOptions.workerSrc = "/pdf.worker.mjs";
      const fonts = await (await fetch("/fonts.css")).text();
      const doc = document.querySelector("iframe").contentDocument;
      const results = [];
      for (const mode of ["info", "code", "off-info", "off-code"]) {
          const name = `qr-${mode}`;
          const source = await (await fetch(`/${name}.html`)).text();
          prepareInvoice(doc, source, fonts);
          void doc.body.offsetHeight;
          await doc.fonts.ready;
          await Promise.all([...doc.images].map((image) => image.decode()));
          const htmlPages = paginateInvoice(doc);
          const task = pdfjs.getDocument({
            url: `/${name}.pdf`,
            useSystemFonts: false,
          });
          const pdf = await task.promise;
          if (pdf.numPages !== htmlPages)
            throw Error(name + ": page count differs");
          const destinations = await pdf.getDestinations();
          const pages = [...doc.querySelectorAll(".html-page")];
          for (const atom of doc.querySelectorAll("[data-flow-id]")) {
            const destination = destinations.get(atom.dataset.flowId);
            if (
              !destination ||
              (await pdf.getPageIndex(destination[0])) !==
                pages.indexOf(atom.closest(".html-page"))
            )
              throw Error(
                name + ": page assignment differs for " + atom.dataset.flowId,
              );
          }
          let text = "",
            images = 0;
          for (let number = 1; number <= pdf.numPages; number++) {
            const p = await pdf.getPage(number);
            text += (await p.getTextContent()).items
              .map((item) => item.str || "")
              .join(" ");
            const operators = await p.getOperatorList();
            images += operators.fnArray.filter(
              (op) => op === pdfjs.OPS.paintImageXObject,
            ).length;
            if (mode === "code" && number === 1) {
              const canvas = document.createElement("canvas");
              const viewport = p.getViewport({ scale: 1.5 });
              canvas.width = Math.ceil(viewport.width);
              canvas.height = Math.ceil(viewport.height);
              await p.render({ canvas, viewport }).promise;
              window.qrPDFImage = canvas.toDataURL("image/png");
            }
          }
          const enabled = !mode.startsWith("off-");
          if (
            images > 0 !== enabled ||
            text.includes("Banking-App") !== (mode === "info")
          )
            throw Error(name + ": incorrect PDF QR visibility");
          if (
            !!doc.querySelector(".payment-qr img") !== enabled ||
            doc.body.textContent.includes("Banking-App") !== (mode === "info")
          )
            throw Error(name + ": incorrect HTML QR visibility");
          await task.destroy();
          results.push({ name, pages: htmlPages, images });
      }
      return results;
    });
    fs.writeFileSync(
      path.join(fixtures, "qr-code-only-pdf.png"),
      Buffer.from(
        (await page.evaluate(() => window.qrPDFImage)).split(",")[1],
        "base64",
      ),
    );
    fs.writeFileSync(
      path.join(fixtures, "results.json"),
      JSON.stringify(results, null, 2),
    );
    assert.deepEqual(errors, []);
    console.log(
      "HTML/PDF: all four QR modes match visibility and page assignments.",
    );
  } finally {
    await browser.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
