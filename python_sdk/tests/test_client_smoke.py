import os
import unittest

from lark_cli_sdk import LarkCLIClient


class SmokeTest(unittest.TestCase):
    def test_catalog_available(self):
        lib = os.getenv("LARK_CLI_SHARED_LIB")
        if not lib:
            self.skipTest("LARK_CLI_SHARED_LIB is not set")
        client = LarkCLIClient(lib)
        catalog = client.command_catalog()
        self.assertIn("commands", catalog)
        self.assertGreater(len(catalog["commands"]), 0)


if __name__ == "__main__":
    unittest.main()
