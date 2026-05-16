# lark-cli Python SDK（本地动态库版）

本目录提供 `lark-cli` 的 Python 封装，底层通过 `ctypes` 调用 Go `c-shared` 动态库，实现：

- 同步调用：`run(args)`
- 流式调用：`start()/poll()/cancel()`
- 自动命令方法：按命令目录自动挂载 `cmd_<path>` 方法

## 1. 前置条件

- Python 3.10+
- 已在仓库根目录构建动态库

## 2. 构建动态库

在仓库根目录执行：

```bash
# macOS
make build-shared-darwin

# Linux
make build-shared-linux

# 生成命令目录（用于方法自动生成）
make build-shared-catalog
```

产物：

- macOS: `dist/darwin/liblarkcli.dylib`
- Linux: `dist/linux/liblarkcli.so`

## 3. 引入 SDK

在仓库根目录运行脚本时，设置：

```bash
export PYTHONPATH=python_sdk
export LARK_CLI_SHARED_LIB=/absolute/path/to/liblarkcli.dylib  # 或 .so
```

然后在 Python 中：

```python
from lark_cli_sdk import LarkCLIClient

client = LarkCLIClient()
```

也可显式传入路径：

```python
client = LarkCLIClient("/absolute/path/to/liblarkcli.dylib")
```

## 4. 基础用法

### 4.1 通用同步调用

```python
result = client.run(["schema", "calendar", "--format", "json"])
print(result.ok, result.exit_code)
print(result.stdout_text)
print(result.stderr_text)
```

返回对象 `LarkCLIResult` 字段：

- `ok: bool`：FFI 层调用是否成功
- `exit_code: int`：CLI 退出码
- `stdout: bytes`
- `stderr: bytes`
- `error: str`：SDK/ABI 层错误
- `json_envelope`：尝试把 `stdout` 解析为 JSON（失败时返回 `None`）

### 4.2 传入 stdin、timeout、环境变量覆盖

```python
result = client.run(
    ["api", "POST", "/open-apis/im/v1/messages", "--data", "-"],
    stdin='{"receive_id":"ou_xxx","msg_type":"text","content":"{\\"text\\":\\"hi\\"}"}',
    timeout_ms=10_000,
    env_overrides={
        "LARKSUITE_CLI_NO_UPDATE_NOTIFIER": "1",
        "LARKSUITE_CLI_NO_SKILLS_NOTIFIER": "1",
    },
)
```

说明：

- `env_overrides` 的 value 传 `None` 表示临时 unset。
- `timeout_ms` 超时后由库侧 context 取消。

## 5. 流式命令调用

适用于 `event consume` / `watch` 这类长运行命令。

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

主动取消：

```python
session.cancel()
session.close()
```

## 6. 自动生成命令方法

初始化 `LarkCLIClient` 后会根据命令目录自动注入方法：

- 规则：`<command path>` -> `cmd_<path>`
- `+` 前缀会去掉，`-` 转成 `_`

示例：

- `calendar +agenda` -> `client.cmd_calendar_agenda(...)`
- `schema` -> `client.cmd_schema(...)`

调用示例：

```python
# 等价于: lark-cli schema calendar --format json
res = client.cmd_schema(format="json", timeout_ms=5000)

# 等价于: lark-cli calendar +agenda --days 3 --format json
res = client.cmd_calendar_agenda(days=3, format="json")
```

参数映射规则：

- flag `--page-limit` -> 关键字参数 `page_limit`
- bool flag: `True` 追加 `--flag`，`False` 追加 `--flag=false`
- `stringArray/stringSlice`：传 `list/tuple`，展开为重复 flag

注意：

- 自动方法目前只覆盖 flag，不覆盖位置参数（有位置参数的命令建议先用 `run(args)`）。
- stream 类型命令的自动方法返回 `LarkCLISession`；sync 命令返回 `LarkCLIResult`。

## 7. 命令目录查询

```python
catalog = client.command_catalog()
print(len(catalog["commands"]))
print(catalog["commands"][0])
```

目录字段包括：

- `path`, `use`, `short`, `hidden`, `leaf`
- `mode`: `sync | stream | interactive-limited`
- `flags`: `name/type/default/required/...`

## 8. 常见问题

1. 找不到动态库
- 设置 `LARK_CLI_SHARED_LIB` 为绝对路径。
- 或将动态库放到 `python_sdk/lark_cli_sdk/` 下（文件名需为 `liblarkcli.dylib`/`liblarkcli.so`）。

2. `authsidecar` 不支持
- 当前 c-shared 版本按设计不支持 `LARKSUITE_CLI_AUTH_PROXY` 场景。

3. `result.ok=True` 但 `exit_code!=0`
- 说明 FFI 调用成功，但 CLI 业务执行失败；请看 `stderr_text` 的 JSON 错误 envelope。

## 9. 快速验证

```bash
make python-sdk-smoke
```
