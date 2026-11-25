<script lang="ts">
	import { onMount } from 'svelte';
	import { Clock, Play } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface CronItem {
		id: string;
		expression: string;
		description: string;
	}

	let crons = $state<CronItem[]>([]);
	let runningId = $state<string | null>(null);

	onMount(async () => {
		try {
			const res = await apiRequest<{ items: CronItem[] }>('/api/crons');
			crons = res.items ?? [];
		} catch {
			// Ignore
		}
	});

	async function runNow(id: string) {
		runningId = id;
		try {
			await apiRequest(`/api/crons/${encodeURIComponent(id)}/run`, { method: 'POST' });
			toast.success(`Executed cron job "${id}"`);
		} catch (err) {
			toast.error(err);
		} finally {
			runningId = null;
		}
	}
</script>

<section class="space-y-4 rounded-lg border border-border bg-card p-5">
	<div>
		<h2 class="flex items-center gap-2 text-sm font-semibold">
			<Clock class="h-4 w-4 text-muted-foreground" />
			Cron Jobs
		</h2>
		<p class="mt-0.5 text-xs text-muted-foreground">Scheduled maintenance tasks you can trigger manually anytime.</p>
	</div>

	<div class="divide-y divide-border overflow-hidden rounded-md border border-border">
		{#each crons as job (job.id)}
			<div class="flex items-center justify-between gap-3 bg-background/50 px-4 py-3 text-xs">
				<div>
					<div class="flex items-center gap-2">
						<span class="font-mono font-semibold">{job.id}</span>
						<span class="rounded bg-muted px-2 py-0.5 font-mono text-xs text-muted-foreground">{job.expression}</span>
					</div>
					<div class="mt-0.5 text-muted-foreground">{job.description}</div>
				</div>
				<button type="button" disabled={runningId === job.id} onclick={() => runNow(job.id)} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-card px-3 py-1.5 text-xs font-medium hover:bg-muted disabled:opacity-50">
					<Play class="h-3.5 w-3.5" />
					{runningId === job.id ? 'Running...' : 'Run'}
				</button>
			</div>
		{/each}
	</div>
</section>
