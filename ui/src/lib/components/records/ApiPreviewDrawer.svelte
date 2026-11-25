<script lang="ts">
	import { Check, Code2, Copy, ExternalLink, X } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';
	import Drawer from '#lib/components/ui/Drawer.svelte';

	interface Props {
		open: boolean;
		collection: Collection;
		onClose: () => void;
	}

	let { open, collection, onClose }: Props = $props();
	let activeTab = $state<'list' | 'view' | 'create' | 'update' | 'delete'>('list');
	let copied = $state(false);

	let origin = $derived(typeof window !== 'undefined' ? window.location.origin : 'http://localhost:3000');
	let isAuth = $derived(collection.type === 'auth');

	let sampleBody = $derived.by(() => {
		if (isAuth) return JSON.stringify({ email: 'user@example.com', password: 'password123' }, null, 2);
		const obj: Record<string, unknown> = {};
		for (const f of collection.fields ?? []) {
			if (f.type === 'boolean') obj[f.name] = true;
			else if (f.type === 'integer' || f.type === 'number' || f.type === 'float') obj[f.name] = 123;
			else if (f.type === 'json') obj[f.name] = { key: 'value' };
			else if (f.type === 'vector') obj[f.name] = [0.1, 0.2, 0.3];
			else obj[f.name] = 'sample_' + f.name;
		}
		return JSON.stringify(obj, null, 2);
	});

	let snippet = $derived.by(() => {
		const base = isAuth
			? `${origin}/api/auth`
			: `${origin}/api/collections/${collection.name}/records`;

		if (isAuth) {
			if (activeTab === 'list') return `// Discover enabled auth methods (Password + OAuth2/OIDC)\nconst res = await fetch('${origin}/api/auth/methods');\nconst methods = await res.json();`;
			if (activeTab === 'create') return `// Register a new user\nconst res = await fetch('${base}/register', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify(${sampleBody})\n});\nconst { token, user } = await res.json();`;
			if (activeTab === 'view') return `// Get current authenticated user\nconst res = await fetch('${base}/me', {\n  headers: { Authorization: 'Bearer <JWT_TOKEN>' }\n});`;
			return `// Login with email & password\nconst res = await fetch('${base}/login', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify(${sampleBody})\n});\nconst { token, user } = await res.json();`;
		}

		switch (activeTab) {
			case 'list':
				return `const res = await fetch('${base}?page=1&per_page=20&search=');\nconst data = await res.json();`;
			case 'view':
				return `const res = await fetch('${base}/1');\nconst record = await res.json();`;
			case 'create':
				return `const res = await fetch('${base}', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify(${sampleBody})\n});\nconst created = await res.json();`;
			case 'update':
				return `const res = await fetch('${base}/1', {\n  method: 'PATCH',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify(${sampleBody})\n});\nconst updated = await res.json();`;
			case 'delete':
				return `await fetch('${base}/1', { method: 'DELETE' });`;
		}
	});

	async function copyCode() {
		await navigator.clipboard.writeText(snippet);
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<Drawer {open} {onClose} maxWidth="max-w-2xl">
	<div class="flex items-center justify-between border-b border-border px-6 py-4">
		<div class="flex items-center gap-2">
			<Code2 class="h-4 w-4 text-muted-foreground" />
			<h2 class="text-base font-semibold">API Preview &mdash; {collection.name}</h2>
		</div>
		<button
			type="button"
			onclick={onClose}
			class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
		>
			<X class="h-4 w-4" />
		</button>
	</div>

	<div class="grid grid-cols-5 border-b border-border px-6">
		{#each (['list', 'view', 'create', 'update', 'delete'] as const) as tab}
			<button
				type="button"
				onclick={() => (activeTab = tab)}
				class="cursor-pointer border-b-2 py-2.5 text-xs font-medium capitalize transition {activeTab === tab
					? 'border-foreground text-foreground'
					: 'border-transparent text-muted-foreground hover:text-foreground'}"
			>
				{tab}
			</button>
		{/each}
	</div>

	<div class="flex-1 space-y-4 overflow-y-auto p-6">
		<div class="overflow-hidden rounded-md border border-border bg-muted/30">
			<div class="flex items-center justify-between border-b border-border bg-muted/50 px-4 py-2 text-xs text-muted-foreground">
				<span>JavaScript (fetch)</span>
				<button
					type="button"
					onclick={copyCode}
					class="inline-flex cursor-pointer items-center gap-1 rounded px-2 py-0.5 font-medium text-foreground hover:bg-accent"
				>
					{#if copied}
						<Check class="h-3.5 w-3.5 text-emerald-500" /> Copied
					{:else}
						<Copy class="h-3.5 w-3.5" /> Copy
					{/if}
				</button>
			</div>
			<pre class="overflow-x-auto p-4 font-mono text-xs leading-relaxed text-foreground">{snippet}</pre>
		</div>

		<div class="flex items-center justify-between rounded-md border border-border bg-muted/20 px-4 py-3 text-xs">
			<span class="text-muted-foreground">Full OpenAPI 3.0 Specification</span>
			<a
				href="/api/openapi.json"
				target="_blank"
				rel="noreferrer"
				class="inline-flex items-center gap-1 font-medium text-primary hover:underline"
			>
				/api/openapi.json <ExternalLink class="h-3.5 w-3.5" />
			</a>
		</div>
	</div>
</Drawer>
