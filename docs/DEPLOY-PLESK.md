# Deploying GenAI Log to Plesk (genailog.vardaam.site)

The backend is a normal Laravel app. On Plesk it needs PHP, PostgreSQL and one
scheduled task. No Redis, no queue worker, no Node, no Composer on the server:
the zip you upload already contains `vendor/` and the built CSS/JS.

## What the server needs

| Item | Value |
| --- | --- |
| PHP | 8.3 or newer, FPM, with `pdo_pgsql`, `mbstring`, `openssl`, `intl`, `fileinfo`, `zip` |
| Database | PostgreSQL 14+ (Plesk: *Databases → Add Database*, type PostgreSQL). MySQL will NOT work — the reports use Postgres SQL. |
| SSL | Let's Encrypt certificate for `genailog.vardaam.site`, "Redirect from HTTP to HTTPS" on |
| Document root | `httpdocs/public` (the app lives in `httpdocs`, only `public/` is served) |

## 1. Build the zip (on your Mac)

```bash
git commit ...                      # the zip is made from the last commit
./scripts/package-plesk.sh          # -> dist/genailog-<commit>.zip
```

## 2. First upload

1. Plesk → *Websites & Domains* → genailog.vardaam.site → *Hosting Settings*:
   set **Document root** to `httpdocs/public`, PHP version 8.3+.
2. *File Manager* → `httpdocs`: upload the zip and *Extract*. The folder now has
   `app/`, `public/`, `vendor/`, `artisan` …
3. Copy `.env.production.example` to `.env` and fill in the `CHANGE_ME` values
   (database name, user, password from step "Databases").
4. Plesk → *SSH Terminal* (or `ssh` in), then:

```bash
cd ~/httpdocs
php artisan key:generate --force          # once, ever: it encrypts stored prompts
php artisan migrate --force
AIUL_ADMIN_EMAIL=you@vardaam.com AIUL_ADMIN_NAME="Your Name" \
  php artisan db:seed --class=SuperAdminSeeder --force
php artisan config:cache && php artisan route:cache && php artisan view:cache
chmod -R u+rwX,go-w storage bootstrap/cache
```

The seeder prints the super admin's password **once**. Sign in at
https://genailog.vardaam.site, accept the notice, then add everyone else from
**People**. Lost the password? Run the seeder again: it prints a new one.

> **Back up `APP_KEY`.** Stored prompt text is encrypted with it. A new key makes
> every stored prompt unreadable.

## 3. Scheduled task (retention)

Plesk → *Scheduled Tasks* → *Add Task*, type **Run a PHP script**:

- Script path: `httpdocs/artisan`, arguments: `schedule:run`
- Run: every minute (`* * * * *`)

This runs the nightly purge of prompt text older than the retention window.

## 4. Point the agents at it

On each device (or in the enrol command):

```bash
# on the server: one token per device
php artisan aiul:provision-device <hostname> --user=<person's email>
# on the device
./scripts/enroll-device.sh --endpoint https://genailog.vardaam.site/api/aiul/events --token aiul_xxx
```

## Updating

```bash
./scripts/package-plesk.sh
```

Upload and extract over `httpdocs` (keep `.env` and `storage/`), then:

```bash
cd ~/httpdocs
php artisan migrate --force
php artisan optimize:clear && php artisan config:cache && php artisan route:cache && php artisan view:cache
```

## Checks after deploying

- `https://genailog.vardaam.site/up` answers 200.
- `https://genailog.vardaam.site/.env` answers 404 (document root is `public/`).
- Signing in lands on **AI usage**; the sun/moon button switches dark mode.
