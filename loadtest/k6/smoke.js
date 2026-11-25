import { sleep } from 'k6';
import {
  loginAdmin,
  ensureBenchCollection,
  cleanupBenchCollection,
  setupTestUser,
  checkHealth,
  checkAuthMe,
  runCrudCycle,
} from './common.js';

export const options = {
  vus: 5,
  duration: '15s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<300'],
  },
};

export function setup() {
  const adminToken = loginAdmin();
  const createdCollection = ensureBenchCollection(adminToken);
  const user = setupTestUser();
  return { adminToken, createdCollection, user };
}

export default function (data) {
  checkHealth();
  checkAuthMe(data.user.token);
  if (data.createdCollection) {
    runCrudCycle(__VU, __ITER);
  }
  sleep(0.2);
}

export function teardown(data) {
  cleanupBenchCollection(data.adminToken, data.createdCollection);
}
