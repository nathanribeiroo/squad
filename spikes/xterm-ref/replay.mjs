// Replay the first <offset> bytes of a raw PTY capture into @xterm/headless
// and print the visible screen (right-trimmed lines, trailing blank lines dropped).
import { readFileSync } from "node:fs";
import xterm from "@xterm/headless";
const { Terminal } = xterm;

const [rawPath, cols, rows, offset] = process.argv.slice(2);
const data = readFileSync(rawPath).subarray(0, Number(offset));
const term = new Terminal({ cols: Number(cols), rows: Number(rows), allowProposedApi: true, scrollback: 0 });
term.write(data, () => {
  const buf = term.buffer.active;
  const lines = [];
  for (let y = 0; y < term.rows; y++) {
    lines.push((buf.getLine(buf.viewportY + y)?.translateToString(true) ?? "").replace(/\s+$/u, ""));
  }
  while (lines.length && lines[lines.length - 1] === "") lines.pop();
  process.stdout.write(lines.join("\n"));
});
