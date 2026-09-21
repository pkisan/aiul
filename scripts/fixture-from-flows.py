"""Turn a recorded mitmproxy flow file into anonymised fixtures for agent/testdata.

Research tool only: mitmproxy is never a dependency of the product. It runs
inside mitmproxy's own interpreter, so nothing needs installing:

    mitmweb --listen-host 127.0.0.1 --listen-port 8080 \
            --save-stream-file /tmp/aiul-h2.flows
    mitmdump -ns scripts/fixture-from-flows.py -r /tmp/aiul-h2.flows \
             --set fixture_out=agent/testdata --set fixture_case=anthropic/messages-h2

One pair of files per matching exchange, plus a .meta.json recording the HTTP
version — the whole point of this capture.

It removes credentials and identifiers, but it is NOT a substitute for reading
what it wrote before committing: use a prompt you invented for the recording.
"""

import json
import os
import re

from mitmproxy import ctx

# Only exchanges that carry a prompt. Anything else is noise for a parser.
INTERESTING = ("/v1/messages", "/v1/chat/completions", "/backend-api/conversation",
               "/backend-api/codex/responses", "/v1/responses")

SECRET_HEADERS = {
    "authorization", "x-api-key", "cookie", "set-cookie", "proxy-authorization",
    "anthropic-api-key", "openai-organization", "x-session-token",
}

HOME = re.compile(r"/Users/[^/\"'\\s]+")
UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", re.I)
EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.-]+")
TOKEN = re.compile(r"\b(sk-ant-[\w-]+|sk-[A-Za-z0-9]{20,}|oat01-[\w-]+)\b")


def load(loader):
    loader.add_option("fixture_out", str, "agent/testdata", "directory to write fixtures into")
    loader.add_option("fixture_case", str, "capture/case", "<provider>/<case> name for the files")


def scrub(text: str) -> str:
    text = TOKEN.sub("REDACTED-TOKEN", text)
    # A home directory carries the person's username, and tools put their paths
    # into system prompts constantly.
    text = HOME.sub("/Users/dev", text)
    text = UUID.sub("00000000-0000-4000-8000-000000000000", text)
    return EMAIL.sub("someone@example.com", text)


def headers(message) -> dict:
    return {
        k.lower(): ("REDACTED" if k.lower() in SECRET_HEADERS else scrub(v))
        for k, v in message.headers.items()
    }


def websocket_message(flow):
    """Record WebSocket frames.

    Codex talks to /backend-api/codex/responses over a WebSocket: the HTTP
    exchange is a bare 101 with no body, and every prompt and answer lives in
    frames. Without these there is nothing for a parser to be written against.

    One JSON object per line: direction, whether it was binary, and the payload.
    """
    if not any(part in flow.request.path for part in INTERESTING):
        return

    out = ctx.options.fixture_out
    case = ctx.options.fixture_case
    os.makedirs(os.path.join(out, os.path.dirname(case)), exist_ok=True)
    path = os.path.join(out, case + ".ws.jsonl")

    message = flow.websocket.messages[-1]
    payload = message.text if not message.is_text is False else None
    try:
        text = scrub(message.text)
    except (UnicodeDecodeError, AttributeError):
        text = None

    with open(path, "a") as f:
        json.dump({
            "from": "client" if message.from_client else "server",
            "binary": not message.is_text,
            "bytes": len(message.content),
            "payload": text,
        }, f, sort_keys=True)
        f.write("\n")


def websocket_end(flow):
    ctx.log.info(f"websocket closed: {flow.request.path} "
                 f"({len(flow.websocket.messages)} frames)")


def response(flow):
    if not any(part in flow.request.path for part in INTERESTING):
        return

    out = ctx.options.fixture_out
    case = ctx.options.fixture_case
    os.makedirs(os.path.join(out, os.path.dirname(case)), exist_ok=True)
    base = os.path.join(out, case)

    # Several exchanges in one recording must not overwrite each other.
    n = 0
    while os.path.exists(f"{base}{'' if n == 0 else '-' + str(n)}.meta.json"):
        n += 1
    if n:
        base = f"{base}-{n}"

    body = scrub(flow.request.get_text(strict=False) or "")
    with open(f"{base}.request.json", "w") as f:
        f.write(body if body.endswith("\n") else body + "\n")

    answer = scrub(flow.response.get_text(strict=False) or "")
    # Codex answers with event-stream framing and NO Content-Type header, which is
    # exactly why its responses are not being parsed — so decide by what the body
    # looks like, not by what the header claims.
    streamed = ("text/event-stream" in flow.response.headers.get("content-type", "")
                or answer.lstrip().startswith(("data:", "event:")))
    with open(f"{base}.response.{'sse' if streamed else 'json'}", "w") as f:
        f.write(answer if answer.endswith("\n") else answer + "\n")

    meta = {
        "request_http_version": flow.request.http_version,
        "response_http_version": flow.response.http_version,
        "method": flow.request.method,
        "host": flow.request.host,
        "path": flow.request.path,
        "status": flow.response.status_code,
        "streamed": streamed,
        "request_headers": headers(flow.request),
        "response_headers": headers(flow.response),
    }
    with open(f"{base}.meta.json", "w") as f:
        json.dump(meta, f, indent=2, sort_keys=True)
        f.write("\n")

    ctx.log.info(f"wrote {base}.* ({flow.request.http_version} -> {flow.response.http_version})")
