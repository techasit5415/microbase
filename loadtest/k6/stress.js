import http from 'k6/http';
import { check, sleep } from 'k6';
import {
  BASE_URL,
  BENCH_COLLECTION,
  jsonHeaders,
  loginAdmin,
  ensureBenchCollection,
  cleanupBenchCollection,
  setupTestUser,
  checkHealth,
  checkAuthMe,
  runCrudCycle,
} from './common.js';

export const options = {
  stages: [
    { duration: '10s', target: 30 },
    { duration: '15s', target: 80 },
    { duration: '20s', target: 150 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<1500'],
  },
};

export function setup() {
  const adminToken = loginAdmin();
  const createdCollection = ensureBenchCollection(adminToken);
  if (createdCollection) {
    const base = `${BASE_URL}/api/collections/${BENCH_COLLECTION}/records`;
    for (let i = 0; i < 20; i++) {
      http.post(
        base,
        JSON.stringify({ title: `stress-seed-${i}`, score: i, active: true }),
        { headers: jsonHeaders(), tags: { name: 'seed_record' } }
      );
    }
  }
  const user = setupTestUser();
  return { adminToken, createdCollection, user };
}

export default function (data) {
  const roll = Math.random();
  if (data.createdCollection && roll < 0.6) {
    const res = http.get(
      `${BASE_URL}/api/collections/${BENCH_COLLECTION}/records?page=1&perPage=20`,
      { tags: { name: 'GET /records (list)' } }
    );
    check(res, { 'list records 200': (r) => r.status === 200 });
  } else if (data.createdCollection && roll < 0.8) {
    runCrudCycle(__VU, __ITER);
  } else if (data.user.token && roll < 0.95) {
    checkAuthMe(data.user.token);
  } else {
    checkHealth();
  }
  sleep(0.05);
}

export function teardown(data) {
  cleanupBenchCollection(data.adminToken, data.createdCollection);
}
