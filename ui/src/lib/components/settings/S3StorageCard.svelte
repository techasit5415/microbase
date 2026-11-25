<script lang="ts">
	import { onMount } from 'svelte';
	import { Cloud, Save } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface S3Response {
		enabled: boolean;
		endpoint: string;
		bucket: string;
		region: string;
		access_key: string;
		has_secret: boolean;
		force_path_style: boolean;
		backend: string;
	}

	let enabled = $state(false);
	let endpoint = $state('');
	let bucket = $state('');
	let region = $state('auto');
	let accessKey = $state('');
	let secretKey = $state('');
	let hasSecret = $state(false);
	let forcePathStyle = $state(true);
	let backend = $state('local');
	let saving = $state(false);

	onMount(async () => {
		try {
			const res = await apiRequest<S3Response>('/api/settings/s3');
			enabled = Boolean(res.enabled);
			endpoint = res.endpoint ?? '';
			bucket = res.bucket ?? '';
			region = res.region || 'auto';
			accessKey = res.access_key ?? '';
			hasSecret = Boolean(res.has_secret);
			forcePathStyle = Boolean(res.force_path_style);
			backend = res.backend || 'local';
		} catch {
			// Ignore on initial load
		}
	});

	async function handleSave() {
		if (enabled) {
			if (!endpoint.trim()) return toast.error('S3 Endpoint URL is required.');
			if (!bucket.trim()) return toast.error('S3 Bucket name is required.');
			if (!accessKey.trim()) return toast.error('S3 Access Key is required.');
			if (!secretKey.trim() && !hasSecret) return toast.error('S3 Secret Key is required.');
		}
		saving = true;
		try {
			const res = await apiRequest<S3Response>('/api/settings/s3', {
				method: 'PUT',
				body: JSON.stringify({
					enabled,
					endpoint: endpoint.trim(),
					bucket: bucket.trim(),
					region: region.trim() || 'auto',
					access_key: accessKey.trim(),
					secret_key: secretKey.trim(),
					force_path_style: forcePathStyle
				})
			});
			backend = res.backend;
			hasSecret = res.has_secret;
			secretKey = '';
			toast.success(
				res.backend === 's3'
					? `Connected to S3 bucket "${res.bucket}"`
					: 'Switched file storage back to local disk'
			);
		} catch (err) {
			toast.error(err);
		} finally {
			saving = false;
		}
	}
</script>

<section class="space-y-4 rounded-lg border border-border bg-card p-5">
	<div class="flex items-center justify-between gap-3">
		<div>
			<h2 class="flex items-center gap-2 text-sm font-semibold">
				<Cloud class="h-4 w-4 text-muted-foreground" />
				Files Storage (S3 / R2 / MinIO)
			</h2>
			<p class="mt-0.5 text-xs text-muted-foreground">
				Use local filesystem by default or connect an S3-compatible bucket (hot-swappable without restart).
			</p>
		</div>
		<div class="flex items-center gap-2.5">
			<span class="rounded border border-border bg-muted px-2 py-0.5 font-mono text-xs uppercase">
				{backend}
			</span>
			<Switch checked={enabled} onChange={(v) => (enabled = v)} />
		</div>
	</div>

	{#if enabled}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
			<div class="sm:col-span-2">
				<label for="s3-endpoint" class="mb-1 block text-xs font-medium">Endpoint URL</label>
				<input
					id="s3-endpoint"
					type="text"
					value={endpoint}
					oninput={(e) => (endpoint = e.currentTarget.value)}
					placeholder="https://s3.ap-southeast-1.amazonaws.com or https://<account>.r2.cloudflarestorage.com"
					class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring"
				/>
			</div>
			<div>
				<label for="s3-bucket" class="mb-1 block text-xs font-medium">Bucket</label>
				<input
					id="s3-bucket"
					type="text"
					value={bucket}
					oninput={(e) => (bucket = e.currentTarget.value)}
					placeholder="microbase-files"
					class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring"
				/>
			</div>
			<div>
				<label for="s3-region" class="mb-1 block text-xs font-medium">Region</label>
				<input
					id="s3-region"
					type="text"
					value={region}
					oninput={(e) => (region = e.currentTarget.value)}
					placeholder="ap-southeast-1 or auto"
					class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring"
				/>
			</div>
			<div>
				<label for="s3-access" class="mb-1 block text-xs font-medium">Access Key</label>
				<input
					id="s3-access"
					type="text"
					value={accessKey}
					oninput={(e) => (accessKey = e.currentTarget.value)}
					placeholder="AKIA..."
					class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring"
				/>
			</div>
			<div>
				<label for="s3-secret" class="mb-1 block text-xs font-medium">
					Secret Key {#if hasSecret}<span class="text-muted-foreground">(saved)</span>{/if}
				</label>
				<input
					id="s3-secret"
					type="password"
					value={secretKey}
					oninput={(e) => (secretKey = e.currentTarget.value)}
					placeholder={hasSecret ? 'Leave blank to keep existing secret' : 'Enter S3 secret key'}
					class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring"
				/>
			</div>
		</div>

		<div class="flex items-center justify-between pt-1">
			<div class="flex items-center gap-2">
				<Switch checked={forcePathStyle} onChange={(v) => (forcePathStyle = v)} />
				<span class="text-xs text-muted-foreground">Force path-style addressing (recommended for MinIO / R2)</span>
			</div>
		</div>
	{/if}

	<div class="flex justify-end border-t border-border pt-3">
		<button
			type="button"
			disabled={saving}
			onclick={handleSave}
			class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-2 text-xs font-medium text-background transition hover:opacity-90 disabled:opacity-50"
		>
			<Save class="h-3.5 w-3.5" />
			{saving ? 'Testing & Saving...' : 'Save S3 Settings'}
		</button>
	</div>
</section>
