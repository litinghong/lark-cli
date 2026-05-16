from lark_cli_sdk import LarkCLIClient


def main() -> None:
    client = LarkCLIClient()

    result = client.run(["schema", "calendar", "--format", "json"], timeout_ms=5000)

    print("ok=", result.ok)
    print("exit_code=", result.exit_code)
    print("stdout=", result.stdout_text[:500])
    print("stderr=", result.stderr_text)


if __name__ == "__main__":
    main()
