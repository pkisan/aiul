// Breeze's app.js imports this file, but Laravel 13's skeleton no longer ships it.
// Axios with the XSRF header is all it ever did.
import axios from 'axios';

window.axios = axios;
window.axios.defaults.headers.common['X-Requested-With'] = 'XMLHttpRequest';
