import contextlib
import importlib.util
import io
import sys
import unittest
from pathlib import Path


SDK_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SDK_ROOT))

spec = importlib.util.spec_from_file_location("auth_example", SDK_ROOT / "examples" / "auth.py")
if spec is None or spec.loader is None:
    raise RuntimeError("failed to load python_sdk/examples/auth.py")
auth_example = importlib.util.module_from_spec(spec)
spec.loader.exec_module(auth_example)


class _FakeSession:
    def __init__(self):
        self.closed = False
        self._chunks = [
            {
                "stdout": b"",
                "stderr": (
                    b"\xe5\x9c\xa8\xe6\xb5\x8f\xe8\xa7\x88\xe5\x99\xa8\xe4\xb8\xad\xe6\x89\x93\xe5\xbc\x80\xe4\xbb\xa5\xe4\xb8\x8b\xe9\x93\xbe\xe6\x8e\xa5\xe8\xbf\x9b\xe8\xa1\x8c\xe8\xae\xa4\xe8\xaf\x81:\n\n"
                    b"https://accounts.feishu.cn/oauth/v1/device/verify?flow_id=test-flow&user_code=TEST-CODE\n"
                    b"\n\xe7\xad\x89\xe5\xbe\x85\xe7\x94\xa8\xe6\x88\xb7\xe6\x8e\x88\xe6\x9d\x83...\n"
                ),
                "done": False,
                "exit_code": 0,
                "error": "",
            }
        ]

    def iter_output(self, max_bytes=65536):
        _ = max_bytes
        for chunk in self._chunks:
            yield chunk

    def poll(self, max_bytes=0):
        _ = max_bytes
        return {
            "done": True,
            "exit_code": 0,
            "stdout": b"",
            "stderr": b"",
            "error": "",
        }

    def close(self):
        self.closed = True


class _FakeClient:
    def __init__(self):
        self.start_args = None
        self.start_timeout_ms = None
        self.session = _FakeSession()

    def start(self, args, timeout_ms=None):
        self.start_args = list(args)
        self.start_timeout_ms = timeout_ms
        return self.session


class AuthExampleTest(unittest.TestCase):
    def test_login_streams_auth_url(self):
        client = _FakeClient()
        out = io.StringIO()
        err = io.StringIO()

        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            exit_code = auth_example._run_login_and_exit(
                client,
                ["auth", "login", "--recommend"],
                timeout_ms=600000,
            )

        self.assertEqual(exit_code, 0)
        self.assertEqual(client.start_args, ["auth", "login", "--recommend"])
        self.assertEqual(client.start_timeout_ms, 600000)
        self.assertIn(
            "https://accounts.feishu.cn/oauth/v1/device/verify?flow_id=test-flow&user_code=TEST-CODE",
            err.getvalue(),
        )
        self.assertIn("等待用户授权", err.getvalue())
        self.assertTrue(client.session.closed)


if __name__ == "__main__":
    unittest.main()
