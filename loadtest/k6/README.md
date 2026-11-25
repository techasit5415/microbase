# Microbase k6 Load Testing Suite

Performance and reliability benchmarks for Microbase powered by [Grafana k6](https://github.com/grafana/k6).

## Scripts

| Script | Profile | What it tests |
| --- | --- | --- |
| `smoke.js` | 5 VUs · 15s | Quick sanity check (`p95 < 300ms`, `< 1%` errors) |
| `load.js` | Ramp 0 → 20 → 50 VUs · 45s | Sustained mixed workload (Reads, Writes, Auth, Health) |
| `stress.js` | Ramp 30 → 80 → 150 VUs · 55s | High-concurrency stress test & breaking-point probe |

## Usage

### 1. Public Health + Auth only
```bash
k6 run loadtest/k6/smoke.js
k6 run loadtest/k6/load.js
k6 run loadtest/k6/stress.js
```

### 2. Full Suite (Health + Auth + Dynamic Collection CRUD Records)
Pass Superuser credentials (or `ADMIN_TOKEN`) so `setup()` automatically creates a temporary `k6_bench_items` collection and `teardown()` deletes it afterward:

```bash
BASE_URL=http://localhost:3000 \
ADMIN_EMAIL=admin@example.com \
ADMIN_PASSWORD=your-password \
k6 run loadtest/k6/load.js
```

> **Note:** Disable or raise Rate Limiting in **Settings → Application → Rate limiting** before running `load.js` / `stress.js`, otherwise Microbase will intentionally return `429 Too Many Requests`.
