# lark-cli Python SDK 集成指南（面向第三方项目）

本目录提供 `lark-cli` 的 Python 封装。SDK 通过 `ctypes` 调用 Go `c-shared` 动态库，适用于后端服务、Agent 平台和工具集成场景。

支持能力：

- 同步调用：`run(args)`
- 流式调用：`start()/poll()/cancel()/iter_output()`
- 自动命令方法：按命令目录自动注入 `cmd_<path>` 方法
- 多用户集成友好授权流：`auth login --no-credential-file` + `auth export`

## 1. 适用场景

- 在 Python 服务中复用 `lark-cli` 的全部能力（Shortcut/API/Raw API）。
- 需要可编排的授权流程（把验证链接交给前端用户点击）。
- 多租户/多用户系统，不希望在可执行目录落盘共享凭据文件。

## 2. 前置条件

- Python 3.10+
- 已在仓库根目录构建动态库

## 3. 构建动态库

在仓库根目录执行：

```bash
# macOS
make build-shared-darwin

# Linux
make build-shared-linux

# Linux 在 Mac 中构建（x86_64）
docker run --rm -it \
  --platform linux/amd64 \
  -v "$PWD":/src \
  -w /src \
  docker.m.daocloud.io/library/golang:1.24 \
  bash -c 'make build-shared-linux'

# 生成命令目录（自动方法依赖）
make build-shared-catalog

# 生成全量命令参考文档（与 command_catalog 对齐）
python3 python_sdk/scripts/generate_command_reference.py
```

产物：

- macOS: `dist/darwin/liblarkcli.dylib`
- Linux: `dist/linux/liblarkcli.so`

## 4. 引入 SDK

设置环境变量（推荐）：

```bash
export PYTHONPATH=python_sdk
export LARK_CLI_SHARED_LIB=/absolute/path/to/liblarkcli.dylib  # Linux 用 .so
```

Python 中使用：

```python
from lark_cli_sdk import LarkCLIClient

client = LarkCLIClient()
```

也可显式传入动态库路径：

```python
client = LarkCLIClient("/absolute/path/to/liblarkcli.so")
```

## 5. 运行模型与返回值

### 5.1 同步命令

```python
result = client.run(["schema", "calendar", "--format", "json"], timeout_ms=5000)
print(result.ok, result.exit_code)
print(result.stdout_text)
print(result.stderr_text)
```

`LarkCLIResult` 字段：

- `ok: bool`：FFI 调用是否成功
- `exit_code: int`：CLI 退出码
- `stdout: bytes`
- `stderr: bytes`
- `error: str`：SDK/ABI 层错误
- `json_envelope`：尝试将 `stdout` 解析为 JSON（失败返回 `None`）

### 5.2 流式命令（长任务）

适用于 `auth login`（交互等待）和 `event consume`（持续输出）：

```python
session = client.start(["event", "consume", "im.message.receive_v1", "--timeout", "20s"])
try:
    for chunk in session.iter_output(max_bytes=65536):
        if chunk["stdout"]:
            print(chunk["stdout"].decode("utf-8", errors="replace"), end="")
        if chunk["stderr"]:
            print(chunk["stderr"].decode("utf-8", errors="replace"), end="")

    final = session.poll()
    print("done=", final["done"], "exit=", final["exit_code"])
finally:
    session.close()
```

取消会话：

```python
session.cancel()
session.close()
```

## 6. 第三方项目的推荐接入方式（重点）

### 6.1 多用户隔离（必须）

不要让所有用户共享默认配置目录。每个租户/用户请求都设置独立 `LARKSUITE_CLI_CONFIG_DIR`：

```python
result = client.run(
    ["auth", "status"],
    env_overrides={
        "LARKSUITE_CLI_CONFIG_DIR": "/srv/appdata/lark-cli/u_12345",
        "LARKSUITE_CLI_NO_UPDATE_NOTIFIER": "1",
        "LARKSUITE_CLI_NO_SKILLS_NOTIFIER": "1",
    },
)
```

说明：

- `env_overrides` 中 value 传 `None` 表示 unset。
- 推荐长期设置两个 notifier 变量，避免业务 JSON 处理链路受 `_notice` 干扰。

### 6.2 禁用凭据文件落盘（多用户强烈推荐）

在授权流程中加 `--no-credential-file`，避免生成 `.lark-cli-credentials.json`：

```bash
lark-cli auth login --recommend --no-credential-file
```

配置初始化同理：

```bash
lark-cli config init --new --no-credential-file
```

### 6.3 授权后通过接口回传凭据

调用：

```bash
lark-cli auth export
```

输出 JSON 与 `.lark-cli-credentials.json` 同结构，可直接由你的应用接管存储（如 KMS/数据库/密钥服务）。

## 7. 可编排授权流程（后端 + 前端）

下面是适合 Web/多用户应用的标准流程。

### 7.0 `auth.py` 示例的两步登录（飞书工作台/Agent 常用）

```bash
# 第一步：配置应用并发起授权，返回 verification_url + device_code
python3 python_sdk/examples/auth.py setup-login --new

# 第二步：用户在浏览器完成授权后，使用 device_code 续上轮询
python3 python_sdk/examples/auth.py login --device-code <DEVICE_CODE> --json --no-credential-file
```
`demo-flow` 现在包含第 4 步自动 refresh（`auth export` 后刷新 token），并将刷新后的凭据 JSON 输出到 stdout，不会写入任何凭据文件。

```bash
# SDK 多租户推荐：不落盘 config.json，直接返回配置 JSON
python3 python_sdk/examples/auth.py setup-login --new --no-save-config --emit-config-json
```

```bash
# 第二步可直接使用 config JSON（auth.py 会自动注入 LARKSUITE_CLI_APP_ID/APP_SECRET/BRAND）
python3 python_sdk/examples/auth.py \
  --config-json '<STEP1_CONFIG_JSON>' \
  login --device-code <DEVICE_CODE> --json --no-credential-file
```

```bash
# 可选：第二步成功后输出授权内容，并合并到 .lark-cli-credentials.json
python3 python_sdk/examples/auth.py login --device-code <DEVICE_CODE> --json --no-credential-file \
  --emit-auth-export \
  --merge-credential-file
```

```bash
# 可选：基于授权结果中的 refresh_token 刷新用户鉴权（只返回 JSON，不做任何落盘）
python3 python_sdk/examples/auth.py refresh \
  --credential-json '<AUTH_EXPORT_JSON>'
```

```bash
# 可选：将配置目录隔离到指定路径，并在配置完成后直接输出 config.json 内容
python3 python_sdk/examples/auth.py --config-dir /tmp/lark-sdk-u123 \
  setup-login --new --emit-config-json
```

### 步骤 A：发起授权（后端）

```python
result = client.run(
    ["auth", "login", "--recommend", "--no-wait", "--json", "--no-credential-file"],
    timeout_ms=15000,
    env_overrides={"LARKSUITE_CLI_CONFIG_DIR": "/srv/appdata/lark-cli/u_12345"},
)
payload = result.json_envelope
# payload["verification_url"] -> 返回给前端
# payload["device_code"] -> 后端保存（短时）
```

### 步骤 B：用户在浏览器授权（前端）

- 前端打开 `verification_url`（必须原样使用，不做改写）。

### 步骤 C：后端续上轮询

```python
device_code = "...保存的 device_code..."
result = client.run(
    ["auth", "login", "--device-code", device_code, "--json", "--no-credential-file"],
    timeout_ms=600000,  # 建议 >= 10 分钟
    env_overrides={"LARKSUITE_CLI_CONFIG_DIR": "/srv/appdata/lark-cli/u_12345"},
)
```

授权成功后，再导出凭据：

```python
exported = client.run(
    ["auth", "export"],
    env_overrides={"LARKSUITE_CLI_CONFIG_DIR": "/srv/appdata/lark-cli/u_12345"},
)
credential_payload = exported.json_envelope
```

## 8. 自动生成命令方法

SDK 启动时读取命令目录并自动挂载方法：

- 规则：`<command path>` -> `cmd_<path>`
- `+` 前缀会去掉，`-` 转 `_`

示例：

- `calendar +agenda` -> `client.cmd_calendar_agenda(...)`
- `schema` -> `client.cmd_schema(...)`

```python
# 等价于: lark-cli schema calendar --format json
res = client.cmd_schema(format="json", timeout_ms=5000)

# 等价于: lark-cli calendar +agenda --days 3 --format json
res = client.cmd_calendar_agenda(days=3, format="json")
```

参数映射：

- `--page-limit` -> `page_limit`
- bool 参数：`True` -> `--flag`，`False` -> `--flag=false`
- `stringArray/stringSlice`：传 `list/tuple`，展开为重复 flag

注意：

- 自动方法目前只覆盖 flag，不覆盖位置参数；含位置参数命令建议用 `run(args)`。
- stream 类型命令自动方法返回 `LarkCLISession`；sync 返回 `LarkCLIResult`。

## 9. 典型错误处理策略

建议统一按以下顺序处理：

1. `result.ok == False`：视为 SDK/ABI 调用失败（优先处理 `result.error`）。
2. `result.ok == True 且 exit_code != 0`：CLI 业务失败（解析 `stderr_text`）。
3. `exit_code == 0`：再解析 `stdout_text` / `json_envelope`。

示例：

```python
res = client.run(["auth", "check", "--scope", "calendar:calendar:readonly"])
if not res.ok:
    raise RuntimeError(f"ffi failed: {res.error}")
if res.exit_code != 0:
    raise RuntimeError(f"cli failed: {res.stderr_text}")
data = res.json_envelope
```

## 10. 命令目录查询

```python
catalog = client.command_catalog()
print(len(catalog["commands"]))
print(catalog["commands"][0])
```

字段：

- `path`, `use`, `short`, `hidden`, `leaf`
- `mode`: `sync | stream | interactive-limited`
- `flags`: `name/type/default/required/...`

## 11. 全量命令能力对齐（与 lark-cli 命令全集同步）

如果你要让第三方调用“所有可用能力”，不要只看示例脚本，应该配合下面两层文档：

1. 入口文档：本 README（调用模型、鉴权、错误处理、多租户隔离）
2. 全量命令参考：`python_sdk/COMMAND_REFERENCE.md`（由 `command_catalog.json` 自动生成）

生成与刷新：

```bash
make build-shared-catalog
python3 python_sdk/scripts/generate_command_reference.py
```

`COMMAND_REFERENCE.md` 覆盖每条命令的：

- CLI path（例如 `docs +create`）
- 对应 Python 方法名（例如 `cmd_docs_create`）
- mode（`sync/stream/interactive-limited`）
- 必填 flags（required）
- use/short 简介
- flags 列表（含类型）

建议第三方按以下流程接入任意能力：

1. 先在 `COMMAND_REFERENCE.md` 找到目标命令和必填参数。
2. 优先使用自动方法 `client.cmd_<...>(...)`。
3. 若命令依赖位置参数（`use` 中有 `<...>`），改用 `client.run([...])` 显式传参。
4. 用 `result.ok` + `exit_code` 双层判错（见第 9 节）。

示例（文档能力）：

```python
# 1) 全量文档索引里可查到: docs +search -> cmd_docs_search
res = client.cmd_docs_search(query="周报", format="json")

# 2) use 为 "api <method> <path>" 的命令，建议用 run 传位置参数
res2 = client.run(["api", "GET", "/open-apis/contact/v3/users/me", "--format", "json"])
```

## 12. 示例脚本

- `python_sdk/examples/sync_run_example.py`
- `python_sdk/examples/stream_session_example.py`
- `python_sdk/examples/generated_method_example.py`
- `python_sdk/examples/auth.py`

## 13. 已知限制

1. 动态库加载失败
- 检查 `LARK_CLI_SHARED_LIB` 是否为绝对路径。
- 或将动态库放到 `python_sdk/lark_cli_sdk/` 下（`liblarkcli.dylib`/`liblarkcli.so`）。

2. `authsidecar` 不支持
- 当前 `c-shared` 版本按设计不支持 `LARKSUITE_CLI_AUTH_PROXY` 场景。

3. `ok=True` 但 `exit_code!=0`
- 属于 CLI 业务失败，不是 SDK 崩溃，需按 `stderr` 错误处理。

## 14. 快速验证

```bash
make python-sdk-smoke
python3 python_sdk/scripts/generate_command_reference.py
python3 -m unittest python_sdk/tests/test_client_smoke.py
python3 -m unittest python_sdk/tests/test_auth_example.py
```
