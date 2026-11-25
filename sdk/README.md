# Microbase TypeScript / JavaScript SDK

Zero-dependency TypeScript SDK for [Microbase](../README.md) with PocketBase-style ergonomics.

## Quick Start

```ts
import Microbase from './sdk/src/index.ts';

const mb = new Microbase('http://localhost:3000');

// 1. Authenticate (users, _superusers, or any custom Auth collection)
await mb.collection('users').authWithPassword('user@example.com', 'password123');

// 2. Query records with filter, sort, expand, fields
const posts = await mb.collection('posts').getList(1, 20, {
  filter: "status = 'published'",
  sort: '-created_at',
  expand: 'author'
});

// 3. Create / Update / Delete
const created = await mb.collection('posts').create({ title: 'Hello Microbase' });
await mb.collection('posts').update(created.id as string, { title: 'Updated!' });
await mb.collection('posts').delete(created.id as string);

// 4. pgvector Similarity Search
const nearest = await mb.collection('docs').vectorSearch('embedding', [0.12, -0.34, 0.56], 5);

// 5. Realtime SSE Subscription
const unsubscribe = mb.collection('posts').subscribe((e) => {
  console.log(e.action, e.record);
});

// 6. OAuth2 / Custom OIDC (e.g. KMITL Science IAM)
window.location.href = mb.getOAuth2Url('oidc', `${window.location.origin}/auth/callback`);
// On callback page:
const code = new URLSearchParams(window.location.search).get('code');
if (code) await mb.exchangeOAuth2Code(code);
```
