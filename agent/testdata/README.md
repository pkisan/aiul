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
```
