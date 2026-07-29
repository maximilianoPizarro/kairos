#!/usr/bin/env node
/**
 * Capture Kairos Console screenshots from a live CRC port-forward.
 * Usage: node capture-screenshots.mjs [baseUrl] [outDir]
 */
import { chromium } from 'playwright';
import { mkdirSync } from 'fs';
import { join } from 'path';

const BASE = process.argv[2] || 'http://127.0.0.1:8181';
const OUT = process.argv[3] || 'docs/images/screenshots';
mkdirSync(OUT, { recursive: true });

const pages = [
  { file: '01-dashboard.png', nav: 'Dashboard', waitMs: 2800 },
  { file: '02-agents.png', nav: 'AI Agents', waitMs: 1800 },
  { file: '03-policies.png', nav: 'Scaling Policies', waitMs: 1800 },
  {
    file: '03b-policies-rules-expanded.png',
    nav: 'Scaling Policies',
    waitMs: 1800,
    after: async (page) => {
      const toggles = page.locator('.pf-v5-c-expandable-section__toggle');
      const n = await toggles.count();
      for (let i = 0; i < n; i++) {
        await toggles.nth(i).click();
        await page.waitForTimeout(400);
      }
      await page.waitForTimeout(900);
    },
  },
  { file: '04-events.png', nav: 'Events', waitMs: 1800 },
  { file: '06-resources.png', nav: 'Managed Resources', waitMs: 1800 },
  { file: '07-approvals.png', nav: 'Approvals', waitMs: 1800 },
  { file: '08-history.png', nav: 'History', waitMs: 1800 },
  { file: '09-diffview.png', nav: 'Diff View', waitMs: 1800 },
];

const browser = await chromium.launch({
  headless: true,
  args: ['--hide-scrollbars'],
});

const context = await browser.newContext({
  viewport: { width: 1440, height: 960 },
  deviceScaleFactor: 2,
});
const page = await context.newPage();

await page.goto(BASE, { waitUntil: 'networkidle', timeout: 60000 });
await page.waitForSelector('.pf-v5-c-nav__link', { timeout: 30000 });
await page.waitForTimeout(1500);

for (const shot of pages) {
  await page.locator('.pf-v5-c-nav__link', { hasText: shot.nav }).first().click();
  await page.waitForTimeout(shot.waitMs);
  if (shot.after) await shot.after(page);
  const path = join(OUT, shot.file);
  await page.screenshot({ path, fullPage: false });
  console.log('wrote', path);
}

await browser.close();
console.log('done');
