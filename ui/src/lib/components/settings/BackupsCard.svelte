<script lang="ts">
	import { onMount } from 'svelte';
	import { Archive, Download, Plus, Trash2 } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface BackupItem {
		key: string;
		size: number;
		modified: string;
	}

	let backups = $state<BackupItem[]>([]);
	let creating = $state(false);

	async function loadBackups() {
		try {
			const res = await apiRequest<{ items: BackupItem[] }>('/api/backups');
			backups = res.items ?? [];
		} catch {
			// Ignore
		}
	}

	async function createBackup() {
		creating = true;
		try {
			await apiRequest('/api/backups', { method: 'POST' });
			toast.success('PostgreSQL backup created');
			await loadBackups();
		} catch (err) {
			toast.error(err);
		} finally {
			creating = false;
		}
	}

	async function removeBackup(key: string) {
		const ok = await confirmAction({
			title: `Delete backup "${key}"?`,
			description: 'This backup file will be permanently removed from disk.',
			confirmLabel: 'Delete',
			destructive: true
		});
		if (!ok) return;
		try {
			await apiRequest(`/api/backups/${encodeURIComponent(key)}`, { method: 'DELETE' });
			toast.success('Backup deleted');
			await loadBackups();
		} catch (err) {
			toast.error(err);
		}
	}

	onMount(loadBackups);
</script>

<section class="space-y-4 rounded-lg border border-border bg-card p-5">
	<div class="flex items-center justify-between gap-3">
		<div>
			<h2 class="flex items-center gap-2 text-sm font-semibold">
				<Archive class="h-4 w-4 text-muted-foreground" />
				Backups (PostgreSQL pg_dump)
			</h2>
			<p class="mt-0.5 text-xs text-muted-foreground">Create and download full SQL snapshots of your database.</p>
		</div>
		<button type="button" disabled={creating} onclick={createBackup} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-3.5 py-2 text-xs font-semibold text-background hover:opacity-90 disabled:opacity-50">
			<Plus class="h-3.5 w-3.5" />
			{creating ? 'Creating...' : 'Create backup'}
		</button>
	</div>

	{#if backups.length === 0}
		<p class="rounded-md border border-dashed border-border py-5 text-center text-xs text-muted-foreground">No backups yet.</p>
	{:else}
		<div class="divide-y divide-border overflow-hidden rounded-md border border-border">
			{#each backups as b (b.key)}
				<div class="flex items-center justify-between gap-2 bg-background/50 px-4 py-2.5 text-xs">
					<div>
						<div class="font-mono font-medium">{b.key}</div>
						<div class="text-muted-foreground">{(b.size / 1024).toFixed(1)} KB &middot; {new Date(b.modified).toLocaleString()}</div>
					</div>
					<div class="flex items-center gap-1.5">
						<a href="/api/backups/{encodeURIComponent(b.key)}" class="inline-flex items-center gap-1 rounded border border-border bg-card px-2.5 py-1 text-xs font-medium hover:bg-muted">
							<Download class="h-3.5 w-3.5" /> Download
						</a>
						<button type="button" onclick={() => removeBackup(b.key)} class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-destructive/15 hover:text-destructive">
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</section>
