# testdata

Recorded, anonymised traffic used to test the parsers.

**Every file here is hand-written or anonymised.** No real API key, account
identifier, customer name or private prompt may ever be committed. When a real
capture is used, replace: the Authorization header, any `user`/`org`/`account` id,
the message ids, and the prompt text itself.

## How these are produced

1. The owner runs ONE tool through `mitmweb` in a separate terminal:
   `mitmweb --listen-host 127.0.0.1 --listen-port 8080`
2. They perform a few real interactions with that tool.
3. We copy the request body, the response body and the headers that matter,
   anonymise them, and save them here.
4. The Go parser is written against those files.

mitmproxy is only ever a research tool. The product never depends on it.

## Layout

```
<provider>/<case>.request.json    the request body
<provider>/<case>.response.sse    a streamed response, raw SSE
<provider>/<case>.response.json   a whole (non-streamed) response
<provider>/<case>.meta.json       method, path, status, headers, HTTP version
<provider>/<case>.ws.jsonl        WebSocket frames, one JSON object per line
```

`.ws.jsonl` exists because Codex does not use HTTP bodies at all: it opens
`GET /backend-api/codex/responses`, gets `101 Switching Protocols`, and every
prompt and answer travels as WebSocket frames. Each line records `from`
("client" or "server"), `binary`, `bytes` and the scrubbed `payload`.

`.meta.json` exists for the HTTP/2 work: a fixture is only proof of an h2
exchange if the version it was recorded over is written down next to it.

## Recording one with the converter

`scripts/fixture-from-flows.py` runs inside mitmproxy's own interpreter, so
nothing needs installing, and it strips credentials, uuids and e-mail addresses
on the way out:

```
mitmweb --listen-host 127.0.0.1 --listen-port 8080 \
        --save-stream-file /tmp/aiul-h2.flows
mitmdump -ns scripts/fixture-from-flows.py -r /tmp/aiul-h2.flows \
         --set fixture_out=agent/testdata --set fixture_case=anthropic/messages-h2
```

It is not a substitute for reading what it wrote before committing. Record with
a prompt you invented for the purpose, so the text in the fixture is safe to
publish whatever the scrubber missed.
