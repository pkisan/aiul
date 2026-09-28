# Deploying GenAI Log to Plesk (genailog.vardaam.site)

The backend is a normal Laravel app. On Plesk it needs PHP 8.3+, PostgreSQL and
one scheduled task. No Redis, no queue worker, and no Node or Composer on the
server: the zip you upload already contains `vendor/` and the built CSS/JS.

## 0. One-time Plesk setup (blank site)

In Plesk, for **genailog.vardaam.site**:

1. **PostgreSQL available?** *Tools & Settings → Database Servers* must list a
   PostgreSQL server. If only MySQL/MariaDB is there, ask the host to add
   PostgreSQL. MySQL will NOT work: the reports use Postgres SQL.
2. **Database:** *Databases → Add Database*. Type **PostgreSQL**, name e.g.
   `genailog`, and a database user and password. Note all three.
3. **PHP:** *PHP Settings* → PHP **8.3 or 8.4**, handler *FPM application
   served by nginx* (or Apache). Make sure `pdo_pgsql` / `pgsql` is ticked in
   the extension list.
4. **Document root:** *Hosting & DNS → Hosting* → Document root
   `httpdocs/public`.
5. **SSL:** *SSL/TLS Certificates* → Let's Encrypt for the domain, then switch
   on "Redirect from HTTP to HTTPS".
6. **SSH:** *Hosting & DNS → Web Hosting Access* → "Access to the server over
   SSH": `/bin/bash`. (Or use Plesk's *SSH Terminal* extension.)

In SSH, `php` may be the system PHP, not the site's. Use the site's own:

```bash
PHP=/opt/plesk/php/8.3/bin/php      # match the version picked in step 3
$PHP -v
```

## 1. Build the zip (on your Mac)

```bash
cd ~/Desktop/Aayatti
git status                           # the zip is built from the last commit
./scripts/package-plesk.sh           # -> dist/genailog-<commit>.zip
```

## 2. Upload

*Files* → `httpdocs`: delete Plesk's placeholder files (`index.html`, `css/`,
`img/` …), upload the zip, right-click → **Extract**. `httpdocs` now holds
`app/`, `public/`, `vendor/`, `artisan`, `.env.production.example` …

## 3. Configure `.env`

In *Files*, copy `.env.production.example` to `.env` and edit it:

- `DB_DATABASE`, `DB_USERNAME`, `DB_PASSWORD` — from step 0.2
- `DB_HOST` — `localhost` (or what Plesk shows under *Databases*)
- `AIUL_TENANT` / `AIUL_TENANT_NAME` — your company slug and name

## 4. First-time commands (SSH)

```bash
cd ~/httpdocs
PHP=/opt/plesk/php/8.3/bin/php

$PHP artisan key:generate --force     # ONCE, ever. It encrypts stored prompts.
$PHP artisan migrate --force          # creates the tables
AIUL_ADMIN_EMAIL=you@vardaam.com AIUL_ADMIN_NAME="Your Name" \
  $PHP artisan db:seed --class=SuperAdminSeeder --force
$PHP artisan optimize                 # caches config, routes, views
chmod -R u+rwX,go-w storage bootstrap/cache
```

The seeder prints the super admin's password **once** — copy it. Lost it?
Run the seeder line again; it prints a new one.

> **Back up the `APP_KEY` line in `.env`.** Stored prompt text is encrypted
> with it. Losing it makes every stored prompt unreadable.

## 5. Scheduled task (deletes old prompt text nightly)

*Scheduled Tasks → Add Task*:

- Task type: **Run a PHP script**
- Script path: `httpdocs/artisan`, arguments: `schedule:run`
- PHP version: same as step 0.3
- Run: **Cron style** `* * * * *`

## 6. Check it

- `https://genailog.vardaam.site/up` → green "Application up" page.
- `https://genailog.vardaam.site/.env` → 404.
- Sign in with the super admin; accept the notice; you land on **AI usage**.
- *People* → add everyone else; each gets a one-time password to send them.

## 7. Point devices at it

```bash
# on the server, one token per device
$PHP artisan aiul:provision-device <hostname> --user=<person's email>
# on the device (Mac / Ubuntu)
./scripts/enroll-device.sh --endpoint https://genailog.vardaam.site/api/aiul/events --token aiul_xxx
```

## Updating later

```bash
./scripts/package-plesk.sh        # on the Mac
```

Upload and extract over `httpdocs` (your `.env` and `storage/` stay), then:

```bash
cd ~/httpdocs
$PHP artisan migrate --force
$PHP artisan optimize:clear && $PHP artisan optimize
```

## If something goes wrong

- Blank page or "500": read `storage/logs/laravel-*.log`.
- "could not find driver": `pdo_pgsql` not enabled for this site's PHP (step 0.3).
- CSS missing / 404 on `/build/...`: document root is not `httpdocs/public` (step 0.4).
