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
INTERESTING = ("/v1/messages", "/v1/chat/completions", "/backend-api/conversation")

SECRET_HEADERS = {
    "authorization", "x-api-key", "cookie", "set-cookie", "proxy-authorization",
    "anthropic-api-key", "openai-organization", "x-session-token",
}

UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", re.I)
EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.-]+")
TOKEN = re.compile(r"\b(sk-ant-[\w-]+|sk-[A-Za-z0-9]{20,}|oat01-[\w-]+)\b")


def load(loader):
    loader.add_option("fixture_out", str, "agent/testdata", "directory to write fixtures into")
    loader.add_option("fixture_case", str, "capture/case", "<provider>/<case> name for the files")


def scrub(text: str) -> str:
    text = TOKEN.sub("REDACTED-TOKEN", text)
    text = UUID.sub("00000000-0000-4000-8000-000000000000", text)
    return EMAIL.sub("someone@example.com", text)


def headers(message) -> dict:
    return {
        k.lower(): ("REDACTED" if k.lower() in SECRET_HEADERS else scrub(v))
        for k, v in message.headers.items()
    }


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
    streamed = "text/event-stream" in flow.response.headers.get("content-type", "")
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
