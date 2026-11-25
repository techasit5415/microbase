<script lang="ts">
	import { onMount } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { slide } from 'svelte/transition';
	import { Activity, ArrowUpDown, ChevronDown, Layers, ShieldCheck } from '@lucide/svelte';
	import RateLimitsCard from '#lib/components/settings/RateLimitsCard.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { appSettings, type AppSettingsData } from '#lib/stores/appSettings.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	let form = $state<AppSettingsData>(JSON.parse(JSON.stringify(appSettings.data)));
	let proxyHeadersText = $state('X-Forwarded-For, X-Real-IP');
	let superuserIPsText = $state('127.0.0.1, ::1');
	let openAccordion = $state<'batch' | 'proxy' | 'ratelimit' | 'ips' | null>(null);
	let saving = $state(false);

	const accentPresets = ['#25272d', '#1856c9', '#059669', '#7c3aed', '#dc2626', '#d97706'];

	function syncFromStore() {
		form = JSON.parse(JSON.stringify(appSettings.data));
		proxyHeadersText = (form.trusted_proxy.headers ?? []).join(', ');
		superuserIPsText = (form.superuser_ips.allowed_ips ?? []).join(', ');
	}

	onMount(async () => {
		if (!appSettings.loaded) await appSettings.load();
		syncFromStore();
	});

	function toggleAccordion(key: 'batch' | 'proxy' | 'ratelimit' | 'ips') {
		openAccordion = openAccordion === key ? null : key;
	}

	async function handleSave(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		try {
			form.trusted_proxy.headers = proxyHeadersText
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
			form.superuser_ips.allowed_ips = superuserIPsText
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
			await appSettings.save(form);
			toast.success('Application settings saved');
		} catch (err) {
			toast.error(err, 'Failed to save application settings');
		} finally {
			saving = false;
		}
	}
</script>

<form onsubmit={handleSave} class="space-y-6">
	<!-- Top Row: Application name, Application URL, Accent color (PocketBase style) -->
	<div class="grid grid-cols-1 gap-4 md:grid-cols-12">
		<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5 transition focus-within:bg-secondary focus-within:ring-2 focus-within:ring-foreground/15 md:col-span-5">
			<label for="app-name" class="mb-1 block text-xs font-medium text-muted-foreground">
				Application name <span class="text-destructive">*</span>
			</label>
			<input
				id="app-name"
				type="text"
				required
				bind:value={form.app_name}
				placeholder="Microbase"
				class="w-full bg-transparent text-sm text-foreground outline-none"
			/>
		</div>

		<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5 transition focus-within:bg-secondary focus-within:ring-2 focus-within:ring-foreground/15 md:col-span-5">
			<label for="app-url" class="mb-1 block text-xs font-medium text-muted-foreground">
				Application URL <span class="text-destructive">*</span>
			</label>
			<input
				id="app-url"
				type="url"
				required
				bind:value={form.app_url}
				placeholder="https://api.example.com"
				class="w-full bg-transparent font-mono text-sm text-foreground outline-none"
			/>
		</div>

		<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5 md:col-span-2">
			<label for="app-accent" class="mb-1 block text-xs font-medium text-muted-foreground">Accent</label>
			<div class="flex items-center gap-2">
				<label
					class="relative flex h-7 flex-1 cursor-pointer items-center justify-center rounded font-mono text-[11px] font-semibold text-white shadow-xs transition hover:opacity-90"
					style="background-color: {form.accent_color};"
				>
					<span>{form.accent_color}</span>
					<input
						id="app-accent"
						type="color"
						bind:value={form.accent_color}
						class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
					/>
				</label>
			</div>
			<div class="mt-2 flex items-center gap-1">
				{#each accentPresets as c (c)}
					<button
						type="button"
						aria-label="Set accent {c}"
						onclick={() => (form.accent_color = c)}
						class="h-4 w-4 cursor-pointer rounded-full border border-white/40 transition hover:scale-110 {form.accent_color === c ? 'ring-2 ring-foreground ring-offset-1' : ''}"
						style="background-color: {c};"
					></button>
				{/each}
			</div>
		</div>
	</div>

	<!-- PocketBase Stacked Accordions -->
	<div class="divide-y divide-border overflow-hidden rounded-md border border-border bg-card">
		<!-- 1. Batch Web API -->
		<div>
			<button
				type="button"
				onclick={() => toggleAccordion('batch')}
				class="flex w-full cursor-pointer items-center justify-between px-4 py-3.5 text-left transition hover:bg-muted/40"
			>
				<span class="flex items-center gap-2.5 text-sm font-medium text-foreground">
					<Layers class="h-4 w-4 text-muted-foreground" />
					Batch Web API
				</span>
				<span class="flex items-center gap-2.5">
					<span
						class="rounded px-2 py-0.5 text-xs font-medium {form.batch.enabled
							? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
							: 'bg-muted text-muted-foreground'}"
					>
						{form.batch.enabled ? 'Enabled' : 'Disabled'}
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openAccordion === 'batch' ? 'rotate-180' : ''}" />
				</span>
			</button>
			{#if openAccordion === 'batch'}
				<div transition:slide={{ duration: 180, easing: cubicOut }} class="space-y-3 border-t border-border bg-muted/20 p-4">
					<Switch
						checked={form.batch.enabled}
						onChange={(v) => (form.batch.enabled = v)}
						label="Enable POST /api/batch endpoint"
					/>
					<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
						<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
							<label for="batch-max" class="mb-0.5 block text-xs text-muted-foreground">Max requests per batch</label>
							<input id="batch-max" type="number" min="1" max="500" bind:value={form.batch.max_requests} class="w-full bg-transparent font-mono text-sm outline-none" />
						</div>
						<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
							<label for="batch-timeout" class="mb-0.5 block text-xs text-muted-foreground">Max processing timeout (sec)</label>
							<input id="batch-timeout" type="number" min="1" max="120" bind:value={form.batch.timeout} class="w-full bg-transparent font-mono text-sm outline-none" />
						</div>
					</div>
				</div>
			{/if}
		</div>

		<!-- 2. IP proxy headers -->
		<div>
			<button
				type="button"
				onclick={() => toggleAccordion('proxy')}
				class="flex w-full cursor-pointer items-center justify-between px-4 py-3.5 text-left transition hover:bg-muted/40"
			>
				<span class="flex items-center gap-2.5 text-sm font-medium text-foreground">
					<ArrowUpDown class="h-4 w-4 text-muted-foreground" />
					IP proxy headers
				</span>
				<span class="flex items-center gap-2.5">
					<span
						class="rounded px-2 py-0.5 text-xs font-medium {form.trusted_proxy.enabled
							? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
							: 'bg-muted text-muted-foreground'}"
					>
						{form.trusted_proxy.enabled ? 'Enabled' : 'Disabled'}
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openAccordion === 'proxy' ? 'rotate-180' : ''}" />
				</span>
			</button>
			{#if openAccordion === 'proxy'}
				<div transition:slide={{ duration: 180, easing: cubicOut }} class="space-y-3 border-t border-border bg-muted/20 p-4">
					<Switch
						checked={form.trusted_proxy.enabled}
						onChange={(v) => (form.trusted_proxy.enabled = v)}
						label="Trust reverse-proxy client IP headers (Cloudflare / Nginx)"
					/>
					<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
						<label for="proxy-hdrs" class="mb-0.5 block text-xs text-muted-foreground">Trusted headers (comma-separated, checked in order)</label>
						<input id="proxy-hdrs" type="text" bind:value={proxyHeadersText} placeholder="CF-Connecting-IP, X-Forwarded-For, X-Real-IP" class="w-full bg-transparent font-mono text-xs outline-none" />
					</div>
				</div>
			{/if}
		</div>

		<!-- 3. Rate limiting -->
		<div>
			<button
				type="button"
				onclick={() => toggleAccordion('ratelimit')}
				class="flex w-full cursor-pointer items-center justify-between px-4 py-3.5 text-left transition hover:bg-muted/40"
			>
				<span class="flex items-center gap-2.5 text-sm font-medium text-foreground">
					<Activity class="h-4 w-4 text-muted-foreground" />
					Rate limiting
				</span>
				<span class="flex items-center gap-2.5">
					<span class="rounded bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">
						Active
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openAccordion === 'ratelimit' ? 'rotate-180' : ''}" />
				</span>
			</button>
			{#if openAccordion === 'ratelimit'}
				<div transition:slide={{ duration: 180, easing: cubicOut }} class="border-t border-border bg-muted/20 p-4">
					<RateLimitsCard />
				</div>
			{/if}
		</div>

		<!-- 4. Superuser IPs -->
		<div>
			<button
				type="button"
				onclick={() => toggleAccordion('ips')}
				class="flex w-full cursor-pointer items-center justify-between px-4 py-3.5 text-left transition hover:bg-muted/40"
			>
				<span class="flex items-center gap-2.5 text-sm font-medium text-foreground">
					<ShieldCheck class="h-4 w-4 text-muted-foreground" />
					Superuser IPs
				</span>
				<span class="flex items-center gap-2.5">
					<span
						class="rounded px-2 py-0.5 text-xs font-medium {form.superuser_ips.enabled
							? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
							: 'bg-muted text-muted-foreground'}"
					>
						{form.superuser_ips.enabled ? 'Enabled' : 'Disabled'}
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openAccordion === 'ips' ? 'rotate-180' : ''}" />
				</span>
			</button>
			{#if openAccordion === 'ips'}
				<div transition:slide={{ duration: 180, easing: cubicOut }} class="space-y-3 border-t border-border bg-muted/20 p-4">
					<Switch
						checked={form.superuser_ips.enabled}
						onChange={(v) => (form.superuser_ips.enabled = v)}
						label="Restrict Superuser API access to allowed IPs / CIDRs only"
					/>
					<div class="rounded-[5px] bg-secondary/75 px-3 py-2">
						<label for="su-ips" class="mb-0.5 block text-xs text-muted-foreground">Allowed IPs / CIDRs (comma-separated, e.g. 127.0.0.1, ::1, 192.168.1.0/24)</label>
						<input id="su-ips" type="text" bind:value={superuserIPsText} placeholder="127.0.0.1, ::1" class="w-full bg-transparent font-mono text-xs outline-none" />
					</div>
				</div>
			{/if}
		</div>
	</div>

	<!-- Hide/Lock collection and record controls -->
	<div class="flex items-center gap-3 pt-1">
		<Switch
			checked={form.hide_controls}
			onChange={(v) => (form.hide_controls = v)}
			label="Hide/Lock collection and record controls"
		/>
	</div>

	<div class="flex justify-end border-t border-border pt-4">
		<button
			type="submit"
			disabled={saving}
			class="cursor-pointer rounded-md bg-primary px-6 py-2 text-xs font-semibold text-primary-foreground transition hover:opacity-90 disabled:opacity-50"
		>
			{saving ? 'Saving...' : 'Save changes'}
		</button>
	</div>
</form>
