<script lang="ts">
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import {
		Archive,
		Bell,
		Clock,
		Database,
		Download,
		HardDrive,
		Home,
		Mail,
		Play,
		Plus,
		RefreshCw,
		ShieldCheck,
		Terminal,
		Trash2,
		Upload
	} from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import ApplicationCard from '#lib/components/settings/ApplicationCard.svelte';
	import BackupsCard from '#lib/components/settings/BackupsCard.svelte';
	import CronsCard from '#lib/components/settings/CronsCard.svelte';
	import DatabaseCard from '#lib/components/settings/DatabaseCard.svelte';
	import MailSettingsCard from '#lib/components/settings/MailSettingsCard.svelte';
	import S3StorageCard from '#lib/components/settings/S3StorageCard.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface HealthStatus {
		status: string;
		service: string;
		version?: string;
		cache_backend: string;
	}

	interface WebhookRow {
		id: number;
		provider: 'generic' | 'discord' | 'fcm';
		collection: string;
		event: string;
		url: string;
		has_token?: boolean;
		has_secret?: boolean;
		headers?: Record<string, string>;
		max_retries?: number;
		enabled: boolean;
		last_run_at?: string;
		last_status?: string;
	}

	type SectionKey =
		| 'application'
		| 'database'
		| 'mail'
		| 'storage'
		| 'backups'
		| 'crons'
		| 'webhooks'
		| 'export'
		| 'import';

	let activeSection = $state<SectionKey>('application');
	let health = $state<HealthStatus | null>(null);
	let redisUrl = $state('');
	let savingRedis = $state(false);
	let webhooks = $state<WebhookRow[]>([]);
	let hookProvider = $state<'discord' | 'fcm' | 'generic'>('discord');
	let hookSecret = $state('');
	let hookHeadersJson = $state('');
	let hookMaxRetries = $state(3);
	let testingHookId = $state<number | null>(null);
	let hookCol = $state('*');
	let hookEvent = $state('create');
	let hookUrl = $state('');
	let hookAuthToken = $state('');
	let importJson = $state('');
	let exportedJson = $state('');
	let importing = $state(false);

	const sectionMeta: Record<SectionKey, { title: string; group: string }> = {
		application: { title: 'Application', group: 'System' },
		database: { title: 'Database & Cache', group: 'System' },
		mail: { title: 'Mail settings', group: 'System' },
		storage: { title: 'Files storage', group: 'System' },
		backups: { title: 'Backups', group: 'System' },
		crons: { title: 'Crons', group: 'System' },
		webhooks: { title: 'Webhooks', group: 'System' },
		export: { title: 'Export collections', group: 'Sync' },
		import: { title: 'Import collections', group: 'Sync' }
	};

	const providerOptions = [
		{ value: 'discord', label: 'Discord Webhook' },
		{ value: 'fcm', label: 'Firebase FCM' },
		{ value: 'generic', label: 'Generic HTTP' }
	];

	const eventOptions = [
		{ value: 'create', label: 'create' },
		{ value: 'update', label: 'update' },
		{ value: 'delete', label: 'delete' },
		{ value: '*', label: '* (all)' }
	];

	async function loadAll() {
		try {
			const res = await fetch('/health');
			const json = await res.json();
			health = json.data ?? json;
		} catch {
			health = null;
		}
		try {
			const r = await apiRequest<{ redis_url: string }>('/api/settings/redis');
			redisUrl = r.redis_url ?? '';
		} catch {
			// ignore
		}
		try {
			const w = await apiRequest<{ items: WebhookRow[] }>('/api/webhooks');
			webhooks = w.items ?? [];
		} catch {
			webhooks = [];
		}
	}

	onMount(loadAll);

	async function saveRedis(e: SubmitEvent) {
		e.preventDefault();
		savingRedis = true;
		try {
			await apiRequest('/api/settings/redis', {
				method: 'PUT',
				body: JSON.stringify({ redis_url: redisUrl.trim() })
			});
			toast.success(redisUrl.trim() ? 'Connected to Redis cache!' : 'Switched to In-Memory cache.');
			await loadAll();
		} catch (err) {
			toast.error(err);
		} finally {
			savingRedis = false;
		}
	}

	async function addWebhook(e: SubmitEvent) {
		e.preventDefault();
		if (!hookUrl.trim()) return;
		let headers: Record<string, string> = {};
		if (hookHeadersJson.trim()) {
			try {
				headers = JSON.parse(hookHeadersJson);
			} catch {
				toast.error('Custom headers must be valid JSON (e.g. {"X-API-Key": "123"})');
				return;
			}
		}
		try {
			await apiRequest('/api/webhooks', {
				method: 'POST',
				body: JSON.stringify({
					provider: hookProvider,
					collection: hookCol.trim() || '*',
					event: hookEvent,
					url: hookUrl.trim(),
					auth_token: hookAuthToken.trim(),
					secret: hookSecret.trim(),
					headers,
					max_retries: Number(hookMaxRetries) || 3
				})
			});
			hookUrl = '';
			hookAuthToken = '';
			hookSecret = '';
			hookHeadersJson = '';
			toast.success('Notification hook added');
			await loadAll();
		} catch (err) {
			toast.error(err);
		}
	}

	async function toggleWebhook(w: WebhookRow, next: boolean) {
		try {
			await apiRequest(`/api/webhooks/${w.id}`, {
				method: 'PATCH',
				body: JSON.stringify({ enabled: next })
			});
			w.enabled = next;
			toast.success(`Webhook #${w.id} ${next ? 'enabled' : 'paused'}`);
		} catch (err) {
			toast.error(err);
		}
	}

	async function testWebhook(id: number) {
		testingHookId = id;
		try {
			const res = await apiRequest<{ status: string }>(`/api/webhooks/${id}/test`, { method: 'POST' });
			toast.success(`Test sent: ${res.status}`);
			await loadAll();
		} catch (err) {
			toast.error(err, 'Webhook test failed');
			await loadAll();
		} finally {
			testingHookId = null;
		}
	}

	async function removeWebhook(id: number) {
		const ok = await confirmAction({
			title: 'Delete notification hook?',
			description: 'This webhook will stop firing on collection events.',
			confirmLabel: 'Delete',
			destructive: true
		});
		if (!ok) return;
		try {
			await apiRequest(`/api/webhooks/${id}`, { method: 'DELETE' });
			toast.success('Hook removed');
			await loadAll();
		} catch (err) {
			toast.error(err);
		}
	}

	async function handleExport() {
		try {
			const data = await apiRequest<unknown>('/api/schema/export');
			exportedJson = JSON.stringify(data, null, 2);
			const blob = new Blob([exportedJson], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `microbase-schema-${new Date().toISOString().slice(0, 10)}.json`;
			a.click();
			URL.revokeObjectURL(url);
			toast.success('Exported schema JSON');
		} catch (err) {
			toast.error(err);
		}
	}

	let importFileInput = $state<HTMLInputElement | null>(null);

	async function handleImportFileSelect(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		try {
			const text = await file.text();
			JSON.parse(text); // Validate JSON syntax immediately
			importJson = text;
			toast.success(`Loaded "${file.name}"`);
		} catch (err) {
			toast.error(err, `Invalid JSON file "${file.name}"`);
		} finally {
			input.value = '';
		}
	}

	async function handleImport() {
		if (!importJson.trim()) return;
		importing = true;
		try {
			const parsed = JSON.parse(importJson);
			const res = await apiRequest<{ created: string[]; skipped: string[] }>('/api/schema/import', {
				method: 'POST',
				body: JSON.stringify(parsed)
			});
			toast.success(`Imported: ${res.created?.length ?? 0} created, ${res.skipped?.length ?? 0} skipped`);
			importJson = '';
		} catch (err) {
			toast.error(err, 'Invalid schema JSON');
		} finally {
			importing = false;
		}
	}
</script>

<div class="flex min-h-0 flex-1 overflow-hidden bg-background">
	<!-- PocketBase-style Settings Left Sub-Sidebar -->
	<aside class="flex w-60 shrink-0 flex-col overflow-y-auto border-r border-border bg-card px-3 py-5">
		<!-- System group -->
		<div class="mb-2 px-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">System</div>
		<nav class="space-y-0.5">
			{#each [
				{ id: 'application', label: 'Application', icon: Home },
				{ id: 'database', label: 'Database & Cache', icon: Database },
				{ id: 'mail', label: 'Mail settings', icon: Mail },
				{ id: 'storage', label: 'Files storage', icon: HardDrive },
				{ id: 'backups', label: 'Backups', icon: Archive },
				{ id: 'crons', label: 'Crons', icon: Clock },
				{ id: 'webhooks', label: 'Webhooks', icon: Bell }
			] as item (item.id)}
				<button
					type="button"
					onclick={() => (activeSection = item.id as SectionKey)}
					class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-3 py-2 text-left text-[length:var(--ui-sidebar-size)] transition {activeSection === item.id
						? 'bg-accent font-medium text-foreground'
						: 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'}"
				>
					<item.icon class="h-4 w-4 shrink-0" />
					<span>{item.label}</span>
				</button>
			{/each}
		</nav>

		<!-- Sync group -->
		<div class="mt-6 mb-2 px-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">Sync</div>
		<nav class="space-y-0.5">
			<button
				type="button"
				onclick={() => (activeSection = 'export')}
				class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-3 py-2 text-left text-[length:var(--ui-sidebar-size)] transition {activeSection === 'export'
					? 'bg-accent font-medium text-foreground'
					: 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'}"
			>
				<Download class="h-4 w-4 shrink-0" />
				<span>Export collections</span>
			</button>
			<button
				type="button"
				onclick={() => (activeSection = 'import')}
				class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-3 py-2 text-left text-[length:var(--ui-sidebar-size)] transition {activeSection === 'import'
					? 'bg-accent font-medium text-foreground'
					: 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'}"
			>
				<Upload class="h-4 w-4 shrink-0" />
				<span>Import collections</span>
			</button>
		</nav>

		<!-- Debug group -->
		<div class="mt-6 mb-2 px-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">Debug</div>
		<nav class="space-y-0.5">
			<a
				href="/_/sql"
				class="flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-left text-[length:var(--ui-sidebar-size)] text-muted-foreground transition hover:bg-accent/60 hover:text-foreground"
			>
				<Terminal class="h-4 w-4 shrink-0" />
				<span>SQL console</span>
			</a>
		</nav>
	</aside>

	<!-- Right Content Workspace -->
	<main class="flex-1 overflow-y-auto px-8 py-6">
		<div class="mx-auto max-w-4xl">
			<!-- PocketBase Breadcrumb Header -->
			<div class="mb-6 flex items-center gap-2 text-base">
				<span class="text-muted-foreground">Settings</span>
				<span class="text-muted-foreground/50">/</span>
				<h1 class="font-semibold text-foreground">{sectionMeta[activeSection].title}</h1>
			</div>

			{#key activeSection}
				<div in:fade={{ duration: 140 }} class="space-y-6">
					{#if activeSection === 'application'}
						<ApplicationCard />
					{:else if activeSection === 'database'}
						<!-- System Status -->
						<div class="rounded-md border border-border bg-card p-5">
							<h2 class="mb-3 text-sm font-semibold">System Status</h2>
							<div class="grid grid-cols-1 gap-3 sm:grid-cols-4">
								<div class="rounded-md border border-border bg-muted/30 px-3.5 py-2.5">
									<div class="text-xs text-muted-foreground">Service</div>
									<div class="mt-0.5 font-mono text-sm font-semibold">{health?.service ?? 'microbase'}</div>
								</div>
								<div class="rounded-md border border-border bg-muted/30 px-3.5 py-2.5">
									<div class="text-xs text-muted-foreground">Version</div>
									<div class="mt-0.5 font-mono text-sm font-semibold">{health?.version ?? 'dev'}</div>
								</div>
								<div class="rounded-md border border-border bg-muted/30 px-3.5 py-2.5">
									<div class="text-xs text-muted-foreground">Status</div>
									<div class="mt-0.5 font-mono text-sm font-semibold text-emerald-600">{health?.status ?? 'offline'}</div>
								</div>
								<div class="rounded-md border border-border bg-muted/30 px-3.5 py-2.5">
									<div class="text-xs text-muted-foreground">Cache Backend</div>
									<div class="mt-0.5 font-mono text-sm font-semibold">{health?.cache_backend ?? 'memory'}</div>
								</div>
							</div>
						</div>

						<DatabaseCard />

						<!-- Redis Cache URL -->
						<form onsubmit={saveRedis} class="rounded-md border border-border bg-card p-5">
							<div class="mb-3 flex items-center gap-2">
								<Database class="h-4 w-4 text-muted-foreground" />
								<h2 class="text-sm font-semibold">Redis Cache URL</h2>
							</div>
							<p class="mb-3 text-xs text-muted-foreground">
								Enter a Redis URL to hot-swap cache & rate limiting to Redis immediately, or leave empty to use In-Memory cache.
							</p>
							<div class="flex flex-col gap-2.5 sm:flex-row">
								<input
									type="text"
									bind:value={redisUrl}
									placeholder="redis://localhost:6379/0 (leave blank for In-Memory)"
									class="min-w-0 flex-1 rounded-md border border-input bg-background px-3.5 py-2 font-mono text-xs outline-none focus:border-ring"
								/>
								<button
									type="submit"
									disabled={savingRedis}
									class="cursor-pointer rounded-md bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground transition hover:opacity-90 disabled:opacity-50"
								>
									{savingRedis ? 'Connecting...' : 'Save & Connect'}
								</button>
							</div>
						</form>
					{:else if activeSection === 'mail'}
						<MailSettingsCard />
					{:else if activeSection === 'storage'}
						<S3StorageCard />
					{:else if activeSection === 'backups'}
						<BackupsCard />
					{:else if activeSection === 'crons'}
						<CronsCard />
					{:else if activeSection === 'webhooks'}
						<div class="rounded-md border border-border bg-card p-5">
							<h2 class="mb-1 text-sm font-semibold">Notification Hooks (Discord / FCM / Webhook)</h2>
							<p class="mb-4 text-xs text-muted-foreground">
								Automatically send Discord Embeds, Firebase FCM push notifications, or HTTP JSON webhooks on record events.
							</p>
							<form onsubmit={addWebhook} class="space-y-2.5">
								<div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
									<Select bind:value={hookProvider} options={providerOptions} />
									<Select bind:value={hookEvent} options={eventOptions} />
									<input
										type="text"
										bind:value={hookCol}
										placeholder="Collection (* for all)"
										class="rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
									/>
								</div>
								<div class="flex flex-col gap-2 sm:flex-row">
									<input
										type="url"
										required
										bind:value={hookUrl}
										placeholder="https://discord.com/api/webhooks/..."
										class="min-w-0 flex-1 rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring"
									/>
									<button
										type="submit"
										class="inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-md bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground transition hover:opacity-90"
									>
										<Plus class="h-3.5 w-3.5" /> Add Hook
									</button>
								</div>
								<div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
									<input
										type="password"
										bind:value={hookSecret}
										placeholder="Signing Secret (HMAC-SHA256, optional)"
										class="rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
									/>
									<input
										type="text"
										bind:value={hookHeadersJson}
										placeholder={'Custom Headers JSON (e.g. {"X-Key":"abc"})'}
										class="rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
									/>
									<div class="flex items-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-xs">
										<span class="text-muted-foreground">Retries:</span>
										<input
											type="number"
											min="1"
											max="5"
											bind:value={hookMaxRetries}
											class="w-12 bg-transparent font-mono outline-none"
										/>
									</div>
								</div>
								{#if hookProvider === 'fcm'}
									<input
										type="password"
										bind:value={hookAuthToken}
										placeholder="Bearer OAuth2 / Server Key for Firebase FCM"
										class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
									/>
								{/if}
							</form>

							{#if webhooks.length > 0}
								<div class="mt-4 divide-y divide-border rounded-md border border-border">
									{#each webhooks as w (w.id)}
										<div class="flex flex-wrap items-center justify-between gap-3 px-3.5 py-3 text-xs">
											<div class="min-w-0 flex-1 space-y-1">
												<div class="flex flex-wrap items-center gap-1.5">
													<span class="rounded bg-muted px-1.5 py-0.5 font-mono font-semibold">{w.provider}</span>
													<span class="font-mono text-muted-foreground">{w.collection}:{w.event}</span>
													{#if w.has_secret}
														<span class="inline-flex items-center gap-0.5 rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400">
															<ShieldCheck class="h-3 w-3" /> HMAC
														</span>
													{/if}
													<span class="rounded bg-muted/70 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">retry {w.max_retries ?? 3}x</span>
												</div>
												<div class="truncate font-mono text-foreground">{w.url}</div>
												{#if w.last_status}
													<div class="flex items-center gap-2 text-[11px]">
														<span
															class="rounded px-1.5 py-0.5 font-mono {w.last_status.startsWith('error')
																? 'bg-destructive/15 text-destructive'
																: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}"
														>
															{w.last_status}
														</span>
													</div>
												{/if}
											</div>
											<div class="flex items-center gap-2">
												<Switch checked={w.enabled} onChange={(v) => toggleWebhook(w, v)} />
												<button
													type="button"
													disabled={testingHookId === w.id}
													onclick={() => testWebhook(w.id)}
													class="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium transition hover:bg-muted disabled:opacity-50"
												>
													{#if testingHookId === w.id}
														<RefreshCw class="h-3 w-3 animate-spin" />
													{:else}
														<Play class="h-3 w-3" />
													{/if}
													Test
												</button>
												<button
													type="button"
													onclick={() => removeWebhook(w.id)}
													class="cursor-pointer rounded p-1 text-muted-foreground hover:text-destructive"
												>
													<Trash2 class="h-3.5 w-3.5" />
												</button>
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{:else if activeSection === 'export'}
						<div class="rounded-md border border-border bg-card p-5">
							<div class="mb-3 flex items-center justify-between">
								<div>
									<h2 class="text-sm font-semibold">Export Collections Schema</h2>
									<p class="mt-0.5 text-xs text-muted-foreground">Download all collection schemas, fields, indexes, and API rules as JSON.</p>
								</div>
								<button
									type="button"
									onclick={handleExport}
									class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground transition hover:opacity-90"
								>
									<Download class="h-3.5 w-3.5" /> Download JSON
								</button>
							</div>
							{#if exportedJson}
								<pre class="max-h-96 overflow-auto rounded-md bg-muted/70 p-4 font-mono text-xs">{exportedJson}</pre>
							{/if}
						</div>
					{:else if activeSection === 'import'}
						<div class="rounded-md border border-border bg-card p-5">
							<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
								<div>
									<h2 class="text-sm font-semibold">Import Collections Schema</h2>
									<p class="mt-0.5 text-xs text-muted-foreground">Upload or paste a Microbase schema JSON or PocketBase <code class="rounded bg-muted px-1">pb_schema.json</code>.</p>
								</div>
								<input
									bind:this={importFileInput}
									type="file"
									accept=".json,application/json"
									onchange={handleImportFileSelect}
									class="hidden"
								/>
								<button
									type="button"
									onclick={() => importFileInput?.click()}
									class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-3.5 py-2 text-xs font-medium text-foreground transition hover:bg-muted"
								>
									<Upload class="h-3.5 w-3.5" /> Load from JSON file
								</button>
							</div>
							<textarea
								rows="10"
								bind:value={importJson}
								placeholder={'{"version":"1.0","collections":[...]} or PocketBase [...]'}
								class="w-full rounded-md border border-input bg-background p-3 font-mono text-xs outline-none focus:border-ring"
							></textarea>
							<div class="mt-3 flex justify-end">
								<button
									type="button"
									disabled={importing || !importJson.trim()}
									onclick={handleImport}
									class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground transition hover:opacity-90 disabled:opacity-50"
								>
									<Upload class="h-3.5 w-3.5" /> {importing ? 'Importing...' : 'Import Schema'}
								</button>
							</div>
						</div>
					{/if}
				</div>
			{/key}
		</div>
	</main>
</div>
