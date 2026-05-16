#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from typing import List, Optional

from lark_cli_sdk import LarkCLIClient, LarkCLIResult


def _append_repeat_flag(args: List[str], flag: str, values: Optional[List[str]]) -> None:
    if not values:
        return
    for value in values:
        args.extend([flag, value])


def _run_and_exit(client: LarkCLIClient, cli_args: List[str], timeout_ms: Optional[int]) -> int:
    result: LarkCLIResult = client.run(cli_args, timeout_ms=timeout_ms)

    if result.stdout:
        sys.stdout.write(result.stdout_text)
    if result.stderr:
        sys.stderr.write(result.stderr_text)

    if not result.ok and result.error:
        sys.stderr.write(result.error + "\n")

    return int(result.exit_code)


def _run_login_and_exit(client: LarkCLIClient, cli_args: List[str], timeout_ms: Optional[int]) -> int:
    session = client.start(cli_args, timeout_ms=timeout_ms)
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


def _build_auth_args(ns: argparse.Namespace) -> List[str]:
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

    return parser


def main() -> int:
    parser = build_parser()
    ns = parser.parse_args()

    client = LarkCLIClient(ns.library_path)
    cli_args = _build_auth_args(ns)
    if ns.command == "login":
        return _run_login_and_exit(client, cli_args, timeout_ms=ns.timeout_ms)
    return _run_and_exit(client, cli_args, timeout_ms=ns.timeout_ms)


if __name__ == "__main__":
    raise SystemExit(main())
