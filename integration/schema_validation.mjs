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

const flexEnvelope = JSON.parse(fs.readFileSync(path.join(pluginRoot, "flexcdn/schema/deploy.json"), "utf8"));
const flexSchema = deriveZodSchema(flexEnvelope);
const flexScenarios = [
  ["FlexCDN accepts a saved string ID", "42", true],
  ["FlexCDN accepts a numeric ID", 42, true],
  ["FlexCDN accepts the maximum safe integer", "9007199254740991", true],
  ["FlexCDN rejects an unsafe string ID", "9007199254740993", false],
  ["FlexCDN rejects an unsafe numeric ID", 9007199254740992, false],
  ["FlexCDN rejects a fractional ID", 1.5, false],
  ["FlexCDN rejects a negative ID", -1, false],
  ["FlexCDN rejects zero", 0, false],
  ["FlexCDN requires an ID", undefined, false],
];

for (const [name, certificateId, expected] of flexScenarios) {
  const result = flexSchema.safeParse({ deployTarget: "certificate", certificateId });
  assert.equal(result.success, expected, `${name}: ${JSON.stringify(result.error?.issues)}`);
  if (result.success) assert.equal(result.data.certificateId, Number(certificateId));
  console.log(`PASS ${name}`);
}

const b1Cases = [
  ["asiaispcdn", "deploy", {domainMatchPattern: "exact", domain: "example.com"}, true],
  ["asiaispcdn", "deploy", {domainMatchPattern: "wildcard", domain: "*.example.com"}, true],
  ["asiaispcdn", "deploy", {domainMatchPattern: "certsan", domain: ""}, true],
  ["asiaispcdn", "deploy", {domainMatchPattern: "exact", domain: "bad domain"}, false],
  ["cdnfly", "deploy", {deployTarget: "website", siteId: "42"}, true],
  ["cdnfly", "deploy", {deployTarget: "website", siteId: 42}, true],
  ["cdnfly", "deploy", {deployTarget: "certificate", certificateId: "42"}, true],
  ["cdnfly", "deploy", {deployTarget: "certificate", certificateId: 42}, true],
  ["cdnfly", "deploy", {deployTarget: "website", certificateId: "42"}, false],
  ["cdnfly", "deploy", {deployTarget: "certificate", certificateId: "bad"}, false],
  ["cdnfly", "deploy", {deployTarget: "certificate", certificateId: "1.5"}, false],
  ["lecdn", "deploy", {deployTarget: "certificate", certificateId: "42", clientId: ""}, true],
  ["lecdn", "deploy", {deployTarget: "certificate", certificateId: 42, clientId: 7}, true],
  ["lecdn", "deploy", {deployTarget: "certificate", certificateId: "42", clientId: "saved-legacy"}, true],
  ["lecdn", "deploy", {deployTarget: "certificate", certificateId: 0}, false],
  ["mohua-mvh", "deploy", {hostId: "42", domainId: "7"}, true],
  ["mohua-mvh", "deploy", {hostId: 42, domainId: 7}, true],
  ["mohua-mvh", "deploy", {hostId: "", domainId: "7"}, false],
  ["mohua-mvh", "deploy", {hostId: 0, domainId: "7"}, false],
  ["unicloud-webhost", "deploy", {spaceProvider: "tencent", spaceId: "space", domain: "example.com"}, true],
  ["unicloud-webhost", "deploy", {spaceProvider: "aliyun", spaceId: "space", domain: "*.example.com"}, false],
  ["unicloud-webhost", "deploy", {spaceProvider: "unsupported", spaceId: "space", domain: "example.com"}, false],
  ["flyio", "deploy", {appName: "app", domain: "*.example.com"}, true],
  ["flyio", "deploy", {appName: "", domain: "example.com"}, false],
];
for (const authMethod of ["password", "apikey"]) {
  for (const apiRole of ["client", "master"]) {
    for (const apiVersion of ["v3", "v4"]) {
      const access = {serverUrl: "https://example.com", apiVersion, apiRole, authMethod,
        ...(authMethod === "password" ? {username: "user", password: "fixture-password"} : {apiKey: "fixture-key"})};
      b1Cases.push(["lecdn", "access", access, true]);
      b1Cases.push(["lecdn", "access", {...access, [authMethod === "password" ? "password" : "apiKey"]: ""}, false]);
    }
  }
}
for (const [provider, category, config, expected] of b1Cases) {
  const form = JSON.parse(fs.readFileSync(path.join(pluginRoot, provider, `schema/${category}.json`), "utf8"));
  const result = deriveZodSchema(form).safeParse(config);
  assert.equal(result.success, expected, `${provider}/${category} ${JSON.stringify(config)}: ${JSON.stringify(result.error?.issues)}`);
  if (result.success) {
    for (const field of ["hostId", "domainId", "siteId", "certificateId", "clientId"]) {
      if (field in config) assert.equal(result.data[field], config[field], `${provider} changed ${field} JSON type`);
    }
  }
}
console.log(`PASS ${b1Cases.length} B1 schema compatibility cases`);

const loadDeployForm = (provider) =>
  deriveZodSchema(JSON.parse(fs.readFileSync(path.join(pluginRoot, provider, "schema/deploy.json"), "utf8")));

const rainyunRCDN = loadDeployForm("rainyun-rcdn");
for (const instanceId of ["42", 42]) {
  assert.deepEqual(rainyunRCDN.parse({ instanceId, domain: "*.example.com" }), {
    instanceId,
    domainMatchPattern: "exact",
    domain: "*.example.com",
  });
}
assert.equal(rainyunRCDN.safeParse({ instanceId: "42", domain: "bad domain" }).success, false);

const rainyunSSL = loadDeployForm("rainyun-sslcenter");
assert.deepEqual(rainyunSSL.parse({}), {});
for (const certificateId of ["42", 42]) {
  assert.deepEqual(rainyunSSL.parse({ certificateId }), { certificateId });
}

const baiduCDN = loadDeployForm("baiducloud-cdn");
assert.deepEqual(baiduCDN.parse({ domain: "example.com" }), { domainMatchPattern: "exact", domain: "example.com" });
assert.deepEqual(baiduCDN.parse({ domainMatchPattern: "certsan" }), { domainMatchPattern: "certsan" });
assert.equal(baiduCDN.safeParse({ domain: "bad domain" }).success, false);

for (const provider of ["baiducloud-appblb", "baiducloud-blb"]) {
  const schema = loadDeployForm(provider);
  const lb = { region: "bj", loadbalancerId: "lb-42" };
  for (const listenerPort of ["443", 443]) {
    const config = { ...lb, deployTarget: "listener", listenerPort, domain: "*.example.com" };
    assert.deepEqual(schema.parse(config), config);
  }
  const listener = { ...lb, deployTarget: "listener" };
  assert.equal(schema.safeParse({ ...listener, listenerPort: "65536" }).success, false);
  assert.deepEqual(schema.parse({ ...lb, deployTarget: "loadbalancer" }), { ...lb, deployTarget: "loadbalancer" });
}
assert.deepEqual(loadDeployForm("baiducloud-cert").parse({}), {});
console.log("PASS B4 form defaults, saved ID/port types, and conditional validation");

const vlbEnvelope = JSON.parse(fs.readFileSync(path.join(pluginRoot, "cmcccloud-vlb/schema/deploy.json"), "utf8"));
const vlbSchema = deriveZodSchema(vlbEnvelope);
const loadbalancer = { poolId: "CIDC-CORE-00", loadbalancerId: "vlb-1" };
const listener = { ...loadbalancer, deployTarget: "listener", listenerId: "listener-1" };
assert.equal(vlbSchema.parse(listener).loadbalancerId, "vlb-1");
assert.equal(vlbSchema.safeParse({ ...listener, loadbalancerId: "" }).success, false);
assert.equal(vlbSchema.safeParse({ ...listener, listenerId: "" }).success, false);
assert.equal(vlbSchema.safeParse({ ...loadbalancer, deployTarget: "loadbalancer" }).success, true);
assert.equal(vlbSchema.safeParse({ ...loadbalancer, deployTarget: "loadbalancer", loadbalancerId: "" }).success, false);
console.log("PASS CMCC VLB requires and preserves the load balancer ID for both targets");
