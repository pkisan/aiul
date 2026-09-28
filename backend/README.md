# GenAI Log — backend

The Laravel app behind https://genailog.vardaam.site: it receives AI usage
events from the `aiul` agent and shows them on the dashboard.

- Local development: `../scripts/setup-backend.sh`, then `php artisan serve`
- Tests: `php artisan test`
- First account: `php artisan db:seed --class=SuperAdminSeeder` (prints the password once)
- Deploy to Plesk: `../docs/DEPLOY-PLESK.md`
