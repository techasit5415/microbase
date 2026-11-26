<script lang="ts">
	import { onMount } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { slide } from 'svelte/transition';
	import { Activity, ArrowUpDown, ChevronDown, Layers, RotateCcw, ShieldCheck, SlidersHorizontal } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import RateLimitsCard from '#lib/components/settings/RateLimitsCard.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { appSettings, type AppSettingsData } from '#lib/stores/appSettings.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import { formatTimestamp, uiPrefs, type BorderRadiusPreset, type DateFormatMode, type FontFamilyPreset, type TableDensity, type ThemeMode } from '#lib/stores/uiPrefs.svelte.ts';

	let form = $state<AppSettingsData>(JSON.parse(JSON.stringify(appSettings.data)));
	let proxyHeadersText = $state((appSettings.data.trusted_proxy.headers ?? []).join(', '));
	let superuserIPsText = $state((appSettings.data.superuser_ips.allowed_ips ?? []).join(', '));
	let rateLimitEnabled = $state<boolean | null>(null);
	let openAccordion = $state<'appearance' | 'batch' | 'proxy' | 'ratelimit' | 'ips' | null>(null);
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
		try {
			const rl = await apiRequest<{ enabled: boolean }>('/api/settings/ratelimits');
			rateLimitEnabled = rl.enabled;
		} catch {
			rateLimitEnabled = null;
		}
	});

	function toggleAccordion(key: 'appearance' | 'batch' | 'proxy' | 'ratelimit' | 'ips') {
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
						oninput={() => appSettings.applyBranding(form)}
						class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
					/>
				</label>
			</div>
			<div class="mt-2 flex flex-wrap items-center gap-1.5">
				{#each accentPresets as c (c)}
					<button
						type="button"
						aria-label="Set accent {c}"
						onclick={() => { form.accent_color = c; appSettings.applyBranding(form); }}
						class="h-4 w-4 shrink-0 cursor-pointer rounded-full border border-white/40 transition hover:scale-110 {form.accent_color === c ? 'ring-2 ring-foreground ring-offset-1' : ''}"
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
					<span
						class="rounded px-2 py-0.5 text-xs font-medium {rateLimitEnabled
							? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
							: 'bg-muted text-muted-foreground'}"
					>
						{rateLimitEnabled === null ? '...' : rateLimitEnabled ? 'Enabled' : 'Disabled'}
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openAccordion === 'ratelimit' ? 'rotate-180' : ''}" />
				</span>
			</button>
			{#if openAccordion === 'ratelimit'}
				<div transition:slide={{ duration: 180, easing: cubicOut }} class="border-t border-border bg-muted/20 p-4">
					<RateLimitsCard onStatusChange={(v) => (rateLimitEnabled = v)} />
				</div>
			{/if}
		</div>

		<!-- 4. UI Appearance (local per-device) -->
		<div>
			<button
				type="button"
				onclick={() => toggleAccordion('appearance')}
				class="flex w-full cursor-pointer items-center justify-between px-4 py-3.5 text-left transition hover:bg-muted/40"
			>
				<span class="flex items-center gap-2.5 text-sm font-medium text-foreground">
					<SlidersHorizontal class="h-4 w-4 text-muted-foreground" />
					UI Appearance (Font size &amp; layout)
				</span>
				<span class="flex items-center gap-2.5">
					<span class="rounded bg-muted px-2 py-0.5 font-mono text-xs font-medium text-foreground">
						UI {uiPrefs.prefs.fontSize}px &middot; Table {uiPrefs.prefs.tableFontSize}px &middot; Sidebar {uiPrefs.prefs.sidebarFontSize}px
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openAccordion === 'appearance' ? 'rotate-180' : ''}" />
				</span>
			</button>
			{#if openAccordion === 'appearance'}
				<div transition:slide={{ duration: 180, easing: cubicOut }} class="space-y-4 border-t border-border bg-muted/20 p-4">
					<!-- Font Size -->
					<div class="space-y-2">
						<div class="flex flex-wrap items-center justify-between gap-2">
							<label for="ui-font-size" class="text-xs font-medium text-foreground">
								Base font size <span class="font-mono text-muted-foreground">({uiPrefs.prefs.fontSize}px)</span>
							</label>
							<div class="flex items-center gap-1">
								{#each [
									{ label: 'Small', size: 15 },
									{ label: 'Default', size: 17 },
									{ label: 'Large', size: 18.5 },
									{ label: 'XL', size: 20 }
								] as preset (preset.size)}
									<button
										type="button"
										onclick={() => uiPrefs.update({ fontSize: preset.size })}
										class="cursor-pointer rounded px-2 py-1 text-xs font-medium transition {uiPrefs.prefs.fontSize === preset.size
											? 'bg-primary text-primary-foreground'
											: 'border border-border bg-card text-muted-foreground hover:bg-muted hover:text-foreground'}"
									>
										{preset.label}
									</button>
								{/each}
							</div>
						</div>
						<input
							id="ui-font-size"
							type="range"
							min="14"
							max="20"
							step="0.5"
							value={uiPrefs.prefs.fontSize}
							oninput={(e) => uiPrefs.update({ fontSize: Number((e.currentTarget as HTMLInputElement).value) })}
							class="w-full cursor-pointer accent-primary"
						/>
						<div class="flex justify-between font-mono text-xs text-muted-foreground">
							<span>14px</span>
							<span>17px</span>
							<span>20px</span>
						</div>
					</div>

					<!-- Independent Sidebar & Table Data Font Sizes -->
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
						<div class="space-y-1.5 rounded-md border border-border bg-card p-3">
							<div class="flex items-center justify-between">
								<label for="ui-sidebar-size" class="text-xs font-medium text-foreground">Sidebar font size</label>
								<span class="font-mono text-xs font-semibold text-primary">{uiPrefs.prefs.sidebarFontSize}px</span>
							</div>
							<input
								id="ui-sidebar-size"
								type="range"
								min="12"
								max="19"
								step="0.5"
								value={uiPrefs.prefs.sidebarFontSize}
								oninput={(e) => uiPrefs.update({ sidebarFontSize: Number((e.currentTarget as HTMLInputElement).value) })}
								class="w-full cursor-pointer accent-primary"
							/>
							<div class="flex justify-between font-mono text-[11px] text-muted-foreground">
								<span>12px</span>
								<span>14px</span>
								<span>19px</span>
							</div>
						</div>

						<div class="space-y-1.5 rounded-md border border-border bg-card p-3">
							<div class="flex items-center justify-between">
								<label for="ui-table-size" class="text-xs font-medium text-foreground">Table data font size</label>
								<span class="font-mono text-xs font-semibold text-primary">{uiPrefs.prefs.tableFontSize}px</span>
							</div>
							<input
								id="ui-table-size"
								type="range"
								min="12"
								max="20"
								step="0.5"
								value={uiPrefs.prefs.tableFontSize}
								oninput={(e) => uiPrefs.update({ tableFontSize: Number((e.currentTarget as HTMLInputElement).value) })}
								class="w-full cursor-pointer accent-primary"
							/>
							<div class="flex justify-between font-mono text-[11px] text-muted-foreground">
								<span>12px</span>
								<span>14px</span>
								<span>20px</span>
							</div>
						</div>
					</div>

					<!-- Table Density & Border Radius -->
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
						<div class="space-y-1.5">
							<span class="block text-xs font-medium text-foreground">Table row density</span>
							<div class="grid grid-cols-3 gap-1 rounded-md border border-border bg-card p-1">
								{#each (['compact', 'normal', 'comfortable'] as TableDensity[]) as d (d)}
									<button
										type="button"
										onclick={() => uiPrefs.update({ density: d })}
										class="cursor-pointer rounded px-2 py-1.5 text-xs font-medium capitalize transition {uiPrefs.prefs.density === d
											? 'bg-primary text-primary-foreground'
											: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
									>
										{d}
									</button>
								{/each}
							</div>
						</div>

						<div class="space-y-1.5">
							<span class="block text-xs font-medium text-foreground">Corner radius</span>
							<div class="grid grid-cols-3 gap-1 rounded-md border border-border bg-card p-1">
								{#each (['sharp', 'default', 'rounded'] as BorderRadiusPreset[]) as r (r)}
									<button
										type="button"
										onclick={() => uiPrefs.update({ radius: r })}
										class="cursor-pointer rounded px-2 py-1.5 text-xs font-medium capitalize transition {uiPrefs.prefs.radius === r
											? 'bg-primary text-primary-foreground'
											: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
									>
										{r}
									</button>
								{/each}
							</div>
						</div>
					</div>

					<!-- Font Family & Theme Mode -->
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
						<div class="space-y-1.5">
							<span class="block text-xs font-medium text-foreground">Font family</span>
							<div class="grid grid-cols-3 gap-1 rounded-md border border-border bg-card p-1">
								{#each [
									{ id: 'ibm-plex-thai' as FontFamilyPreset, label: 'Plex Thai' },
									{ id: 'inter' as FontFamilyPreset, label: 'Inter' },
									{ id: 'system' as FontFamilyPreset, label: 'System' }
								] as f (f.id)}
									<button
										type="button"
										onclick={() => uiPrefs.update({ fontFamily: f.id })}
										class="cursor-pointer rounded px-2 py-1.5 text-xs font-medium transition {uiPrefs.prefs.fontFamily === f.id
											? 'bg-primary text-primary-foreground'
											: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
									>
										{f.label}
									</button>
								{/each}
							</div>
						</div>

						<div class="space-y-1.5">
							<span class="block text-xs font-medium text-foreground">Theme mode</span>
							<div class="grid grid-cols-3 gap-1 rounded-md border border-border bg-card p-1">
								{#each (['light', 'dark', 'system'] as ThemeMode[]) as tm (tm)}
									<button
										type="button"
										onclick={() => uiPrefs.update({ theme: tm })}
										class="cursor-pointer rounded px-2 py-1.5 text-xs font-medium capitalize transition {uiPrefs.prefs.theme === tm
											? 'bg-primary text-primary-foreground'
											: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
									>
										{tm}
									</button>
								{/each}
							</div>
						</div>
					</div>

					<!-- Date Format & Default Rows Per Page -->
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
						<div class="space-y-1.5">
							<span class="block text-xs font-medium text-foreground">Date &amp; time format</span>
							<div class="grid grid-cols-3 gap-1 rounded-md border border-border bg-card p-1">
								{#each [
									{ id: 'local' as DateFormatMode, label: 'Local' },
									{ id: 'utc' as DateFormatMode, label: 'UTC' },
									{ id: 'relative' as DateFormatMode, label: 'Relative' }
								] as df (df.id)}
									<button
										type="button"
										onclick={() => uiPrefs.update({ dateFormat: df.id })}
										class="cursor-pointer rounded px-2 py-1.5 text-xs font-medium transition {uiPrefs.prefs.dateFormat === df.id
											? 'bg-primary text-primary-foreground'
											: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
									>
										{df.label}
									</button>
								{/each}
							</div>
						</div>

						<div class="space-y-1.5">
							<span class="block text-xs font-medium text-foreground">Default rows per page</span>
							<div class="grid grid-cols-5 gap-1 rounded-md border border-border bg-card p-1">
								{#each [10, 20, 30, 50, 100] as n (n)}
									<button
										type="button"
										onclick={() => uiPrefs.update({ defaultPerPage: n })}
										class="cursor-pointer rounded px-1.5 py-1.5 font-mono text-xs font-medium transition {uiPrefs.prefs.defaultPerPage === n
											? 'bg-primary text-primary-foreground'
											: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
									>
										{n}
									</button>
								{/each}
							</div>
						</div>
					</div>

					<!-- Live Preview (Sidebar + Table Data + Density + Radius) -->
					<div class="space-y-1.5">
						<span class="block text-xs font-medium text-muted-foreground">Live preview</span>
						<div class="flex overflow-hidden rounded-[var(--radius)] border border-border bg-background">
							<!-- Mini Sidebar -->
							<div class="w-36 shrink-0 space-y-1 border-r border-border bg-card p-2">
								<div
									class="truncate rounded-[var(--radius)] bg-accent px-2.5 py-1.5 font-medium text-foreground"
									style="font-size: var(--ui-sidebar-size);"
								>
									users
								</div>
								<div
									class="truncate rounded-[var(--radius)] px-2.5 py-1.5 text-muted-foreground"
									style="font-size: var(--ui-sidebar-size);"
								>
									announcements
								</div>
							</div>
							<!-- Mini Table -->
							<div class="min-w-0 flex-1 overflow-x-auto bg-card">
								<table class="w-full border-collapse text-left" style="font-size: var(--ui-table-size);">
									<thead class="border-b border-border bg-muted/40 text-muted-foreground" style="font-size: calc(var(--ui-table-size) * 0.92);">
										<tr>
											<th class="px-3 py-1.5 font-semibold">id</th>
											<th class="px-3 py-1.5 font-semibold">email</th>
											<th class="px-3 py-1.5 font-semibold">verified</th>
											<th class="px-3 py-1.5 font-semibold">name</th>
											<th class="px-3 py-1.5 font-semibold">created_at</th>
										</tr>
									</thead>
									<tbody class="divide-y divide-border">
										<tr>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">
												<span class="rounded-[var(--radius)] bg-secondary px-1.5 py-0.5 font-mono text-xs">zvxpzeryp55umz4</span>
											</td>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">67050200@kmitl.ac.th</td>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">
												<span class="rounded-[var(--radius)] bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">True</span>
											</td>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">นาย เตชสิทธิ์</td>
											<td class="px-3 py-[var(--row-py)] font-mono text-xs whitespace-nowrap">{formatTimestamp(new Date(Date.now() - 300000).toISOString(), uiPrefs.prefs.dateFormat)}</td>
										</tr>
										<tr>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">
												<span class="rounded-[var(--radius)] bg-secondary px-1.5 py-0.5 font-mono text-xs">ztq33z0yf1gu50m</span>
											</td>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">66050762@kmitl.ac.th</td>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">
												<span class="rounded-[var(--radius)] bg-secondary px-2 py-0.5 text-xs font-medium text-muted-foreground">False</span>
											</td>
											<td class="px-3 py-[var(--row-py)] whitespace-nowrap">สมชาย ใจดี</td>
											<td class="px-3 py-[var(--row-py)] font-mono text-xs whitespace-nowrap">{formatTimestamp(new Date(Date.now() - 86400000 * 2).toISOString(), uiPrefs.prefs.dateFormat)}</td>
										</tr>
									</tbody>
								</table>
							</div>
						</div>
					</div>

					<div class="flex items-center justify-between pt-1 text-xs text-muted-foreground">
						<span>Saved automatically in this browser (applied live).</span>
						<button
							type="button"
							onclick={() => {
								uiPrefs.reset();
								toast.success('Reset UI appearance to defaults');
							}}
							class="inline-flex cursor-pointer items-center gap-1 rounded border border-border bg-card px-2.5 py-1 font-medium text-foreground transition hover:bg-muted"
						>
							<RotateCcw class="h-3 w-3" /> Reset
						</button>
					</div>
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
