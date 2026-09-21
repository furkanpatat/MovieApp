// k6 load test for the MovieApp stack, driven through the API Gateway.
//
//   users:   a pool is registered + logged in once in setup() (bcrypt makes
//            per-VU signup far too heavy); 5,000 VUs then share the pool's tokens.
//            A trickle of *real* signup+login journeys runs alongside so the
//            auth path (bcrypt) is exercised under load too.
//   journey: ~85% read  GET  /api/v1/interaction/movies/{id}/interactions
//            ~10% write POST /api/v1/interaction/movies/{id}/rate     -> 202
//            ~ 5% write POST /api/v1/interaction/movies/{id}/comment  -> 202
//
// Pass criteria (thresholds): p95 < 200 ms, zero 5xx, writes accepted (202),
// and zero 429s (proves the temporary rate-limit override is in effect).
import http from 'k6/http';
import exec from 'k6/execution';
import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://gateway:8080';
const VUS = int('VUS', 5000);
const RAMP = __ENV.RAMP || '2m';
const HOLD = __ENV.HOLD || '3m';
const DOWN = __ENV.DOWN || '30s';
const POOL_SIZE = int('POOL_SIZE', 200);
const SIGNUP_RATE = int('SIGNUP_RATE', 2); // full register+login journeys per second
const MOVIES = int('MOVIES', 50); // write/read spread: movie ids 1..MOVIES
const P_READ = num('P_READ', 0.85);
const P_RATE = num('P_RATE', 0.10); // remainder = comments
const THINK_MIN = num('THINK_MIN', 0.5); // seconds between actions
const THINK_MAX = num('THINK_MAX', 1.5);
const RUN_ID = __ENV.RUN_ID || String(Date.now());

function int(k, d) { return __ENV[k] ? parseInt(__ENV[k], 10) : d; }
function num(k, d) { return __ENV[k] ? parseFloat(__ENV[k]) : d; }

const serverErrors = new Rate('server_errors'); // 5xx (or no response) -> must be 0
const rateLimited = new Counter('rate_limited'); // 429 -> must be 0 during the test
const writeAccepted = new Rate('write_accepted'); // POST rate/comment == 202
const authOK = new Rate('auth_flow_ok');
// Exact counts of accepted (202) writes, reconciled against Postgres after the run.
const acceptedRatings = new Counter('accepted_ratings');
const acceptedComments = new Counter('accepted_comments');

export const options = {
  setupTimeout: '10m',
  scenarios: {
    journeys: {
      executor: 'ramping-vus',
      exec: 'journey',
      startVUs: 0,
      stages: [
        { duration: RAMP, target: VUS },
        { duration: HOLD, target: VUS },
        { duration: DOWN, target: 0 },
      ],
      gracefulRampDown: '30s',
    },
    signups: {
      executor: 'constant-arrival-rate',
      exec: 'signup',
      rate: SIGNUP_RATE,
      timeUnit: '1s',
      duration: `${toSeconds(RAMP) + toSeconds(HOLD) + toSeconds(DOWN)}s`,
      preAllocatedVUs: 5,
      maxVUs: 50,
    },
  },
  thresholds: {
    // The SLO: 95% of API requests complete in under 200 ms.
    'http_req_duration{kind:api}': ['p(95)<200'],
    'http_req_duration{op:read}': ['p(95)<200'],
    'http_req_duration{op:rate}': ['p(95)<200'],
    'http_req_duration{op:comment}': ['p(95)<200'],
    // No 500/502/503 (or connection failures) anywhere.
    server_errors: ['rate==0'],
    // CQRS writes are acknowledged with 202 Accepted.
    write_accepted: ['rate>0.999'],
    // The rate-limit override must be active, or this run measures 429s.
    rate_limited: ['count==0'],
    // Signup/login run bcrypt (cost 12): judged separately and much looser.
    'http_req_duration{kind:auth}': ['p(95)<3000'],
    auth_flow_ok: ['rate>0.99'],
  },
  // Keep memory per VU low: we only need status codes in the journey.
  discardResponseBodies: false,
  noConnectionReuse: false,
  userAgent: 'movieapp-k6/1.0',
};

function toSeconds(d) {
  const m = /^(\d+)(s|m|h)$/.exec(d);
  if (!m) throw new Error(`bad duration ${d}`);
  return parseInt(m[1], 10) * { s: 1, m: 60, h: 3600 }[m[2]];
}

const JSON_HEADERS = { 'Content-Type': 'application/json' };

function credentials(i) {
  const name = `lt${RUN_ID}_${i}`;
  return { username: name, email: `${name}@loadtest.local`, password: 'load-test-password' };
}

// setup(): register + log in POOL_SIZE users once. Chunks keep bcrypt from
// starving the auth service (each hash is ~250 ms of CPU at cost 12).
export function setup() {
  const tokens = [];
  const CHUNK = 10;
  for (let start = 0; start < POOL_SIZE; start += CHUNK) {
    const n = Math.min(CHUNK, POOL_SIZE - start);
    const regs = [];
    for (let i = 0; i < n; i++) {
      regs.push(['POST', `${BASE}/api/v1/auth/register`, JSON.stringify(credentials(start + i)), { headers: JSON_HEADERS, tags: { kind: 'setup' }, timeout: '60s' }]);
    }
    const regRes = http.batch(regs);
    regRes.forEach((r, i) => {
      if (r.status !== 201) throw new Error(`setup: register ${start + i} -> ${r.status} ${r.body}`);
    });
    const logins = [];
    for (let i = 0; i < n; i++) {
      const c = credentials(start + i);
      logins.push(['POST', `${BASE}/api/v1/auth/login`, JSON.stringify({ login: c.username, password: c.password }), { headers: JSON_HEADERS, tags: { kind: 'setup' }, timeout: '60s' }]);
    }
    http.batch(logins).forEach((r, i) => {
      if (r.status !== 200) throw new Error(`setup: login ${start + i} -> ${r.status} ${r.body}`);
      tokens.push(r.json('access_token'));
    });
  }
  console.log(`setup: ${tokens.length} users registered and authenticated`);
  return { tokens };
}

function record(res, op) {
  serverErrors.add(res.status === 0 || res.status >= 500);
  if (res.status === 429) rateLimited.add(1);
  if (op !== 'read') {
    writeAccepted.add(res.status === 202);
    if (res.status === 202) (op === 'comment' ? acceptedComments : acceptedRatings).add(1);
  }
}

export function journey(data) {
  const token = data.tokens[exec.vu.idInTest % data.tokens.length];
  const movie = 1 + Math.floor(Math.random() * MOVIES);
  const roll = Math.random();
  const auth = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };

  if (roll < P_READ) {
    // Hot key: most reads hit movie 1 (as specified); the rest spread out.
    const id = Math.random() < 0.7 ? 1 : movie;
    const res = http.get(`${BASE}/api/v1/interaction/movies/${id}/interactions`, {
      tags: { kind: 'api', op: 'read' },
      responseType: 'none',
    });
    record(res, 'read');
    check(res, { 'read 200': (r) => r.status === 200 });
  } else if (roll < P_READ + P_RATE) {
    const res = http.post(`${BASE}/api/v1/interaction/movies/${movie}/rate`,
      JSON.stringify({ score: 1 + Math.floor(Math.random() * 10) }),
      { headers: auth, tags: { kind: 'api', op: 'rate' }, responseType: 'none' });
    record(res, 'rate');
    check(res, { 'rate 202': (r) => r.status === 202 });
  } else {
    const res = http.post(`${BASE}/api/v1/interaction/movies/${movie}/comment`,
      JSON.stringify({ text: `load test comment ${Math.random().toString(36).slice(2, 10)}` }),
      { headers: auth, tags: { kind: 'api', op: 'comment' }, responseType: 'none' });
    record(res, 'comment');
    check(res, { 'comment 202': (r) => r.status === 202 });
  }
  sleep(THINK_MIN + Math.random() * (THINK_MAX - THINK_MIN));
}

// A complete new-user journey: register -> login -> use the token once.
export function signup() {
  const c = credentials(`s${exec.scenario.iterationInTest}_${exec.vu.idInTest}`);
  const reg = http.post(`${BASE}/api/v1/auth/register`, JSON.stringify(c), { headers: JSON_HEADERS, tags: { kind: 'auth', op: 'register' }, timeout: '30s' });
  serverErrors.add(reg.status === 0 || reg.status >= 500);
  if (reg.status === 429) rateLimited.add(1);
  if (reg.status !== 201) { authOK.add(false); return; }

  const login = http.post(`${BASE}/api/v1/auth/login`, JSON.stringify({ login: c.username, password: c.password }), { headers: JSON_HEADERS, tags: { kind: 'auth', op: 'login' }, timeout: '30s' });
  serverErrors.add(login.status === 0 || login.status >= 500);
  if (login.status === 429) rateLimited.add(1);
  if (login.status !== 200) { authOK.add(false); return; }

  const rate = http.post(`${BASE}/api/v1/interaction/movies/${1 + (exec.vu.idInTest % MOVIES)}/rate`, JSON.stringify({ score: 8 }),
    { headers: { Authorization: `Bearer ${login.json('access_token')}`, 'Content-Type': 'application/json' }, tags: { kind: 'api', op: 'rate' }, responseType: 'none' });
  record(rate, 'rate');
  authOK.add(rate.status === 202);
}
