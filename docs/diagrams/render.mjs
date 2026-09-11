// Documentation-only renderer. Never reads application config, accounts or profiles.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { chromium } from 'playwright-core';

const directory = path.dirname(fileURLToPath(import.meta.url));
const fontPath = process.argv[2];
if (!fontPath || process.argv.length !== 3) {
  throw new Error('Usage: node docs/diagrams/render.mjs /path/to/PretendardVariable.woff2');
}
const font = await fs.readFile(fontPath);
assert.equal(createHash('sha256').update(font).digest('hex'), '9599f12fd42fc0bce1cd50b47a0c022e108d7aa64dd0d1bb0ed44f3282d900b4', 'Expected the original Pretendard v1.3.9 variable WOFF2');
const source = await fs.readFile(path.join(directory, 'readme-source.html'), 'utf8');
assert.equal(source.split('/* PRETENDARD_FONT */').length, 2);
const license = await fs.readFile(path.join(directory, 'OFL.txt'), 'utf8');
const escapedLicense = license.replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;');
const html = source.replace('/* PRETENDARD_FONT */', `@font-face{font-family:"Pretendard";font-style:normal;font-weight:100 900;font-display:block;src:url(data:font/woff2;base64,${font.toString('base64')}) format("woff2")}`).replace('<!-- FONT_LICENSE -->', `<footer style="padding:24px 40px"><details><summary>Pretendard · SIL Open Font License 1.1</summary><pre style="white-space:pre-wrap;font:14px/1.6 Pretendard,sans-serif">${escapedLicense}</pre></details></footer>`);
assert(!/<script\b|<link\b|<iframe\b|<img\b/i.test(html), 'Generated diagrams must not load scripts or external resources');
const cssURLs = [...html.matchAll(/url\(([^)]+)\)/g)].map(m=>m[1]);
assert(cssURLs.every(url=>url.startsWith('#')||url.startsWith('data:font/woff2;base64,')), 'Only fragment markers and the pinned embedded font are allowed');
const output = path.join(directory, 'readme.html');
await fs.writeFile(output, html);

const browser = await chromium.launch({ headless:true });
try {
  const page = await browser.newPage({ viewport:{width:720,height:1000}, deviceScaleFactor:2 });
  await page.route('**/*', route => route.request().url().startsWith('file:') ? route.continue() : route.abort());
  await page.goto(pathToFileURL(output).href);
  await page.evaluate(() => document.fonts.ready);
  const fonts = await page.evaluate(() => [...document.fonts].map(f => ({family:f.family,status:f.status})));
  assert(fonts.some(f => f.family === 'Pretendard' && f.status === 'loaded'), 'Pretendard must actually load');
  for (const id of ['architecture','recommendation','orders']) {
    const figure = page.locator(`svg#${id}`);
    const errors = await figure.evaluate(svg => {
      const errors = [];
      const ids = svg.getAttribute('aria-labelledby').split(' ');
      if(svg.firstElementChild.tagName!=='title' || ids.some(id=>!svg.querySelector(`[id="${id}"]`))) errors.push('accessible title/description');
      for(const text of svg.querySelectorAll('text')) {
        if(!getComputedStyle(text).fontFamily.includes('Pretendard')) errors.push('font family');
        const b=text.getBBox();
        if(b.x<0 || b.x+b.width>svg.viewBox.baseVal.width) errors.push(`canvas overflow: ${text.textContent}`);
      }
      for(const node of svg.querySelectorAll('.node')) {
        const r=node.querySelector('rect');
        for(const text of node.querySelectorAll('text')) {
          const b=text.getBBox();
          if(b.x<8 || b.y<0 || b.x+b.width>Number(r.getAttribute('width'))-8 || b.y+b.height>Number(r.getAttribute('height'))) errors.push(`node overflow: ${text.textContent}`);
        }
      }
      return errors;
    });
    assert.deepEqual(errors, [], `${id} geometry`);
    await figure.screenshot({path:path.join(directory, `${id}.png`), omitBackground:true});
    console.log(`${id}: Pretendard loaded; geometry checked; PNG @2 exported`);
  }
  for(const width of [390,320]) {
    await page.setViewportSize({width,height:844});
    assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth), `overflow at ${width}px`);
  }
  console.log('390px / 320px: no horizontal overflow; network disabled');
} finally {
  await browser.close();
}
