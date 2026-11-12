import assert from "node:assert/strict";
import fs from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const pluginRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const uiRoot = process.env.CERTIMATE_CORE_UI || path.resolve(pluginRoot, "../certimate/ui");
const requireUI = createRequire(path.join(uiRoot, "package.json"));
const ts = requireUI("typescript");
const modules = new Map();

function loadSource(file) {
  if (modules.has(file)) return modules.get(file).exports;
  const module = { exports: {} };
  modules.set(file, module);
  const source = ts.transpileModule(fs.readFileSync(file, "utf8"), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
  }).outputText;
  const requireSource = (name) => {
    if (name === "@/i18n") return { default: { t: (key) => key }, __esModule: true };
    if (name.startsWith("@/")) return loadSource(path.join(uiRoot, "src", `${name.slice(2)}.ts`));
    if (name.startsWith(".")) return loadSource(path.resolve(path.dirname(file), `${name}.ts`));
    return requireUI(name);
  };
  new Function("require", "module", "exports", source)(requireSource, module, module.exports);
  return module.exports;
}

const { deriveZodSchema } = loadSource(path.join(uiRoot, "src/components/workflow/designer/forms/schemaValidation.ts"));
const envelope = JSON.parse(fs.readFileSync(path.join(pluginRoot, "ksyun-cdn/schema/deploy.json"), "utf8"));
const schema = deriveZodSchema(envelope);
const scenarios = [
  ["SAN matching permits an empty domain", { deployTarget: "domain", domainMatchPattern: "certsan", domain: "" }, true],
  ["SAN matching permits an omitted domain", { deployTarget: "domain", domainMatchPattern: "certsan" }, true],
  ["certificate replacement ignores a retained matching mode", { deployTarget: "certificate", domainMatchPattern: "exact", domain: "", certificateId: "cert-1" }, true],
  ["exact matching accepts a valid domain", { deployTarget: "domain", domainMatchPattern: "exact", domain: "cdn.example.com" }, true],
  ["wildcard matching accepts a valid pattern", { deployTarget: "domain", domainMatchPattern: "wildcard", domain: "*.example.com" }, true],
  ["domain deployment rejects a malformed domain", { deployTarget: "domain", domainMatchPattern: "exact", domain: "not a domain" }, false],
  ["certificate replacement requires its target ID", { deployTarget: "certificate", domainMatchPattern: "exact", domain: "" }, false],
];

for (const [name, config, expected] of scenarios) {
  const result = schema.safeParse(config);
  assert.equal(result.success, expected, `${name}: ${JSON.stringify(result.error?.issues)}`);
  console.log(`PASS ${name}`);
}
