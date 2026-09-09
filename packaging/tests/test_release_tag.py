import importlib.util
import pathlib
import subprocess
import sys
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).parents[1] / "release_tag.py"
sys.path.insert(0, str(MODULE_PATH.parent))
SPEC = importlib.util.spec_from_file_location("headroom_release_tag", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
SPEC.loader.exec_module(MODULE)


class ReleaseTagTests(unittest.TestCase):
    def repository(self, root: pathlib.Path) -> pathlib.Path:
        subprocess.run(["git", "init", "-q", root], check=True)
        subprocess.run(["git", "-C", root, "config", "user.name", "Fixture"], check=True)
        subprocess.run(["git", "-C", root, "config", "user.email", "fixture@example.invalid"], check=True)
        (root / "file").write_text("fixture\n", encoding="utf-8")
        subprocess.run(["git", "-C", root, "add", "file"], check=True)
        subprocess.run(["git", "-C", root, "commit", "-qm", "fixture"], check=True)
        return root

    def test_reuses_annotated_stable_tag_and_preserves_message(self):
        with tempfile.TemporaryDirectory() as temporary:
            repo = self.repository(pathlib.Path(temporary))
            self.assertIsNone(MODULE.find_existing(repo, "HEAD"))
            subprocess.run(
                ["git", "-C", repo, "tag", "-a", "v1.2.3+build.7", "-m", "Release notes\n\nDetails"],
                check=True,
            )
            self.assertEqual(
                MODULE.find_existing(repo, "HEAD"),
                ("v1.2.3+build.7", "1.2.3+build.7", "Release notes\n\nDetails"),
            )
            output = repo / "github-output"
            MODULE.append_output(output, MODULE.find_existing(repo, "HEAD"))
            written = output.read_text(encoding="utf-8")
            self.assertIn("reused=true\ntag=v1.2.3+build.7\nversion=1.2.3+build.7\n", written)
            self.assertIn("\nRelease notes\n\nDetails\nHEADROOM_CHANGELOG_", written)

    def test_rejects_ambiguous_stable_tags_on_same_commit(self):
        with tempfile.TemporaryDirectory() as temporary:
            repo = self.repository(pathlib.Path(temporary))
            subprocess.run(["git", "-C", repo, "tag", "v1.2.3"], check=True)
            subprocess.run(["git", "-C", repo, "tag", "v1.2.4"], check=True)
            with self.assertRaises(ValueError):
                MODULE.find_existing(repo, "HEAD")


if __name__ == "__main__":
    unittest.main()
