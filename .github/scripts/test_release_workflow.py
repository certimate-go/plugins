import json
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


WORKFLOW = Path(__file__).resolve().parents[1] / "workflows" / "release.yml"


def detection_script():
    lines = WORKFLOW.read_text().splitlines()
    start = lines.index("      - id: detect")
    start = lines.index("        run: |", start) + 1
    end = start
    while end < len(lines) and (not lines[end] or lines[end].startswith("          ")):
        end += 1
    return textwrap.dedent("\n".join(lines[start:end]))


class ReleaseDetectionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Release test")
        self.git("config", "user.email", "release@example.test")
        for provider in ["alpha", "beta", "_template"]:
            path = self.root / provider / "manifest.json"
            path.parent.mkdir()
            path.write_text(json.dumps({
                "provider_type": provider,
                "version": "0.1.0",
                "release": {"tag": provider + "/v0.1.0"},
            }))
        (self.root / "go.mod").write_text("module example.test/plugins\n")
        self.before = self.commit()

    def git(self, *args):
        return subprocess.check_output(
            ["git", *args], cwd=self.root, text=True, stderr=subprocess.STDOUT
        ).strip()

    def commit(self):
        self.git("add", "alpha", "beta", "_template", "go.mod")
        self.git("commit", "-qm", "Fixture state")
        return self.git("rev-parse", "HEAD")

    def detect(self, before=None, requested=""):
        output = self.root / "github-output"
        env = os.environ | {
            "BEFORE": self.before if before is None else before,
            "AFTER": self.git("rev-parse", "HEAD"),
            "INPUT_PLUGINS": requested,
            "GITHUB_OUTPUT": str(output),
        }
        subprocess.run(
            ["bash", "-ec", detection_script()], cwd=self.root, env=env,
            check=True, text=True, capture_output=True,
        )
        return dict(line.split("=", 1) for line in output.read_text().splitlines())

    def test_core_dependency_update_selects_all_plugins(self):
        (self.root / "go.mod").write_text("module example.test/plugins\n\ngo 1.27.4\n")
        self.commit()
        result = self.detect()
        self.assertEqual(result["plugins"], "alpha,beta")
        self.assertEqual(result["shared_build_change"], "1")

    def test_plugin_change_selects_only_that_plugin(self):
        (self.root / "alpha" / "main.go").write_text("package main\n")
        self.commit()
        self.assertEqual(self.detect()["plugins"], "alpha")

    def test_shared_helper_change_selects_all_plugins(self):
        helper = self.root / "internal" / "shared.go"
        helper.parent.mkdir()
        helper.write_text("package internal\n")
        self.git("add", "internal/shared.go")
        self.commit()
        self.assertEqual(self.detect()["plugins"], "alpha,beta")

    def test_initial_release_excludes_untracked_test_plugins(self):
        fixture = self.root / "mockdns" / "manifest.json"
        fixture.parent.mkdir()
        fixture.write_text("{}")
        self.assertEqual(self.detect(before="")["plugins"], "alpha,beta")

    def test_explicit_selection_stays_scoped(self):
        self.assertEqual(self.detect(requested="beta")["plugins"], "beta")

    def test_unpublished_plugin_is_found_after_unrelated_change(self):
        manifest = self.root / "beta" / "manifest.json"
        fields = json.loads(manifest.read_text())
        fields.pop("release")
        manifest.write_text(json.dumps(fields))
        self.before = self.commit()
        (self.root / "alpha" / "main.go").write_text("package main\n")
        self.commit()
        self.assertEqual(self.detect()["plugins"], "alpha,beta")


if __name__ == "__main__":
    unittest.main()
