// Switch this to 'prod' before uploading a release build. The mini-program
// runtime enforces HTTPS and the request-domain allowlist in production, so
// the dev host below only works in the devtools with urlCheck disabled.
const ENV = 'dev'

const ENV_CONFIG = {
  dev: {
    API_BASE_URL: 'http://127.0.0.1:8080/api/v1',
    WS_BASE_URL: 'ws://127.0.0.1:8080/api/v1',
  },
  prod: {
    // TODO: point these at the deployed HTTPS/WSS domain and add it to the
    // mini-program request + socket allowlists.
    API_BASE_URL: 'https://example.com/api/v1',
    WS_BASE_URL: 'wss://example.com/api/v1',
  },
}

const active = ENV_CONFIG[ENV] || ENV_CONFIG.dev

const API_BASE_URL = active.API_BASE_URL
const WS_BASE_URL = active.WS_BASE_URL
const USE_MOCK_AUTH = false
const TENCENT_MAP_KEY = ''

module.exports = {
  ENV,
  API_BASE_URL,
  WS_BASE_URL,
  USE_MOCK_AUTH,
  TENCENT_MAP_KEY,
}
