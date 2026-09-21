# TESTING — driving the whole thing by hand

How to take the module from nothing to a captured, redacted, task-tagged, scored
interaction on a dashboard, on one Mac, and then put the Mac back as it was.

Every step says what to expect. If a step does not match, stop there rather than
continuing: later steps assume the earlier ones worked.

**Two things this does to your Mac**, both undone in step 10 and at any moment by
`sudo ./scripts/killswitch.sh`:

- trusts a certificate authority it creates, so software here accepts certificates
  the proxy mints
- points the HTTPS proxy of every network service at `127.0.0.1:8899`

**What it does NOT do:** touch traffic to anything but the AI hostnames on the
allow-list. Everything else passes through sealed — no certificate minted, nothing
decrypted, nothing recorded but the hostname, byte counts and timing.

---

## 0. Before you start

Four terminals. Numbers below refer to them.

```sh
# TERMINAL 1 — the data services
open -a Docker && sleep 40
cd ~/Desktop/Aayatti && docker compose up -d && docker compose ps
```

Expect `aiul-postgres`, `aiul-redis`, `aiul-minio` all `(healthy)`.

---

## 1. The backend

```sh
# TERMINAL 1
cd ~/Desktop/Aayatti/backend
php artisan migrate --force
php artisan db:seed --class=DevUsersSeeder      # prints a random password ONCE — save it
php artisan serve --port=8088
```

The seeder makes three users in tenant `dev`, all with that one password:

| Email | Role | May read raw prompt text |
| --- | --- | --- |
| `dev@example.com` | member | no |
| `manager@example.com` | manager | no |
| `admin@example.com` | admin | yes, with a typed reason, and every read is logged |

```sh
# TERMINAL 2 — the queue, which does the scoring
cd ~/Desktop/Aayatti/backend && php artisan queue:work
```

## 2. A device token

```sh
# TERMINAL 3
cd ~/Desktop/Aayatti/backend
php artisan aiul:provision-device "$(hostname)" --tenant=dev
```

Copy the token it prints — it is shown once and stored only as a hash. Keep it in
a shell variable for the next step:

```sh
export AIUL_DEVICE_TOKEN='aiul_...'
```

## 3. Build and package

```sh
# TERMINAL 3
cd ~/Desktop/Aayatti
AIUL_VERSION=0.9.0 ./scripts/build.sh
AIUL_VERSION=0.9.0 ./scripts/package.sh
```

Expect `architectures: x86_64 arm64`, and a package at `dist/aiul-0.9.0.pkg` whose
contents are the single file `./usr/local/bin/aiul`.

`NOT SIGNED` is expected and correct: there is no Apple Developer account yet.

## 4. Install

Two files first. **`installer` does not pass its environment to package scripts**,
so nothing here can be given on the command line — it is silently ignored.

```sh
# TERMINAL 3

# 1. Lets an unmanaged Mac run the agent at all. The MDM check refuses otherwise.
sudo touch /etc/aiul-dev-unmanaged

# 2. Tells the agent where to send events, and what to authenticate with.
sudo mkdir -p /etc/aiul
sudo tee /etc/aiul/agent.conf >/dev/null <<EOF
AIUL_ENDPOINT=http://127.0.0.1:8088/api/aiul/events
AIUL_DEVICE_TOKEN=$AIUL_DEVICE_TOKEN
AIUL_DEBUG=1
EOF
sudo chmod 640 /etc/aiul/agent.conf    # it holds a credential

# The WORKER reads this file and the worker is not root, so it must be readable by
# the service account. `aiul install` sets root:_aiul 0640 for you; 0600 root-only
# means the agent starts with no endpoint and forwards nothing.

sudo installer -pkg dist/aiul-0.9.0.pkg -target /
```

`AIUL_DEBUG=1` is worth having for a first run: without it the worker logs only
that it started, and the lines saying why a particular request was or was not
recorded are invisible.

The install prints `Forwarding events to http://127.0.0.1:8088/...` when it has
somewhere to send them, and says so loudly when it does not.

**Installing is not capturing.** The agent is now running and watching, but
nothing has been sent through it. That is step 7.

Expect `The install was successful.` If it fails, it has already rolled itself
back and your Mac still works; the reason is in `/var/log/aiul-install.log`.

```sh
aiul status
sudo grep forwarding /var/log/aiul/agent.err.log | tail -1
```

Expect all of: `proxy listening yes`, `background job yes`, `CA trusted yes`,
`4 of 4 network services`, `env vars written 9` — and the log line must say
`forwarding=true`. If it says `forwarding=false`, the agent is capturing into its
spool and sending nothing: the config file above is missing or unreadable.

To change the endpoint later, edit `/etc/aiul/agent.conf` and restart the worker —
no reinstall:

```sh
sudo launchctl kickstart -k system/com.aiul.agent
```

## 5. Check the privilege split and the certificate chain

```sh
ps -axo user,command | grep 'aiul \(run\|helper\)' | grep -v grep
```

Expect exactly two processes, and the account column matters:

```
root   /usr/local/bin/aiul helper --group 448
_aiul  /usr/local/bin/aiul run --manage-proxy
```

The code that parses network traffic is the second one. It is not root.

```sh
sudo aiul ca device
```

Expect a certificate issued by the root, valid about 7 days, and a list of 19
permitted domains. That certificate cannot sign for anything else — a client
refuses it for any other name, so this device's key is useless for impersonating a
bank even if the laptop is stolen.

```sh
echo | openssl s_client -connect api.openai.com:443 -proxy 127.0.0.1:8899 2>/dev/null \
  | openssl x509 -noout -issuer
```

Expect `CN=AIUL Dev Root Device - <your hostname>` — the leaf was signed by the
device certificate, not by the root.

## 6. Prove only AI hosts are decrypted

```sh
echo | openssl s_client -connect example.com:443 -proxy 127.0.0.1:8899 2>/dev/null \
  | openssl x509 -noout -issuer
```

Expect a REAL issuer (a Cloudflare or DigiCert CA). Not ours. That host was passed
through sealed and nothing about its content exists anywhere.

## 7. Capture a real interaction, tagged to a task

Task tagging comes from the git branch of the directory the tool was run in, so
work in a checkout on a ticket-shaped branch.

```sh
# TERMINAL 4 — a NEW terminal, so it picks up the environment variables
mkdir -p /tmp/aiul-test && cd /tmp/aiul-test && git init -q .
git checkout -q -b feature/AIUL-100-first-capture
echo "$HTTPS_PROXY"     # must print http://127.0.0.1:8899
```

Then either use a real tool:

```sh
claude -p "explain what a reverse proxy does in two sentences"
```

or, with no API account, drive the endpoint directly — a 401 still proves capture,
tagging and redaction, because those happen on the request:

```sh
curl -sS -o /dev/null -w '%{http_code}\n' https://api.anthropic.com/v1/messages \
  -H 'content-type: application/json' -H 'anthropic-version: 2023-06-01' \
  -H 'x-api-key: sk-ant-not-a-real-key' \
  -d '{"model":"claude-opus-5","max_tokens":32,"messages":[{"role":"user","content":"my key is sk-ant-api03-EXAMPLEFAKEKEY and my email is someone@example.com, review this"}]}'
```

Watch terminal 2: the queue worker should score the interaction within seconds.

### What to check

```sh
cd ~/Desktop/Aayatti/backend
php artisan tinker --execute='
use App\Models\AiInteraction;
$i = AiInteraction::withoutGlobalScope("tenant")->latest("id")->first();
print_r([
  "host" => $i->host, "parser" => $i->parser, "model" => $i->model,
  "task_id" => $i->task_id, "branch" => $i->branch, "tool" => $i->tool,
  "redacted" => $i->redacted, "score" => $i->score?->score,
  "prompt_object" => $i->prompt_object,
]);'
```

Expect:

- `task_id` = `AIUL-100` and the branch name — **no one typed that**; it came from
  the connection's source port, to the process, to its working directory, to
  `.git/HEAD`
- `redacted` naming the rules that fired (an Anthropic key and an email address) —
  rule NAMES only, never the values
- `prompt_object` a path in object storage, and a score once the queue has run

Prove the stored body is encrypted and the secret is gone:

```sh
php artisan tinker --execute='
use App\Models\AiInteraction;
use App\Services\BodyStore;
use Illuminate\Support\Facades\Storage;
$i = AiInteraction::withoutGlobalScope("tenant")->latest("id")->first();
echo "RAW IN BUCKET: ".substr(Storage::disk("s3")->get($i->prompt_object), 0, 60)."\n\n";
echo "DECRYPTED:     ".app(BodyStore::class)->get($i->prompt_object)."\n";'
```

Expect the bucket line to be unreadable ciphertext, and the decrypted line to show
your sentence with the key and the email replaced by masks while the rest stays
readable. **The provider received the original bytes** — redaction only ever
touches our copy.

## 7b. The other surfaces: browser, IDE, extension

The CLI is the proven path. These are the ones where the answer is partly "not
yet", and knowing exactly which part is the point of the exercise.

**GUI applications do not see the environment variables until you log out and back
in.** The install writes a LaunchAgent that sets them at login. To test without
logging out, start the application from a terminal that already has them:

```sh
# Cursor, VS Code — Electron apps read NODE_EXTRA_CA_CERTS
open -a "Visual Studio Code" --env NODE_EXTRA_CA_CERTS=/usr/local/share/aiul/root.crt \
                             --env HTTPS_PROXY=http://127.0.0.1:8899
open -a Cursor --env NODE_EXTRA_CA_CERTS=/usr/local/share/aiul/root.crt \
               --env HTTPS_PROXY=http://127.0.0.1:8899
```

Quit the application first — `open` will not re-launch one that is already
running, and it keeps the environment it started with.

### Browser: chatgpt.com and claude.ai

Safari and Chrome use the macOS keychain, where our CA is trusted, so the
handshake should succeed. Have a short conversation in each, then:

```sh
sudo grep -E "claude.ai|chatgpt.com|openai.com" /var/log/aiul/agent.err.log | tail -20
```

Three outcomes, all informative:

| What the log says | What it means |
| --- | --- |
| `no parser for this endpoint; nothing recorded` | **Expected today.** We decrypted it; nobody has written a parser for that endpoint. Note the exact `path=` — that is the input to writing one. |
| `client rejected our certificate; tunneling this host from now on` | That client pins its certificate. It will keep working, recorded as metadata only, and **no parser can ever change that.** |
| `masked secrets before storing` / an event appears | it was captured and recorded |

Firefox will fail differently: it has its own trust store and ignores the
keychain, so it needs a policy file before it can be captured at all.

### IDE: GitHub Copilot in VS Code

Copilot's API is allow-listed and parsed. Open a file, let Copilot suggest
something, or use Copilot Chat, then:

```sh
sudo grep -E "githubcopilot|copilot-proxy" /var/log/aiul/agent.err.log | tail -10
```

Completions are a different shape from chat — expect chat to parse and inline
completions possibly not.

### IDE: Cursor

**Expect nothing.** Cursor talks to `api2.cursor.sh` and similar, which are NOT on
the allow-list, so that traffic is passed through sealed and never decrypted. This
is the gap, not a bug. To see it for yourself, watch what goes past:

```sh
sudo grep -E "cursor" /var/log/aiul/agent.err.log | tail -10
```

`decision=pass` lines with a cursor hostname are the evidence that it is invisible,
and the hostnames you see there are exactly what the allow-list is missing.

### Capturing what a tool sends, to write a parser for it

The agent can write down the exchanges it cannot parse, which is what a new parser
is written against. No second proxy and no second CA: it is already decrypting
this traffic.

```sh
sudo mkdir -p /var/db/aiul/research
sudo chown _aiul:_aiul /var/db/aiul/research
echo 'AIUL_RESEARCH_DUMP=/var/db/aiul/research' | sudo tee -a /etc/aiul/agent.conf
sudo launchctl kickstart -k system/com.aiul.agent
```

Use the tool — one short conversation is enough — then look:

```sh
sudo ls -t /var/db/aiul/research | head
sudo cat "/var/db/aiul/research/$(sudo ls -t /var/db/aiul/research | head -1)"
```

Each file holds one exchange: host, path, the headers a parser might key on, and
the request and response bodies. **Bodies are redacted** — values are masked, the
JSON structure a parser needs is not. Static assets are skipped, so a web page
load does not bury the one file that matters.

Turn it off when finished, and delete what it wrote:

```sh
sudo sed -i '' '/AIUL_RESEARCH_DUMP/d' /etc/aiul/agent.conf
sudo launchctl kickstart -k system/com.aiul.agent
sudo rm -rf /var/db/aiul/research
```

It is still a record of real conversations, redacted or not: the directory is
0700, the files 0600, and the agent says loudly in its log while it is on.

### Write down what you find

For each surface: hostname, endpoint path, whether the certificate was accepted.
That list is the input to the capture work in `docs/ROADMAP.md` step 1, and it is
worth more than any guess I can make about what these tools do.

## 8. The dashboard

Open <http://127.0.0.1:8088/usage> and sign in as `manager@example.com`.

- the interaction appears under task `AIUL-100`, with its score and the weakest
  dimensions of the prompt
- click through to it and try to read the prompt text: **refused, 403.** A manager
  sees aggregates, never words.

Sign out, sign in as `admin@example.com`:

- the raw view now asks for a reason, and refuses without one
- with a reason typed, the text appears — and <http://127.0.0.1:8088/usage/audit>
  records who looked, at what, why, and from which IP

Then <http://127.0.0.1:8088/my-data> as any of the three: what was captured about
that person, and who has read it.

## 9. The safety properties

**Fail open.** Kill the worker and watch the proxy setting be removed rather than
traffic being blocked:

```sh
sudo launchctl kill SIGKILL system/com.aiul.agent
sleep 35                                 # the health loop runs every 30s
networksetup -getsecurewebproxy Wi-Fi    # expect Enabled: No
curl -sI https://example.com | head -1   # expect HTTP/2 200
```

launchd restarts the worker, which re-applies the setting within another 30
seconds. `aiul status` will show it back.

**Retention.** Bodies older than the tenant's window are deleted and everything
derived from them survives:

```sh
cd ~/Desktop/Aayatti/backend
php artisan tinker --execute='
use App\Models\AiInteraction;
AiInteraction::withoutGlobalScope("tenant")->latest("id")->first()
  ->forceFill(["occurred_at" => now()->subDays(200)])->save();'
php artisan aiul:purge-bodies --dry-run     # says what it would purge
php artisan aiul:purge-bodies                # does it
```

Then re-run the check from step 7: `prompt_object` is now null, the object is gone
from the bucket, and `task_id`, the token counts and the score are untouched.

## 10. Put the Mac back

```sh
cd ~/Desktop/Aayatti
sudo aiul uninstall
sudo rm -f /etc/aiul-dev-unmanaged
aiul status                              # "This Mac has no aiul settings applied."
curl -sI https://example.com | head -1   # internet works
```

Confirm by hand that nothing is left:

```sh
ps -axo user,command | grep 'aiul ' | grep -v grep     # nothing
security find-certificate -c 'AIUL Dev Root' /Library/Keychains/System.keychain   # not found
dscl . -read /Users/_aiul                              # eDSRecordNotFound
grep -c AIUL /etc/zshenv                               # no such file, or 0
```

`/var/db/aiul` is left behind on purpose — it holds the device certificate and any
spooled events, and an uninstall must never destroy captured data unasked. It
contains prompt text in plaintext, so remove it when finished:

```sh
sudo rm -rf /var/db/aiul
```

Stop the data services when done: `docker compose down` (add `-v` to delete their
data too).

---

## If something goes wrong

| Symptom | Where the answer is |
| --- | --- |
| install failed | `/var/log/aiul-install.log`, last 20 lines |
| agent not capturing | `sudo tail -50 /var/log/aiul/agent.err.log` |
| nothing in the log at all | the job may not have started: `sudo launchctl print system/com.aiul.agent` |
| need more detail | reinstall with `AIUL_DEBUG=1` in the installer environment |
| captures happen but nothing reaches the dashboard | `forwarding=false` in the worker log: `/etc/aiul/agent.conf` is missing, and events are waiting in `/var/db/aiul/spool` |
| anything at all | `sudo ./scripts/killswitch.sh` reverts every system change in one command |

## What this test does NOT cover

- **The web applications.** `claude.ai`, `chatgpt.com`, `gemini.google.com` and
  `aistudio.google.com` are decrypted but have no parser, so a browser
  conversation is recorded as metadata only. Their endpoints are private and need
  real captures first — the research workflow in CLAUDE.md.
- **Providers other than OpenAI, Anthropic and Gemini.** Nine more are parsed from
  their documented shapes and have never been driven live.
- **A managed device.** Every run so far has skipped the MDM check with the marker
  file. The gate itself is unproven against a real enrollment.
- **Signing.** Unsigned packages install with `installer` and are refused by
  Gatekeeper on a double-click and by every MDM.
