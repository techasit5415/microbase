<script lang="ts">
	import { Check, Copy, X } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';
	import { fade } from 'svelte/transition';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import { highlightCode, type HighlightLang } from '#lib/utils/highlight.ts';

	interface Props {
		open: boolean;
		collection: Collection;
		onClose: () => void;
	}

	type TabKey =
		| 'list'
		| 'view'
		| 'create'
		| 'update'
		| 'delete'
		| 'realtime'
		| 'batch'
		| 'auth-methods'
		| 'auth-password'
		| 'auth-oauth2'
		| 'auth-otp'
		| 'auth-refresh'
		| 'auth-verify'
		| 'auth-reset'
		| 'auth-email';

	interface ParamRow {
		name: string;
		type: string;
		required?: boolean;
		desc: string;
	}

	let { open, collection, onClose }: Props = $props();

	let activeTab = $state<TabKey>('list');
	let lang = $state<'js' | 'ts' | 'curl' | 'go'>('js');
	let respCode = $state<'200' | '400' | '403'>('200');
	let copied = $state(false);

	let origin = $derived(typeof window !== 'undefined' ? window.location.origin : 'http://localhost:3000');
	let isAuth = $derived(collection.type === 'auth' || collection.name === 'users' || collection.name === '_superusers');
	let isView = $derived(collection.type === 'view');

	$effect(() => {
		if (open) {
			activeTab = 'list';
			respCode = '200';
		}
	});

	let crudTabs = $derived.by(() => {
		const base: { key: TabKey; label: string }[] = [
			{ key: 'list', label: 'List/Search' },
			{ key: 'view', label: 'View' }
		];
		if (!isView) {
			base.push(
				{ key: 'create', label: 'Create' },
				{ key: 'update', label: 'Update' },
				{ key: 'delete', label: 'Delete' }
			);
		}
		base.push({ key: 'realtime', label: 'Realtime' }, { key: 'batch', label: 'Batch' });
		return base;
	});

	const authTabs: { key: TabKey; label: string }[] = [
		{ key: 'auth-methods', label: 'List auth methods' },
		{ key: 'auth-password', label: 'Auth with password' },
		{ key: 'auth-oauth2', label: 'Auth with OAuth2' },
		{ key: 'auth-otp', label: 'Auth with OTP' },
		{ key: 'auth-refresh', label: 'Auth refresh' },
		{ key: 'auth-verify', label: 'Verification' },
		{ key: 'auth-reset', label: 'Password reset' },
		{ key: 'auth-email', label: 'Email change' }
	];

	function sampleRecord(): Record<string, unknown> {
		const rec: Record<string, unknown> = {
			id: collection.id_type === 'text' ? 'a950ka0mdi16u6s' : 1
		};
		if (isAuth) {
			rec.email = 'user@example.com';
			rec.role = collection.name === '_superusers' ? 'superuser' : 'user';
			if (collection.name === 'users') rec.verified = true;
		}
		for (const f of collection.fields ?? []) {
			if (f.hidden) continue;
			if (f.type === 'boolean') rec[f.name] = true;
			else if (f.type === 'integer' || f.type === 'number' || f.type === 'float' || f.type === 'relation') rec[f.name] = 1;
			else if (f.type === 'json') rec[f.name] = { key: 'value' };
			else if (f.type === 'geopoint') rec[f.name] = { lat: 13.7563, lon: 100.5018 };
			else if (f.type === 'vector') rec[f.name] = [0.12, -0.34, 0.56];
			else if (f.type === 'email') rec[f.name] = 'hello@example.com';
			else if (f.type === 'file') rec[f.name] = '/api/files/abc123_avatar.png';
			else rec[f.name] = `sample_${f.name}`;
		}
		rec.created_at = '2026-11-25T13:46:52Z';
		rec.updated_at = '2026-11-25T13:46:52Z';
		return rec;
	}

	function sampleBody(): Record<string, unknown> {
		if (collection.name === '_superusers') {
			return { email: 'admin@example.com', password: 'supersecretpassword' };
		}
		if (collection.name === 'users') {
			return { email: 'user@example.com', password: 'password123' };
		}
		const body: Record<string, unknown> = {};
		for (const f of collection.fields ?? []) {
			if (f.type === 'boolean') body[f.name] = true;
			else if (f.type === 'integer' || f.type === 'number' || f.type === 'float' || f.type === 'relation') body[f.name] = 1;
			else if (f.type === 'json') body[f.name] = { key: 'value' };
			else if (f.type === 'geopoint') body[f.name] = { lat: 13.7563, lon: 100.5018 };
			else if (f.type === 'vector') body[f.name] = [0.12, -0.34];
			else body[f.name] = `example_${f.name}`;
		}
		return body;
	}


	function tsTypeName(): string {
		const raw = collection.name.replace(/^_+/, '');
		return raw
			.split(/[_-]+/)
			.filter(Boolean)
			.map((p) => p[0].toUpperCase() + p.slice(1))
			.join('') + 'Record';
	}

	function buildTsInterface(): string {
		const lines: string[] = [];
		lines.push(`  id: ${collection.id_type === 'text' ? 'string' : 'number'};`);
		if (isAuth) {
			lines.push('  email: string;');
			if (collection.name === 'users') {
				lines.push('  verified: boolean;');
				lines.push('  email_visibility: boolean;');
			}
		}
		for (const f of collection.fields ?? []) {
			if (f.hidden) continue;
			if (isAuth && ['email', 'verified', 'email_visibility', 'password', 'token_key'].includes(f.name)) continue;
			let t = 'string';
			if (f.type === 'boolean') t = 'boolean';
			else if (f.type === 'integer' || f.type === 'number' || f.type === 'float') t = 'number';
			else if (f.type === 'vector') t = 'number[]';
			else if (f.type === 'geopoint') t = '{ lat: number; lon: number }';
			else if (f.type === 'json') t = 'Record<string, unknown>';
			lines.push(`  ${f.name}${f.required ? '' : '?'}: ${t};`);
		}
		lines.push('  created_at: string;');
		lines.push('  updated_at: string;');
		return `interface ${tsTypeName()} {\n${lines.join('\n')}\n}`;
	}

	function buildTsSnippet(jsSnippet: string): string {
		const iface = buildTsInterface();
		const typeName = tsTypeName();
		if (activeTab === 'list') {
			return `${iface}\n\ninterface ListResponse<T> {\n  data: { page: number; per_page: number; total_items: number; items: T[] };\n}\n\n` +
				jsSnippet.replace('const data = await res.json();', `const { data }: ListResponse<${typeName}> = await res.json();`);
		}
		if (activeTab === 'view' || activeTab === 'create' || activeTab === 'update') {
			return `${iface}\n\n` +
				jsSnippet
					.replace('const record = await res.json();', `const { data: record }: { data: ${typeName} } = await res.json();`)
					.replace('const created = await res.json();', `const { data: created }: { data: ${typeName} } = await res.json();`)
					.replace('const updated = await res.json();', `const { data: updated }: { data: ${typeName} } = await res.json();`);
		}
		if (activeTab === 'auth-password' || activeTab === 'auth-otp' || activeTab === 'auth-refresh') {
			return `${iface}\n\ninterface AuthResponse {\n  data: { token: string; user: ${typeName} };\n}\n\n` +
				jsSnippet.replace('const auth = await res.json();', 'const { data: auth }: AuthResponse = await res.json();');
		}
		return `${iface}\n\n${jsSnippet}`;
	}

	let baseEndpoint = $derived(
		collection.name === 'users'
			? '/api/users'
			: collection.name === '_superusers'
				? '/api/superusers'
				: `/api/collections/${collection.name}/records`
	);

	let doc = $derived.by((): { title: string; desc: string; method: string; path: string; authNote: string; paramTitle: string; params: ParamRow[]; js: string; curl: string; go: string; resp200: unknown } => {
		const col = collection.name;
		const rec = sampleRecord();
		const body = sampleBody();
		const idSample = collection.id_type === 'text' ? 'a950ka0mdi16u6s' : '1';
		const rules = collection.api_rules ?? {};
		const ruleBadge = (action: string) => {
			if (col === '_superusers') return 'Requires superuser Authorization: Bearer TOKEN';
			if (!Object.prototype.hasOwnProperty.call(rules, action)) return 'Requires superuser Authorization: Bearer TOKEN';
			const r = (rules[action] ?? '').trim();
			return r === '' ? 'Public (unlocked)' : `Rule: ${r}`;
		};

		const fieldParams: ParamRow[] = (collection.fields ?? []).map((f) => ({
			name: f.name,
			type: f.type,
			required: f.required,
			desc: f.help_text || (f.type === 'relation' && f.collection_name ? `Relation to "${f.collection_name}" record ID.` : `Field of type ${f.type}.`)
		}));

		switch (activeTab) {
			case 'list':
				return {
					title: `List/Search (${col})`,
					desc: `Fetch a paginated ${col} records list, supporting sorting, full-text/Thai search, PocketBase filter syntax, relation expansion, and field projection.`,
					method: 'GET',
					path: baseEndpoint,
					authNote: ruleBadge('list'),
					paramTitle: '?query params',
					params: [
						{ name: 'page', type: 'Number', desc: 'Page number of the paginated list (default: 1).' },
						{ name: 'per_page', type: 'Number', desc: 'Max returned records per page (default: 20, max: 100).' },
						{ name: 'sort', type: 'String', desc: 'Sort field(s). Prefix with - for DESC, e.g. ?sort=-created_at,id.' },
						{ name: 'search', type: 'String', desc: 'Full-text (English FTS) + Thai ILIKE search across all text columns.' },
						{ name: 'filter', type: 'String', desc: "PocketBase filter expression, e.g. ?filter=(views>10 && published=true) || title~'hello'." },
						{ name: 'expand', type: 'String', desc: 'Auto-expand relation fields into record.expand, e.g. ?expand=author,category.' },
						{ name: 'fields', type: 'String', desc: 'Comma-separated projection of fields to return, e.g. ?fields=id,title,created_at.' }
					] as ParamRow[],
					js: `const res = await fetch('${origin}${baseEndpoint}?page=1&per_page=30&sort=-created_at', {\n  headers: { Authorization: 'Bearer YOUR_TOKEN' }\n});\nconst data = await res.json();\nconsole.log(data.data.items);`,
					curl: `curl "${origin}${baseEndpoint}?page=1&per_page=30&sort=-created_at" \\\n  -H "Authorization: Bearer YOUR_TOKEN"`,
					go: `req, _ := http.NewRequest("GET", "${origin}${baseEndpoint}?page=1&per_page=30", nil)\nreq.Header.Set("Authorization", "Bearer YOUR_TOKEN")\nresp, err := http.DefaultClient.Do(req)`,
					resp200: { data: { page: 1, per_page: 30, total_items: 2, items: [rec] } }
				};
			case 'view':
				return {
					title: `View (${col})`,
					desc: `Fetch a single ${col} record by its ID.`,
					method: 'GET',
					path: `${baseEndpoint}/:id`,
					authNote: ruleBadge('view'),
					paramTitle: '?query params',
					params: [
						{ name: 'expand', type: 'String', desc: 'Auto-expand relation fields into record.expand.' },
						{ name: 'fields', type: 'String', desc: 'Comma-separated fields to return.' }
					] as ParamRow[],
					js: `const res = await fetch('${origin}${baseEndpoint}/${idSample}?expand=author', {\n  headers: { Authorization: 'Bearer YOUR_TOKEN' }\n});\nconst record = await res.json();`,
					curl: `curl "${origin}${baseEndpoint}/${idSample}" \\\n  -H "Authorization: Bearer YOUR_TOKEN"`,
					go: `req, _ := http.NewRequest("GET", "${origin}${baseEndpoint}/${idSample}", nil)\nreq.Header.Set("Authorization", "Bearer YOUR_TOKEN")`,
					resp200: { data: rec }
				};
			case 'create': {
				const createPath = col === 'users' ? '/api/auth/register' : baseEndpoint;
				return {
					title: `Create (${col})`,
					desc: `Create a new ${col} record. Any text field or text ID with an Autogenerate pattern is filled automatically when omitted.`,
					method: 'POST',
					path: createPath,
					authNote: ruleBadge('create'),
					paramTitle: 'Body parameters (JSON)',
					params: fieldParams,
					js: `const res = await fetch('${origin}${createPath}', {\n  method: 'POST',\n  headers: {\n    'Content-Type': 'application/json',\n    Authorization: 'Bearer YOUR_TOKEN'\n  },\n  body: JSON.stringify(${JSON.stringify(body, null, 2)})\n});\nconst created = await res.json();`,
					curl: `curl -X POST "${origin}${createPath}" \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer YOUR_TOKEN" \\\n  -d '${JSON.stringify(body)}'`,
					go: `payload := strings.NewReader(\`${JSON.stringify(body)}\`)\nreq, _ := http.NewRequest("POST", "${origin}${createPath}", payload)\nreq.Header.Set("Content-Type", "application/json")`,
					resp200: { data: rec }
				};
			}
			case 'update':
				return {
					title: `Update (${col})`,
					desc: `Partially update an existing ${col} record by ID.`,
					method: 'PATCH',
					path: `${baseEndpoint}/:id`,
					authNote: ruleBadge('update'),
					paramTitle: 'Body parameters (JSON)',
					params: fieldParams,
					js: `const res = await fetch('${origin}${baseEndpoint}/${idSample}', {\n  method: 'PATCH',\n  headers: {\n    'Content-Type': 'application/json',\n    Authorization: 'Bearer YOUR_TOKEN'\n  },\n  body: JSON.stringify(${JSON.stringify(body, null, 2)})\n});`,
					curl: `curl -X PATCH "${origin}${baseEndpoint}/${idSample}" \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer YOUR_TOKEN" \\\n  -d '${JSON.stringify(body)}'`,
					go: `payload := strings.NewReader(\`${JSON.stringify(body)}\`)\nreq, _ := http.NewRequest("PATCH", "${origin}${baseEndpoint}/${idSample}", payload)`,
					resp200: { data: rec }
				};
			case 'delete':
				return {
					title: `Delete (${col})`,
					desc: `Permanently delete a single ${col} record by ID.`,
					method: 'DELETE',
					path: `${baseEndpoint}/:id`,
					authNote: ruleBadge('delete'),
					paramTitle: 'Path parameters',
					params: [{ name: 'id', type: 'String | Number', required: true, desc: 'ID of the record to delete.' }],
					js: `await fetch('${origin}${baseEndpoint}/${idSample}', {\n  method: 'DELETE',\n  headers: { Authorization: 'Bearer YOUR_TOKEN' }\n});`,
					curl: `curl -X DELETE "${origin}${baseEndpoint}/${idSample}" \\\n  -H "Authorization: Bearer YOUR_TOKEN"`,
					go: `req, _ := http.NewRequest("DELETE", "${origin}${baseEndpoint}/${idSample}", nil)`,
					resp200: { data: { deleted: idSample } }
				};
			case 'realtime':
				return {
					title: `Realtime SSE (${col})`,
					desc: `Subscribe to real-time create, update, and delete events via Server-Sent Events (SSE).`,
					method: 'GET',
					path: `/api/realtime?collection=${col}`,
					authNote: 'Server-Sent Events stream',
					paramTitle: '?query params',
					params: [{ name: 'collection', type: 'String', desc: 'Filter stream to a specific collection name (optional).' }],
					js: `const es = new EventSource('${origin}/api/realtime?collection=${col}');\nes.onmessage = (event) => {\n  console.log('Realtime event:', JSON.parse(event.data));\n};`,
					curl: `curl -N "${origin}/api/realtime?collection=${col}"`,
					go: `resp, err := http.Get("${origin}/api/realtime?collection=${col}")`,
					resp200: { action: 'create', collection: col, record: rec }
				};
			case 'batch':
				return {
					title: 'Batch Requests',
					desc: 'Execute up to 50 create/update/delete/read sub-requests in a single HTTP round-trip.',
					method: 'POST',
					path: '/api/batch',
					authNote: 'Inherits Authorization header',
					paramTitle: 'Body parameters (JSON)',
					params: [{ name: 'requests', type: 'Array<{method, url, body}>', required: true, desc: 'Ordered list of /api/* sub-requests (1..50).' }],
					js: `const res = await fetch('${origin}/api/batch', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json', Authorization: 'Bearer YOUR_TOKEN' },\n  body: JSON.stringify({\n    requests: [\n      { method: 'POST', url: '${baseEndpoint}', body: ${JSON.stringify(body)} }\n    ]\n  })\n});`,
					curl: `curl -X POST "${origin}/api/batch" -H "Content-Type: application/json" -d '{"requests":[{"method":"GET","url":"${baseEndpoint}"}]}'`,
					go: `// POST ${origin}/api/batch with {"requests":[...]}`,
					resp200: { data: { results: [{ status: 201, body: { data: rec } }] } }
				};
			case 'auth-methods':
				return {
					title: 'List Auth Methods',
					desc: 'Returns enabled OAuth2/OIDC providers and password login availability.',
					method: 'GET',
					path: '/api/auth/methods',
					authNote: 'Public',
					paramTitle: 'Query parameters',
					params: [],
					js: `const res = await fetch('${origin}/api/auth/methods');\nconst methods = await res.json();`,
					curl: `curl "${origin}/api/auth/methods"`,
					go: `resp, err := http.Get("${origin}/api/auth/methods")`,
					resp200: { data: { password: { enabled: true }, oauth2: { enabled: true, providers: [{ name: 'discord', display_name: 'Discord', auth_url: '/api/auth/oauth/discord' }] } } }
				};
			case 'auth-password': {
				const loginPath = col === '_superusers' ? '/api/superusers/login' : '/api/auth/login';
				return {
					title: `Auth with Password (${col})`,
					desc: 'Authenticate with email and password to receive a Bearer JWT token.',
					method: 'POST',
					path: loginPath,
					authNote: 'Public',
					paramTitle: 'Body parameters (JSON)',
					params: [
						{ name: 'email', type: 'String', required: true, desc: 'Account email address.' },
						{ name: 'password', type: 'String', required: true, desc: 'Account password.' }
					],
					js: `const res = await fetch('${origin}${loginPath}', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({ email: 'user@example.com', password: 'password123' })\n});\nconst { data } = await res.json();\nconsole.log(data.token);`,
					curl: `curl -X POST "${origin}${loginPath}" -H "Content-Type: application/json" -d '{"email":"user@example.com","password":"password123"}'`,
					go: `// POST ${origin}${loginPath}`,
					resp200: { data: { token: 'eyJhbGciOiJIUzI1NiIs...', user: rec } }
				};
			}
			case 'auth-oauth2':
				return {
					title: 'Auth with OAuth2 / OIDC',
					desc: 'Redirect the user to /api/auth/oauth/:provider?redirect_to=..., then exchange the 60s one-time code for a JWT.',
					method: 'POST',
					path: '/api/auth/oauth/exchange',
					authNote: 'Public (one-time code)',
					paramTitle: 'Body parameters (JSON)',
					params: [{ name: 'code', type: 'String', required: true, desc: 'One-time code received in your redirect_to callback query string.' }],
					js: `// 1. Redirect user to:\n// ${origin}/api/auth/oauth/discord?redirect_to=http://localhost:5173/callback\n// 2. Exchange code for JWT:\nconst res = await fetch('${origin}/api/auth/oauth/exchange', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({ code: 'ONE_TIME_CODE' })\n});`,
					curl: `curl -X POST "${origin}/api/auth/oauth/exchange" -H "Content-Type: application/json" -d '{"code":"ONE_TIME_CODE"}'`,
					go: `// POST ${origin}/api/auth/oauth/exchange`,
					resp200: { data: { token: 'eyJhbGciOiJIUzI1NiIs...', user: rec } }
				};
			case 'auth-otp':
				return {
					title: 'Auth with Email OTP',
					desc: 'Request a 6-digit email OTP (valid 5m) and exchange it for a Bearer JWT.',
					method: 'POST',
					path: '/api/auth/request-otp',
					authNote: 'Public',
					paramTitle: 'Body parameters (JSON)',
					params: [
						{ name: 'email', type: 'String', required: true, desc: 'Step 1 (request-otp): User email address.' },
						{ name: 'otp_id & password', type: 'String', required: true, desc: 'Step 2 (auth-with-otp): Returned otp_id and 6-digit code.' }
					],
					js: `// Step 1: Request OTP\nconst r1 = await fetch('${origin}/api/auth/request-otp', {\n  method: 'POST', headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({ email: 'user@example.com' })\n});\nconst { data: { otp_id } } = await r1.json();\n\n// Step 2: Verify 6-digit OTP\nconst r2 = await fetch('${origin}/api/auth/auth-with-otp', {\n  method: 'POST', headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({ otp_id, password: '123456' })\n});`,
					curl: `curl -X POST "${origin}/api/auth/request-otp" -H "Content-Type: application/json" -d '{"email":"user@example.com"}'`,
					go: `// POST ${origin}/api/auth/request-otp -> POST ${origin}/api/auth/auth-with-otp`,
					resp200: { data: { token: 'eyJhbGciOiJIUzI1NiIs...', user: rec } }
				};
			case 'auth-refresh':
				return {
					title: 'Auth Refresh',
					desc: 'Issues a fresh Bearer JWT token for the currently authenticated user.',
					method: 'POST',
					path: '/api/auth/refresh',
					authNote: 'Requires Authorization: Bearer TOKEN',
					paramTitle: 'Headers',
					params: [{ name: 'Authorization', type: 'String', required: true, desc: 'Bearer <current_jwt>' }],
					js: `const res = await fetch('${origin}/api/auth/refresh', {\n  method: 'POST',\n  headers: { Authorization: 'Bearer YOUR_TOKEN' }\n});`,
					curl: `curl -X POST "${origin}/api/auth/refresh" -H "Authorization: Bearer YOUR_TOKEN"`,
					go: `// POST ${origin}/api/auth/refresh`,
					resp200: { data: { token: 'eyJhbGciOiJIUzI1NiIs...', user: rec } }
				};
			case 'auth-verify':
				return {
					title: 'Email Verification',
					desc: 'Send a 15-minute verification token to the user email and confirm it.',
					method: 'POST',
					path: '/api/auth/request-verification',
					authNote: 'Public',
					paramTitle: 'Body parameters (JSON)',
					params: [
						{ name: 'email', type: 'String', required: true, desc: 'Step 1 (request-verification): Email to verify.' },
						{ name: 'token', type: 'String', required: true, desc: 'Step 2 (confirm-verification): Token received by email.' }
					],
					js: `await fetch('${origin}/api/auth/request-verification', {\n  method: 'POST', headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({ email: 'user@example.com' })\n});`,
					curl: `curl -X POST "${origin}/api/auth/request-verification" -H "Content-Type: application/json" -d '{"email":"user@example.com"}'`,
					go: `// POST ${origin}/api/auth/request-verification`,
					resp200: { data: { sent: true } }
				};
			case 'auth-reset':
				return {
					title: 'Password Reset',
					desc: 'Request a 15-minute password reset token and confirm with a new password.',
					method: 'POST',
					path: '/api/auth/request-password-reset',
					authNote: 'Public',
					paramTitle: 'Body parameters (JSON)',
					params: [
						{ name: 'email', type: 'String', required: true, desc: 'Step 1 (request-password-reset): Account email.' },
						{ name: 'token & password', type: 'String', required: true, desc: 'Step 2 (confirm-password-reset): Reset token + new password (min 8 chars).' }
					],
					js: `await fetch('${origin}/api/auth/request-password-reset', {\n  method: 'POST', headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({ email: 'user@example.com' })\n});`,
					curl: `curl -X POST "${origin}/api/auth/request-password-reset" -H "Content-Type: application/json" -d '{"email":"user@example.com"}'`,
					go: `// POST ${origin}/api/auth/request-password-reset`,
					resp200: { data: { sent: true } }
				};
			default:
				return {
					title: 'Email Change',
					desc: 'Request an email address change for the authenticated user and confirm with the emailed token.',
					method: 'POST',
					path: '/api/auth/request-email-change',
					authNote: 'Requires Authorization: Bearer TOKEN',
					paramTitle: 'Body parameters (JSON)',
					params: [
						{ name: 'new_email', type: 'String', required: true, desc: 'Step 1 (request-email-change): New email address.' },
						{ name: 'token', type: 'String', required: true, desc: 'Step 2 (confirm-email-change): Token sent to new_email.' }
					],
					js: `await fetch('${origin}/api/auth/request-email-change', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json', Authorization: 'Bearer YOUR_TOKEN' },\n  body: JSON.stringify({ new_email: 'new@example.com' })\n});`,
					curl: `curl -X POST "${origin}/api/auth/request-email-change" -H "Authorization: Bearer YOUR_TOKEN" -H "Content-Type: application/json" -d '{"new_email":"new@example.com"}'`,
					go: `// POST ${origin}/api/auth/request-email-change`,
					resp200: { data: { sent: true } }
				};
		}
	});

	let activeSnippet = $derived(lang === 'js' ? doc.js : lang === 'ts' ? buildTsSnippet(doc.js) : lang === 'curl' ? doc.curl : doc.go);

	let activeResponseJSON = $derived(
		respCode === '200'
			? JSON.stringify(doc.resp200, null, 2)
			: respCode === '400'
				? JSON.stringify({ error: { code: 'INVALID_INPUT', message: 'Validation failed for request payload.' } }, null, 2)
				: JSON.stringify({ error: { code: 'FORBIDDEN', message: 'only superusers can perform this action' } }, null, 2)
	);

	async function copySnippet() {
		await navigator.clipboard?.writeText(activeSnippet).catch(() => {});
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<Drawer {open} {onClose} maxWidth="max-w-5xl">
	<div class="flex min-h-0 flex-1 overflow-hidden">
		<!-- Left Sidebar (PocketBase API Preview Navigation) -->
		<aside class="w-52 shrink-0 overflow-y-auto border-r border-border bg-background/60 p-3">
			<div class="space-y-0.5">
				{#each crudTabs as tab (tab.key)}
					<button
						type="button"
						onclick={() => (activeTab = tab.key)}
						class="flex w-full cursor-pointer items-center rounded-md px-3 py-2 text-left text-xs font-medium transition {activeTab === tab.key
							? 'bg-muted text-foreground'
							: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'}"
					>
						{tab.label}
					</button>
				{/each}
			</div>

			{#if isAuth}
				<div class="my-3 border-t border-border"></div>
				<div class="space-y-0.5">
					{#each authTabs as tab (tab.key)}
						<button
							type="button"
							onclick={() => (activeTab = tab.key)}
							class="flex w-full cursor-pointer items-center rounded-md px-3 py-2 text-left text-xs font-medium transition {activeTab === tab.key
								? 'bg-muted text-foreground'
								: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'}"
						>
							{tab.label}
						</button>
					{/each}
				</div>
			{/if}
		</aside>

		<!-- Right Detailed Documentation Pane -->
		<div class="flex min-w-0 flex-1 flex-col overflow-hidden bg-card">
			<div class="flex items-start justify-between gap-4 border-b border-border px-6 py-4">
				<div>
					<h2 class="text-base font-semibold text-foreground">{doc.title}</h2>
					<p class="mt-1 text-xs text-muted-foreground">{doc.desc}</p>
				</div>
				<button
					type="button"
					onclick={onClose}
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<div class="flex-1 space-y-6 overflow-y-auto px-6 py-5">
				<!-- Code Snippet Tabs -->
				<div>
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-1">
							{#each [
								{ id: 'js', label: 'JS (fetch)' },
								{ id: 'ts', label: 'TypeScript' },
								{ id: 'curl', label: 'curl' },
								{ id: 'go', label: 'Go' }
							] as l (l.id)}
								<button
									type="button"
									onclick={() => (lang = l.id as 'js' | 'ts' | 'curl' | 'go')}
									class="cursor-pointer rounded-t-md px-3.5 py-2 text-xs font-medium transition {lang === l.id
										? 'bg-muted text-foreground'
										: 'text-muted-foreground hover:text-foreground'}"
								>
									{l.label}
								</button>
							{/each}
						</div>
						<button
							type="button"
							onclick={copySnippet}
							class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-muted"
						>
							{#if copied}
								<Check class="h-3.5 w-3.5 text-emerald-600" /> Copied
							{:else}
								<Copy class="h-3.5 w-3.5" /> Copy
							{/if}
						</button>
					</div>
					{#key lang + activeTab}<pre in:fade={{ duration: 120 }} class="overflow-x-auto rounded-b-md rounded-tr-md bg-muted/85 p-4 font-mono text-xs leading-relaxed text-foreground">{@html highlightCode(activeSnippet, lang as HighlightLang)}</pre>{/key}
				</div>

				<!-- API Details Bar -->
				<div class="space-y-3">
					<h3 class="text-xs font-semibold text-foreground">API details</h3>
					<div class="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/40 px-3.5 py-2.5">
						<div class="flex items-center gap-2.5 font-mono text-xs">
							<span
								class="rounded px-2 py-0.5 font-semibold {doc.method === 'DELETE'
									? 'bg-destructive/15 text-destructive'
									: doc.method === 'POST' || doc.method === 'PATCH'
										? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
										: 'bg-foreground/10 text-foreground'}"
							>
								{doc.method}
							</span>
							<span class="font-medium text-foreground">{doc.path}</span>
						</div>
						<span class="text-xs text-muted-foreground">{doc.authNote}</span>
					</div>
				</div>

				<!-- Query / Body Parameters Table -->
				{#if doc.params.length > 0}
					<div class="overflow-hidden rounded-md border border-border">
						<table class="w-full border-collapse text-left text-xs">
							<thead class="border-b border-border bg-muted/50 text-muted-foreground">
								<tr>
									<th class="px-4 py-2.5 font-semibold">{doc.paramTitle}</th>
									<th class="w-28 px-4 py-2.5 font-semibold">Type</th>
									<th class="px-4 py-2.5 font-semibold">Description</th>
								</tr>
							</thead>
							<tbody class="divide-y divide-border">
								{#each doc.params as p (p.name)}
									<tr class="bg-card">
										<td class="px-4 py-2.5 font-mono font-medium whitespace-nowrap">
											{p.name}{#if p.required}<span class="text-destructive"> *</span>{/if}
										</td>
										<td class="px-4 py-2.5">
											<span class="rounded bg-muted px-2 py-0.5 font-mono text-xs text-muted-foreground">{p.type}</span>
										</td>
										<td class="px-4 py-2.5 text-muted-foreground">{p.desc}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}

				<!-- Example Responses -->
				<div class="space-y-2">
					<div class="flex items-center justify-between">
						<h3 class="text-xs font-semibold text-foreground">Example responses</h3>
						<div class="flex items-center gap-1">
							{#each ['200', '400', '403'] as code (code)}
								<button
									type="button"
									onclick={() => (respCode = code as '200' | '400' | '403')}
									class="cursor-pointer rounded px-2.5 py-1 font-mono text-xs font-semibold transition {respCode === code
										? 'bg-foreground text-background'
										: 'bg-muted text-muted-foreground hover:text-foreground'}"
								>
									{code}
								</button>
							{/each}
						</div>
					</div>
					{#key respCode + activeTab}<pre in:fade={{ duration: 120 }} class="overflow-x-auto rounded-md bg-muted/85 p-4 font-mono text-xs leading-relaxed text-foreground">{@html highlightCode(activeResponseJSON, 'json')}</pre>{/key}
				</div>
			</div>
		</div>
	</div>
</Drawer>
