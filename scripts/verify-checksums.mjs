import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const directory = resolve(root, process.argv[2] ?? "dist/release");
const manifest = resolve(directory, process.argv[3] ?? "SHA256SUMS");

if (!existsSync(manifest)) {
  console.error(`checksum manifest not found: ${manifest}`);
  process.exit(2);
}

const lines = readFileSync(manifest, "utf8")
  .split(/\r?\n/)
  .filter(Boolean);

if (lines.length === 0) {
  console.error("checksum manifest is empty");
  process.exit(2);
}

for (const line of lines) {
  const match = /^([a-f0-9]{64})  (.+)$/.exec(line);
  if (match === null) {
    console.error(`invalid checksum line: ${line}`);
    process.exit(2);
  }
  const [, expected, name] = match;
  const file = resolve(directory, name);
  if (!existsSync(file)) {
    console.error(`checksum subject not found: ${name}`);
    process.exit(1);
  }
  const actual = createHash("sha256").update(readFileSync(file)).digest("hex");
  if (actual !== expected) {
    console.error(`checksum mismatch: ${name}`);
    process.exit(1);
  }
}

console.log(`verified ${lines.length} SHA-256 checksums from ${manifest}`);
