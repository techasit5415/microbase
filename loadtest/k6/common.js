import http from 'k6/http';
import { check } from 'k6';

export const BASE_URL = (__ENV.BASE_URL || 'http://localhost:3000').replace(/\/+$/, '');
export const BENCH_COLLECTION = __ENV.COLLECTION || 'k6_bench_items';

export function jsonHeaders(token) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return headers;
}

export function loginAdmin() {
  if (__ENV.ADMIN_TOKEN) {
    return __ENV.ADMIN_TOKEN;
  }
  const email = __ENV.ADMIN_EMAIL;
  const password = __ENV.ADMIN_PASSWORD;
  if (!email || !password) {
    return '';
  }
  const res = http.post(
    `${BASE_URL}/api/superusers/login`,
    JSON.stringify({ email, password }),
    { headers: jsonHeaders(), tags: { name: 'admin_login' } }
  );
  if (res.status !== 200) {
    console.warn(`Admin login failed (${res.status}): ${res.body}`);
    return '';
  }
  return res.json('token') || '';
}

export function ensureBenchCollection(adminToken) {
  if (!adminToken) return false;
  const payload = {
    name: BENCH_COLLECTION,
    type: 'base',
    list_rule: '',
    view_rule: '',
    create_rule: '',
    update_rule: '',
    delete_rule: '',
    fields: [
      { name: 'title', type: 'text', required: true },
      { name: 'score', type: 'number', required: false },
      { name: 'active', type: 'bool', required: false },
    ],
  };
  const res = http.post(
    `${BASE_URL}/api/collections`,
    JSON.stringify(payload),
    { headers: jsonHeaders(adminToken), tags: { name: 'setup_collection' } }
  );
  return res.status === 201 || res.status === 200;
}

export function cleanupBenchCollection(adminToken, created) {
  if (!adminToken || !created) return;
  http.del(`${BASE_URL}/api/collections/${BENCH_COLLECTION}`, null, {
    headers: jsonHeaders(adminToken),
    tags: { name: 'teardown_collection' },
  });
}

export function setupTestUser() {
  const email = `k6_bench_${Date.now()}@bench.local`;
  const password = 'K6BenchPassword123!';
  const regRes = http.post(
    `${BASE_URL}/api/auth/register`,
    JSON.stringify({ email, password, passwordConfirm: password }),
    { headers: jsonHeaders(), tags: { name: 'setup_register' } }
  );
  if (regRes.status !== 201 && regRes.status !== 200) {
    console.warn(`User register skipped (${regRes.status}): ${regRes.body}`);
    return { email: '', password: '', token: '' };
  }
  const loginRes = http.post(
    `${BASE_URL}/api/auth/login`,
    JSON.stringify({ email, password }),
    { headers: jsonHeaders(), tags: { name: 'setup_login' } }
  );
  const token = loginRes.status === 200 ? (loginRes.json('token') || '') : '';
  return { email, password, token };
}

export function checkHealth() {
  const res = http.get(`${BASE_URL}/health`, { tags: { name: 'GET /health' } });
  check(res, {
    'health 200': (r) => r.status === 200,
    'health status ok': (r) => r.json('status') === 'ok',
  });
  return res;
}

export function checkAuthMe(userToken) {
  if (!userToken) return null;
  const res = http.get(`${BASE_URL}/api/auth/me`, {
    headers: jsonHeaders(userToken),
    tags: { name: 'GET /api/auth/me' },
  });
  check(res, {
    'auth/me 200': (r) => r.status === 200,
  });
  return res;
}

export function runCrudCycle(vuId, iter) {
  const base = `${BASE_URL}/api/collections/${BENCH_COLLECTION}/records`;

  // Create
  const createRes = http.post(
    base,
    JSON.stringify({
      title: `k6-vu${vuId}-iter${iter}`,
      score: iter % 100,
      active: iter % 2 === 0,
    }),
    { headers: jsonHeaders(), tags: { name: 'POST /records' } }
  );
  const createdOk = check(createRes, {
    'create record 201': (r) => r.status === 201,
  });
  if (!createdOk) return;

  const id = createRes.json('id');
  if (!id) return;

  // List
  const listRes = http.get(`${base}?page=1&perPage=20`, {
    tags: { name: 'GET /records (list)' },
  });
  check(listRes, {
    'list records 200': (r) => r.status === 200,
  });

  // Get by ID
  const getRes = http.get(`${base}/${id}`, {
    tags: { name: 'GET /records/:id' },
  });
  check(getRes, {
    'get record 200': (r) => r.status === 200,
  });

  // Update
  const patchRes = http.patch(
    `${base}/${id}`,
    JSON.stringify({ score: (iter % 100) + 1 }),
    { headers: jsonHeaders(), tags: { name: 'PATCH /records/:id' } }
  );
  check(patchRes, {
    'update record 200': (r) => r.status === 200,
  });

  // Delete
  const delRes = http.del(`${base}/${id}`, null, {
    tags: { name: 'DELETE /records/:id' },
  });
  check(delRes, {
    'delete record 200/204': (r) => r.status === 200 || r.status === 204,
  });
}
