import importlib.util
from pathlib import Path
import tempfile
import unittest


SPEC = importlib.util.spec_from_file_location(
    "headroom_package_macos", Path(__file__).parents[1] / "package-macos.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class MacAssemblyTests(unittest.TestCase):
    def test_canonical_bundle_name_preserves_payload(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "headroom.app").mkdir()
            (root / "headroom.app/payload").write_text("fixture")
            app = MODULE.canonical_app_bundle(root)
            self.assertEqual([path.name for path in root.iterdir()], ["Headroom.app"])
            self.assertEqual((app / "payload").read_text(), "fixture")

    def test_qml_plugin_links_become_regular_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            app = Path(temporary)
            plugins = app / "Contents/PlugIns"
            qml = app / "Contents/Resources/qml/QtQuick"
            plugins.mkdir(parents=True)
            qml.mkdir(parents=True)
            target = plugins / "fixture.dylib"
            target.write_bytes(b"synthetic plugin")
            link = qml / "fixture.dylib"
            link.symlink_to("../../../PlugIns/fixture.dylib")
            MODULE.materialize_qml_plugin_links(app)
            self.assertFalse(link.is_symlink())
            self.assertEqual(link.read_bytes(), target.read_bytes())
            self.assertEqual(link.stat().st_mode & 0o777, 0o755)

    def test_qml_links_cannot_escape_the_plugin_directory(self):
        with tempfile.TemporaryDirectory() as temporary:
            app = Path(temporary)
            (app / "Contents/PlugIns").mkdir(parents=True)
            qml = app / "Contents/Resources/qml"
            qml.mkdir(parents=True)
            (app / "external.dylib").write_bytes(b"outside")
            link = qml / "fixture.dylib"
            link.symlink_to("../../../external.dylib")
            with self.assertRaises(ValueError):
                MODULE.materialize_qml_plugin_links(app)
            self.assertTrue(link.is_symlink())


if __name__ == "__main__":
    unittest.main()
