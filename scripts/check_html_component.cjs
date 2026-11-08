const fs = require("fs");
(async () => {
  const { createServer } =
    await import("../frontend/node_modules/vite/dist/node/index.js");
  const { svelte } =
    await import("../frontend/node_modules/@sveltejs/vite-plugin-svelte/src/index.js");
  const playwright = require(process.env.PLAYWRIGHT_MODULE || "playwright");
  const browserType = process.env.PLAYWRIGHT_BROWSER || "chromium";
  const server = await createServer({
    root: process.cwd() + "/frontend",
    configFile: false,
    server: { host: "127.0.0.1", port: 0 },
    plugins: [
      {
        name: "preview-test",
        enforce: "pre",
        resolveId(id, importer) {
          if (
            importer?.endsWith("InvoiceHTML.svelte") &&
            id.includes("invoiceservice")
          )
            return "\0mock-service";
          if (
            importer?.endsWith("ReceivedPDFPreview.svelte") &&
            id.includes("pdfViewer")
          )
            return "\0mock-pdf";
          if (id === "/test-entry.js") return "\0test-entry";
          if (id === "/received-entry.js") return "\0received-entry";
        },
        load(id) {
          if (id === "\0mock-service")
            return `export async function InvoiceDocumentHTML(d){window.htmlCalls=(window.htmlCalls||0)+1;window.lastDraft=d;if(window.htmlFailure)throw Error('Preview unavailable');return fetch('/fixture.html').then(r=>r.text())};export async function InvoicePreviewFonts(){return fetch('/fonts.css').then(r=>r.text())}`;
          if (id === "\0test-entry")
            return `import {mount} from 'svelte';import {SvelteMap} from 'svelte/reactivity';const state=new SvelteMap([['draft',{id:'test',status:'draft'}],['company',{}],['pending',false],['disabled',false]]);window.previewState=state;import Preview from '/src/features/invoices/InvoiceHTML.svelte';mount(Preview,{target:document.body,props:{get draft(){return state.get('draft')},get company(){return state.get('company')},get pending(){return state.get('pending')},get disabled(){return state.get('disabled')},onedit:part=>window.edited=part}});`;
          if (id === "\0received-entry")
            return `import {mount,unmount} from 'svelte';import {SvelteMap} from 'svelte/reactivity';const state=new SvelteMap([['encoded','multi']]);window.receivedState=state;import Preview from '/src/features/received/ReceivedPDFPreview.svelte';const app=mount(Preview,{target:document.body,props:{get encoded(){return state.get('encoded')}}});window.disposePreview=()=>unmount(app);`;
          if (id === "\0mock-pdf")
            return `
              export function loadPDF(source) {
                window.pdfLoads=(window.pdfLoads||0)+1;
                if(source==='malformed')throw Error('Invalid base64');
                const pdf={numPages:source==='multi'?3:1,getPage:async(number)=>({source,number})};
                return {
                  promise:window.pdfFailure?Promise.reject(Error('PDF unavailable')):source==='slow-load'?new Promise(resolve=>window.finishLoad=()=>resolve(pdf)):Promise.resolve(pdf),
                  destroy:async()=>{window.pdfDestroyed=(window.pdfDestroyed||0)+1}
                };
              }
              export async function renderPDFPage(page,number) {
                if(page.source==='slow-render')await new Promise(resolve=>window.finishRender=resolve);
                const canvas=document.createElement('canvas');
                canvas.dataset.source=page.source;canvas.dataset.page=String(number);
                const accessibleText=document.createElement('p');accessibleText.textContent='Seite '+number;
                return {canvas,accessibleText};
              }`;
        },
        configureServer(s) {
          s.middlewares.use((req, res, next) => {
            if (req.url === "/harness" || req.url === "/received-harness") {
              res.setHeader("Content-Type", "text/html");
              res.end(
                '<style>body{margin:0;width:794px}</style><script type="module" src="/' +
                  (req.url === "/harness"
                    ? "test-entry.js"
                    : "received-entry.js") +
                  '"></script>',
              );
            } else if (
              req.url === "/fixture.html" ||
              req.url === "/fonts.css"
            ) {
              res.setHeader("Content-Type", "text/plain");
              res.end(
                fs.readFileSync(
                  "tmp/html-preview/" +
                    (req.url === "/fixture.html"
                      ? "compact-1.html"
                      : "fonts.css"),
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
  const browser = await playwright[browserType].launch({
    executablePath: process.env.EDGE_PATH || undefined,
    headless: true,
  });
  const page = await browser.newPage({
    viewport: { width: 1000, height: 1200 },
  });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(
    "http://127.0.0.1:" + server.httpServer.address().port + "/harness",
  );
  await page
    .locator('.html-preview[aria-busy="false"]')
    .waitFor({ timeout: 20000 });
  if (await page.locator(".preview-timing").count())
    throw Error("Diagnostic label remains");
  if ((await page.evaluate(() => window.htmlCalls)) !== 1)
    throw Error("Initial double render");
  await page.evaluate(() => window.previewState.set("company", {}));
  await page.waitForTimeout(300);
  if ((await page.evaluate(() => window.htmlCalls)) !== 1)
    throw Error("Identical company caused another render");
  await page.evaluate(() => {
    window.previewState.set("pending", true);
    window.previewState.set("draft", {
      id: "test",
      status: "draft",
      number: "edit",
    });
  });
  await page.waitForTimeout(450);
  if ((await page.evaluate(() => window.htmlCalls)) !== 1)
    throw Error("Rendered before validation settled");
  await page.evaluate(() => {
    window.previewState.set("draft", {
      id: "test",
      status: "draft",
      number: "normalized",
    });
    window.previewState.set("company", {});
    window.previewState.set("pending", false);
  });
  await page
    .locator('.html-preview[aria-busy="false"]')
    .waitFor({ timeout: 20000 });
  await page.waitForTimeout(300);
  if ((await page.evaluate(() => window.htmlCalls)) !== 2)
    throw Error("Edit did not render exactly once");
  if ((await page.evaluate(() => window.lastDraft.number)) !== "normalized")
    throw Error("Stale draft rendered");
  console.log(
    "Single render on mount and edit; identical company ignored; diagnostics removed.",
  );
  await page.frameLocator("iframe").locator("[data-edit=customer]").click();
  if ((await page.evaluate(() => window.edited)) !== "customer")
    throw Error("Editing failed");
  const footer = page.frameLocator("iframe").locator(".pagefooter").first();
  if (
    (await footer.getAttribute("aria-label")) !==
    "Unternehmensprofil bearbeiten"
  )
    throw Error("Invoice footer is not exposed as company settings");
  await page.evaluate(() => (window.edited = ""));
  await footer.click();
  if ((await page.evaluate(() => window.edited)) !== "company")
    throw Error("Invoice footer click did not open the company profile");
  await page.evaluate(() => (window.edited = ""));
  await footer.press("Enter");
  if ((await page.evaluate(() => window.edited)) !== "company")
    throw Error(
      "Invoice footer keyboard action did not open the company profile",
    );
  for (const part of [
    "company",
    "customer",
    "dates",
    "lines",
    "payment",
    "qr",
  ]) {
    const region = page
      .frameLocator("iframe")
      .locator(`[data-edit=${part}]`)
      .first();
    for (const action of ["click", "Enter", "Space"]) {
      await page.evaluate(() => (window.edited = ""));
      if (action === "click") await region.click();
      else await region.press(action);
      if ((await page.evaluate(() => window.edited)) !== part)
        throw Error(`${part}: ${action} did not open the editor`);
    }
  }
  await page.evaluate(() => {
    window.edited = "";
    window.previewState.set("disabled", true);
  });
  await page.frameLocator("iframe").locator("[data-edit=customer]").click();
  if (await page.evaluate(() => window.edited))
    throw Error("Disabled invoice allows editing");
  await page.evaluate(() => window.previewState.set("disabled", false));
  // allow-scripts is needed for WebKit's parent-installed listeners. The CSP
  // must still prevent invoice content from running its own code.
  await page.locator("iframe").evaluate((frame) => {
    const doc = frame.contentDocument;
    const script = doc.createElement("script");
    script.textContent = "parent.invoiceScriptExecuted = true";
    doc.body.append(script);
    const button = doc.createElement("button");
    button.id = "csp-probe";
    button.textContent = "CSP probe";
    button.setAttribute("onclick", "parent.invoiceScriptExecuted = true");
    doc.body.append(button);
  });
  await page.frameLocator("iframe").locator("#csp-probe").click();
  if (await page.evaluate(() => window.invoiceScriptExecuted))
    throw Error("Invoice content bypassed the script policy");
  await page.locator("iframe").evaluate((frame) => {
    frame.contentDocument.querySelector("#csp-probe").remove();
    frame.contentDocument.querySelector("script").remove();
  });
  console.log(
    `${browserType}: all edit regions, keyboard actions, busy lock and script policy passed.`,
  );
  await page.evaluate(() => (document.body.style.width = "500px"));
  await page.waitForTimeout(200);
  const metrics = await page.locator("iframe").evaluate((f) => ({
    width: f.clientWidth,
    height: f.clientHeight,
    scroll: f.contentDocument.documentElement.scrollWidth,
    zoom: f.contentDocument.documentElement.style.zoom,
  }));
  console.log("Resized:", metrics);
  if (metrics.width !== 500 || metrics.height > 800)
    throw Error("Responsive fit failed");
  if (errors.length) throw Error(errors.join("\n"));
  await page.screenshot({
    path: "tmp/html-preview/component-500.png",
    fullPage: true,
  });
  await page.evaluate(() => {
    window.htmlFailure = true;
    window.previewState.set("draft", { id: "broken", status: "draft" });
  });
  await page.getByRole("alert").waitFor();
  if (
    await page
      .locator("iframe")
      .evaluate((f) => f.contentDocument.body.childElementCount)
  )
    throw Error("Failed HTML preview retains the previous invoice");
  const editButtons = page.getByRole("button", { name: /bearbeiten$/ });
  if ((await editButtons.count()) !== 6)
    throw Error("Recovery editing is missing");
  await page
    .getByRole("button", { name: "Zahlungsangaben bearbeiten" })
    .click();
  if ((await page.evaluate(() => window.edited)) !== "payment")
    throw Error("Recovery editing failed");
  await page.evaluate(() => window.previewState.set("disabled", true));
  if (
    await editButtons.evaluateAll((buttons) => buttons.some((b) => !b.disabled))
  )
    throw Error("Busy preview permits editing");
  await page.evaluate(() =>
    window.previewState.set("draft", { id: "issued", status: "issued" }),
  );
  await page.getByRole("alert").waitFor();
  if (await editButtons.count()) throw Error("Issued preview offers editing");
  await page.evaluate(() => {
    window.htmlFailure = false;
  });
  await page.getByRole("button", { name: "Erneut versuchen" }).click();
  await page.locator('.html-preview[aria-busy="false"]').waitFor();
  if (await page.getByRole("alert").count()) throw Error("HTML retry failed");
  if (await page.frameLocator("iframe").locator("[data-edit]").count())
    throw Error("Issued HTML remains editable");
  console.log(
    "HTML failure clears content; draft editing, busy lock, issued lock and retry passed.",
  );

  await page.goto(
    "http://127.0.0.1:" +
      server.httpServer.address().port +
      "/received-harness",
  );
  const receivedReady = () =>
    page.locator('.received-pdf-page[aria-busy="false"]').waitFor();
  await receivedReady();
  await page.getByRole("button", { name: "Nächste Seite" }).click();
  await receivedReady();
  if ((await page.locator("canvas").getAttribute("data-page")) !== "2")
    throw Error("Received page navigation failed");
  if ((await page.evaluate(() => window.pdfLoads)) !== 1)
    throw Error("Page navigation reloaded the PDF document");
  await page.evaluate(() => (document.body.style.width = "500px"));
  await page.waitForTimeout(350);
  await receivedReady();
  if ((await page.evaluate(() => window.pdfLoads)) !== 1)
    throw Error("Resizing reloaded the PDF document");
  if (await page.evaluate(() => window.pdfDestroyed || 0))
    throw Error("Paging or resizing destroyed the active PDF document");
  await page.evaluate(() => window.receivedState.set("encoded", "single"));
  await receivedReady();
  if (
    (await page.locator(".page-controls span").textContent()) !==
    "Seite 1 von 1"
  )
    throw Error("Document switch did not reset the page number");
  if ((await page.locator("canvas").getAttribute("data-page")) !== "1")
    throw Error("Wrong page after document switch");
  for (const source of ["slow-load", "slow-render"]) {
    await page.evaluate(
      (source) => window.receivedState.set("encoded", source),
      source,
    );
    await page.waitForFunction(
      (source) =>
        source === "slow-load" ? !!window.finishLoad : !!window.finishRender,
      source,
    );
    if (await page.locator("canvas").count())
      throw Error("Old canvas remains during loading");
    await page.evaluate(() => window.receivedState.set("encoded", "single"));
    await receivedReady();
    await page.evaluate(
      (source) =>
        source === "slow-load" ? window.finishLoad() : window.finishRender(),
      source,
    );
    await page.waitForTimeout(100);
    if ((await page.locator("canvas").getAttribute("data-source")) !== "single")
      throw Error("Late PDF response replaced the current document");
  }
  await page.evaluate(() => {
    window.pdfFailure = true;
    window.receivedState.set("encoded", "retry");
  });
  await page.getByRole("alert").waitFor();
  if (await page.locator("canvas").count())
    throw Error("Failed PDF retains an old canvas");
  await page.evaluate(() => {
    window.pdfFailure = false;
  });
  await page.getByRole("button", { name: "Erneut versuchen" }).click();
  await receivedReady();
  if ((await page.locator("canvas").getAttribute("data-source")) !== "retry")
    throw Error("PDF retry failed");
  await page.evaluate(() => window.receivedState.set("encoded", "malformed"));
  await page.getByRole("alert").waitFor();
  if (await page.locator("canvas").count())
    throw Error("Malformed PDF retains an old canvas");
  await page.evaluate(() => window.disposePreview());
  if (await page.evaluate(() => window.pdfLoads !== window.pdfDestroyed + 1))
    throw Error("PDF loading tasks leaked");
  if (errors.length) throw Error(errors.join("\n"));
  console.log(
    "Received PDF: source reset, stale responses, failed loads, malformed data, retry and cleanup passed.",
  );
  await browser.close();
  await server.close();
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
