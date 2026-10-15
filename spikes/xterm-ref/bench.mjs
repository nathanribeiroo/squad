// Memory per @xterm/headless terminal and parse throughput.
import { readFileSync } from "node:fs";
import xterm from "@xterm/headless";
const { Terminal } = xterm;
const data = readFileSync(process.argv[2]);
const n = Number(process.argv[3]);
global.gc?.();
const before = process.memoryUsage();
const terms = [];
const t0 = performance.now();
for (let i = 0; i < n; i++) {
  const t = new Terminal({ cols: 120, rows: 36, scrollback: Number(process.env.SB), allowProposedApi: true });
  for (let k = 0; k < 4; k++) await new Promise((r) => t.write(data, r));
  terms.push(t);
}
const el = (performance.now() - t0) / 1000;
global.gc?.();
const after = process.memoryUsage();
const mb = (data.length * 4 * n) / 1e6;
console.log(`sb_lines=${terms[n-1].buffer.normal.length - 36} xterm: ${n} terminals, fed ${mb.toFixed(1)} MB in ${el.toFixed(2)}s (${(mb / el).toFixed(1)} MB/s), heap per terminal ${((after.heapUsed - before.heapUsed) / 1e6 / n).toFixed(2)} MB, process rss ${(after.rss / 1e6).toFixed(1)} MB`);
