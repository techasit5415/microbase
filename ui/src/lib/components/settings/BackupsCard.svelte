<script lang="ts">
	import { onMount } from 'svelte';
	import { Archive, Download, Plus, RotateCcw, Trash2, Upload } from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY, apiRequest } from '#lib/api/client.ts';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface BackupItem {
		key: string;
		size: number;
		modified: string;
	}

	let backups = $state<BackupItem[]>([]);
	let creating = $state(false);
	let restoringKey = $state<string | null>(null);
	let downloadingKey = $state<string | null>(null);

	async function downloadBackup(key: string) {
		downloadingKey = key;
		try {
			const token = localStorage.getItem(ADMIN_TOKEN_KEY);
			const res = await fetch(`/api/backups/${encodeURIComponent(key)}`, {
				headers: token ? { Authorization: `Bearer ${token}` } : {}
			});
			if (!res.ok) {
				const json = await res.json().catch(() => null);
				throw new Error(json?.message || json?.error?.message || `Download failed (${res.status})`);
			}
			const blob = await res.blob();
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = key;
			document.body.appendChild(a);
			a.click();
			a.remove();
			URL.revokeObjectURL(url);
		} catch (err) {
			toast.error(err);
		} finally {
			downloadingKey = null;
		}
	}

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

	async function uploadBackup(file: File | undefined) {
		if (!file) return;
		try {
			const fd = new FormData();
			fd.append('file', file);
			const token = localStorage.getItem(ADMIN_TOKEN_KEY);
			const res = await fetch('/api/backups/upload', {
				method: 'POST',
				headers: token ? { Authorization: `Bearer ${token}` } : {},
				body: fd
			});
			const json = await res.json();
			if (!res.ok) throw new Error(json?.error?.message || 'Upload failed');
			toast.success(`Uploaded backup "${file.name}"`);
			await loadBackups();
		} catch (err) {
			toast.error(err);
		}
	}

	async function restoreBackup(key: string) {
		const ok = await confirmAction({
			title: `Restore "${key}"?`,
			description: 'This will execute the SQL backup against your PostgreSQL database. Make sure you have a fresh backup first.',
			confirmLabel: 'Restore backup',
			destructive: true
		});
		if (!ok) return;
		restoringKey = key;
		try {
			await apiRequest(`/api/backups/${encodeURIComponent(key)}/restore`, { method: 'POST' });
			toast.success(`Restored backup "${key}"`);
		} catch (err) {
			toast.error(err);
		} finally {
			restoringKey = null;
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
	<div class="flex flex-wrap items-center justify-between gap-3">
		<div>
			<h2 class="flex items-center gap-2 text-sm font-semibold">
				<Archive class="h-4 w-4 text-muted-foreground" />
				Backups &amp; Restore (PostgreSQL)
			</h2>
			<p class="mt-0.5 text-xs text-muted-foreground">Create, upload, download, or restore full SQL snapshots.</p>
		</div>
		<div class="flex items-center gap-2">
			<label class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-3 py-2 text-xs font-medium hover:bg-muted">
				<Upload class="h-3.5 w-3.5" />
				Upload .sql
				<input type="file" accept=".sql" class="hidden" onchange={(e) => uploadBackup(e.currentTarget.files?.[0])} />
			</label>
			<button type="button" disabled={creating} onclick={createBackup} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-3.5 py-2 text-xs font-semibold text-background hover:opacity-90 disabled:opacity-50">
				<Plus class="h-3.5 w-3.5" />
				{creating ? 'Creating...' : 'Create backup'}
			</button>
		</div>
	</div>

	{#if backups.length === 0}
		<p class="rounded-md border border-dashed border-border py-5 text-center text-xs text-muted-foreground">No backups yet.</p>
	{:else}
		<div class="divide-y divide-border overflow-hidden rounded-md border border-border">
			{#each backups as b (b.key)}
				<div class="flex flex-wrap items-center justify-between gap-2 bg-background/50 px-4 py-2.5 text-xs">
					<div>
						<div class="font-mono font-medium">{b.key}</div>
						<div class="text-muted-foreground">{(b.size / 1024).toFixed(1)} KB &middot; {new Date(b.modified).toLocaleString()}</div>
					</div>
					<div class="flex items-center gap-1.5">
						<button type="button" disabled={downloadingKey === b.key} onclick={() => downloadBackup(b.key)} class="inline-flex cursor-pointer items-center gap-1 rounded border border-border bg-card px-2.5 py-1 text-xs font-medium hover:bg-muted disabled:opacity-50">
							<Download class="h-3.5 w-3.5" /> {downloadingKey === b.key ? 'Downloading...' : 'Download'}
						</button>
						<button type="button" disabled={restoringKey === b.key} onclick={() => restoreBackup(b.key)} class="inline-flex cursor-pointer items-center gap-1 rounded border border-border bg-card px-2.5 py-1 text-xs font-medium hover:bg-muted disabled:opacity-50">
							<RotateCcw class="h-3.5 w-3.5" /> {restoringKey === b.key ? 'Restoring...' : 'Restore'}
						</button>
						<button type="button" onclick={() => removeBackup(b.key)} class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-destructive/15 hover:text-destructive">
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</section>
