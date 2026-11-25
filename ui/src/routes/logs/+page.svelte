<script lang="ts">
	import { onMount } from 'svelte';
	import { RefreshCw, Search } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface LogEntry {
		timestamp: string;
		method: string;
		path: string;
		status: number;
		latency_ms: number;
		ip: string;
		error?: string;
	}

	let logs = $state<LogEntry[]>([]);
	let filter = $state('');
	let loading = $state(true);

	let filtered = $derived(
		logs.filter((l) =>
			`${l.method} ${l.path} ${l.status} ${l.error ?? ''}`
				.toLowerCase()
				.includes(filter.trim().toLowerCase())
		)
	);

	async function loadLogs() {
		loading = true;
		try {
			const res = await apiRequest<{ items: LogEntry[] }>('/api/logs');
			logs = res.items ?? [];
		} catch (err) {
			toast.error(err, 'Failed to load logs');
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadLogs();
	});
</script>

<main class="flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
	<header class="space-y-3 border-b border-border px-6 py-4">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2 text-sm">
				<h1 class="font-semibold text-foreground">Logs</h1>
				<button
					type="button"
					onclick={loadLogs}
					title="Refresh logs"
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-accent hover:text-foreground"
				>
					<RefreshCw class="h-4 w-4" />
				</button>
			</div>
			<span class="text-xs text-muted-foreground">Showing {filtered.length} recent requests</span>
		</div>

		<div class="relative">
			<Search class="pointer-events-none absolute top-2.5 left-3.5 h-4 w-4 text-muted-foreground" />
			<input
				type="text"
				bind:value={filter}
				placeholder="Filter by method, path, or status..."
				class="w-full rounded-full border border-border bg-muted/40 py-2 pr-4 pl-10 text-xs outline-none focus:border-ring focus:bg-background"
			/>
		</div>
	</header>

	<div class="flex-1 overflow-auto">
		<table class="w-full border-collapse text-left text-xs">
			<thead class="sticky top-0 border-b border-border bg-background/95 text-muted-foreground">
				<tr>
					<th class="px-5 py-3 font-medium">Status</th>
					<th class="px-5 py-3 font-medium">Method</th>
					<th class="px-5 py-3 font-medium">Path</th>
					<th class="px-5 py-3 font-medium">Latency</th>
					<th class="px-5 py-3 font-medium">IP</th>
					<th class="px-5 py-3 font-medium">Timestamp</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-border font-mono">
				{#if loading}
					<tr><td colspan="6" class="px-5 py-10 text-center text-muted-foreground">Loading logs...</td></tr>
				{:else if filtered.length === 0}
					<tr><td colspan="6" class="px-5 py-10 text-center text-muted-foreground">No logs yet.</td></tr>
				{:else}
					{#each filtered as row, i (i)}
						<tr class="hover:bg-muted/40">
							<td class="px-5 py-2.5">
								<span
									class="rounded px-2 py-0.5 text-[11px] font-semibold {row.status >= 500
										? 'bg-destructive/15 text-destructive'
										: row.status >= 400
											? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
											: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}"
								>
									{row.status}
								</span>
							</td>
							<td class="px-5 py-2.5 font-semibold">{row.method}</td>
							<td class="max-w-md truncate px-5 py-2.5">
								{row.path}
								{#if row.error}
									<span class="ml-2 text-[11px] text-destructive">({row.error})</span>
								{/if}
							</td>
							<td class="px-5 py-2.5 text-muted-foreground">{row.latency_ms}ms</td>
							<td class="px-5 py-2.5 text-muted-foreground">{row.ip}</td>
							<td class="px-5 py-2.5 text-muted-foreground">{new Date(row.timestamp).toLocaleTimeString()}</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
	</div>
</main>
