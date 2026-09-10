import importlib.util
import pathlib
import unittest


MODULE_PATH = pathlib.Path(__file__).parents[1] / "validate_version.py"
SPEC = importlib.util.spec_from_file_location("headroom_validate_version", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
SPEC.loader.exec_module(MODULE)


class ValidateVersionTests(unittest.TestCase):
    def test_preserves_build_metadata_and_derives_numeric_version(self):
        self.assertEqual(MODULE.validate("1.2.3+build.7"), ("1.2.3+build.7", "1.2.3"))

    def test_rejects_prerelease_leading_zero_and_multiple_build_markers(self):
        for value in ("1.2.3-rc.1", "01.2.3", "v1.2.3", "1.2.3+a+b", "1.2"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                MODULE.validate(value)

    def test_rejects_numeric_component_that_cannot_be_a_windows_resource(self):
        self.assertEqual(
            MODULE.validate("65534.65534.65534+build.7"),
            ("65534.65534.65534+build.7", "65534.65534.65534"),
        )
        for value in ("65535.0.0", "0.65535.0", "0.0.65535"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                MODULE.validate(value)


if __name__ == "__main__":
    unittest.main()
