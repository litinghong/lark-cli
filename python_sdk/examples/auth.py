#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from typing import Dict, List, Optional

from lark_cli_sdk import LarkCLIClient, LarkCLIResult


def _append_repeat_flag(args: List[str], flag: str, values: Optional[List[str]]) -> None:
    if not values:
        return
    for value in values:
        args.extend([flag, value])


def _load_json_text(raw: str) -> Optional[Dict[str, object]]:
    if not raw:
        return None
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError:
        return None
    if isinstance(payload, dict):
        return payload
    return None


def _current_app_from_config_payload(config_payload: Dict[str, object]) -> Optional[Dict[str, object]]:
    apps = config_payload.get("apps")
    if not isinstance(apps, list) or not apps:
        return None
    current = str(config_payload.get("currentApp", "") or "")
    if current:
        for app in apps:
            if not isinstance(app, dict):
                continue
            app_id = str(app.get("appId", "") or "")
            app_name = str(app.get("name", "") or "")
            if current == app_id or current == app_name:
                return app
    for app in apps:
        if isinstance(app, dict):
            return app
    return None


def _inline_credential_payload_from_config(config_payload: Dict[str, object]) -> Optional[Dict[str, object]]:
    app = _current_app_from_config_payload(config_payload)
    if not app:
        return None
    app_id = str(app.get("appId", "") or "")
    if not app_id:
        return None
    secret_val = app.get("appSecret", "")
    if isinstance(secret_val, str):
        app_secret = secret_val
    else:
        # keychain/file references in config JSON cannot be used as inline
        # credential payload because --user-credential-json expects a plain secret.
        app_secret = ""
    if not app_secret:
        return None
    brand = str(app.get("brand", "") or "feishu")
    default_as = str(app.get("defaultAs", "") or "")
    users = app.get("users", [])
    user_open_id = ""
    user_name = ""
    if isinstance(users, list) and users:
        first = users[0]
        if isinstance(first, dict):
            user_open_id = str(first.get("userOpenId", "") or "")
            user_name = str(first.get("userName", "") or "")

    inline = {
        "app_id": app_id,
        "app_secret": app_secret,
        "brand": brand,
    }
    if default_as:
        inline["default_as"] = default_as
    if user_open_id:
        inline["user_open_id"] = user_open_id
    if user_name:
        inline["user_name"] = user_name
    return inline


def _env_credential_overrides_from_config(config_payload: Dict[str, object]) -> Optional[Dict[str, Optional[str]]]:
    # Accept credential-record style payloads first:
    # {"app_id":"...","app_secret":"...","brand":"feishu"}
    app_id_snake = str(config_payload.get("app_id", "") or "")
    app_secret_snake = str(config_payload.get("app_secret", "") or "")
    if app_id_snake and app_secret_snake:
        brand_snake = str(config_payload.get("brand", "") or "feishu")
        return {
            "LARKSUITE_CLI_APP_ID": app_id_snake,
            "LARKSUITE_CLI_APP_SECRET": app_secret_snake,
            "LARKSUITE_CLI_BRAND": brand_snake,
            "LARKSUITE_CLI_STRICT_MODE": "off",
        }

    # Also accept flat camelCase payloads:
    # {"appId":"...","appSecret":"...","brand":"feishu"}
    app_id_camel = str(config_payload.get("appId", "") or "")
    app_secret_camel = str(config_payload.get("appSecret", "") or "")
    if app_id_camel and app_secret_camel:
        brand_camel = str(config_payload.get("brand", "") or "feishu")
        return {
            "LARKSUITE_CLI_APP_ID": app_id_camel,
            "LARKSUITE_CLI_APP_SECRET": app_secret_camel,
            "LARKSUITE_CLI_BRAND": brand_camel,
            "LARKSUITE_CLI_STRICT_MODE": "off",
        }

    # Finally accept config.json style payloads:
    # {"apps":[{"appId":"...","appSecret":"...","brand":"feishu"}]}
    app = _current_app_from_config_payload(config_payload)
    if not app:
        return None
    app_id = str(app.get("appId", "") or "")
    brand = str(app.get("brand", "") or "feishu")
    secret_val = app.get("appSecret", "")
    app_secret = secret_val if isinstance(secret_val, str) else ""
    if not app_id or not app_secret:
        return None
    return {
        "LARKSUITE_CLI_APP_ID": app_id,
        "LARKSUITE_CLI_APP_SECRET": app_secret,
        "LARKSUITE_CLI_BRAND": brand,
        # For env provider path, explicitly open strict mode so user login is allowed.
        "LARKSUITE_CLI_STRICT_MODE": "off",
    }


def _credential_fallback_path() -> Path:
    cfg_dir = os.getenv("LARKSUITE_CLI_CONFIG_DIR")
    if cfg_dir:
        return Path(cfg_dir).expanduser() / ".lark-cli-credentials.json"
    return Path.home() / ".lark-cli" / ".lark-cli-credentials.json"


def _resolve_config_dir(explicit: Optional[str]) -> Path:
    if explicit:
        return Path(explicit).expanduser()
    cfg_dir = os.getenv("LARKSUITE_CLI_CONFIG_DIR")
    if cfg_dir:
        return Path(cfg_dir).expanduser()
    return Path.home() / ".lark-cli"


def _emit_config_json(config_dir: Optional[str]) -> int:
    cfg_path = _resolve_config_dir(config_dir) / "config.json"
    try:
        raw = cfg_path.read_text(encoding="utf-8")
    except OSError as err:
        sys.stderr.write(f"[auth.py] ERROR: failed to read config file {cfg_path}: {err}\n")
        return 1
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as err:
        sys.stderr.write(f"[auth.py] ERROR: invalid JSON in config file {cfg_path}: {err}\n")
        return 1
    sys.stdout.write(json.dumps(payload, ensure_ascii=False) + "\n")
    return 0


def _load_config_payload_from_ns(ns: argparse.Namespace) -> Optional[Dict[str, object]]:
    if getattr(ns, "config_json", ""):
        return _load_json_text(str(ns.config_json))
    if getattr(ns, "config_json_file", ""):
        p = Path(str(ns.config_json_file)).expanduser()
        try:
            payload = json.loads(p.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            return None
        if isinstance(payload, dict):
            return payload
    return None


def _remove_credential_fallback_file() -> None:
    cred_path = _credential_fallback_path()
    try:
        cred_path.unlink()
        sys.stderr.write(f"[auth.py] removed stale credential file: {cred_path}\n")
    except FileNotFoundError:
        return
    except OSError as err:
        sys.stderr.write(f"[auth.py] WARN: failed to remove credential file {cred_path}: {err}\n")


def _merge_json_file(path: Path, payload: Dict[str, object]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    merged: Dict[str, object] = {}
    if path.exists():
        try:
            with path.open("r", encoding="utf-8") as f:
                existing = json.load(f)
            if isinstance(existing, dict):
                merged.update(existing)
        except (OSError, json.JSONDecodeError) as err:
            sys.stderr.write(f"[auth.py] WARN: failed to read existing credential file {path}: {err}\n")
    merged.update(payload)
    with path.open("w", encoding="utf-8") as f:
        json.dump(merged, f, ensure_ascii=False, indent=2)
        f.write("\n")


def _post_login_export_and_maybe_merge(
    client: LarkCLIClient,
    timeout_ms: Optional[int],
    emit_auth_export: bool,
    merge_credential_file: bool,
    credential_file_path: Optional[str],
    env_overrides: Optional[Dict[str, Optional[str]]] = None,
) -> int:
    export_result: LarkCLIResult = client.run(
        ["auth", "export"],
        timeout_ms=timeout_ms,
        env_overrides=env_overrides,
    )
    if export_result.stderr:
        sys.stderr.write(export_result.stderr_text)
    if not export_result.ok:
        if export_result.error:
            sys.stderr.write(export_result.error + "\n")
        return int(export_result.exit_code) if int(export_result.exit_code) != 0 else 1
    if int(export_result.exit_code) != 0:
        if export_result.stdout:
            sys.stdout.write(export_result.stdout_text)
        return int(export_result.exit_code)

    payload = export_result.json_envelope
    if not isinstance(payload, dict):
        try:
            payload = json.loads(export_result.stdout_text or "{}")
        except json.JSONDecodeError:
            sys.stderr.write("[auth.py] WARN: auth export returned non-JSON output\n")
            payload = {}
    if not isinstance(payload, dict) or not payload:
        sys.stderr.write("[auth.py] WARN: auth export payload is empty\n")
        return 1

    if emit_auth_export:
        sys.stdout.write(json.dumps(payload, ensure_ascii=False) + "\n")

    if merge_credential_file:
        out_path = Path(credential_file_path).expanduser() if credential_file_path else _credential_fallback_path()
        try:
            _merge_json_file(out_path, payload)
            sys.stderr.write(f"[auth.py] merged exported credential into: {out_path}\n")
        except OSError as err:
            sys.stderr.write(f"[auth.py] ERROR: failed to merge credential file {out_path}: {err}\n")
            return 1

    return 0


def _run_and_exit(
    client: LarkCLIClient,
    cli_args: List[str],
    timeout_ms: Optional[int],
    env_overrides: Optional[Dict[str, Optional[str]]] = None,
) -> int:
    result: LarkCLIResult = client.run(cli_args, timeout_ms=timeout_ms, env_overrides=env_overrides)

    if result.stdout:
        sys.stdout.write(result.stdout_text)
    if result.stderr:
        sys.stderr.write(result.stderr_text)

    if not result.ok and result.error:
        sys.stderr.write(result.error + "\n")

    return int(result.exit_code)


def _run_login_and_exit(
    client: LarkCLIClient,
    cli_args: List[str],
    timeout_ms: Optional[int],
    env_overrides: Optional[Dict[str, Optional[str]]] = None,
) -> int:
    session = client.start(cli_args, timeout_ms=timeout_ms, env_overrides=env_overrides)
    try:
        for chunk in session.iter_output(max_bytes=65536):
            if chunk["stdout"]:
                sys.stdout.write(chunk["stdout"].decode("utf-8", errors="replace"))
            if chunk["stderr"]:
                sys.stderr.write(chunk["stderr"].decode("utf-8", errors="replace"))

        final = session.poll()
        if final["stdout"]:
            sys.stdout.write(final["stdout"].decode("utf-8", errors="replace"))
        if final["stderr"]:
            sys.stderr.write(final["stderr"].decode("utf-8", errors="replace"))
        if final["error"]:
            sys.stderr.write(final["error"] + "\n")
        return int(final["exit_code"])
    finally:
        session.close()


def _run_login_stream_capture(
    client: LarkCLIClient,
    cli_args: List[str],
    timeout_ms: Optional[int],
    env_overrides: Optional[Dict[str, Optional[str]]] = None,
) -> tuple[int, str, str, str]:
    session = client.start(cli_args, timeout_ms=timeout_ms, env_overrides=env_overrides)
    stdout_chunks: List[bytes] = []
    stderr_chunks: List[bytes] = []
    final_error = ""
    final_exit = 1
    try:
        for chunk in session.iter_output(max_bytes=65536):
            if chunk["stdout"]:
                stdout_chunks.append(chunk["stdout"])
                sys.stdout.write(chunk["stdout"].decode("utf-8", errors="replace"))
            if chunk["stderr"]:
                stderr_chunks.append(chunk["stderr"])
                sys.stderr.write(chunk["stderr"].decode("utf-8", errors="replace"))

        final = session.poll()
        if final["stdout"]:
            stdout_chunks.append(final["stdout"])
            sys.stdout.write(final["stdout"].decode("utf-8", errors="replace"))
        if final["stderr"]:
            stderr_chunks.append(final["stderr"])
            sys.stderr.write(final["stderr"].decode("utf-8", errors="replace"))
        if final["error"]:
            final_error = str(final["error"])
            sys.stderr.write(final_error + "\n")
        final_exit = int(final["exit_code"])
    finally:
        session.close()
    return (
        final_exit,
        b"".join(stdout_chunks).decode("utf-8", errors="replace"),
        b"".join(stderr_chunks).decode("utf-8", errors="replace"),
        final_error,
    )


def _run_setup_login_and_exit(
    client: LarkCLIClient,
    ns: argparse.Namespace,
    env_overrides_base: Optional[Dict[str, Optional[str]]] = None,
) -> int:
    strict_off_env: Dict[str, Optional[str]] = {"LARKSUITE_CLI_STRICT_MODE": "off"}
    if env_overrides_base:
        strict_off_env.update(env_overrides_base)

    if ns.no_save_config:
        # 1) Initialize config in memory-only mode and capture config JSON payload.
        init_args = ["config", "init"]
        if ns.new:
            init_args.append("--new")
        init_args.extend(["--no-save-config", "--emit-config-json"])
        if ns.no_credential_file:
            init_args.append("--no-credential-file")
        init_exit, init_stdout, _, init_error = _run_login_stream_capture(
            client,
            init_args,
            timeout_ms=ns.timeout_ms,
            env_overrides=strict_off_env,
        )
        if init_error:
            return init_exit if init_exit != 0 else 1
        if init_exit != 0:
            return init_exit

        config_payload = _load_json_text(init_stdout)
        if not isinstance(config_payload, dict):
            sys.stderr.write("[auth.py] ERROR: failed to parse config payload from `config init --no-save-config`\n")
            return 1
        if ns.emit_config_json:
            sys.stdout.write(json.dumps(config_payload, ensure_ascii=False) + "\n")

        env_cred = _env_credential_overrides_from_config(config_payload)
        if not env_cred:
            sys.stderr.write("[auth.py] ERROR: failed to build credential env from config JSON\n")
            return 1
        login_env = dict(strict_off_env)
        login_env.update(env_cred)

        # 2) Start auth login with inline credential JSON and return device code.
        login_args = ["auth", "login"]
        if ns.scope:
            login_args.extend(["--scope", ns.scope])
        if ns.recommend:
            login_args.append("--recommend")
        _append_repeat_flag(login_args, "--domain", ns.domain)
        _append_repeat_flag(login_args, "--exclude", ns.exclude)
        if ns.json:
            login_args.append("--json")
        if ns.no_wait:
            login_args.append("--no-wait")
        if ns.no_credential_file:
            login_args.append("--no-credential-file")

        first = client.run(login_args, timeout_ms=ns.timeout_ms, env_overrides=login_env)
        if first.stdout:
            sys.stdout.write(first.stdout_text)
        if first.stderr:
            sys.stderr.write(first.stderr_text)
        if not first.ok and first.error:
            sys.stderr.write(first.error + "\n")
        return int(first.exit_code)

    # 1) Initialize config/profile (interactive/blocking).
    init_args = ["config", "init"]
    if ns.new:
        init_args.append("--new")
    if ns.no_credential_file:
        init_args.append("--no-credential-file")
    init_exit = _run_login_and_exit(client, init_args, timeout_ms=ns.timeout_ms, env_overrides=strict_off_env)
    if init_exit != 0:
        return init_exit

    # 2) Ensure strict-mode allows user identity commands.
    strict_exit = _run_and_exit(
        client,
        ["config", "strict-mode", "off"],
        timeout_ms=ns.timeout_ms,
        env_overrides=strict_off_env,
    )
    if strict_exit != 0:
        return strict_exit

    if ns.no_credential_file:
        # Prevent exe_file provider from forcing bot-only identity when a stale
        # app-only credential file exists from previous setup runs.
        _remove_credential_fallback_file()

    # 3) Start auth login and return verification URL/device code.
    login_args = ["auth", "login"]
    if ns.scope:
        login_args.extend(["--scope", ns.scope])
    if ns.recommend:
        login_args.append("--recommend")
    _append_repeat_flag(login_args, "--domain", ns.domain)
    _append_repeat_flag(login_args, "--exclude", ns.exclude)
    if ns.json:
        login_args.append("--json")
    if ns.no_wait:
        login_args.append("--no-wait")
    if ns.no_credential_file:
        login_args.append("--no-credential-file")

    first = client.run(login_args, timeout_ms=ns.timeout_ms, env_overrides=strict_off_env)
    if first.stdout:
        sys.stdout.write(first.stdout_text)
    if first.stderr:
        sys.stderr.write(first.stderr_text)
    if not first.ok and first.error:
        sys.stderr.write(first.error + "\n")

    # Some environments enforce strict-mode via provider/env precedence.
    # Retry once with explicit strict-mode override and emit a clear hint.
    env_data = first.json_envelope or {}
    err_type = ""
    if isinstance(env_data.get("error"), dict):
        err_type = str(env_data["error"].get("type", "") or "")
    if first.exit_code != 0 and err_type == "strict_mode":
        sys.stderr.write(
            "[auth.py] strict_mode still enforced by runtime provider; "
            "retrying auth login with explicit env override LARKSUITE_CLI_STRICT_MODE=off\n"
        )
        return _run_and_exit(client, login_args, timeout_ms=ns.timeout_ms, env_overrides=strict_off_env)

    return int(first.exit_code)


def _run_demo_flow_and_exit(
    client: LarkCLIClient,
    ns: argparse.Namespace,
    env_overrides_base: Optional[Dict[str, Optional[str]]] = None,
) -> int:
    strict_off_env: Dict[str, Optional[str]] = {"LARKSUITE_CLI_STRICT_MODE": "off"}
    if env_overrides_base:
        strict_off_env.update(env_overrides_base)

    sys.stderr.write("[auth.py] step 1/3: config init (click link and complete verification)\n")
    init_args = ["config", "init"]
    if ns.new:
        init_args.append("--new")
    if ns.no_credential_file:
        init_args.append("--no-credential-file")
    init_exit, _, init_stderr, init_error = _run_login_stream_capture(
        client,
        init_args,
        timeout_ms=ns.timeout_ms,
        env_overrides=strict_off_env,
    )
    needs_new_fallback = (
        init_exit != 0
        and "--new" not in init_args
        and (
            "requires a terminal for interactive mode" in init_stderr
            or "requires a terminal for interactive mode" in init_error
        )
    )
    if needs_new_fallback:
        sys.stderr.write("[auth.py] config init requires TTY in this environment; retrying with --new\n")
        retry_args = list(init_args)
        retry_args.append("--new")
        init_exit = _run_login_and_exit(client, retry_args, timeout_ms=ns.timeout_ms, env_overrides=strict_off_env)
    if init_exit != 0:
        return init_exit

    strict_exit = _run_and_exit(
        client,
        ["config", "strict-mode", "off"],
        timeout_ms=ns.timeout_ms,
        env_overrides=strict_off_env,
    )
    if strict_exit != 0:
        return strict_exit

    if ns.no_credential_file:
        _remove_credential_fallback_file()

    sys.stderr.write("[auth.py] step 2/3: auth login (click link and complete authorization)\n")
    login_args = ["auth", "login"]
    if ns.scope:
        login_args.extend(["--scope", ns.scope])
    if ns.recommend:
        login_args.append("--recommend")
    _append_repeat_flag(login_args, "--domain", ns.domain)
    _append_repeat_flag(login_args, "--exclude", ns.exclude)
    if ns.json:
        login_args.append("--json")
    if ns.no_credential_file:
        login_args.append("--no-credential-file")

    login_exit = _run_login_and_exit(client, login_args, timeout_ms=ns.timeout_ms, env_overrides=strict_off_env)
    if login_exit != 0:
        return login_exit

    sys.stderr.write("[auth.py] step 3/3: auth status --verify\n")
    status_args = ["auth", "status"]
    if ns.verify:
        status_args.append("--verify")
    return _run_and_exit(client, status_args, timeout_ms=ns.timeout_ms, env_overrides=strict_off_env)


def _build_auth_args(ns: argparse.Namespace) -> List[str]:
    inline_cred_json = str(getattr(ns, "user_credential_json", "") or "").strip()

    if ns.command == "config-init":
        args = ["config", "init"]
        if ns.new:
            args.append("--new")
        if ns.no_credential_file:
            args.append("--no-credential-file")
        if ns.no_save_config:
            args.append("--no-save-config")
        if ns.emit_config_json:
            args.append("--emit-config-json")
        return args

    args = ["auth", ns.command]

    if ns.command == "login":
        if ns.scope:
            args.extend(["--scope", ns.scope])
        if ns.recommend:
            args.append("--recommend")
        _append_repeat_flag(args, "--domain", ns.domain)
        _append_repeat_flag(args, "--exclude", ns.exclude)
        if ns.json:
            args.append("--json")
        if ns.no_wait:
            args.append("--no-wait")
        if ns.device_code:
            args.extend(["--device-code", ns.device_code])
        if ns.no_credential_file:
            args.append("--no-credential-file")

    elif ns.command == "status":
        if ns.verify:
            args.append("--verify")

    elif ns.command == "check":
        args.extend(["--scope", ns.scope])

    elif ns.command == "scopes":
        if ns.format:
            args.extend(["--format", ns.format])

    # logout/list have no extra flags
    if inline_cred_json:
        return ["--user-credential-json", inline_cred_json] + args
    return args


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Auth examples for lark-cli Python SDK",
    )
    parser.add_argument(
        "--library-path",
        default=None,
        help="Optional shared library path (fallback: LARK_CLI_SHARED_LIB)",
    )
    parser.add_argument(
        "--timeout-ms",
        type=int,
        default=None,
        help="Optional command timeout in milliseconds",
    )
    parser.add_argument(
        "--config-dir",
        default="",
        help="Override LARKSUITE_CLI_CONFIG_DIR for this run",
    )
    parser.add_argument(
        "--user-credential-json",
        default="",
        help="Pass through global --user-credential-json to lark-cli",
    )
    parser.add_argument(
        "--config-json",
        default="",
        help="Config JSON text from `config init --no-save-config --emit-config-json` (used as env credentials)",
    )
    parser.add_argument(
        "--config-json-file",
        default="",
        help="Path to config JSON file (used as env credentials)",
    )

    sub = parser.add_subparsers(dest="command", required=True)

    login = sub.add_parser("login", help="auth login")
    login.add_argument("--scope", default="", help="Scopes (space/comma separated)")
    login.add_argument("--recommend", action="store_true", help="Request recommended scopes only")
    login.add_argument("--domain", action="append", default=[], help="Domain filter (repeatable)")
    login.add_argument("--exclude", action="append", default=[], help="Exclude scope (repeatable)")
    login.add_argument("--json", action="store_true", help="Structured JSON output")
    login.add_argument("--no-wait", action="store_true", help="Return device code without waiting")
    login.add_argument("--device-code", default="", help="Resume with a previous device code")
    login.add_argument("--no-credential-file", action="store_true", help="Do not persist .lark-cli-credentials.json")
    login.add_argument(
        "--emit-auth-export",
        action="store_true",
        help="After successful login, run `auth export` and print credential JSON",
    )
    login.add_argument(
        "--merge-credential-file",
        action="store_true",
        help="After successful login, run `auth export` and merge into credential file",
    )
    login.add_argument(
        "--credential-file-path",
        default="",
        help="Credential file path used with --merge-credential-file (default: config dir fallback path)",
    )

    logout = sub.add_parser("logout", help="auth logout")
    _ = logout

    status = sub.add_parser("status", help="auth status")
    status.add_argument("--verify", action="store_true", help="Verify token against server")

    check = sub.add_parser("check", help="auth check")
    check.add_argument("--scope", required=True, help="Scopes to check (space-separated)")

    scopes = sub.add_parser("scopes", help="auth scopes")
    scopes.add_argument("--format", default="json", choices=["json", "pretty"], help="Output format")

    list_cmd = sub.add_parser("list", help="auth list")
    _ = list_cmd

    export_cmd = sub.add_parser("export", help="auth export")
    _ = export_cmd

    config_init = sub.add_parser("config-init", help="config init")
    config_init.add_argument("--new", action="store_true", help="Create and switch to a new profile")
    config_init.add_argument("--no-credential-file", action="store_true", help="Do not persist .lark-cli-credentials.json")
    config_init.add_argument("--no-save-config", action="store_true", help="Do not persist config.json/keychain; return config payload only")
    config_init.add_argument("--emit-config-json", action="store_true", help="Print config.json content after init succeeds")

    setup_login = sub.add_parser("setup-login", help="config init + strict-mode off + auth login")
    setup_login.add_argument("--new", action="store_true", help="Run `config init --new` before login")
    setup_login.add_argument("--scope", default="", help="Scopes (space/comma separated)")
    setup_login.add_argument("--recommend", action="store_true", default=True, help="Request recommended scopes only")
    setup_login.add_argument("--domain", action="append", default=[], help="Domain filter (repeatable)")
    setup_login.add_argument("--exclude", action="append", default=[], help="Exclude scope (repeatable)")
    setup_login.add_argument("--json", action="store_true", default=True, help="Structured JSON output for auth login")
    setup_login.add_argument("--no-wait", action="store_true", default=True, help="Return device code without waiting")
    setup_login.add_argument("--no-credential-file", action="store_true", default=True, help="Do not persist .lark-cli-credentials.json")
    setup_login.add_argument("--no-save-config", action="store_true", help="Do not persist config.json/keychain; run with inline credentials")
    setup_login.add_argument("--emit-config-json", action="store_true", help="Print config JSON payload after setup succeeds")

    demo_flow = sub.add_parser(
        "demo-flow",
        help="Single-method demo: config init + auth login + auth status",
    )
    demo_flow.add_argument("--new", action="store_true", help="Run `config init --new` before login")
    demo_flow.add_argument("--scope", default="", help="Scopes (space/comma separated)")
    demo_flow.add_argument("--recommend", action="store_true", default=True, help="Request recommended scopes only")
    demo_flow.add_argument("--domain", action="append", default=[], help="Domain filter (repeatable)")
    demo_flow.add_argument("--exclude", action="append", default=[], help="Exclude scope (repeatable)")
    demo_flow.add_argument("--json", action="store_true", default=True, help="Structured JSON output for auth login")
    demo_flow.add_argument("--no-credential-file", action="store_true", default=True, help="Do not persist .lark-cli-credentials.json")
    demo_flow.add_argument("--verify", action="store_true", default=True, help="Use --verify for the final auth status check")

    return parser


def main() -> int:
    parser = build_parser()
    ns = parser.parse_args()

    client = LarkCLIClient(ns.library_path)
    env_overrides = {}
    if ns.config_dir:
        env_overrides["LARKSUITE_CLI_CONFIG_DIR"] = ns.config_dir
    cfg_payload = _load_config_payload_from_ns(ns)
    if (ns.config_json or ns.config_json_file) and not isinstance(cfg_payload, dict):
        sys.stderr.write("[auth.py] ERROR: failed to parse --config-json / --config-json-file\n")
        return 1
    if isinstance(cfg_payload, dict):
        env_cred = _env_credential_overrides_from_config(cfg_payload)
        if not env_cred:
            sys.stderr.write("[auth.py] ERROR: config JSON missing appId/appSecret/brand for credential injection\n")
            return 1
        env_overrides.update(env_cred)

    if ns.command == "setup-login":
        exit_code = _run_setup_login_and_exit(client, ns, env_overrides_base=env_overrides or None)
        if exit_code == 0 and ns.emit_config_json and not ns.no_save_config:
            emit_code = _emit_config_json(ns.config_dir or None)
            if emit_code != 0:
                return emit_code
        return exit_code

    if ns.command == "demo-flow":
        return _run_demo_flow_and_exit(client, ns, env_overrides_base=env_overrides or None)

    try:
        cli_args = _build_auth_args(ns)
    except ValueError as err:
        sys.stderr.write(f"[auth.py] ERROR: {err}\n")
        return 1
    if ns.command in {"login", "config-init"}:
        exit_code = _run_login_and_exit(client, cli_args, timeout_ms=ns.timeout_ms, env_overrides=env_overrides or None)
        if ns.command == "config-init" and exit_code == 0 and ns.emit_config_json and not ns.no_save_config:
            emit_code = _emit_config_json(ns.config_dir or None)
            if emit_code != 0:
                return emit_code
        if ns.command == "login" and exit_code == 0 and (ns.emit_auth_export or ns.merge_credential_file):
            post = _post_login_export_and_maybe_merge(
                client=client,
                timeout_ms=ns.timeout_ms,
                emit_auth_export=bool(ns.emit_auth_export),
                merge_credential_file=bool(ns.merge_credential_file),
                credential_file_path=ns.credential_file_path or None,
                env_overrides=env_overrides or None,
            )
            if post != 0:
                return post
        return exit_code
    return _run_and_exit(client, cli_args, timeout_ms=ns.timeout_ms, env_overrides=env_overrides or None)


if __name__ == "__main__":
    raise SystemExit(main())
