import { compile } from "@tailwindcss/node";
import { Scanner } from "@tailwindcss/oxide";
import { readFileSync } from "node:fs";

const base = process.cwd() + "/src";
const scanner = new Scanner({ sources: [{ base, pattern: "**/*.{ts,tsx}", negated: false }] });
const candidates = scanner.scan();

async function generated(file) {
  const css = readFileSync(base + "/" + file, "utf8");
  const c = await compile(css, { base, onDependency() {} });
  const empty = c.build([]).length;
  const out = new Set();
  for (const k of candidates) {
    const fresh = await compile(css, { base, onDependency() {} });
    if (fresh.build([k]).length > empty) out.add(k);
  }
  return out;
}

const before = await generated("index.base-probe.css");
const after = await generated("index.css");
console.log("lost:", [...before].filter((k) => !after.has(k)).join(" "));
console.log("gained:", [...after].filter((k) => !before.has(k)).join(" "));
