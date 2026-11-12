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

const cases = [
 ["bunny-cdn", {pullZoneId:"42",hostname:"edge.example.com"}, true],
 ["bunny-cdn", {pullZoneId:42,hostname:"*.example.com"}, true],
 ["bunny-cdn", {pullZoneId:"legacy-id",hostname:"edge.example.com"}, true],
 ["bunny-cdn", {pullZoneId:-1,hostname:"edge.example.com"}, false],
 ["bunny-cdn", {pullZoneId:"42",hostname:"not a domain"}, false],
 ["cloudflare-ssl", {zoneId:"zone", environment:"production",certificateId:"cert"}, true],
 ["cloudflare-ssl", {zoneId:"zone", environment:"custom-future",certificateId:null}, true],
 ["cloudflare-ssl", {zoneId:"zone"}, true],
 ["cloudflare-ssl", {zoneId:""}, false],
 ["cpanel", {deployTarget:"website",domain:"edge.example.com"}, true],
 ["cpanel", {deployTarget:"website",domain:"*.example.com"}, false],
 ["cpanel", {deployTarget:"website"}, false],
 ["cpanel", {deployTarget:"other",domain:"edge.example.com"}, false],
 ["digitalocean-certificate", {}, true],
];
for (const [provider,value,expected] of cases) {
 const envelope=JSON.parse(fs.readFileSync(path.join(pluginRoot,provider,"schema/deploy.json")));
 const result=deriveZodSchema(envelope).safeParse(value);
 assert.equal(result.success,expected,JSON.stringify({provider,value,issues:result.error?.issues}));
 if(result.success) for(const key of Object.keys(value)) assert.deepEqual(result.data[key],value[key]);
}
console.log(`PASS ${cases.length} edge form cases`);
