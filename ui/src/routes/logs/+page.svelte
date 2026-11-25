<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { fade, scale } from 'svelte/transition';
	import { onMount } from 'svelte';
	import { RefreshCw, Search, X } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface LogEntry {
		id?: number;
		timestamp: string;
		method: string;
		path: string;
		status: number;
		code?: string;
		message?: string;
		latency_ms: number;
		ip: string;
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
			`${l.method} ${l.path} ${l.status} ${l.code ?? ''} ${l.message ?? ''}`
				.toLowerCase()
				.includes(filter.trim().toLowerCase())
		)
	);

	// Group recent logs into 20 visual chart buckets
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
				placeholder="Filter by method, path, code, message, or status..."
				class="w-full rounded-full border border-transparent bg-muted/80 py-2 pr-4 pl-10 text-xs outline-none transition focus:border-border focus:bg-accent/70"
			/>
		</div>

		<!-- Mini PocketBase-style Request Volume Chart -->
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
					<th class="px-4 py-3 font-semibold">Path</th>
					<th class="px-4 py-3 font-semibold">Code</th>
					<th class="px-4 py-3 font-semibold">Message</th>
					<th class="px-4 py-3 font-semibold">Latency</th>
					<th class="px-4 py-3 font-semibold">IP</th>
					<th class="px-5 py-3 font-semibold">Timestamp</th>
				</tr>
			</thead>
			{#key refreshGen}
				<tbody class="divide-y divide-border font-mono">
					{#if loading && logs.length === 0}
						<tr><td colspan="8" class="px-5 py-10 text-center text-muted-foreground">Loading logs...</td></tr>
					{:else if filtered.length === 0}
						<tr><td colspan="8" class="px-5 py-10 text-center text-muted-foreground">No logs yet.</td></tr>
					{:else}
						{#each filtered as row, i (row.id ?? `${row.timestamp}-${i}`)}
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
								<td class="max-w-xs truncate px-4 py-2.5 text-foreground" title={row.path}>{row.path}</td>
								<td class="px-4 py-2.5">
									<span
										class="rounded px-1.5 py-0.5 text-xs {row.status >= 400
											? 'bg-destructive/10 text-destructive'
											: 'bg-muted text-muted-foreground'}"
									>
										{row.code || (row.status >= 400 ? String(row.status) : 'OK')}
									</span>
								</td>
								<td
									class="max-w-xs truncate px-4 py-2.5 font-sans text-xs {row.status >= 400
										? 'text-destructive'
										: 'text-muted-foreground'}"
									title={row.message || ''}
								>
									{row.message || '—'}
								</td>
								<td class="px-4 py-2.5 text-muted-foreground">{row.latency_ms}ms</td>
								<td class="px-4 py-2.5 text-muted-foreground">{row.ip}</td>
								<td class="px-5 py-2.5 text-muted-foreground">
									{new Date(row.timestamp).toLocaleString()}
								</td>
							</tr>
						{/each}
					{/if}
				</tbody>
			{/key}
		</table>
	</div>
</main>

<!-- Log Preview Modal -->
{#if selectedLog}
	<div transition:fade={{ duration: 150 }} class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<button
			type="button"
			aria-label="Close log preview"
			onclick={() => (selectedLog = null)}
			class="animate-fade-in fixed inset-0 bg-black/45 backdrop-blur-[2px]"
		></button>
		<div class="animate-modal-in relative z-10 w-full max-w-lg rounded-lg border border-border bg-card p-6 shadow-xl">
			<div class="mb-4 flex items-center justify-between">
				<h3 class="text-base font-semibold">Request Log #{selectedLog.id ?? ''}</h3>
				<button
					type="button"
					onclick={() => (selectedLog = null)}
					class="cursor-pointer rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
				>
					<X class="h-4 w-4" />
				</button>
			</div>
			<pre class="overflow-auto rounded-md border border-border bg-muted/50 p-4 font-mono text-xs leading-relaxed">{JSON.stringify(selectedLog, null, 2)}</pre>
		</div>
	</div>
{/if}
