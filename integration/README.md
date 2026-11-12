# Verify the cloud deployment plugins

Run the production-process tests from the repository root.

```sh
go test ./integration -run '^TestCloudDeploymentBatch$' -count=1 -v
```

The suite builds Ksyun CDN, KCM and SLB, Wangsu CDN, CDN Pro and Certificate, and Baishan CDN. It discovers and invokes the binaries through the core plugin manager over gRPC. Generated certificates and a local TLS proxy simulate the vendor APIs and record uploaded certificates, keys, replacements, and bindings.

The 40 scenarios cover successful deployments, malformed configuration, API failures and recovery, certificate reuse and replacement, domain matching, CDN Pro encryption and polling, and Baishan HTTPS settings preservation. They do not exercise live vendor accounts.

Linux runs the binaries directly. On other platforms the suite requires Docker and runs Linux binaries with networking disabled. Set `CERTIMATE_E2E_IMAGE` to override the default `golang:1.26-bookworm` image.

With the core frontend dependencies installed, check the Ksyun CDN schema against its actual validator.

```sh
CERTIMATE_CORE_UI=/path/to/certimate/ui node integration/schema_validation.mjs
```

The default core frontend path is `../certimate/ui`. The seven cases cover SAN matching without a domain, certificate replacement with a retained matching mode, valid domain matching, malformed domains, and a missing certificate ID. Exact and wildcard matching still require a domain at deployment time.
