<script lang="ts">
	import { onMount } from 'svelte';
	import { Bell, Database, Download, Plus, Trash2, Upload } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import { getSchemaExportURL } from '#lib/api/collections.ts';
		import BackupsCard from '#lib/components/settings/BackupsCard.svelte';
	import CronsCard from '#lib/components/settings/CronsCard.svelte';
	import DatabaseCard from '#lib/components/settings/DatabaseCard.svelte';
	import MailSettingsCard from '#lib/components/settings/MailSettingsCard.svelte';
	import RateLimitsCard from '#lib/components/settings/RateLimitsCard.svelte';
	import S3StorageCard from '#lib/components/settings/S3StorageCard.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface WebhookItem {
		id: number;
		provider: string;
		collection: string;
		event: string;
		url: string;
		has_token: boolean;
		enabled: boolean;
	}

	let health = $state<{ status?: string; cache_backend?: string; service?: string } | null>(null);

	// Redis settings state
	let redisUrl = $state('');
	let savingRedis = $state(false);

	// Webhooks / Notification hooks state
	let webhooks = $state<WebhookItem[]>([]);
	let whProvider = $state('discord');
	let whCol = $state('*');
	let whEvent = $state('create');
	let whUrl = $state('');
	let whToken = $state('');

	// Schema import state
	let importJSON = $state('');
	let importing = $state(false);

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

	let urlPlaceholder = $derived(
		whProvider === 'discord'
			? 'https://discord.com/api/webhooks/...'
			: whProvider === 'fcm'
				? 'https://fcm.googleapis.com/v1/projects/<project-id>/messages:send'
				: 'https://hooks.example.com/webhook'
	);

	async function loadWebhooks() {
		const res = await apiRequest<{ items: WebhookItem[] }>('/api/webhooks');
		webhooks = res.items ?? [];
	}

	onMount(async () => {
		try {
			const [h, r] = await Promise.all([
				apiRequest<{ status: string; cache_backend: string; service: string }>('/health'),
				apiRequest<{ redis_url: string; backend: string }>('/api/settings/redis')
			]);
			health = h;
			redisUrl = r.redis_url ?? '';
			await loadWebhooks();
		} catch (err) {
			toast.error(err);
		}
	});

	async function saveRedis(e: SubmitEvent) {
		e.preventDefault();
		savingRedis = true;
		try {
			const res = await apiRequest<{ redis_url: string; backend: string }>('/api/settings/redis', {
				method: 'PUT',
				body: JSON.stringify({ redis_url: redisUrl })
			});
			if (health) health = { ...health, cache_backend: res.backend };
			toast.success(`Cache switched to "${res.backend}"`);
		} catch (err) {
			toast.error(err, 'Failed to connect to Redis');
		} finally {
			savingRedis = false;
		}
	}

	async function addWebhook(e: SubmitEvent) {
		e.preventDefault();
		try {
			await apiRequest('/api/webhooks', {
				method: 'POST',
				body: JSON.stringify({
					provider: whProvider,
					collection: whCol,
					event: whEvent,
					url: whUrl,
					auth_token: whToken
				})
			});
			whUrl = '';
			whToken = '';
			toast.success('Notification hook added');
			await loadWebhooks();
		} catch (err) {
			toast.error(err);
		}
	}

	async function removeWebhook(id: number) {
		try {
			await apiRequest(`/api/webhooks/${id}`, { method: 'DELETE' });
			toast.success('Hook deleted');
			await loadWebhooks();
		} catch (err) {
			toast.error(err);
		}
	}

	async function handleImport() {
		if (!importJSON.trim()) return;
		importing = true;
		try {
			const parsed = JSON.parse(importJSON);
			const res = await apiRequest<{ created: string[]; skipped: string[] }>('/api/schema/import', {
				method: 'POST',
				body: JSON.stringify(parsed)
			});
			toast.success(`Imported: ${res.created.length} created, ${res.skipped.length} skipped`);
			importJSON = '';
		} catch (err) {
			toast.error(err, 'Invalid schema JSON');
		} finally {
			importing = false;
		}
	}
</script>

<main class="flex-1 overflow-y-auto bg-background p-8">
	<div class="mx-auto max-w-4xl space-y-6">
		<div>
			<h1 class="text-xl font-semibold">Settings</h1>
			<p class="mt-1 text-sm text-muted-foreground">Database connection, Redis cache, S3 storage, SMTP mail, backups, crons, rate limits, webhooks, and schema portability.</p>
		</div>

		<!-- System Status -->
		<section class="rounded-md border border-border bg-card p-5">
			<h2 class="mb-3 text-sm font-semibold">System Status</h2>
			<div class="grid gap-3 text-sm sm:grid-cols-3">
				<div class="rounded-md border border-border bg-muted/30 p-3.5">
					<div class="text-xs text-muted-foreground">Service</div>
					<div class="mt-1 font-mono font-semibold">{health?.service ?? 'microbase'}</div>
				</div>
				<div class="rounded-md border border-border bg-muted/30 p-3.5">
					<div class="text-xs text-muted-foreground">Status</div>
					<div class="mt-1 font-mono font-semibold text-emerald-600 dark:text-emerald-400">
						{health?.status ?? 'ok'}
					</div>
				</div>
				<div class="rounded-md border border-border bg-muted/30 p-3.5">
					<div class="text-xs text-muted-foreground">Active Cache Backend</div>
					<div class="mt-1 font-mono font-semibold">{health?.cache_backend ?? 'memory'}</div>
				</div>
			</div>
		</section>

		<!-- Redis Cache Settings (Hot-Swap) -->
		<section class="space-y-3 rounded-md border border-border bg-card p-5">
			<div class="flex items-center gap-2">
				<Database class="h-4 w-4 text-muted-foreground" />
				<div>
					<h2 class="text-sm font-semibold">Redis Cache URL</h2>
					<p class="text-xs text-muted-foreground">
						Enter a Redis URL to hot-swap cache &amp; rate limiting to Redis immediately, or leave empty to use In-Memory cache.
					</p>
				</div>
			</div>

			<form onsubmit={saveRedis} class="flex flex-wrap items-center gap-2.5">
				<input
					type="text"
					value={redisUrl}
					oninput={(e) => (redisUrl = e.currentTarget.value)}
					placeholder="redis://localhost:6379/0 (leave blank for In-Memory)"
					class="min-w-64 flex-1 rounded-md border border-input bg-background px-3.5 py-2 font-mono text-sm outline-none focus:border-ring"
				/>
				<button
					type="submit"
					disabled={savingRedis}
					class="cursor-pointer rounded-md bg-foreground px-4 py-2 text-xs font-medium text-background transition hover:opacity-90 disabled:opacity-50"
				>
					{savingRedis ? 'Connecting...' : 'Save & Connect'}
				</button>
			</form>
		</section>

		<!-- PostgreSQL Database Connection -->
		<DatabaseCard />

		<!-- Files Storage (S3 / R2 / MinIO) -->
		<S3StorageCard />

		<!-- Mail Settings (SMTP) -->
		<MailSettingsCard />

		<!-- Backups & Crons -->
		<BackupsCard />
		<CronsCard />

		<!-- Per-Route Rate Limits -->
		<RateLimitsCard />

		<!-- Notification Hooks (Discord / Firebase FCM / Generic HTTP) -->
		<section class="space-y-4 rounded-md border border-border bg-card p-5">
			<div class="flex items-center gap-2">
				<Bell class="h-4 w-4 text-muted-foreground" />
				<div>
					<h2 class="text-sm font-semibold">Notification Hooks (Discord / FCM / Webhook)</h2>
					<p class="text-xs text-muted-foreground">
						Automatically send Discord Embeds, Firebase FCM push notifications, or HTTP JSON webhooks on record events.
					</p>
				</div>
			</div>

			<form onsubmit={addWebhook} class="space-y-3">
				<div class="flex flex-wrap items-center gap-2.5">
					<Select bind:value={whProvider} options={providerOptions} />
					<Select bind:value={whEvent} options={eventOptions} />
					<input
						type="text"
						value={whCol}
						oninput={(e) => (whCol = e.currentTarget.value)}
						placeholder="collection or *"
						class="w-40 rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
					/>
				</div>

				<div class="grid gap-2.5 sm:grid-cols-12">
					<input
						type="url"
						required
						value={whUrl}
						oninput={(e) => (whUrl = e.currentTarget.value)}
						placeholder={urlPlaceholder}
						class="rounded-md border border-input bg-background px-3 py-2 text-xs outline-none focus:border-ring {whProvider === 'discord' ? 'sm:col-span-10' : 'sm:col-span-6'}"
					/>
					{#if whProvider !== 'discord'}
						<input
							type="password"
							value={whToken}
							oninput={(e) => (whToken = e.currentTarget.value)}
							placeholder={whProvider === 'fcm' ? 'FCM OAuth2 Bearer Token' : 'Optional Bearer Token'}
							class="rounded-md border border-input bg-background px-3 py-2 text-xs outline-none focus:border-ring sm:col-span-4"
						/>
					{/if}
					<button
						type="submit"
						class="inline-flex cursor-pointer items-center justify-center gap-1 rounded-md bg-foreground px-3 py-2 text-xs font-medium text-background hover:opacity-90 sm:col-span-2"
					>
						<Plus class="h-3.5 w-3.5" /> Add Hook
					</button>
				</div>
			</form>

			{#if webhooks.length > 0}
				<div class="divide-y divide-border rounded-md border border-border">
					{#each webhooks as w (w.id)}
						<div class="flex items-center justify-between gap-3 px-4 py-2.5 text-xs">
							<div class="flex min-w-0 items-center gap-2 font-mono">
								<span class="rounded bg-primary/15 px-2 py-0.5 font-semibold text-primary uppercase">
									{w.provider}
								</span>
								<span class="rounded bg-muted px-2 py-0.5">{w.collection}</span>
								<span class="rounded bg-emerald-500/15 px-2 py-0.5 text-emerald-600 dark:text-emerald-400">
									{w.event}
								</span>
								<span class="truncate text-muted-foreground">{w.url}</span>
							</div>
							<button
								type="button"
								onclick={() => removeWebhook(w.id)}
								class="cursor-pointer rounded p-1 text-muted-foreground hover:bg-destructive/15 hover:text-destructive"
							>
								<Trash2 class="h-3.5 w-3.5" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</section>

		<!-- Export / Import Schema -->
		<section class="space-y-4 rounded-md border border-border bg-card p-5">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-sm font-semibold">Export / Import Collections Schema</h2>
					<p class="text-xs text-muted-foreground">Move your table schemas between Dev and Production.</p>
				</div>
				<a
					href={getSchemaExportURL()}
					target="_blank"
					rel="noreferrer"
					class="inline-flex items-center gap-1.5 rounded-md border border-border bg-background px-3 py-2 text-xs font-medium hover:bg-accent"
				>
					<Download class="h-3.5 w-3.5" /> Export JSON
				</a>
			</div>

			<textarea
				rows="4"
				bind:value={importJSON}
				placeholder={'Paste Schema Export JSON: {"version":"1.0","collections":[...]}'}
				class="w-full rounded-md border border-input bg-background p-3 font-mono text-xs outline-none focus:border-ring"
			></textarea>

			<div class="flex justify-end">
				<button
					type="button"
					disabled={importing || !importJSON.trim()}
					onclick={handleImport}
					class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-2 text-xs font-medium text-background transition hover:opacity-90 disabled:opacity-40"
				>
					<Upload class="h-3.5 w-3.5" />
					{importing ? 'Importing...' : 'Import Schema'}
				</button>
			</div>
		</section>
	</div>
</main>