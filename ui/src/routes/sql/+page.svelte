<script lang="ts">
	import { onMount } from 'svelte';
	import { GitBranch, Play, Terminal } from '@lucide/svelte';
	import ErdCanvas from '#lib/components/erd/ErdCanvas.svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import { listCollections } from '#lib/api/collections.ts';
	import type { Collection } from '#lib/api/types.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface SQLResult {
		columns: string[];
		rows: Record<string, unknown>[];
		row_count: number;
		duration_ms: number;
	}

	let activeTab = $state<'sql' | 'erd'>('sql');
	let query = $state('SELECT id, name, type, created_at FROM _collections ORDER BY id ASC;');
	let running = $state(false);
	let result = $state<SQLResult | null>(null);
	let sqlError = $state('');
	let collections = $state<Collection[]>([]);

	const presets = [
		{ label: 'Collections', sql: 'SELECT id, name, type, created_at FROM _collections ORDER BY id ASC;' },
		{ label: 'Users', sql: 'SELECT id, email, role, verified, created_at FROM "users" ORDER BY id DESC LIMIT 50;' },
		{ label: 'Recent Logs', sql: 'SELECT id, status, method, path, code, latency_ms, created_at FROM _logs ORDER BY id DESC LIMIT 50;' },
		{ label: 'Postgres Tables', sql: "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;" }
	];

	async function runQuery() {
		if (!query.trim()) return;
		running = true;
		sqlError = '';
		try {
			result = await apiRequest<SQLResult>('/api/sql', {
				method: 'POST',
				body: JSON.stringify({ query })
			});
		} catch (err) {
			sqlError = err instanceof Error ? err.message : 'SQL execution failed';
			toast.error(sqlError);
		} finally {
			running = false;
		}
	}

	onMount(async () => {
		try {
			collections = await listCollections();
		} catch {
			// Ignore
		}
	});
</script>

<main class="flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
	<header class="flex items-center justify-between border-b border-border bg-card px-6 py-3.5">
		<div class="flex items-center gap-4">
			<h1 class="text-sm font-semibold text-foreground">Tools</h1>
			<div class="flex items-center gap-1 rounded-md bg-muted/70 p-1">
				<button
					type="button"
					onclick={() => (activeTab = 'sql')}
					class="inline-flex cursor-pointer items-center gap-1.5 rounded px-3 py-1 text-xs font-medium transition {activeTab === 'sql'
						? 'bg-card text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'}"
				>
					<Terminal class="h-3.5 w-3.5" />
					SQL Console
				</button>
				<button
					type="button"
					onclick={() => (activeTab = 'erd')}
					class="inline-flex cursor-pointer items-center gap-1.5 rounded px-3 py-1 text-xs font-medium transition {activeTab === 'erd'
						? 'bg-card text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'}"
				>
					<GitBranch class="h-3.5 w-3.5" />
					ERD Schema
				</button>
			</div>
		</div>
	</header>

	{#if activeTab === 'sql'}
		<div class="flex min-h-0 flex-1 flex-col gap-4 p-6">
			<div class="space-y-3 rounded-lg border border-border bg-card p-4">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<div class="flex flex-wrap items-center gap-1.5">
						<span class="text-xs text-muted-foreground">Presets:</span>
						{#each presets as p (p.label)}
							<button
								type="button"
								onclick={() => {
									query = p.sql;
									runQuery();
								}}
								class="cursor-pointer rounded border border-border bg-muted/50 px-2.5 py-1 text-xs font-medium transition hover:bg-accent"
							>
								{p.label}
							</button>
						{/each}
					</div>
					<button
						type="button"
						disabled={running}
						onclick={runQuery}
						class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-1.5 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-50"
					>
						<Play class="h-3.5 w-3.5" />
						{running ? 'Running...' : 'Execute (Ctrl+Enter)'}
					</button>
				</div>

				<textarea
					rows="5"
					bind:value={query}
					onkeydown={(e) => {
						if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
							e.preventDefault();
							runQuery();
						}
					}}
					placeholder="SELECT * FROM posts LIMIT 50;"
					class="w-full rounded-md border border-input bg-muted/45 p-3 font-mono text-xs leading-relaxed outline-none focus:border-ring focus:bg-card"
				></textarea>

				{#if sqlError}
					<div class="rounded-md bg-destructive/12 px-3.5 py-2.5 font-mono text-xs text-destructive">
						{sqlError}
					</div>
				{/if}
			</div>

			{#if result}
				<div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
					<div class="flex items-center justify-between border-b border-border bg-muted/35 px-4 py-2 text-xs text-muted-foreground">
						<span><strong>{result.row_count}</strong> rows returned</span>
						<span class="font-mono">{result.duration_ms}ms</span>
					</div>
					<div class="flex-1 overflow-auto">
						<table class="w-full border-collapse text-left text-xs">
							<thead class="sticky top-0 border-b border-border bg-card text-muted-foreground">
								<tr>
									{#each result.columns as col (col)}
										<th class="px-4 py-2.5 font-mono font-semibold">{col}</th>
									{/each}
								</tr>
							</thead>
							<tbody class="divide-y divide-border font-mono">
								{#each result.rows as row, i (i)}
									<tr class="hover:bg-muted/45">
										{#each result.columns as col (col)}
											<td class="max-w-xs truncate px-4 py-2">
												{typeof row[col] === 'object' && row[col] !== null
													? JSON.stringify(row[col])
													: String(row[col] ?? 'NULL')}
											</td>
										{/each}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			{/if}
		</div>
	{:else}
		<ErdCanvas {collections} />
	{/if}
</main>
