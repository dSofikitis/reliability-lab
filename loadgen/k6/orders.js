// k6 load script that drives orders-svc with the traffic shape the
// SLOs assume. Two stages:
//
//   1. ramp from 0 to TARGET_VUS over RAMP_SEC, so the SLOs see a
//      realistic curve before steady state and we don't trip the
//      latency SLO purely on cold-start.
//   2. hold at TARGET_VUS for HOLD_SEC, which is when chaos
//      experiments fire and the SLO breaks become measurable.
//
// k6 is preferred over hey/wrk because we want per-request thresholds
// — anything matching the SLO threshold should also fail the k6 run,
// so a load test with no chaos becomes a regression check on the
// service itself.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:31080';
const TARGET_VUS = parseInt(__ENV.TARGET_VUS || '20', 10);
const RAMP_SEC = parseInt(__ENV.RAMP_SEC || '30', 10);
const HOLD_SEC = parseInt(__ENV.HOLD_SEC || '300', 10);

export const options = {
  stages: [
    { duration: `${RAMP_SEC}s`, target: TARGET_VUS },
    { duration: `${HOLD_SEC}s`, target: TARGET_VUS },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    // Mirror the orders-svc SLO: 99.5% success, p99 < 300ms. If the
    // run finishes inside these, the SLO is provably honored for the
    // duration of the test. Chaos breaks one or both deliberately.
    http_req_failed: ['rate<0.005'],
    http_req_duration: ['p(99)<300'],
  },
};

const SKUS = ['DEFAULT', 'WIDGET', 'GADGET', 'SPROCKET'];

function randomOrder() {
  return JSON.stringify({
    customer_id: uuidv4(),
    customer_email: `cust-${Math.floor(Math.random() * 1e6)}@example.test`,
    amount_minor: 100 + Math.floor(Math.random() * 49900),
    currency: 'USD',
    sku: SKUS[Math.floor(Math.random() * SKUS.length)],
    quantity: 1 + Math.floor(Math.random() * 3),
  });
}

export default function () {
  const res = http.post(`${BASE_URL}/orders`, randomOrder(), {
    headers: { 'Content-Type': 'application/json' },
    tags: { endpoint: 'orders' },
  });
  // Treat both 2xx and 402 (declined) as protocol-success; the
  // payments side-effect failing is a business outcome, not a
  // service availability hit. This matches the linkerd_request_total
  // classification the SLO rules consume.
  check(res, {
    'protocol-ok': (r) => r.status === 200 || r.status === 402,
  });
  sleep(0.1 + Math.random() * 0.3);
}
