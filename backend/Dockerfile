# The backend as one image, for the demo: a tester needs Docker and nothing else —
# no PHP, Composer or Node on their machine, whichever OS it is.
#
# Built by compose.demo.yaml at the repo root. Three stages, because each tool is
# only needed to produce files, not to run the app:
#   1. composer  -> vendor/            (PHP dependencies)
#   2. node      -> public/build/      (the compiled dashboard; needs vendor/ for Ziggy)
#   3. runtime   -> PHP plus the two outputs above

FROM composer:2 AS vendor
WORKDIR /app
COPY composer.json composer.lock ./
# --ignore-platform-reqs: this stage only downloads code; the extensions are
# installed in the runtime stage. --no-scripts: artisan is not copied yet.
RUN composer install --no-dev --no-interaction --no-scripts --prefer-dist --ignore-platform-reqs
COPY . .
RUN composer dump-autoload --optimize --no-dev

FROM node:22-alpine AS assets
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --silent
COPY . .
COPY --from=vendor /app/vendor ./vendor
RUN npm run build

FROM php:8.4-cli
# install-php-extensions handles the build dependencies each extension needs.
COPY --from=mlocati/php-extension-installer /usr/bin/install-php-extensions /usr/local/bin/
RUN install-php-extensions pdo_pgsql redis pcntl zip intl bcmath
WORKDIR /app
COPY --from=vendor /app /app
COPY --from=assets /app/public/build /app/public/build
RUN chmod +x docker/entrypoint.sh
ENTRYPOINT ["docker/entrypoint.sh"]
CMD ["serve"]
