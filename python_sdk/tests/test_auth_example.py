import contextlib
import argparse
import importlib.util
import io
import json
import shutil
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
        self.run_calls = []
        self.run_result = None

    def start(self, args, timeout_ms=None, env_overrides=None):
        _ = env_overrides
        self.start_args = list(args)
        self.start_timeout_ms = timeout_ms
        return self.session

    def run(self, args, timeout_ms=None, env_overrides=None):
        _ = timeout_ms
        _ = env_overrides
        self.run_calls.append(list(args))
        return self.run_result


class _FakeRunResult:
    def __init__(self, payload=None, ok=True, exit_code=0, error="", stderr_text=""):
        self.ok = ok
        self.exit_code = exit_code
        self.error = error
        self.json_envelope = payload
        self.stdout_text = json.dumps(payload or {}, ensure_ascii=False) + "\n"
        self.stderr_text = stderr_text
        self.stdout = self.stdout_text.encode("utf-8")
        self.stderr = self.stderr_text.encode("utf-8")


class _FakeCmdResult:
    def __init__(self, ok=True, exit_code=0, error="", stdout_text="", stderr_text="", payload=None):
        self.ok = ok
        self.exit_code = exit_code
        self.error = error
        self.stdout_text = stdout_text
        self.stderr_text = stderr_text
        self.stdout = self.stdout_text.encode("utf-8")
        self.stderr = self.stderr_text.encode("utf-8")
        self.json_envelope = payload if payload is not None else {}


class _QueueSession:
    def __init__(self, stderr_text, exit_code=0, error=""):
        self.closed = False
        self._stderr = stderr_text.encode("utf-8")
        self._exit_code = int(exit_code)
        self._error = str(error or "")

    def iter_output(self, max_bytes=65536):
        _ = max_bytes
        yield {
            "stdout": b"",
            "stderr": self._stderr,
            "done": False,
            "exit_code": 0,
            "error": "",
        }

    def poll(self, max_bytes=0):
        _ = max_bytes
        return {
            "done": True,
            "exit_code": self._exit_code,
            "stdout": b"",
            "stderr": b"",
            "error": self._error,
        }

    def close(self):
        self.closed = True


class _FlowFakeClient:
    def __init__(self, sessions, run_result_map):
        self.sessions = list(sessions)
        self.run_result_map = dict(run_result_map)
        self.start_calls = []
        self.run_calls = []

    def start(self, args, timeout_ms=None, env_overrides=None):
        _ = timeout_ms
        _ = env_overrides
        self.start_calls.append(list(args))
        if not self.sessions:
            raise AssertionError("unexpected start call with no remaining session")
        return self.sessions.pop(0)

    def run(self, args, timeout_ms=None, env_overrides=None):
        _ = timeout_ms
        _ = env_overrides
        key = tuple(args)
        self.run_calls.append(list(args))
        if key not in self.run_result_map:
            raise AssertionError(f"unexpected run call: {args}")
        return self.run_result_map[key]


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

    def test_post_login_export_emit_and_merge(self):
        client = _FakeClient()
        payload = {
            "app_id": "cli_x",
            "app_secret": "sec_x",
            "brand": "feishu",
            "user_open_id": "ou_x",
            "user_name": "tester",
            "user_access_token": "u_x",
        }
        client.run_result = _FakeRunResult(payload=payload)

        out = io.StringIO()
        err = io.StringIO()
        cred_path = Path(self.id().replace("/", "_") + ".credentials.json")
        if cred_path.exists():
            cred_path.unlink()
        self.addCleanup(lambda: cred_path.exists() and cred_path.unlink())

        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            exit_code = auth_example._post_login_export_and_maybe_merge(
                client=client,
                timeout_ms=30000,
                emit_auth_export=True,
                merge_credential_file=True,
                credential_file_path=str(cred_path),
            )

        self.assertEqual(exit_code, 0)
        self.assertEqual(client.run_calls, [["auth", "export"]])
        self.assertIn('"app_id": "cli_x"', out.getvalue())
        self.assertIn("merged exported credential into", err.getvalue())
        self.assertTrue(cred_path.exists())
        saved = json.loads(cred_path.read_text(encoding="utf-8"))
        self.assertEqual(saved["app_id"], "cli_x")
        self.assertEqual(saved["user_open_id"], "ou_x")

    def test_emit_config_json(self):
        temp_dir = Path(self.id().replace("/", "_") + ".cfg")
        temp_dir.mkdir(parents=True, exist_ok=True)
        self.addCleanup(lambda: temp_dir.exists() and shutil.rmtree(temp_dir))

        cfg = {
            "currentApp": "cli_x",
            "apps": [{"appId": "cli_x", "appSecret": "sec_x", "brand": "feishu", "users": []}],
        }
        (temp_dir / "config.json").write_text(json.dumps(cfg, ensure_ascii=False), encoding="utf-8")

        out = io.StringIO()
        err = io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            exit_code = auth_example._emit_config_json(str(temp_dir))

        self.assertEqual(exit_code, 0)
        got = json.loads(out.getvalue().strip())
        self.assertEqual(got["currentApp"], "cli_x")
        self.assertEqual(got["apps"][0]["appId"], "cli_x")
        self.assertEqual(err.getvalue(), "")

    def test_inline_credential_payload_from_config(self):
        cfg = {
            "currentApp": "cli_x",
            "apps": [
                {
                    "name": "default",
                    "appId": "cli_x",
                    "appSecret": "sec_x",
                    "brand": "feishu",
                    "users": [],
                }
            ],
        }
        inline = auth_example._inline_credential_payload_from_config(cfg)
        self.assertIsNotNone(inline)
        self.assertEqual(inline["app_id"], "cli_x")
        self.assertEqual(inline["app_secret"], "sec_x")
        self.assertEqual(inline["brand"], "feishu")

    def test_env_credential_overrides_from_config(self):
        cfg = {
            "currentApp": "cli_x",
            "apps": [{"appId": "cli_x", "appSecret": "sec_x", "brand": "feishu", "users": []}],
        }
        env = auth_example._env_credential_overrides_from_config(cfg)
        self.assertIsNotNone(env)
        self.assertEqual(env["LARKSUITE_CLI_APP_ID"], "cli_x")
        self.assertEqual(env["LARKSUITE_CLI_APP_SECRET"], "sec_x")
        self.assertEqual(env["LARKSUITE_CLI_BRAND"], "feishu")
        self.assertEqual(env["LARKSUITE_CLI_STRICT_MODE"], "off")

    def test_env_credential_overrides_from_snake_case_credential_json(self):
        cfg = {
            "app_id": "cli_x",
            "app_secret": "sec_x",
            "brand": "feishu",
            "default_as": "bot",
        }
        env = auth_example._env_credential_overrides_from_config(cfg)
        self.assertIsNotNone(env)
        self.assertEqual(env["LARKSUITE_CLI_APP_ID"], "cli_x")
        self.assertEqual(env["LARKSUITE_CLI_APP_SECRET"], "sec_x")
        self.assertEqual(env["LARKSUITE_CLI_BRAND"], "feishu")

    def test_run_demo_flow_and_exit(self):
        config_link = "https://accounts.feishu.cn/device/config-step"
        login_link = "https://accounts.feishu.cn/device/login-step"
        sessions = [
            _QueueSession(f"请点击链接完成配置鉴权:\n{config_link}\n等待完成...\n"),
            _QueueSession(f"请点击链接完成授权:\n{login_link}\n等待完成...\n"),
        ]
        run_result_map = {
            ("config", "strict-mode", "off"): _FakeCmdResult(),
            ("auth", "status", "--verify"): _FakeCmdResult(stdout_text='{"ok":true}\n'),
        }
        client = _FlowFakeClient(sessions=sessions, run_result_map=run_result_map)
        ns = argparse.Namespace(
            timeout_ms=12345,
            new=False,
            no_credential_file=False,
            scope="",
            recommend=True,
            domain=[],
            exclude=[],
            json=True,
            verify=True,
        )

        out = io.StringIO()
        err = io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            exit_code = auth_example._run_demo_flow_and_exit(client, ns, env_overrides_base=None)

        self.assertEqual(exit_code, 0)
        self.assertEqual(client.start_calls, [["config", "init"], ["auth", "login", "--recommend", "--json"]])
        self.assertEqual(client.run_calls, [["config", "strict-mode", "off"], ["auth", "status", "--verify"]])
        self.assertIn(config_link, err.getvalue())
        self.assertIn(login_link, err.getvalue())
        self.assertIn('{"ok":true}', out.getvalue())

    def test_run_demo_flow_retries_config_init_with_new_when_tty_required(self):
        first_err = (
            "config init requires a terminal for interactive mode. "
            "Run with --new to create a new app."
        )
        config_link = "https://accounts.feishu.cn/device/config-new-step"
        login_link = "https://accounts.feishu.cn/device/login-step"
        sessions = [
            _QueueSession(first_err + "\n", exit_code=1),
            _QueueSession(f"请点击链接完成配置鉴权:\n{config_link}\n等待完成...\n"),
            _QueueSession(f"请点击链接完成授权:\n{login_link}\n等待完成...\n"),
        ]
        run_result_map = {
            ("config", "strict-mode", "off"): _FakeCmdResult(),
            ("auth", "status", "--verify"): _FakeCmdResult(stdout_text='{"ok":true}\n'),
        }
        client = _FlowFakeClient(sessions=sessions, run_result_map=run_result_map)
        ns = argparse.Namespace(
            timeout_ms=12345,
            new=False,
            no_credential_file=False,
            scope="",
            recommend=True,
            domain=[],
            exclude=[],
            json=True,
            verify=True,
        )

        out = io.StringIO()
        err = io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            exit_code = auth_example._run_demo_flow_and_exit(client, ns, env_overrides_base=None)

        self.assertEqual(exit_code, 0)
        self.assertEqual(
            client.start_calls,
            [["config", "init"], ["config", "init", "--new"], ["auth", "login", "--recommend", "--json"]],
        )
        self.assertIn("retrying with --new", err.getvalue())
        self.assertIn(config_link, err.getvalue())
        self.assertIn(login_link, err.getvalue())


if __name__ == "__main__":
    unittest.main()
