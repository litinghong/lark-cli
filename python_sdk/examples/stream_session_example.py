from lark_cli_sdk import LarkCLIClient


def main() -> None:
    client = LarkCLIClient()

    session = client.start(["event", "consume", "im.message.receive_v1", "--timeout", "10s"])
    try:
        for chunk in session.iter_output(max_bytes=65536):
            if chunk["stdout"]:
                print(chunk["stdout"].decode("utf-8", errors="replace"), end="")
            if chunk["stderr"]:
                print(chunk["stderr"].decode("utf-8", errors="replace"), end="")

        final = session.poll()
        print("done=", final["done"], "exit_code=", final["exit_code"], "error=", final["error"])
    finally:
        session.close()


if __name__ == "__main__":
    main()
