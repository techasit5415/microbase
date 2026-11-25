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
  cleanupTestUser,
  checkHealth,
  checkAuthMe,
  runCrudCycle,
} from './common.js';

export const options = {
  stages: [
    { duration: '10s', target: 20 },
    { duration: '25s', target: 50 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.02'],
    http_req_duration: ['p(95)<500'],
  },
};

export function setup() {
  const adminToken = loginAdmin();
  const createdCollection = ensureBenchCollection(adminToken);
  if (createdCollection) {
    // Seed 10 records for read-heavy queries
    const base = `${BASE_URL}/api/collections/${BENCH_COLLECTION}/records`;
    for (let i = 0; i < 10; i++) {
      http.post(
        base,
        JSON.stringify({ title: `seed-${i}`, score: i * 10, active: true }),
        { headers: jsonHeaders(), tags: { name: 'seed_record' } }
      );
    }
  }
  const user = setupTestUser(adminToken);
  return { adminToken, createdCollection, user };
}

export default function (data) {
  const roll = Math.random();
  if (data.createdCollection && roll < 0.5) {
    const res = http.get(
      `${BASE_URL}/api/collections/${BENCH_COLLECTION}/records?page=1&perPage=20`,
      { tags: { name: 'GET /records (list)' } }
    );
    check(res, { 'list records 200': (r) => r.status === 200 });
  } else if (data.createdCollection && roll < 0.75) {
    runCrudCycle(__VU, __ITER);
  } else if (roll < 0.9 && data.user.token) {
    checkAuthMe(data.user.token);
  } else {
    checkHealth();
  }
  sleep(0.1);
}

export function teardown(data) {
  cleanupBenchCollection(data.adminToken, data.createdCollection);
  cleanupTestUser(data.adminToken, data.user);
}
