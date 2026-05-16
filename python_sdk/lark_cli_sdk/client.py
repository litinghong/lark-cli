from __future__ import annotations

import base64
import ctypes
import json
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Dict, Iterable, Optional


@dataclass
class LarkCLIResult:
    ok: bool
    exit_code: int
    stdout: bytes
    stderr: bytes
    error: str = ""

    @property
    def stdout_text(self) -> str:
        return self.stdout.decode("utf-8", errors="replace")

    @property
    def stderr_text(self) -> str:
        return self.stderr.decode("utf-8", errors="replace")

    @property
    def json_envelope(self) -> Optional[Dict[str, Any]]:
        try:
            return json.loads(self.stdout_text)
        except Exception:
            return None


class LarkCLISession:
    def __init__(self, client: "LarkCLIClient", session_id: str):
        self._client = client
        self.session_id = session_id

    def poll(self, max_bytes: int = 0) -> Dict[str, Any]:
        req = {"session_id": self.session_id}
        if max_bytes > 0:
            req["max_bytes"] = max_bytes
        resp = self._client._call_json("LarkCLIPollSessionJSON", req)
        if not resp.get("ok", False):
            raise RuntimeError(resp.get("error", "poll failed"))
        return {
            "done": bool(resp.get("done", False)),
            "exit_code": int(resp.get("exit_code", 0)),
            "stdout": _b64decode(resp.get("stdout_b64", "")),
            "stderr": _b64decode(resp.get("stderr_b64", "")),
            "error": str(resp.get("error", "") or ""),
        }

    def iter_output(self, max_bytes: int = 65536):
        while True:
            chunk = self.poll(max_bytes=max_bytes)
            if chunk["stdout"] or chunk["stderr"]:
                yield chunk
            if chunk["done"]:
                break

    def cancel(self) -> None:
        resp = self._client._call_json("LarkCLICancelSessionJSON", {"session_id": self.session_id})
        if not resp.get("ok", False):
            raise RuntimeError(resp.get("error", "cancel failed"))

    def close(self) -> None:
        self._client._call_json("LarkCLIFreeSessionJSON", {"session_id": self.session_id})


class LarkCLIClient:
    def __init__(self, library_path: Optional[str] = None):
        self._lib = ctypes.CDLL(str(_resolve_library_path(library_path)))
        self._setup_ffi()
        self._catalog = self.command_catalog()
        self._install_generated_methods()

    def _setup_ffi(self) -> None:
        for fn in [
            "LarkCLIInvokeJSON",
            "LarkCLIStartSessionJSON",
            "LarkCLIPollSessionJSON",
            "LarkCLICancelSessionJSON",
            "LarkCLIFreeSessionJSON",
        ]:
            f = getattr(self._lib, fn)
            f.argtypes = [ctypes.c_char_p]
            f.restype = ctypes.c_void_p

        self._lib.LarkCLICommandCatalogJSON.argtypes = []
        self._lib.LarkCLICommandCatalogJSON.restype = ctypes.c_void_p

        self._lib.LarkCLIFreeCString.argtypes = [ctypes.c_void_p]
        self._lib.LarkCLIFreeCString.restype = None

    def _call_json(self, fn_name: str, payload: Dict[str, Any]) -> Dict[str, Any]:
        raw = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        cptr = getattr(self._lib, fn_name)(ctypes.c_char_p(raw))
        text = self._copy_and_free(cptr)
        return json.loads(text)

    def _copy_and_free(self, ptr: int) -> str:
        if not ptr:
            raise RuntimeError("ffi call returned null pointer")
        try:
            return ctypes.cast(ptr, ctypes.c_char_p).value.decode("utf-8")
        finally:
            self._lib.LarkCLIFreeCString(ptr)

    def command_catalog(self) -> Dict[str, Any]:
        ptr = self._lib.LarkCLICommandCatalogJSON()
        text = self._copy_and_free(ptr)
        return json.loads(text)

    def run(
        self,
        args: Iterable[str],
        stdin: Optional[bytes | str] = None,
        timeout_ms: Optional[int] = None,
        env_overrides: Optional[Dict[str, Optional[str]]] = None,
        enable_embedded_event_bus: bool = True,
    ) -> LarkCLIResult:
        stdin_bytes: bytes
        if stdin is None:
            stdin_bytes = b""
        elif isinstance(stdin, str):
            stdin_bytes = stdin.encode("utf-8")
        else:
            stdin_bytes = stdin

        req: Dict[str, Any] = {
            "args": list(args),
            "stdin_b64": base64.b64encode(stdin_bytes).decode("ascii"),
            "enable_embedded_event_bus": enable_embedded_event_bus,
        }
        if timeout_ms is not None:
            req["timeout_ms"] = int(timeout_ms)
        if env_overrides is not None:
            req["env_overrides"] = env_overrides

        resp = self._call_json("LarkCLIInvokeJSON", req)
        return LarkCLIResult(
            ok=bool(resp.get("ok", False)),
            exit_code=int(resp.get("exit_code", 1)),
            stdout=_b64decode(resp.get("stdout_b64", "")),
            stderr=_b64decode(resp.get("stderr_b64", "")),
            error=str(resp.get("error", "") or ""),
        )

    def start(
        self,
        args: Iterable[str],
        stdin: Optional[bytes | str] = None,
        timeout_ms: Optional[int] = None,
        env_overrides: Optional[Dict[str, Optional[str]]] = None,
        enable_embedded_event_bus: bool = True,
    ) -> LarkCLISession:
        stdin_bytes: bytes
        if stdin is None:
            stdin_bytes = b""
        elif isinstance(stdin, str):
            stdin_bytes = stdin.encode("utf-8")
        else:
            stdin_bytes = stdin

        req: Dict[str, Any] = {
            "args": list(args),
            "stdin_b64": base64.b64encode(stdin_bytes).decode("ascii"),
            "enable_embedded_event_bus": enable_embedded_event_bus,
        }
        if timeout_ms is not None:
            req["timeout_ms"] = int(timeout_ms)
        if env_overrides is not None:
            req["env_overrides"] = env_overrides

        resp = self._call_json("LarkCLIStartSessionJSON", req)
        if not resp.get("ok", False):
            raise RuntimeError(resp.get("error", "start session failed"))
        return LarkCLISession(self, str(resp["session_id"]))

    def _install_generated_methods(self) -> None:
        for item in self._catalog.get("commands", []):
            path = str(item.get("path", "")).strip()
            if not path:
                continue
            method_name = _path_to_method_name(path)
            if hasattr(self, method_name):
                continue
            command_tokens = path.split()
            flags = [f for f in item.get("flags", []) if f.get("name") != "help"]
            mode = item.get("mode", "sync")
            func = _build_command_method(command_tokens, flags, mode)
            setattr(self, method_name, func.__get__(self, self.__class__))


def _build_command_method(command_tokens: list[str], flags: list[dict[str, Any]], mode: str) -> Callable[..., Any]:
    flag_names = [_flag_name_to_param(f["name"]) for f in flags if f.get("name")]

    def method(self: LarkCLIClient, **kwargs: Any):
        args = list(command_tokens)
        timeout_ms = kwargs.pop("timeout_ms", None)
        stdin = kwargs.pop("stdin", None)
        env_overrides = kwargs.pop("env_overrides", None)
        enable_embedded_event_bus = kwargs.pop("enable_embedded_event_bus", True)

        for f in flags:
            name = f.get("name")
            if not name:
                continue
            param = _flag_name_to_param(name)
            if param not in kwargs:
                continue
            value = kwargs.pop(param)
            if value is None:
                continue

            ftype = str(f.get("type", "string"))
            flag_token = f"--{name}"
            if ftype == "bool":
                if value is True:
                    args.append(flag_token)
                elif value is False:
                    args.append(f"{flag_token}=false")
                else:
                    raise TypeError(f"{param} must be bool")
                continue

            if ftype in ("stringArray", "stringSlice"):
                if not isinstance(value, (list, tuple)):
                    raise TypeError(f"{param} must be list/tuple")
                for one in value:
                    args.extend([flag_token, str(one)])
                continue

            args.extend([flag_token, str(value)])

        if kwargs:
            unknown = ", ".join(sorted(kwargs.keys()))
            known = ", ".join(sorted(flag_names + ["timeout_ms", "stdin", "env_overrides", "enable_embedded_event_bus"]))
            raise TypeError(f"unknown parameters: {unknown}; expected: {known}")

        if mode == "stream":
            return self.start(
                args,
                stdin=stdin,
                timeout_ms=timeout_ms,
                env_overrides=env_overrides,
                enable_embedded_event_bus=enable_embedded_event_bus,
            )

        return self.run(
            args,
            stdin=stdin,
            timeout_ms=timeout_ms,
            env_overrides=env_overrides,
            enable_embedded_event_bus=enable_embedded_event_bus,
        )

    method.__name__ = "_".join(command_tokens)
    method.__doc__ = f"Auto-generated wrapper for: {' '.join(command_tokens)}"
    return method


def _flag_name_to_param(flag_name: str) -> str:
    return flag_name.replace("-", "_")


def _path_to_method_name(path: str) -> str:
    parts = []
    for raw in path.split():
        token = raw.strip().lstrip("+").replace("-", "_")
        if token:
            parts.append(token)
    return "cmd_" + "_".join(parts)


def _b64decode(text: str) -> bytes:
    if not text:
        return b""
    return base64.b64decode(text.encode("ascii"))


def _resolve_library_path(explicit: Optional[str]) -> Path:
    if explicit:
        return Path(explicit).expanduser().resolve()

    env = os.getenv("LARK_CLI_SHARED_LIB")
    if env:
        return Path(env).expanduser().resolve()

    base = Path(__file__).resolve().parent
    names = ["liblarkcli.dylib", "liblarkcli.so"]
    for n in names:
        p = base / n
        if p.exists():
            return p

    raise FileNotFoundError(
        "shared library not found; pass library_path or set LARK_CLI_SHARED_LIB"
    )
