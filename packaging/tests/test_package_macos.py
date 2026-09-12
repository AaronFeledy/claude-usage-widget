import importlib.util
import os
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

    def test_qml_plugins_use_central_location_at_nested_depths(self):
        with tempfile.TemporaryDirectory() as temporary:
            app = Path(temporary)
            plugins = app / "Contents/PlugIns"
            qml = app / "Contents/Resources/qml"
            plugins.mkdir(parents=True)
            shallow = qml / "QtQuick"
            nested = qml / "QtQuick/Controls/Basic/impl"
            shallow.mkdir(parents=True)
            nested.mkdir(parents=True)
            (shallow / "qmldir").write_text("module QtQuick\nplugin quickplugin\n")
            (nested / "qmldir").write_text(
                "module QtQuick.Controls.Basic.impl\noptional plugin basicimplplugin\n")
            for directory, name in ((shallow, "quickplugin"), (nested, "basicimplplugin")):
                target = plugins / f"lib{name}.dylib"
                target.write_bytes(b"synthetic plugin")
                if directory == shallow:
                    (directory / target.name).symlink_to(
                        Path(os.path.relpath(target, directory)))
                else:
                    # A second macdeployqt pass can replace Qt's link with a
                    # separately patched regular copy.
                    (directory / target.name).write_bytes(b"patched plugin")

            MODULE.relocate_qml_plugins(app)

            self.assertEqual((shallow / "qmldir").read_text(),
                "module QtQuick\nplugin quickplugin ../../../PlugIns\n")
            self.assertEqual((nested / "qmldir").read_text(),
                "module QtQuick.Controls.Basic.impl\n"
                "optional plugin basicimplplugin ../../../../../../PlugIns\n")
            self.assertFalse((shallow / "libquickplugin.dylib").exists())
            self.assertFalse((nested / "libbasicimplplugin.dylib").exists())

    def test_qml_foreign_links_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            app = Path(temporary)
            (app / "Contents/PlugIns").mkdir(parents=True)
            qml = app / "Contents/Resources/qml"
            qml.mkdir(parents=True)
            (app / "external.dylib").write_bytes(b"outside")
            link = qml / "fixture.dylib"
            link.symlink_to("../../../external.dylib")
            with self.assertRaises(ValueError):
                MODULE.relocate_qml_plugins(app)
            self.assertTrue(link.is_symlink())

    def test_qml_declared_plugin_must_match_central_filename(self):
        with tempfile.TemporaryDirectory() as temporary:
            app = Path(temporary)
            plugins = app / "Contents/PlugIns"
            qml = app / "Contents/Resources/qml/QtQuick/Controls"
            plugins.mkdir(parents=True)
            qml.mkdir(parents=True)
            (plugins / "libexpectedplugin.dylib").write_bytes(b"expected")
            (plugins / "libotherplugin.dylib").write_bytes(b"other")
            (qml / "qmldir").write_text("module QtQuick.Controls\nplugin expectedplugin\n")
            (qml / "libexpectedplugin.dylib").symlink_to(
                "../../../../PlugIns/libotherplugin.dylib")
            with self.assertRaisesRegex(ValueError, "unexpected QML deployment link"):
                MODULE.relocate_qml_plugins(app)


if __name__ == "__main__":
    unittest.main()
