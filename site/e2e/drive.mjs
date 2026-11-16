// Drives site/index.html in a headless browser over the Chrome DevTools
// protocol. Started by run.sh, which sets the E2E_* variables.
//
// The expected counts come from the dataset itself, so a data refresh does not
// break the test. It does assume the dataset has Jawa Barat > Kota Bandung >
// Andir, which has been mapped for a long time.
import fs from "node:fs";

const { E2E_PORT: PORT, E2E_CDP: CDP, E2E_DIST: DIST, E2E_SHOTS: SHOTS } = process.env;
const BASE = `http://127.0.0.1:${PORT}/`;
if (SHOTS) fs.mkdirSync(SHOTS, { recursive: true });

const readJSON = (p) => JSON.parse(fs.readFileSync(`${DIST}/${p}`, "utf8"));
const provinces = readJSON("index/provinces.json");
const jabar = provinces.find((p) => p.name === "Jawa Barat");
const regencies = readJSON(`index/regencies/${jabar.osm_id}.json`);
const bandung = regencies.find((r) => (r.official_name || r.name) === "Kota Bandung" || r.name === "Kota Bandung");
const districts = readJSON(`index/districts/${bandung.osm_id}.json`);
const andir = districts.find((d) => d.name === "Andir");
const villages = readJSON(`index/villages/${andir.osm_id}.json`);
const labelOf = (e) => e.official_name || e.name;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const target = (await (await fetch(`http://127.0.0.1:${CDP}/json/list`)).json()).find((t) => t.type === "page");
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));

let seq = 0;
const pending = new Map(), problems = [], results = [];
ws.onmessage = (m) => {
  const d = JSON.parse(m.data);
  if (d.id && pending.has(d.id)) { pending.get(d.id)(d); pending.delete(d.id); return; }
  if (d.method === "Runtime.exceptionThrown") problems.push("exception: " + (d.params.exceptionDetails.exception?.description || d.params.exceptionDetails.text));
  if (d.method === "Runtime.consoleAPICalled" && ["error", "warning"].includes(d.params.type)) problems.push("console." + d.params.type + ": " + d.params.args.map((a) => a.value ?? a.description).join(" "));
  if (d.method === "Log.entryAdded" && d.params.entry.level === "error") problems.push("log: " + d.params.entry.text + " " + (d.params.entry.url || ""));
};
const send = (method, params = {}) => new Promise((res) => { const id = ++seq; pending.set(id, res); ws.send(JSON.stringify({ id, method, params })); });
const ev = async (expr) => {
  const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
  if (r.result.exceptionDetails) throw new Error(JSON.stringify(r.result.exceptionDetails));
  return r.result.result.value;
};
const shot = async (name) => {
  if (!SHOTS) return;
  const r = await send("Page.captureScreenshot", { format: "png" });
  fs.writeFileSync(`${SHOTS}/${name}.png`, Buffer.from(r.result.data, "base64"));
};
const state = () => ev(`({
  hash: location.hash, act: document.querySelectorAll('#act path').length, ctx: document.querySelectorAll('#ctx path').length,
  under: document.querySelectorAll('#under path').length,
  crumbs: document.getElementById('crumbs').innerText.replace(/\\n/g, ' '), up: !document.getElementById('up').disabled })`);
const check = (name, ok, detail) => results.push({ name, ok: !!ok, detail });
const waitFor = async (expr, ms = 15000) => {
  const t = Date.now();
  while (Date.now() - t < ms) { if (await ev(expr)) return true; await sleep(150); }
  check("timed out waiting for " + expr, false, await state().catch(() => null));
  return false;
};
const depth = (s) => s.hash.split("/").length - 1; // "#/a/b" is two levels deep
const click = (e) => ev(`(() => { const p=[...document.querySelectorAll('#act path')].find(x=>x.getAttribute('aria-label').startsWith(${JSON.stringify(labelOf(e))})); if(!p) return false; p.dispatchEvent(new MouseEvent('click',{bubbles:true})); return true; })()`);
const settled = (n, count) => waitFor(`location.hash.split('/').length - 1 === ${n} && document.querySelectorAll('#act path').length === ${count}`).then(() => sleep(900));

await send("Page.enable"); await send("Runtime.enable"); await send("Log.enable");
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false });

// 1. The first view: every province, drawn from one small file.
await send("Page.navigate", { url: BASE });
await waitFor(`document.querySelectorAll('#act path').length > 0`); await sleep(800); await shot("1-provinces");
let s = await state();
check(`draws all ${provinces.length} provinces`, s.act === provinces.length, s);

// 2. Real mouse input: hover, wait (a re-append loop would show up as flicker), then click.
const pt = await ev(`(() => { const p=[...document.querySelectorAll('#act path')].find(x=>x.getAttribute('aria-label').startsWith('Jawa Barat')); const r=p.getBoundingClientRect();
  for (let i=1;i<24;i++) for (let j=1;j<24;j++) { const x=r.left+r.width*i/24, y=r.top+r.height*j/24; if (document.elementFromPoint(x,y)===p) return {x,y}; } return null; })()`);
check("finds a point inside Jawa Barat", pt, pt);
await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: pt.x, y: pt.y }); await sleep(300);
const hover = () => ev(`({ tip: !document.getElementById('tip').hidden, text: document.getElementById('tip').innerText, hl: document.querySelectorAll('#act path.hl').length })`);
let hv = await hover();
check("a real hover shows a tooltip and highlights one region", hv.tip && /Jawa Barat/.test(hv.text) && hv.hl === 1, hv);
await sleep(1200); hv = await hover();
check("the hover stays stable (no flicker loop)", hv.tip && hv.hl === 1, hv);
await shot("2-hover");
await send("Input.dispatchMouseEvent", { type: "mousePressed", x: pt.x, y: pt.y, button: "left", clickCount: 1 });
await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: pt.x, y: pt.y, button: "left", clickCount: 1 });
await settled(1, regencies.length); s = await state();
check(`a real click opens Jawa Barat: ${regencies.length} regencies`, depth(s) === 1 && s.act === regencies.length && s.crumbs.includes("Jawa Barat") && s.up, s);
await shot("3-regencies");

// 3. Down through the hierarchy.
check("clicks Kota Bandung", await click(bandung));
await settled(2, districts.length); s = await state();
check(`Kota Bandung shows ${districts.length} districts`, s.act === districts.length, s);
check("the focused region has a hatched gap underlay", s.under === 1, s);
const gapStyle = await ev(`(() => { const p = document.querySelector('#under path'); const cs = getComputedStyle(p); return { fill: cs.fill, pointerEvents: cs.pointerEvents }; })()`);
check("the gap underlay is hatched and not a click target",
  gapStyle.fill.includes("#nodata") && gapStyle.pointerEvents === "none", gapStyle);
await shot("4-districts");

check("clicks Andir", await click(andir));
await settled(3, villages.length); s = await state();
check(`Andir shows ${villages.length} villages`, s.act === villages.length, s); await shot("5-villages");

check("clicks a village", await click(villages[0]));
await waitFor(`location.hash.split('/').length - 1 === 4`); await sleep(900);
s = await state();
check("a village has nothing below it: siblings stay as context", s.act === 0 && s.ctx >= 1, s); await shot("6-leaf");
check("a village is the finest level: no missing-area hatch is drawn", s.under === 0, s);
const leafFill = await ev(`getComputedStyle(document.querySelector('#foc path')).fill`);
check("a selected village keeps its palette colour instead of a hatch", leafFill !== "none", leafFill);
const note = await ev(`document.getElementById('list').innerText.slice(0, 200)`);
check("a village reads as the finest level, not as missing data", /finest level/.test(note), note);

// 4. Ways back.
await ev(`history.back()`); await sleep(1500); s = await state();
check("the browser's Back button goes up one level", depth(s) === 3 && s.act === villages.length, s);
await ev(`document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))`); await sleep(1500); s = await state();
check("Esc goes up one level", depth(s) === 2 && s.act === districts.length, s);
await ev(`document.getElementById('reset').click()`); await sleep(1500); s = await state();
check("the Indonesia button resets the view", s.hash === "#/" && s.act === provinces.length, s);

// 5. Search.
await ev(`(() => { const q=document.getElementById('q'); q.focus(); q.value='bandung'; q.dispatchEvent(new Event('input', { bubbles: true })); })()`);
await waitFor(`document.querySelectorAll('#hits button').length > 0`);
const hits = await ev(`[...document.querySelectorAll('#hits button')].map(b => b.innerText.replace(/\\s+/g, ' '))`);
check("search finds Bandung", hits.some((x) => x.startsWith("Kota Bandung")), hits); await shot("7-search");
await ev(`[...document.querySelectorAll('#hits button')].find(b => b.innerText.startsWith('Kota Bandung')).click()`);
await settled(2, districts.length); s = await state();
check("picking Kota Bandung from search shows its districts", s.act === districts.length && s.crumbs.includes("Kota Bandung"), s);

// 6. A shared link opens the same view.
const link = s.hash;
await send("Page.navigate", { url: "about:blank" }); await sleep(300); // leave the page, then come back by URL
await send("Page.navigate", { url: BASE + link }); await waitFor(`document.querySelectorAll('#act path').length > 0`); await sleep(1500);
const s2 = await state();
check("a shared link restores the same view", s2.hash === link && s2.act === s.act, { link, s2 });

// 7. Dark mode and a phone.
await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "dark" }] });
await send("Page.navigate", { url: "about:blank" }); await sleep(300);
await send("Page.navigate", { url: BASE + "#/" + jabar.osm_id });
await waitFor(`document.querySelectorAll('#act path').length === ${regencies.length}`); await sleep(1000); await shot("8-dark");
await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "light" }] });
await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 2, mobile: true });
await send("Page.navigate", { url: BASE }); await waitFor(`document.querySelectorAll('#act path').length > 0`); await sleep(900); await shot("9-phone");
const overflow = await ev(`document.documentElement.scrollWidth - document.documentElement.clientWidth`);
check("no horizontal scrolling on a phone", overflow <= 1, overflow);

// 8. A region with no children is a normal coloured region, not a gap.
let childless = null;
for (const p of provinces) {
  const r = readJSON(`index/regencies/${p.osm_id}.json`).find((x) => !x.children);
  if (r) { childless = { p, r }; break; }
}
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false });
await send("Page.navigate", { url: BASE + "#/" + childless.p.osm_id + "/" + childless.r.osm_id });
await waitFor(`document.querySelectorAll('#foc path').length === 1`); await sleep(900);
const sc = await state();
check("a regency with no districts gets no hatch", sc.act === 0 && sc.under === 0, { regency: childless.r.name, ...sc });
const childlessFill = await ev(`getComputedStyle(document.querySelector('#foc path')).fill`);
check("the childless regency is filled with its palette colour", childlessFill !== "none", childlessFill);
await shot("10-childless-regency");

check("no console errors or exceptions", problems.length === 0, problems);

let failed = 0;
for (const r of results) { if (!r.ok) failed++; console.log((r.ok ? "PASS " : "FAIL ") + r.name + (r.ok ? "" : "\n     " + JSON.stringify(r.detail))); }
console.log(`\n${results.length - failed} of ${results.length} checks passed`);
ws.close();
process.exit(failed ? 1 : 0);
