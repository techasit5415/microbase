<script lang="ts">
	import { onMount } from 'svelte';
	import { Download, RefreshCw, Search, X } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import { formatTimestamp, uiPrefs } from '#lib/stores/uiPrefs.svelte.ts';

	interface LogEntry {
		id?: number;
		timestamp: string;
		method: string;
		path: string;
		url?: string;
		status: number;
		code?: string;
		message?: string;
		latency_ms: number;
		exec_time_ms?: number;
		ip: string;
		remote_ip?: string;
		referer?: string;
		user_agent?: string;
		auth?: string;
		auth_role?: string;
		auth_email?: string;
		auth_id?: string;
	}

	let logs = $state<LogEntry[]>([]);
	let filter = $state('');
	let loading = $state(true);
	let spinning = $state(false);
	let refreshGen = $state(0);
	let selectedLog = $state<LogEntry | null>(null);
	let maxDays = $state(7);

	let filtered = $derived(
		logs.filter((l) =>
			`${l.method} ${l.url || l.path} ${l.status} ${l.code ?? ''} ${l.message ?? ''} ${l.auth && l.auth !== '_guest' ? l.auth : ''} ${l.auth_role ?? ''} ${l.auth_email ?? ''} ${l.auth_id ?? ''} ${l.user_agent ?? ''}`
				.toLowerCase()
				.includes(filter.trim().toLowerCase())
		)
	);

	let chartBars = $derived.by(() => {
		const buckets: { label: string; ok: number; err: number; total: number }[] = [];
		const byMinute = new Map<string, { ok: number; err: number }>();
		for (const row of filtered) {
			const d = new Date(row.timestamp);
			const key = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
			const cur = byMinute.get(key) ?? { ok: 0, err: 0 };
			if (row.status >= 400) cur.err += 1;
			else cur.ok += 1;
			byMinute.set(key, cur);
		}
		for (const [label, v] of Array.from(byMinute.entries()).reverse().slice(-24)) {
			buckets.push({ label, ok: v.ok, err: v.err, total: v.ok + v.err });
		}
		return buckets;
	});

	let maxBar = $derived(Math.max(1, ...chartBars.map((b) => b.total)));

	function logLevelMeta(status: number): { label: string; dotClass: string } {
		if (status >= 500) return { label: 'ERROR (8)', dotClass: 'bg-destructive' };
		if (status >= 400) return { label: 'WARN (4)', dotClass: 'bg-amber-500' };
		return { label: 'INFO (0)', dotClass: 'bg-emerald-500' };
	}

	function formatExecTime(row: LogEntry): string {
		if (typeof row.exec_time_ms === 'number' && row.exec_time_ms > 0) {
			return `${Number(row.exec_time_ms.toFixed(6))}ms`;
		}
		return `${row.latency_ms}ms`;
	}

	function downloadLogJson(row: LogEntry) {
		const payload = {
			id: row.id,
			level: row.status >= 500 ? 8 : row.status >= 400 ? 4 : 0,
			created: row.timestamp,
			data: {
				execTime: row.exec_time_ms ?? row.latency_ms,
				type: 'request',
				auth: row.auth && row.auth !== '_guest' ? row.auth : '',
				authEmail: row.auth_email || undefined,
				authId: row.auth_id || undefined,
				status: row.status,
				method: row.method,
				url: row.url || row.path,
				referer: row.referer || '',
				remoteIP: row.remote_ip || row.ip,
				userIP: row.ip,
				userAgent: row.user_agent || ''
			}
		};
		const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `log_${row.id ?? 'entry'}.json`;
		a.click();
		URL.revokeObjectURL(url);
	}

	async function loadLogs() {
		loading = true;
		spinning = true;
		const startedAt = Date.now();
		try {
			const res = await apiRequest<{ items: LogEntry[] }>('/api/logs');
			logs = res.items ?? [];
			refreshGen += 1;
		} catch (err) {
			toast.error(err, 'Failed to load logs');
		} finally {
			loading = false;
			const elapsed = Date.now() - startedAt;
			setTimeout(() => {
				spinning = false;
			}, Math.max(0, 450 - elapsed));
		}
	}

	async function saveRetention(days: number) {
		maxDays = days;
		try {
			await apiRequest('/api/settings/logs', { method: 'PUT', body: JSON.stringify({ max_days: days }) });
			toast.success(`Log retention set to ${days} days`);
		} catch (err) {
			toast.error(err);
		}
	}

	onMount(async () => {
		loadLogs();
		try {
			const r = await apiRequest<{ max_days: number }>('/api/settings/logs');
			if (r.max_days) maxDays = r.max_days;
		} catch {}
	});
</script>

<main class="flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
	<header class="space-y-3 border-b border-border bg-card px-6 py-4">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2 text-sm">
				<h1 class="font-semibold text-foreground">Logs</h1>
				<button
					type="button"
					onclick={loadLogs}
					disabled={spinning}
					title="Refresh logs"
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground disabled:opacity-70"
				>
					<RefreshCw class="h-4 w-4 transition-transform {spinning ? 'animate-spin text-foreground' : ''}" />
				</button>
			</div>
			<div class="flex items-center gap-3 text-xs text-muted-foreground">
				<label class="flex items-center gap-1.5">
					<span>Retention (days):</span>
					<input
						type="number"
						min="1"
						max="365"
						value={maxDays}
						onchange={(e) => saveRetention(parseInt(e.currentTarget.value, 10) || 7)}
						class="w-16 rounded border border-input bg-background px-2 py-0.5 font-mono text-xs text-foreground outline-none focus:border-ring"
					/>
				</label>
				<span>Showing {filtered.length} persisted requests</span>
			</div>
		</div>

		<div class="relative">
			<Search class="pointer-events-none absolute top-2.5 left-3.5 h-4 w-4 text-muted-foreground" />
			<input
				type="text"
				bind:value={filter}
				placeholder="Filter by method, url, auth (users / _superusers / email), userAgent, status..."
				class="w-full rounded-full border border-transparent bg-muted/80 py-2 pr-4 pl-10 text-xs outline-none transition focus:border-border focus:bg-accent/70"
			/>
		</div>

		{#if chartBars.length > 0}
			<div class="flex h-16 items-end gap-1 rounded-md border border-border bg-background/70 px-3 pt-2 pb-1">
				{#each chartBars as bar (bar.label)}
					<div
						class="group relative flex h-full flex-1 flex-col justify-end"
						title="{bar.label} — {bar.total} reqs ({bar.ok} ok, {bar.err} err)"
					>
						<div
							class="w-full rounded-t-xs transition-all {bar.err > 0 ? 'bg-amber-500/80' : 'bg-emerald-500/75'} group-hover:opacity-100"
							style="height: {Math.max(12, Math.round((bar.total / maxBar) * 100))}%;"
						></div>
					</div>
				{/each}
			</div>
		{/if}
	</header>

	<div class="flex-1 overflow-auto">
		<table class="w-full border-collapse text-left text-xs">
			<thead class="sticky top-0 z-10 border-b border-border bg-card/95 text-muted-foreground backdrop-blur-xs">
				<tr>
					<th class="px-5 py-3 font-semibold">Status</th>
					<th class="px-4 py-3 font-semibold">Method</th>
					<th class="px-4 py-3 font-semibold">URL</th>
					<th class="px-4 py-3 font-semibold">Auth</th>
					<th class="px-4 py-3 font-semibold">Exec time</th>
					<th class="px-4 py-3 font-semibold">IP</th>
					<th class="px-5 py-3 font-semibold">Timestamp</th>
				</tr>
			</thead>
			{#key refreshGen}
				<tbody class="divide-y divide-border font-mono">
					{#if loading && logs.length === 0}
						<tr><td colspan="7" class="px-5 py-10 text-center text-muted-foreground">Loading logs...</td></tr>
					{:else if filtered.length === 0}
						<tr><td colspan="7" class="px-5 py-10 text-center text-muted-foreground">No logs yet.</td></tr>
					{:else}
						{#each filtered as row, i (row.id ?? `${row.timestamp}-${i}`)}
							{@const authName = (row.auth && row.auth !== '_guest' ? row.auth : '') || (row.auth_role === 'superuser' ? '_superusers' : row.auth_role ? 'users' : '')}
							<tr
								onclick={() => (selectedLog = row)}
								class="animate-row-in cursor-pointer bg-card transition-colors hover:bg-muted/60"
								style="animation-delay: {Math.min(i * 15, 180)}ms;"
							>
								<td class="px-5 py-2.5">
									<span
										class="rounded px-2 py-0.5 text-xs font-semibold {row.status >= 500
											? 'bg-destructive/15 text-destructive'
											: row.status >= 400
												? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
												: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}"
									>
										{row.status}
									</span>
								</td>
								<td class="px-4 py-2.5 font-semibold">{row.method}</td>
								<td class="max-w-sm truncate px-4 py-2.5 text-foreground" title={row.url || row.path}>{row.url || row.path}</td>
								<td class="max-w-56 px-4 py-2.5 whitespace-nowrap">
									{#if authName}
										<span class="inline-flex max-w-full items-center gap-1.5">
											<span
												class="shrink-0 rounded px-1.5 py-0.5 text-xs font-semibold {authName === '_superusers'
													? 'bg-violet-500/15 text-violet-700 dark:text-violet-400'
													: 'bg-sky-500/15 text-sky-700 dark:text-sky-400'}"
											>
												{authName}
											</span>
											{#if row.auth_email || row.auth_id}
												<span class="truncate font-sans text-xs text-foreground/90" title={row.auth_email || row.auth_id}>
													{row.auth_email || row.auth_id}
												</span>
											{/if}
										</span>
									{:else}
										<span class="text-muted-foreground/60">&mdash;</span>
									{/if}
								</td>
								<td class="px-4 py-2.5 text-muted-foreground">{formatExecTime(row)}</td>
								<td class="px-4 py-2.5 text-muted-foreground">{row.ip}</td>
								<td class="px-5 py-2.5 text-muted-foreground">
									{formatTimestamp(row.timestamp, uiPrefs.prefs.dateFormat)}
								</td>
							</tr>
						{/each}
					{/if}
				</tbody>
			{/key}
		</table>
	</div>
</main>

<!-- PocketBase-style Log details Slide-over Drawer -->
<Drawer
	open={selectedLog !== null}
	maxWidth="max-w-xl"
	onClose={() => (selectedLog = null)}
>
	{#if selectedLog}
		{@const lvl = logLevelMeta(selectedLog.status)}
		{@const authCol = (selectedLog.auth && selectedLog.auth !== '_guest' ? selectedLog.auth : '') || (selectedLog.auth_role === 'superuser' ? '_superusers' : selectedLog.auth_role ? 'users' : '')}
		<div class="flex h-full flex-col bg-card">
			<div class="flex items-center justify-between border-b border-border px-6 py-4">
				<h2 class="text-base font-semibold text-foreground">Log details</h2>
				<button
					type="button"
					aria-label="Close log details"
					onclick={() => (selectedLog = null)}
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<div class="flex-1 overflow-y-auto p-6">
				<div class="overflow-hidden rounded-md border border-border bg-card text-sm">
					<table class="w-full border-collapse text-left">
						<tbody class="divide-y divide-border">
							<tr>
								<td class="w-40 px-4 py-3 font-medium text-muted-foreground">id</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.id ?? '—'}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">level</td>
								<td class="px-4 py-3">
									<span class="inline-flex items-center gap-1.5 rounded bg-secondary px-2.5 py-0.5 text-xs font-semibold text-foreground">
										<span class="h-2 w-2 rounded-full {lvl.dotClass}"></span>
										{lvl.label}
									</span>
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">created</td>
								<td class="px-4 py-3">
									<div class="font-mono text-foreground">{formatTimestamp(selectedLog.timestamp, 'local')}</div>
									<div class="font-mono text-xs text-muted-foreground">{new Date(selectedLog.timestamp).toISOString()}</div>
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.execTime</td>
								<td class="px-4 py-3 font-mono text-foreground">{formatExecTime(selectedLog)}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.type</td>
								<td class="px-4 py-3 font-mono text-foreground">request</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.auth</td>
								<td class="px-4 py-3 font-mono {authCol ? 'text-foreground' : 'text-muted-foreground/60'}">{authCol || 'N/A'}</td>
							</tr>
							{#if selectedLog.auth_email}
								<tr>
									<td class="px-4 py-3 font-medium text-muted-foreground">data.authEmail</td>
									<td class="px-4 py-3 font-mono text-foreground">{selectedLog.auth_email}</td>
								</tr>
							{/if}
							{#if selectedLog.auth_id}
								<tr>
									<td class="px-4 py-3 font-medium text-muted-foreground">data.authId</td>
									<td class="px-4 py-3 font-mono text-foreground">{selectedLog.auth_id}</td>
								</tr>
							{/if}
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.status</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.status}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.method</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.method}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.url</td>
								<td class="px-4 py-3 font-mono break-all text-foreground">{selectedLog.url || selectedLog.path}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.referer</td>
								<td class="px-4 py-3 font-mono break-all {selectedLog.referer ? 'text-foreground' : 'text-muted-foreground/60'}">
									{selectedLog.referer || 'N/A'}
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.remoteIP</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.remote_ip || selectedLog.ip}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.userIP</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.ip}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.userAgent</td>
								<td class="px-4 py-3 font-mono break-all {selectedLog.user_agent ? 'text-foreground' : 'text-muted-foreground/60'}">
									{selectedLog.user_agent || 'N/A'}
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</div>

			<div class="flex shrink-0 items-center justify-between border-t border-border bg-card px-6 py-4">
				<button
					type="button"
					onclick={() => (selectedLog = null)}
					class="cursor-pointer rounded-md px-4 py-2 text-xs font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					Close
				</button>
				<button
					type="button"
					onclick={() => selectedLog && downloadLogJson(selectedLog)}
					class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-2 text-xs font-semibold text-background transition hover:opacity-90"
				>
					<Download class="h-3.5 w-3.5" />
					Download JSON
				</button>
			</div>
		</div>
	{/if}
</Drawer>
