<script lang="ts">
	import { onMount } from 'svelte';
	import { CheckCircle2, Database, Plug, Save } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import Select from '#lib/components/ui/Select.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';

	let host = $state('localhost');
	let port = $state('5432');
	let user = $state('microbase');
	let password = $state('');
	let hasPassword = $state(false);
	let database = $state('microbase');
	let sslmode = $state('disable');
	let version = $state('');
	let testing = $state(false);
	let saving = $state(false);
	let needsRestart = $state(false);

	const sslOptions = [
		{ value: 'disable', label: 'disable' },
		{ value: 'prefer', label: 'prefer' },
		{ value: 'require', label: 'require' },
		{ value: 'verify-full', label: 'verify-full' }
	];

	onMount(async () => {
		try {
			const res = await apiRequest<{
				host: string;
				port: string;
				user: string;
				has_password: boolean;
				database: string;
				sslmode: string;
				version: string;
			}>('/api/settings/database');
			host = res.host || 'localhost';
			port = res.port || '5432';
			user = res.user || 'microbase';
			hasPassword = Boolean(res.has_password);
			database = res.database || 'microbase';
			sslmode = res.sslmode || 'disable';
			version = res.version || '';
		} catch {
			// Ignore
		}
	});

	async function handleTest() {
		if (!host.trim() || !database.trim()) return toast.error('Host / IP and Database name are required.');
		testing = true;
		try {
			const res = await apiRequest<{ version: string }>('/api/settings/database/test', {
				method: 'POST',
				body: JSON.stringify({ host, port, user, password, database, sslmode })
			});
			toast.success(`Connection OK! ${res.version?.split(',')[0] || 'PostgreSQL ready'}`);
		} catch (err) {
			toast.error(err);
		} finally {
			testing = false;
		}
	}

	async function handleSave() {
		if (!host.trim() || !database.trim()) return toast.error('Host / IP and Database name are required.');
		saving = true;
		try {
			await apiRequest('/api/settings/database', {
				method: 'PUT',
				body: JSON.stringify({ host, port, user, password, database, sslmode })
			});
			password = '';
			hasPassword = true;
			needsRestart = true;
			toast.success('Updated DATABASE_URL in .env! Restart ./microbase to switch active connection.');
		} catch (err) {
			toast.error(err);
		} finally {
			saving = false;
		}
	}
</script>

<section class="space-y-4 rounded-lg border border-border bg-card p-5">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<div>
			<h2 class="flex items-center gap-2 text-sm font-semibold">
				<Database class="h-4 w-4 text-muted-foreground" />
				PostgreSQL Database Connection
			</h2>
			<p class="mt-0.5 text-xs text-muted-foreground">
				Change target PostgreSQL IP/host, test connectivity live, and persist to <code class="rounded bg-muted px-1">.env</code>.
			</p>
		</div>
		{#if version}
			<span class="inline-flex items-center gap-1.5 rounded bg-emerald-500/15 px-2.5 py-1 font-mono text-xs font-medium text-emerald-700 dark:text-emerald-400">
				<CheckCircle2 class="h-3.5 w-3.5" />
				{version.split(' ').slice(0, 2).join(' ')}
			</span>
		{/if}
	</div>

	<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
		<div class="sm:col-span-2">
			<label for="db-host" class="mb-1 block text-xs font-medium">Host / IP Address</label>
			<input id="db-host" type="text" bind:value={host} placeholder="e.g. 192.168.5.159 or localhost" class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
		</div>
		<div>
			<label for="db-port" class="mb-1 block text-xs font-medium">Port</label>
			<input id="db-port" type="text" bind:value={port} placeholder="5432" class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
		</div>
		<div>
			<label for="db-name" class="mb-1 block text-xs font-medium">Database Name</label>
			<input id="db-name" type="text" bind:value={database} placeholder="microbase" class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
		</div>
		<div>
			<label for="db-user" class="mb-1 block text-xs font-medium">Username</label>
			<input id="db-user" type="text" bind:value={user} placeholder="microbase" class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
		</div>
		<div>
			<label for="db-pass" class="mb-1 block text-xs font-medium">
				Password {#if hasPassword}<span class="text-muted-foreground">(saved)</span>{/if}
			</label>
			<input id="db-pass" type="password" bind:value={password} placeholder={hasPassword ? 'Leave blank to keep' : 'Password'} class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
		</div>
		<div>
			<div class="mb-1 text-xs font-medium">SSL Mode</div>
			<Select bind:value={sslmode} options={sslOptions} />
		</div>
	</div>

	{#if needsRestart}
		<div class="rounded-md border border-amber-500/30 bg-amber-500/10 px-3.5 py-2.5 text-xs text-amber-800 dark:text-amber-300">
			Saved new <code>DATABASE_URL</code> to <code>.env</code>. Restart <code>./microbase</code> to connect all services to the new PostgreSQL server.
		</div>
	{/if}

	<div class="flex items-center justify-end gap-2 border-t border-border pt-3">
		<button type="button" disabled={testing} onclick={handleTest} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-3.5 py-2 text-xs font-medium hover:bg-muted disabled:opacity-50">
			<Plug class="h-3.5 w-3.5" />
			{testing ? 'Testing...' : 'Test Connection'}
		</button>
		<button type="button" disabled={saving} onclick={handleSave} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-2 text-xs font-semibold text-background hover:opacity-90 disabled:opacity-50">
			<Save class="h-3.5 w-3.5" />
			{saving ? 'Saving...' : 'Save to .env'}
		</button>
	</div>
</section>
