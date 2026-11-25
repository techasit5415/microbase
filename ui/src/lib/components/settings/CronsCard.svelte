<script lang="ts">
	import { onMount } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { slide } from 'svelte/transition';
	import { Clock, Database, Globe, Play, Plus, RefreshCw, Trash2, X } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import Select from '#lib/components/ui/Select.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface CronJob {
		id: string;
		expression: string;
		kind: 'system' | 'http' | 'sql';
		target?: string;
		method?: string;
		payload?: string;
		description: string;
		enabled: boolean;
		is_system?: boolean;
		last_run_at?: string;
		last_status?: string;
	}

	let crons = $state<CronJob[]>([]);
	let runningId = $state<string | null>(null);
	let showCreate = $state(false);
	let creating = $state(false);

	// New cron form state
	let newId = $state('');
	let newExpr = $state('*/5 * * * *');
	let newKind = $state<'http' | 'sql'>('http');
	let newMethod = $state('POST');
	let newTarget = $state('');
	let newPayload = $state('');
	let newDesc = $state('');

	const kindOptions = [
		{ value: 'http', label: 'HTTP Request / Webhook' },
		{ value: 'sql', label: 'SQL Query (PostgreSQL)' }
	];

	const methodOptions = [
		{ value: 'POST', label: 'POST' },
		{ value: 'GET', label: 'GET' },
		{ value: 'PUT', label: 'PUT' },
		{ value: 'DELETE', label: 'DELETE' }
	];

	const presets = [
		{ label: 'Every 5 min', expr: '*/5 * * * *' },
		{ label: 'Hourly', expr: '0 * * * *' },
		{ label: 'Daily 08:00', expr: '0 8 * * *' },
		{ label: 'Daily 00:00', expr: '0 0 * * *' },
		{ label: 'Weekly Mon', expr: '0 9 * * 1' }
	];

	async function load() {
		try {
			const res = await apiRequest<{ items: CronJob[] }>('/api/crons');
			crons = res.items ?? [];
		} catch {
			// ignore
		}
	}

	onMount(load);

	async function createCron(e: SubmitEvent) {
		e.preventDefault();
		if (!newId.trim() || !newExpr.trim() || !newTarget.trim()) return;
		creating = true;
		try {
			await apiRequest('/api/crons', {
				method: 'POST',
				body: JSON.stringify({
					id: newId.trim(),
					expression: newExpr.trim(),
					kind: newKind,
					method: newKind === 'http' ? newMethod : '',
					target: newTarget.trim(),
					payload: newKind === 'http' ? newPayload.trim() : '',
					description: newDesc.trim() || (newKind === 'http' ? `${newMethod} ${newTarget.trim()}` : 'Scheduled SQL query'),
					enabled: true
				})
			});
			toast.success(`Created cron job "${newId.trim()}"`);
			newId = '';
			newTarget = '';
			newPayload = '';
			newDesc = '';
			showCreate = false;
			await load();
		} catch (err) {
			toast.error(err, 'Failed to create cron job');
		} finally {
			creating = false;
		}
	}

	async function toggleEnabled(job: CronJob, next: boolean) {
		try {
			await apiRequest(`/api/crons/${encodeURIComponent(job.id)}`, {
				method: 'PATCH',
				body: JSON.stringify({ enabled: next })
			});
			job.enabled = next;
			toast.success(`${job.id} ${next ? 'enabled' : 'paused'}`);
		} catch (err) {
			toast.error(err);
		}
	}

	async function runNow(id: string) {
		runningId = id;
		try {
			const res = await apiRequest<{ result?: string }>(`/api/crons/${encodeURIComponent(id)}/run`, {
				method: 'POST'
			});
			toast.success(`Executed "${id}"${res?.result ? `: ${res.result}` : ''}`);
			await load();
		} catch (err) {
			toast.error(err, `Failed to execute "${id}"`);
			await load();
		} finally {
			runningId = null;
		}
	}

	async function removeCron(id: string) {
		const ok = await confirmAction({
			title: `Delete cron "${id}"?`,
			description: 'This scheduled job will be permanently removed.',
			confirmLabel: 'Delete',
			destructive: true
		});
		if (!ok) return;
		try {
			await apiRequest(`/api/crons/${encodeURIComponent(id)}`, { method: 'DELETE' });
			toast.success(`Deleted cron "${id}"`);
			await load();
		} catch (err) {
			toast.error(err);
		}
	}

	function fmtTime(iso?: string): string {
		if (!iso) return '';
		const m = iso.match(/^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}:\d{2})/);
		return m ? `${m[1]} ${m[2]}` : iso;
	}
</script>

<div class="rounded-md border border-border bg-card p-5">
	<div class="mb-4 flex items-center justify-between gap-3">
		<div>
			<div class="flex items-center gap-2">
				<Clock class="h-4 w-4 text-muted-foreground" />
				<h2 class="text-sm font-semibold">Cron Jobs</h2>
			</div>
			<p class="mt-0.5 text-xs text-muted-foreground">
				Built-in maintenance tasks and custom scheduled HTTP Webhooks or PostgreSQL SQL queries.
			</p>
		</div>
		<button
			type="button"
			onclick={() => (showCreate = !showCreate)}
			class="inline-flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md bg-primary px-3.5 py-2 text-xs font-semibold text-primary-foreground transition hover:opacity-90"
		>
			{#if showCreate}
				<X class="h-3.5 w-3.5" /> Close
			{:else}
				<Plus class="h-3.5 w-3.5" /> New cron job
			{/if}
		</button>
	</div>

	{#if showCreate}
		<form
			onsubmit={createCron}
			transition:slide={{ duration: 180, easing: cubicOut }}
			class="mb-5 space-y-3.5 rounded-md border border-border bg-muted/30 p-4"
		>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
					<label for="cron-id" class="mb-0.5 block text-xs font-medium text-muted-foreground">Job ID *</label>
					<input
						id="cron-id"
						type="text"
						required
						bind:value={newId}
						placeholder="e.g. cleanup_expired_tokens"
						class="w-full bg-transparent font-mono text-xs outline-none"
					/>
				</div>
				<div>
					<div class="mb-1 text-xs font-medium text-muted-foreground">Action Type</div>
					<Select bind:value={newKind} options={kindOptions} />
				</div>
			</div>

			<div>
				<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
					<label for="cron-expr" class="mb-0.5 block text-xs font-medium text-muted-foreground">
						Cron expression * (minute hour day month weekday)
					</label>
					<input
						id="cron-expr"
						type="text"
						required
						bind:value={newExpr}
						placeholder="*/5 * * * *"
						class="w-full bg-transparent font-mono text-xs outline-none"
					/>
				</div>
				<div class="mt-1.5 flex flex-wrap gap-1.5">
					{#each presets as p (p.expr)}
						<button
							type="button"
							onclick={() => (newExpr = p.expr)}
							class="cursor-pointer rounded border border-border bg-card px-2 py-0.5 text-[11px] text-muted-foreground transition hover:bg-accent hover:text-foreground"
						>
							{p.label} <code class="ml-0.5 font-mono">{p.expr}</code>
						</button>
					{/each}
				</div>
			</div>

			{#if newKind === 'http'}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-4">
					<div>
						<div class="mb-1 text-xs font-medium text-muted-foreground">Method</div>
						<Select bind:value={newMethod} options={methodOptions} />
					</div>
					<div class="rounded-[5px] bg-secondary/75 px-3 py-2 sm:col-span-3">
						<label for="cron-url" class="mb-0.5 block text-xs font-medium text-muted-foreground">Target URL *</label>
						<input
							id="cron-url"
							type="url"
							required
							bind:value={newTarget}
							placeholder="https://discord.com/api/webhooks/... or https://api.example.com/hook"
							class="w-full bg-transparent font-mono text-xs outline-none"
						/>
					</div>
				</div>
				{#if newMethod !== 'GET'}
					<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
						<label for="cron-body" class="mb-0.5 block text-xs font-medium text-muted-foreground">JSON Payload (optional)</label>
						<textarea
							id="cron-body"
							rows="2"
							bind:value={newPayload}
							placeholder={'{"content": "Daily report ping from Microbase!"}'}
							class="w-full bg-transparent font-mono text-xs outline-none"
						></textarea>
					</div>
				{/if}
			{:else}
				<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
					<label for="cron-sql" class="mb-0.5 block text-xs font-medium text-muted-foreground">PostgreSQL Query *</label>
					<textarea
						id="cron-sql"
						rows="3"
						required
						bind:value={newTarget}
						placeholder="DELETE FROM _logs WHERE created_at < NOW() - INTERVAL '14 days';"
						class="w-full bg-transparent font-mono text-xs outline-none"
					></textarea>
				</div>
			{/if}

			<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
				<label for="cron-desc" class="mb-0.5 block text-xs font-medium text-muted-foreground">Description (optional)</label>
				<input
					id="cron-desc"
					type="text"
					bind:value={newDesc}
					placeholder="Short description of what this cron job does..."
					class="w-full bg-transparent text-xs outline-none"
				/>
			</div>

			<div class="flex justify-end gap-2">
				<button
					type="button"
					onclick={() => (showCreate = false)}
					class="cursor-pointer rounded-md px-3.5 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted"
				>
					Cancel
				</button>
				<button
					type="submit"
					disabled={creating}
					class="cursor-pointer rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-primary-foreground transition hover:opacity-90 disabled:opacity-50"
				>
					{creating ? 'Creating...' : 'Create cron job'}
				</button>
			</div>
		</form>
	{/if}

	<div class="divide-y divide-border rounded-md border border-border">
		{#each crons as job (job.id)}
			<div class="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
				<div class="min-w-0 flex-1 space-y-1">
					<div class="flex flex-wrap items-center gap-2">
						<span class="font-mono text-xs font-semibold text-foreground">{job.id}</span>
						<code class="rounded bg-muted px-2 py-0.5 font-mono text-[11px] text-muted-foreground">{job.expression}</code>
						{#if job.is_system}
							<span class="rounded bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground uppercase">System</span>
						{:else if job.kind === 'sql'}
							<span class="inline-flex items-center gap-1 rounded bg-sky-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-sky-700 dark:text-sky-400">
								<Database class="h-3 w-3" /> SQL
							</span>
						{:else}
							<span class="inline-flex items-center gap-1 rounded bg-purple-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-purple-700 dark:text-purple-400">
								<Globe class="h-3 w-3" /> {job.method || 'HTTP'}
							</span>
						{/if}
					</div>
					<p class="truncate text-xs text-muted-foreground">{job.description || job.target}</p>
					{#if job.last_run_at || job.last_status}
						<div class="flex flex-wrap items-center gap-2 text-[11px]">
							{#if job.last_run_at}
								<span class="font-mono text-muted-foreground">Last run: {fmtTime(job.last_run_at)}</span>
							{/if}
							{#if job.last_status}
								<span
									class="rounded px-1.5 py-0.5 font-mono {job.last_status.startsWith('error')
										? 'bg-destructive/15 text-destructive'
										: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}"
								>
									{job.last_status}
								</span>
							{/if}
						</div>
					{/if}
				</div>

				<div class="flex items-center gap-2.5">
					{#if !job.is_system}
						<Switch checked={job.enabled} onChange={(v) => toggleEnabled(job, v)} />
					{/if}
					<button
						type="button"
						disabled={runningId === job.id}
						onclick={() => runNow(job.id)}
						class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground transition hover:bg-muted disabled:opacity-50"
					>
						{#if runningId === job.id}
							<RefreshCw class="h-3.5 w-3.5 animate-spin" /> Running...
						{:else}
							<Play class="h-3.5 w-3.5" /> Run
						{/if}
					</button>
					{#if !job.is_system}
						<button
							type="button"
							onclick={() => removeCron(job.id)}
							title="Delete cron job"
							class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-destructive/15 hover:text-destructive"
						>
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</div>
