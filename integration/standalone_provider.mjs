import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const workspace = process.env.CERTIMATE_PLUGIN_WORKSPACE || fileURLToPath(new URL("../../", import.meta.url));

export function standaloneProvider(provider) {
  const directoryName = "plugin-provider-" + provider;
  for (const entry of fs.readdirSync(workspace, { withFileTypes: true })) {
    if (!entry.isDirectory() || (entry.name !== directoryName && !entry.name.startsWith(directoryName + "-"))) continue;
    const directory = path.join(workspace, entry.name);
    const manifest = path.join(directory, "manifest.json");
    if (fs.existsSync(manifest) && JSON.parse(fs.readFileSync(manifest, "utf8")).provider_type === provider) return directory;
  }
  throw new Error(`Initialize the standalone ${provider} submodule or set CERTIMATE_PLUGIN_WORKSPACE`);
}
