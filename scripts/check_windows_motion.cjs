const path = require("path");

(async () => {
  const { createServer } =
    await import("../frontend/node_modules/vite/dist/node/index.js");
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
  const root = path.resolve(__dirname, "..");
  const server = await createServer({
    root: path.join(root, "frontend"),
    configFile: false,
    server: { host: "127.0.0.1", port: 0 },
    plugins: [
      {
        name: "windows-motion-test",
        resolveId(id) {
          if (id === "/motion-entry.js") return "\0motion-entry";
        },
        load(id) {
          if (id !== "\0motion-entry") return;
          return `
            import '/src/styles/app.css';
            import '/src/styles/features.css';
            import '/src/styles/document.css';
            import '/src/styles/onboarding.css';
            import '/src/styles/shell.css';
            import '/src/styles/document-preview.css';
            import '/src/styles/controls.css';
            import '/src/styles/theme.css'; import '/src/brand/logo.css';
            import '/src/styles/responsive.css';
            import '/src/styles/ui.css';
            import '/src/styles/windows.css';
            document.documentElement.dataset.platform = 'windows';
            document.querySelector('dialog').showModal();
          `;
        },
        configureServer(vite) {
          vite.middlewares.use((request, response, next) => {
            if (request.url !== "/motion") return next();
            response.setHeader("Content-Type", "text/html; charset=utf-8");
            response.end(`<!doctype html><html><body>
              <div class="shell app-shell">
                <aside class="sidebar"><div class="logo"><span>ERS</span><div><strong>E-Rechnung-Studio</strong><small>Windows</small></div></div><nav><button class="active">Rechnungen</button><button>Kunden</button><button>Artikel</button></nav></aside>
                <main class="workspace"><div class="window-toolbar"><button class="sidebar-toggle">☰</button><span>Rechnungen</span></div><header class="topbar"><div><h1>Rechnungen</h1><p class="heading-subtitle">Übersicht</p></div><button class="primary">Neue Rechnung</button></header><div class="cards"><button class="entity-card"><span class="avatar">MG</span><div><strong>Muster GmbH</strong><small>Berlin, DE</small></div></button><button class="entity-card"><span class="avatar product">BL</span><div><strong>Beratung</strong><small>120,00 €</small></div></button></div><div class="document-stage">Das Rechnungsblatt bleibt transformfrei.</div></main>
              </div>
              <dialog class="app-dialog"><form class="modal form-dialog"><header><h2>Neue Rechnung</h2></header><div class="dialog-body"><div class="form-grid"><label>Name<input value="Muster GmbH"></label></div></div><footer><button class="secondary">Abbrechen</button><button class="primary">Speichern</button></footer></form></dialog>
              <script type="module" src="/motion-entry.js"></script>
            </body></html>`);
          });
        },
      },
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
    const url =
      "http://127.0.0.1:" + server.httpServer.address().port + "/motion";
    const normal = await browser.newPage({
      viewport: { width: 1100, height: 760 },
      reducedMotion: "no-preference",
    });
    await normal.goto(url);
    await normal.locator("dialog[open]").waitFor();
    const motion = await normal.evaluate(() => {
      const style = (selector) =>
        getComputedStyle(document.querySelector(selector));
      return {
        buttonTransition: style("button.primary").transitionDuration,
        pageAnimation: style(".topbar").animationName,
        paperAnimation: style(".document-stage").animationName,
        dialogAnimation: style("dialog").animationName,
      };
    });
    if (!motion.buttonTransition.includes("0.09s"))
      throw Error("Windows button timing is missing");
    if (motion.pageAnimation !== "win-content-enter")
      throw Error("Page entry motion is missing");
    if (motion.paperAnimation !== "win-fade-enter")
      throw Error("Invoice-safe fade is missing");
    if (motion.dialogAnimation !== "win-dialog-enter")
      throw Error("Dialog entry motion is missing");
    await normal.screenshot({
      path: path.join(root, "tmp/windows-motion.png"),
    });

    const reduced = await browser.newPage({
      viewport: { width: 800, height: 600 },
      reducedMotion: "reduce",
    });
    await reduced.goto(url);
    await reduced.locator("dialog[open]").waitFor();
    const reducedMotion = await reduced.evaluate(() => ({
      transition: getComputedStyle(document.querySelector("button.primary"))
        .transitionDuration,
      animation: getComputedStyle(document.querySelector("dialog"))
        .animationDuration,
    }));
    if (
      parseFloat(reducedMotion.transition) > 0.000011 ||
      parseFloat(reducedMotion.animation) > 0.000011
    )
      throw Error(
        "Reduced-motion preference is not honored: " +
          JSON.stringify(reducedMotion),
      );
    console.log(
      "Windows motion timings, page/dialog entry, sharp-paper fade and reduced motion passed.",
    );
  } finally {
    await browser.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
