#!/usr/bin/env node
/**
 * Headless check: merged CSS is v8+ and /rptview has .status-pill after footer JS runs.
 *
 *   TWOWIKI_BASE=http://192.168.20.155:8083 node scripts/verify-twowiki-skin-browser.cjs
 *
 * Requires: npm install in ./scripts (puppeteer).
 */
const puppeteer = require("puppeteer");

const base = (process.env.TWOWIKI_BASE || "http://192.168.20.155:8083").replace(/\/$/, "");
const rptPath = process.env.TWOWIKI_RPT || "/rptview/1";

async function main() {
  const cssUrl = `${base}/style.css`;
  const res = await fetch(cssUrl);
  if (!res.ok) {
    throw new Error(`GET ${cssUrl} → ${res.status}`);
  }
  const css = await res.text();
  if (!css.includes("TwoWiki Fossil Skin v8")) {
    throw new Error("Merged /style.css does not contain 'TwoWiki Fossil Skin v8'");
  }

  const browser = await puppeteer.launch({
    headless: "new",
    args: ["--no-sandbox", "--disable-setuid-sandbox"],
  });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(String(e)));

    await page.goto(`${base}${rptPath}`, { waitUntil: "networkidle2", timeout: 60000 });
    await page.waitForSelector("table.report", { timeout: 15000 });

    const pillCount = await page.$$eval(".status-pill", (els) => els.length);
    if (pillCount < 1) {
      const hasStatusHeader = await page.evaluate(() => {
        const ths = Array.from(document.querySelectorAll("table.report thead th"));
        return ths.some((th) => (th.textContent || "").trim().toUpperCase() === "STATUS");
      });
      if (hasStatusHeader) {
        throw new Error(
          "STATUS column present but no .status-pill nodes — deploy footer (apply --with-footer --confirm-footer) or check footer JS."
        );
      }
      console.warn(
        "verify: no STATUS column on this report; skipping pill assertion (css v8 ok)."
      );
    } else {
      console.log(`verify: ok — ${pillCount} .status-pill node(s) on ${rptPath}`);
    }

    if (errors.length) {
      throw new Error("page errors: " + errors.join("; "));
    }
  } finally {
    await browser.close();
  }

  console.log("verify: ok — css v8 + browser load");
}

main().catch((e) => {
  console.error("verify: FAIL —", e.message || e);
  process.exit(1);
});
