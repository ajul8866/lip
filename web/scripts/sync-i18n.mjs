#!/usr/bin/env node
// Copies the backend-owned locale JSON into the web app before typecheck/build.
// The backend files are the single source of truth; this keeps UI copy out of
// component code and prevents EN/KO drift.
import { cpSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const source = join(here, "..", "..", "internal", "i18n", "locales");
const target = join(here, "..", "src", "i18n", "locales");
mkdirSync(target, { recursive: true });
cpSync(join(source, "en.json"), join(target, "en.json"));
cpSync(join(source, "ko.json"), join(target, "ko.json"));

const en = JSON.parse(readFileSync(join(source, "en.json"), "utf8"));
const ko = JSON.parse(readFileSync(join(source, "ko.json"), "utf8"));
const enKeys = Object.keys(en).sort();
const koKeys = Object.keys(ko).sort();
if (JSON.stringify(enKeys) !== JSON.stringify(koKeys)) {
  console.error("i18n key mismatch between en.json and ko.json");
  process.exit(1);
}
console.log(`i18n: ${enKeys.length} keys synced`);
