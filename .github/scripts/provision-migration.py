import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time

REGISTRY = "certimate-go/plugins"
SAMPLE = "certimate-go/plugin-provider-volcengine-dns-acmedns01challenger"
RULES = {
    "Provider protected main",
    "Formal plugin tag writer",
    "Immutable formal plugin tags",
    "Immutable plugin release requests",
}
RULE_FIELDS = ("name", "target", "enforcement", "conditions", "rules", "bypass_actors")


def run(command, payload=None, allow_missing=False):
    result = subprocess.run(
        command, input=payload, text=True, capture_output=True, timeout=180
    )
    if result.returncode == 0:
        return result.stdout
    if allow_missing and "HTTP 404" in result.stderr:
        return None
    raise RuntimeError(result.stderr.strip() or f"{command[0]} returned {result.returncode}")


def api(endpoint, method="GET", body=None, allow_missing=False):
    command = ["gh", "api", "--method", method, endpoint]
    if body is not None:
        command += ["--input", "-"]
    response = run(command, json.dumps(body) if body is not None else None, allow_missing)
    return json.loads(response) if response else None


def rule_templates():
    rules = api(f"repos/{SAMPLE}/rulesets")
    templates = {
        rule["name"]: api(f"repos/{SAMPLE}/rulesets/{rule['id']}")
        for rule in rules
        if rule["name"] in RULES and rule["source"] == SAMPLE
    }
    if templates.keys() != RULES or any(rule["enforcement"] != "active" for rule in templates.values()):
        raise RuntimeError("Official provider ruleset templates do not match")
    return {
        name: {field: rule[field] for field in RULE_FIELDS}
        for name, rule in templates.items()
    }


def protected_environment(repo, private_key, client_id):
    endpoint = f"repos/{repo}/environments/provider-release"
    policy = {"protected_branches": False, "custom_branch_policies": True}
    environment = api(endpoint, allow_missing=True)
    if environment is None:
        api(endpoint, "PUT", {"deployment_branch_policy": policy})
    elif environment.get("deployment_branch_policy") != policy or any(
        rule["type"] != "branch_policy" for rule in environment["protection_rules"]
    ):
        raise RuntimeError(f"Unexpected provider-release environment in {repo}")
    branches = api(endpoint + "/deployment-branch-policies")
    existing = {(row["name"], row["type"]) for row in branches["branch_policies"]}
    if existing - {("main", "branch")}:
        raise RuntimeError(f"Unexpected deployment branch policy in {repo}")
    if not existing:
        api(endpoint + "/deployment-branch-policies", "POST", {"name": "main", "type": "branch"})
    variable = api(endpoint + "/variables/REGISTRY_RELEASE_APP_CLIENT_ID", allow_missing=True)
    if variable is None:
        api(endpoint + "/variables", "POST", {"name": "REGISTRY_RELEASE_APP_CLIENT_ID", "value": client_id})
    elif variable["value"] != client_id:
        raise RuntimeError(f"Unexpected release App client ID in {repo}")
    secrets = api(endpoint + "/secrets")
    if not any(secret["name"] == "REGISTRY_RELEASE_APP_PRIVATE_KEY" for secret in secrets["secrets"]):
        run(
            ["gh", "secret", "set", "REGISTRY_RELEASE_APP_PRIVATE_KEY", "--repo", repo, "--env", "provider-release"],
            private_key,
        )


def provision(entry, directory, templates, installation_id, client_id, private_key):
    repo = entry["repository"]
    name = repo.split("/", 1)[1]
    endpoint = f"repos/{repo}"
    metadata = api(endpoint, allow_missing=True)
    if metadata is None:
        metadata = api("orgs/certimate-go/repos", "POST", {"name": name, "private": False, "auto_init": False})
    if metadata["full_name"] != repo or metadata["private"] or metadata["archived"] or metadata["default_branch"] != "main":
        raise RuntimeError(f"Unexpected repository state for {repo}")
    branch = api(endpoint + "/branches/main", allow_missing=True)
    if branch is None:
        remote = f"git@github.com:{repo}.git"
        run(["git", "-C", str(directory), "push", remote, entry["target_commit"] + ":refs/heads/main"])
    elif branch["commit"]["sha"] != entry["target_commit"]:
        raise RuntimeError(f"Protected source commit differs for {repo}")

    existing = {
        rule["name"]: rule["id"]
        for rule in api(endpoint + "/rulesets")
        if rule["source"] == repo
    }
    for name, template in templates.items():
        if name not in existing:
            api(endpoint + "/rulesets", "POST", template)
            continue
        remote = api(f"{endpoint}/rulesets/{existing[name]}")
        if {field: remote[field] for field in RULE_FIELDS} != template:
            raise RuntimeError(f"Unexpected ruleset {name} in {repo}")

    api(f"user/installations/{installation_id}/repositories/{metadata['id']}", "PUT")
    protected_environment(repo, private_key, client_id)
    branch = api(endpoint + "/branches/main")
    if not branch["protected"] or branch["commit"]["sha"] != entry["target_commit"]:
        raise RuntimeError(f"Expected protected target revision in {repo}")


def wait_for_ci(entries):
    pending = list(entries)
    ready = []
    deadline = time.monotonic() + 600
    while pending and time.monotonic() < deadline:
        waiting = []
        for entry in pending:
            repo = entry["repository"]
            commit = entry["target_commit"]
            workflow = api(
                f"repos/{repo}/actions/workflows/ci.yml/runs?branch=main&event=push&head_sha={commit}&per_page=1",
                allow_missing=True,
            )
            runs = workflow["workflow_runs"] if workflow else []
            if runs and runs[0]["conclusion"] == "success":
                ready.append(entry)
            elif not runs or runs[0]["status"] != "completed":
                waiting.append(entry)
            else:
                print(f"{repo}: Provider CI {runs[0]['conclusion']}", file=sys.stderr)
        pending = waiting
        if pending:
            time.sleep(10)
    for entry in pending:
        print(f"{entry['repository']}: timed out waiting for Provider CI", file=sys.stderr)
    return ready


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("batch", type=Path)
    parser.add_argument("--providers-dir", type=Path, required=True)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--dispatch", action="store_true")
    args = parser.parse_args()
    if args.dispatch and not args.apply:
        parser.error("--dispatch requires --apply")
    protected_main = api(f"repos/{REGISTRY}/branches/main")["commit"]["sha"]
    body = run(["go", "run", "-mod=readonly", "./cmd/registryctl", "plan-admission", "-base", protected_main, "-batch", str(args.batch)])
    entries = json.loads(body)["entries"]
    if args.apply and entries:
        app = next(
            item for item in api("orgs/certimate-go/installations")["installations"]
            if item["app_slug"] == "certimate-plugin-dispatcher"
        )
        client_id = run([
            "gh", "variable", "get", "REGISTRY_RELEASE_APP_CLIENT_ID", "--repo",
            REGISTRY, "--env", "registry-reconcile", "--json", "value", "--jq", ".value"
        ]).strip()
        if not client_id:
            raise RuntimeError("Missing protected release App client ID")
        private_key = Path(os.environ["REGISTRY_RELEASE_APP_PRIVATE_KEY_FILE"]).read_text()
        templates = rule_templates()
    ready = []
    for entry in entries:
        directory = args.providers_dir / entry["repository"].split("/", 1)[1]
        try:
            commit = entry["target_commit"]
            if run(["git", "-C", str(directory), "rev-parse", "main"]).strip() != commit:
                raise RuntimeError("The local main does not match the migration commit")
            if run(["git", "-C", str(directory), "rev-parse", f"{commit}^{{tree}}"]).strip() != entry["target_tree"]:
                raise RuntimeError("The local tree does not match the migration manifest")
            if args.apply:
                provision(entry, directory, templates, app["id"], client_id, private_key)
            ready.append(entry)
        except (OSError, RuntimeError, subprocess.TimeoutExpired) as error:
            print(f"{entry['repository']}: {error}", file=sys.stderr)
    if args.dispatch and ready:
        ready = wait_for_ci(ready)
        if ready:
            run(
                ["gh", "workflow", "run", "registry-admit.yml", "--repo", REGISTRY, "--ref", "main", "--json"],
                json.dumps({"batch": json.dumps(ready)}),
            )
    print(json.dumps(ready))
    return int(len(ready) != len(entries))


if __name__ == "__main__":
    sys.exit(main())
