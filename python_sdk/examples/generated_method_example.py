from lark_cli_sdk import LarkCLIClient


def main() -> None:
    client = LarkCLIClient()

    # 自动生成方法示例（具体参数以 command_catalog 为准）
    # 注意：此处只是示例，实际命令是否成功取决于本地配置与权限
    result = client.cmd_schema(format="json", timeout_ms=5000)

    print("method:", "cmd_schema")
    print("exit_code:", result.exit_code)
    print("stdout preview:", result.stdout_text[:300])


if __name__ == "__main__":
    main()
