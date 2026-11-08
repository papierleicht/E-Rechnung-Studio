const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const ts = require("../frontend/node_modules/typescript");
const fs = require("fs"),
  http = require("http"),
  path = require("path"),
  assert = require("assert/strict");
const root = path.resolve(__dirname, "..");
const dir = path.join(root, "tmp/html-preview");
const server = http.createServer((req, res) => {
  try {
    if (req.url === "/") {
      res.setHeader("Content-Type", "text/html");
      res.end(
        '<style>body{margin:0;background:#ddd}iframe{border:0;width:794px;height:1150px;display:block}</style><iframe sandbox="allow-same-origin" srcdoc="<!doctype html><html><head></head><body></body></html>"></iframe>',
      );
      return;
    }
    let data,
      type = "text/javascript";
    if (req.url === "/htmlInvoice.js")
      data = ts.transpileModule(
        fs.readFileSync(
          path.join(root, "frontend/src/lib/htmlInvoice.ts"),
          "utf8",
        ),
        {
          compilerOptions: {
            target: ts.ScriptTarget.ES2022,
            module: ts.ModuleKind.ES2022,
          },
        },
      ).outputText;
    else if (req.url === "/pdf.mjs" || req.url === "/pdf.worker.mjs")
      data = fs.readFileSync(
        path.join(
          root,
          "frontend/node_modules/pdfjs-dist/legacy/build",
          path.basename(req.url),
        ),
      );
    else {
      const file = path.join(dir, path.basename(req.url));
      data = fs.readFileSync(file);
      type = file.endsWith(".pdf") ? "application/pdf" : "text/plain";
    }
    res.setHeader("Content-Type", type);
    res.end(data);
  } catch {
    res.writeHead(404);
    res.end();
  }
});
(async () => {
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  const browser = await chromium.launch({
    executablePath:
      process.env.EDGE_PATH ||
      "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
    headless: true,
  });
  const page = await browser.newPage({
    viewport: { width: 1000, height: 1200 },
    deviceScaleFactor: 1,
  });
  await page.goto("http://127.0.0.1:" + server.address().port);
  await page.evaluate(async () => {
    window.htmlTools = await import("/htmlInvoice.js");
    window.pdfjs = await import("/pdf.mjs");
    pdfjs.GlobalWorkerOptions.workerSrc = "/pdf.worker.mjs";
    window.fonts = await (await fetch("/fonts.css")).text();
  });
  const results = [];
  for (const count of process.env.E_RECHNUNG_STUDIO_HTML_STRESS_MATRIX
    ? [3, 9, 18]
    : process.env.E_RECHNUNG_STUDIO_HTML_PAGINATION_MATRIX
      ? [1, 5, 10, 15, 20, 25, 30, 40, 50, 60, 70, 80, 90, 100]
      : [1, 40]) {
      const name = "compact-" + count;
      const result = await page.evaluate(
        async ({ name, count, matrix }) => {
          const html = await (await fetch("/" + name + ".html")).text();
          const bytes = new Uint8Array(
            await (await fetch("/" + name + ".pdf")).arrayBuffer(),
          );
          const frame = document.querySelector("iframe"),
            doc = frame.contentDocument;
          const times = [];
          let pages;
          for (let i = 0; i < (matrix ? 1 : 4); i++) {
            const start = performance.now();
            htmlTools.prepareInvoice(doc, html, fonts);
            void doc.body.offsetHeight;
            await doc.fonts.ready;
            await Promise.all([...doc.images].map((im) => im.decode()));
            pages = htmlTools.paginateInvoice(doc);
            await new Promise(requestAnimationFrame);
            times.push(performance.now() - start);
          }
          htmlTools.enableInvoiceEditing(
            doc,
            true,
            (part) => (window.edited = part),
          );
          doc.querySelector("[data-edit=customer]").click();
          if (window.edited !== "customer") throw Error("edit click failed");
          for (let i = 1; i <= count; i++)
            if (
              !doc.body.textContent.includes(
                "Beratung " + String(i).padStart(2, "0"),
              )
            )
              throw Error("Missing line " + i);
          for (const el of doc.querySelectorAll(".html-content"))
            if (el.scrollHeight > el.clientHeight + 1)
              throw Error("page overflow");
          let pdfPages;
          const pdfTimes = [];
          let pdfText = "";
          let checkedAnchors = 0;
          for (let i = 0; i < (matrix ? 1 : 4); i++) {
            const start = performance.now();
            const task = pdfjs.getDocument({
              data: bytes.slice(),
              useSystemFonts: false,
            });
            const pdf = await task.promise;
            pdfPages = pdf.numPages;
            if (pages !== pdfPages)
              throw Error(
                `${name}: page count differs: HTML ${pages}, PDF ${pdfPages}`,
              );
            if (i === 0) {
              const destinations = await pdf.getDestinations();
              const htmlPages = Array.from(doc.querySelectorAll(".html-page"));
              for (const atom of doc.querySelectorAll("[data-flow-id]")) {
                const dest = destinations.get(atom.dataset.flowId);
                if (!dest)
                  throw Error(
                    "Missing PDF destination: " + atom.dataset.flowId,
                  );
                const expected = await pdf.getPageIndex(dest[0]);
                const actual = htmlPages.indexOf(atom.closest(".html-page"));
                if (expected !== actual)
                  throw Error(
                    `${name}: ${atom.dataset.flowId} is on HTML ${actual + 1}, PDF ${expected + 1}`,
                  );
                checkedAnchors++;
              }
            }
            for (let n = 1; n <= pdf.numPages; n++) {
              const p = await pdf.getPage(n),
                v = p.getViewport({ scale: 1 }),
                c = document.createElement("canvas");
              const scale = (794 / v.width) * 2;
              c.width = Math.floor(v.width * scale);
              c.height = Math.floor(v.height * scale);
              if (!matrix)
                await p.render({
                  canvas: c,
                  viewport: v,
                  transform: [
                    c.width / v.width,
                    0,
                    0,
                    c.height / v.height,
                    0,
                    0,
                  ],
                }).promise;
              const text = await p.getTextContent();
              if (i === 0)
                pdfText += text.items.map((x) => x.str || "").join(" ");
            }
            await new Promise(requestAnimationFrame);
            pdfTimes.push(performance.now() - start);
            await task.destroy();
          }
          const bodyText = doc.body.textContent;
          const tokens = pdfText
            .split(/\s+/)
            .filter((t) => /\d+,\d{2}/.test(t));
          for (const token of tokens)
            if (!bodyText.includes(token))
              throw Error("Missing financial amount " + token);
          return {
            name,
            htmlPages: pages,
            pdfPages,
            checkedAnchors,
            htmlCold: times[0],
            htmlWarm: times.slice(1),
            pdfCold: pdfTimes[0],
            pdfWarm: pdfTimes.slice(1),
            fonts: doc.fonts.check("10pt sans"),
          };
        },
        {
          name,
          count,
          matrix: Boolean(process.env.E_RECHNUNG_STUDIO_HTML_PAGINATION_MATRIX),
        },
      );
      results.push(result);
      console.log(JSON.stringify(result));
      if (count === 1) {
        await page.screenshot({
          path: path.join(dir, name + ".png"),
          clip: { x: 0, y: 0, width: 794, height: 1150 },
        });
        await page.evaluate(async (name) => {
          const task = pdfjs.getDocument({ url: "/" + name + ".pdf" });
          const pdf = await task.promise;
          const p = await pdf.getPage(1),
            v = p.getViewport({ scale: 794 / 595.28 });
          const c = document.createElement("canvas");
          c.id = "comparison";
          c.width = Math.ceil(v.width);
          c.height = Math.ceil(v.height);
          c.style = "position:absolute;top:0;left:0";
          document.body.append(c);
          await p.render({ canvas: c, viewport: v }).promise;
          await task.destroy();
        }, name);
        await page.screenshot({
          path: path.join(dir, name + "-pdf.png"),
          clip: { x: 0, y: 0, width: 794, height: 1150 },
        });
        await page.evaluate(() =>
          document.querySelector("#comparison").remove(),
        );
      }
  }
  fs.writeFileSync(
    path.join(
      dir,
      process.env.E_RECHNUNG_STUDIO_HTML_STRESS_MATRIX
        ? "stress-results.json"
        : process.env.E_RECHNUNG_STUDIO_HTML_PAGINATION_MATRIX
          ? "pagination-results.json"
          : "browser-results.json",
    ),
    JSON.stringify(results, null, 2),
  );
  await browser.close();
  server.close();
})().catch((e) => {
  console.error(e);
  server.close();
  process.exit(1);
});
