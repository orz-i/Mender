import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { relative, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const directory = resolve(root, process.argv[2] ?? "dist/release");
const output = resolve(directory, process.argv[3] ?? "SHA256SUMS");

function walk(path) {
  const files = [];
  for (const entry of readdirSync(path).sort()) {
    const full = resolve(path, entry);
    if (full === output) {
      continue;
    }
    const stat = statSync(full);
    if (stat.isDirectory()) {
      files.push(...walk(full));
    } else if (stat.isFile()) {
      files.push(full);
    }
  }
  return files;
}

const lines = walk(directory).map((file) => {
  const digest = createHash("sha256").update(readFileSync(file)).digest("hex");
  const name = relative(directory, file).replaceAll("\\", "/");
  return `${digest}  ${name}`;
});

writeFileSync(output, lines.join("\n") + "\n");
console.log(`wrote ${lines.length} SHA-256 checksums to ${output}`);
