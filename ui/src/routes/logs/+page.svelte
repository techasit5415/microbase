<script lang="ts">
	import { onMount } from 'svelte';
	import { Download, Info, RefreshCw, Search, Settings, Trash2, X } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import PaginationBar from '#lib/components/records/PaginationBar.svelte';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import { formatTimestamp, uiPrefs } from '#lib/stores/uiPrefs.svelte.ts';

	interface LogEntry {
		id?: number;
		level?: number;
		timestamp: string;
		method: string;
		path: string;
		url?: string;
		status: number;
		code?: string;
		message?: string;
		latency_ms: number;
		exec_time_ms?: number;
		ip: string;
		remote_ip?: string;
		referer?: string;
		user_agent?: string;
		auth?: string;
		auth_role?: string;
		auth_email?: string;
		auth_id?: string;
	}

	interface LogSettings {
		max_days: number;
		min_level: number;
		max_bytes: number;
		log_ip: boolean;
		log_auth_id: boolean;
	}

	const LEVEL_PRESETS = [
		{ value: -4, tag: '-4:DEBUG' },
		{ value: 0, tag: '0:INFO' },
		{ value: 4, tag: '4:WARN' },
		{ value: 8, tag: '8:ERROR' }
	];

	let logs = $state<LogEntry[]>([]);
	let filter = $state('');
	let page = $state(1);
	let perPage = $state(uiPrefs.prefs.defaultPerPage || 30);
	let totalItems = $state(0);
	let loading = $state(true);
	let spinning = $state(false);
	let refreshGen = $state(0);
	let selectedLog = $state<LogEntry | null>(null);
	let searchTimer: ReturnType<typeof setTimeout> | null = null;

	let settings = $state<LogSettings>({
		max_days: 7,
		min_level: 0,
		max_bytes: 0,
		log_ip: true,
		log_auth_id: true
	});

	let settingsOpen = $state(false);
	let savingSettings = $state(false);
	let clearingLogs = $state(false);
	let draftMaxDays = $state('7');
	let draftMinLevel = $state('0');
	let draftMaxBytes = $state('');
	let draftLogIP = $state(true);
	let draftLogAuthID = $state(true);

	let hasSettingsChanges = $derived.by(() => {
		const parsedDays = Number.parseInt(draftMaxDays, 10);
		const parsedLevel = Number.parseInt(draftMinLevel, 10);
		const parsedBytes = draftMaxBytes.trim() === '' ? 0 : Number.parseInt(draftMaxBytes, 10);
		return (
			parsedDays !== settings.max_days ||
			parsedLevel !== settings.min_level ||
			parsedBytes !== settings.max_bytes ||
			draftLogIP !== settings.log_ip ||
			draftLogAuthID !== settings.log_auth_id
		);
	});

	function resolveLevel(row: LogEntry): number {
		if (typeof row.level === 'number' && (row.level !== 0 || row.status < 400)) {
			return row.level;
		}
		if (row.status >= 500) return 8;
		if (row.status >= 400) return 4;
		return 0;
	}

	function logLevelMeta(level: number): { short: string; label: string; dotClass: string; badgeClass: string } {
		if (level >= 8) {
			return {
				short: 'ERROR',
				label: `ERROR (${level})`,
				dotClass: 'bg-destructive',
				badgeClass: 'bg-destructive/10 text-destructive'
			};
		}
		if (level >= 4) {
			return {
				short: 'WARN',
				label: `WARN (${level})`,
				dotClass: 'bg-amber-500',
				badgeClass: 'bg-amber-500/10 text-amber-700 dark:text-amber-400'
			};
		}
		if (level < 0) {
			return {
				short: 'DEBUG',
				label: `DEBUG (${level})`,
				dotClass: 'bg-sky-500',
				badgeClass: 'bg-sky-500/10 text-sky-700 dark:text-sky-400'
			};
		}
		return {
			short: 'INFO',
			label: `INFO (${level})`,
			dotClass: 'bg-emerald-500',
			badgeClass: 'bg-secondary text-foreground'
		};
	}

	let filtered = $derived(logs);

	let chartBars = $derived.by(() => {
		const buckets: { label: string; ok: number; err: number; total: number }[] = [];
		const byMinute = new Map<string, { ok: number; err: number }>();
		for (const row of filtered) {
			const d = new Date(row.timestamp);
			const key = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
			const cur = byMinute.get(key) ?? { ok: 0, err: 0 };
			if (row.status >= 400) cur.err += 1;
			else cur.ok += 1;
			byMinute.set(key, cur);
		}
		for (const [label, v] of Array.from(byMinute.entries()).reverse().slice(-24)) {
			buckets.push({ label, ok: v.ok, err: v.err, total: v.ok + v.err });
		}
		return buckets;
	});

	let maxBar = $derived(Math.max(1, ...chartBars.map((b) => b.total)));

	function formatExecTime(row: LogEntry): string {
		if (typeof row.exec_time_ms === 'number' && row.exec_time_ms > 0) {
			return `${Number(row.exec_time_ms.toFixed(6))}ms`;
		}
		return `${row.latency_ms}ms`;
	}

	function openSettingsModal() {
		draftMaxDays = String(settings.max_days);
		draftMinLevel = String(settings.min_level);
		draftMaxBytes = settings.max_bytes > 0 ? String(settings.max_bytes) : '';
		draftLogIP = settings.log_ip;
		draftLogAuthID = settings.log_auth_id;
		settingsOpen = true;
	}

	async function saveLogSettings(e: SubmitEvent) {
		e.preventDefault();
		const maxDays = Number.parseInt(draftMaxDays, 10);
		const minLevel = Number.parseInt(draftMinLevel, 10);
		const maxBytes = draftMaxBytes.trim() === '' ? 0 : Number.parseInt(draftMaxBytes, 10);

		if (Number.isNaN(maxDays) || maxDays < 0 || maxDays > 365) {
			toast.error('Max days retention must be between 0 and 365');
			return;
		}
		if (Number.isNaN(minLevel) || minLevel < -8 || minLevel > 12) {
			toast.error('Min log level must be between -8 and 12');
			return;
		}
		if (Number.isNaN(maxBytes) || maxBytes < 0 || maxBytes > 1048576) {
			toast.error('Max data size must be between 0 and 1048576 bytes');
			return;
		}

		savingSettings = true;
		try {
			const updated = await apiRequest<LogSettings>('/api/settings/logs', {
				method: 'PUT',
				body: JSON.stringify({
					max_days: maxDays,
					min_level: minLevel,
					max_bytes: maxBytes,
					log_ip: draftLogIP,
					log_auth_id: draftLogAuthID
				})
			});
			settings = updated;
			settingsOpen = false;
			toast.success(
				updated.max_days === 0
					? 'Logs persistence disabled and all logs cleared'
					: 'Log settings saved'
			);
			await loadLogs();
		} catch (err) {
			toast.error(err, 'Failed to save log settings');
		} finally {
			savingSettings = false;
		}
	}

	async function handleClearLogs() {
		const ok = await confirmAction({
			title: 'Delete all logs?',
			description: 'This will permanently delete all persisted request logs from the database.',
			confirmLabel: 'Delete all logs',
			destructive: true
		});
		if (!ok) return;

		clearingLogs = true;
		try {
			await apiRequest('/api/logs', { method: 'DELETE' });
			logs = [];
			totalItems = 0;
			page = 1;
			selectedLog = null;
			toast.success('All logs cleared');
		} catch (err) {
			toast.error(err, 'Failed to clear logs');
		} finally {
			clearingLogs = false;
		}
	}

	function downloadLogJson(row: LogEntry) {
		const payload = {
			id: row.id,
			level: resolveLevel(row),
			created: row.timestamp,
			data: {
				execTime: row.exec_time_ms ?? row.latency_ms,
				type: 'request',
				auth: row.auth && row.auth !== '_guest' ? row.auth : '',
				authEmail: row.auth_email || undefined,
				authId: row.auth_id || undefined,
				status: row.status,
				method: row.method,
				url: row.url || row.path,
				referer: row.referer || '',
				remoteIP: row.remote_ip || row.ip || '',
				userIP: row.ip || '',
				userAgent: row.user_agent || ''
			}
		};
		const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `log_${row.id ?? 'entry'}.json`;
		a.click();
		URL.revokeObjectURL(url);
	}

	interface LogsPageResponse {
		page?: number;
		per_page?: number;
		total_items?: number;
		total_pages?: number;
		items: LogEntry[];
	}

	async function loadLogs(targetPage = page, targetPerPage = perPage, targetSearch = filter) {
		loading = true;
		spinning = true;
		const startedAt = Date.now();
		try {
			const params = new URLSearchParams({
				page: String(targetPage),
				per_page: String(targetPerPage)
			});
			const q = targetSearch.trim();
			if (q) params.set('search', q);

			const res = await apiRequest<LogsPageResponse>(`/api/logs?${params.toString()}`);
			logs = res.items ?? [];
			totalItems = typeof res.total_items === 'number' ? res.total_items : logs.length;
			page = typeof res.page === 'number' ? res.page : targetPage;
			refreshGen += 1;
		} catch (err) {
			toast.error(err, 'Failed to load logs');
		} finally {
			loading = false;
			const elapsed = Date.now() - startedAt;
			setTimeout(() => {
				spinning = false;
			}, Math.max(0, 450 - elapsed));
		}
	}

	function handleSearchInput() {
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			page = 1;
			loadLogs(1, perPage, filter);
		}, 250);
	}

	function handlePageChange(nextPage: number) {
		page = nextPage;
		loadLogs(nextPage, perPage, filter);
	}

	function handlePerPageChange(nextPerPage: number) {
		perPage = nextPerPage;
		page = 1;
		loadLogs(1, nextPerPage, filter);
	}

	async function loadSettings() {
		try {
			const r = await apiRequest<Partial<LogSettings>>('/api/settings/logs');
			settings = {
				max_days: typeof r.max_days === 'number' ? r.max_days : 7,
				min_level: typeof r.min_level === 'number' ? r.min_level : 0,
				max_bytes: typeof r.max_bytes === 'number' ? r.max_bytes : 0,
				log_ip: typeof r.log_ip === 'boolean' ? r.log_ip : true,
				log_auth_id: typeof r.log_auth_id === 'boolean' ? r.log_auth_id : true
			};
		} catch {}
	}

	onMount(() => {
		loadLogs();
		loadSettings();
	});
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape' && settingsOpen) settingsOpen = false;
	}}
/>

<main class="flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
	<header class="space-y-3 border-b border-border bg-card px-6 py-4">
		<div class="flex items-center justify-between gap-4">
			<div class="flex items-center gap-1.5 text-sm">
				<h1 class="mr-1 font-semibold text-foreground">Logs</h1>
				<button
					type="button"
					onclick={() => loadLogs()}
					disabled={spinning}
					title="Refresh logs"
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground disabled:opacity-70"
				>
					<RefreshCw class="h-4 w-4 transition-transform {spinning ? 'animate-spin text-foreground' : ''}" />
				</button>
				<button
					type="button"
					onclick={openSettingsModal}
					title="Logs settings"
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					<Settings class="h-4 w-4" />
				</button>
			</div>

			<div class="flex items-center gap-3 text-xs text-muted-foreground">
				<button
					type="button"
					onclick={openSettingsModal}
					class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-2 py-1 font-mono text-[11px] text-foreground transition hover:bg-muted"
					title="Configure logs settings"
				>
					<span class="h-1.5 w-1.5 rounded-full {logLevelMeta(settings.min_level).dotClass}"></span>
					<span>Min: {logLevelMeta(settings.min_level).label}</span>
					<span class="text-muted-foreground">·</span>
					<span>{settings.max_days === 0 ? 'Disabled' : `${settings.max_days}d`}</span>
				</button>
				<span>Showing {logs.length} of {totalItems.toLocaleString()} logs</span>
			</div>
		</div>

		<div class="relative">
			<Search class="pointer-events-none absolute top-2.5 left-3.5 h-4 w-4 text-muted-foreground" />
			<input
				type="text"
				bind:value={filter}
				oninput={handleSearchInput}
				placeholder="Filter by level (INFO / WARN / ERROR), method, url, auth, email, ip, status..."
				class="w-full rounded-full border border-transparent bg-muted/80 py-2 pr-4 pl-10 text-xs outline-none transition focus:border-border focus:bg-accent/70"
			/>
		</div>

		{#if chartBars.length > 0}
			<div class="flex h-16 items-end gap-1 rounded-md border border-border bg-background/70 px-3 pt-2 pb-1">
				{#each chartBars as bar (bar.label)}
					<div
						class="group relative flex h-full flex-1 flex-col justify-end"
						title="{bar.label} — {bar.total} reqs ({bar.ok} ok, {bar.err} err)"
					>
						<div
							class="w-full rounded-t-xs transition-all {bar.err > 0 ? 'bg-amber-500/80' : 'bg-emerald-500/75'} group-hover:opacity-100"
							style="height: {Math.max(12, Math.round((bar.total / maxBar) * 100))}%;"
						></div>
					</div>
				{/each}
			</div>
		{/if}
	</header>

	<div class="flex-1 overflow-auto">
		<table class="w-full border-collapse text-left text-xs">
			<thead class="sticky top-0 z-10 border-b border-border bg-card/95 text-muted-foreground backdrop-blur-xs">
				<tr>
					<th class="px-5 py-3 font-semibold">Level</th>
					<th class="px-3 py-3 font-semibold">Status</th>
					<th class="px-3 py-3 font-semibold">Method</th>
					<th class="px-4 py-3 font-semibold">URL</th>
					<th class="px-4 py-3 font-semibold">Auth</th>
					<th class="px-4 py-3 font-semibold">Exec time</th>
					<th class="px-4 py-3 font-semibold">IP</th>
					<th class="px-5 py-3 font-semibold">Timestamp</th>
				</tr>
			</thead>
			{#key refreshGen}
				<tbody class="divide-y divide-border font-mono">
					{#if loading && logs.length === 0}
						<tr><td colspan="8" class="px-5 py-10 text-center text-muted-foreground">Loading logs...</td></tr>
					{:else if filtered.length === 0}
						<tr>
							<td colspan="8" class="px-5 py-10 text-center text-muted-foreground">
								{settings.max_days === 0 ? 'Logs persistence is disabled (Max days retention = 0).' : 'No logs yet.'}
							</td>
						</tr>
					{:else}
						{#each filtered as row, i (row.id ?? `${row.timestamp}-${i}`)}
							{@const lvl = logLevelMeta(resolveLevel(row))}
							{@const authName = (row.auth && row.auth !== '_guest' ? row.auth : '') || (row.auth_role === 'superuser' ? '_superusers' : row.auth_role ? 'users' : '')}
							<tr
								onclick={() => (selectedLog = row)}
								class="animate-row-in cursor-pointer bg-card transition-colors hover:bg-muted/60"
								style="animation-delay: {Math.min(i * 15, 180)}ms;"
							>
								<td class="px-5 py-2.5 whitespace-nowrap">
									<span class="inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] font-semibold {lvl.badgeClass}">
										<span class="h-1.5 w-1.5 rounded-full {lvl.dotClass}"></span>
										{lvl.short}
									</span>
								</td>
								<td class="px-3 py-2.5 whitespace-nowrap">
									<span
										class="rounded px-2 py-0.5 text-xs font-semibold {row.status >= 500
											? 'bg-destructive/15 text-destructive'
											: row.status >= 400
												? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
												: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}"
									>
										{row.status}
									</span>
								</td>
								<td class="px-3 py-2.5 font-semibold whitespace-nowrap">{row.method}</td>
								<td class="max-w-sm truncate px-4 py-2.5 text-foreground" title={row.url || row.path}>{row.url || row.path}</td>
								<td class="max-w-56 px-4 py-2.5 whitespace-nowrap">
									{#if authName}
										<span class="inline-flex max-w-full items-center gap-1.5">
											<span
												class="shrink-0 rounded px-1.5 py-0.5 text-xs font-semibold {authName === '_superusers'
													? 'bg-violet-500/15 text-violet-700 dark:text-violet-400'
													: 'bg-sky-500/15 text-sky-700 dark:text-sky-400'}"
											>
												{authName}
											</span>
											{#if row.auth_email || row.auth_id}
												<span class="truncate font-sans text-xs text-foreground/90" title={row.auth_email || row.auth_id}>
													{row.auth_email || row.auth_id}
												</span>
											{/if}
										</span>
									{:else}
										<span class="text-muted-foreground/60">&mdash;</span>
									{/if}
								</td>
								<td class="px-4 py-2.5 whitespace-nowrap text-muted-foreground">{formatExecTime(row)}</td>
								<td class="px-4 py-2.5 whitespace-nowrap text-muted-foreground">{row.ip || '—'}</td>
								<td class="px-5 py-2.5 whitespace-nowrap text-muted-foreground">
									{formatTimestamp(row.timestamp, uiPrefs.prefs.dateFormat)}
								</td>
							</tr>
						{/each}
					{/if}
				</tbody>
			{/key}
		</table>
	</div>

	<PaginationBar
		{page}
		{perPage}
		{totalItems}
		onPageChange={handlePageChange}
		onPerPageChange={handlePerPageChange}
	/>
</main>

<!-- PocketBase-style Logs settings Modal -->
{#if settingsOpen}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4 backdrop-blur-[1px]"
		role="presentation"
		onclick={(e) => {
			if (e.target === e.currentTarget) settingsOpen = false;
		}}
	>
		<form
			onsubmit={saveLogSettings}
			class="w-full max-w-md overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
		>
			<div class="px-6 pt-6 pb-4 text-center">
				<h2 class="text-lg font-semibold text-foreground">Logs settings</h2>
			</div>

			<div class="space-y-5 px-6 pb-6">
				<!-- Max days retention -->
				<div class="space-y-1.5">
					<div class="rounded-md border border-border/80 bg-muted/60 px-3.5 py-2.5 focus-within:border-ring focus-within:bg-background">
						<label for="logs-max-days" class="block text-xs font-semibold text-muted-foreground">
							Max days retention <span class="text-destructive">*</span>
						</label>
						<input
							id="logs-max-days"
							type="number"
							min="0"
							max="365"
							required
							bind:value={draftMaxDays}
							class="mt-1 w-full bg-transparent text-sm text-foreground outline-none"
						/>
					</div>
					<p class="text-xs text-muted-foreground">
						Set to <code class="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-foreground">0</code> to delete all logs and disable logs persistence.
					</p>
				</div>

				<!-- Min log level -->
				<div class="space-y-1.5">
					<div class="rounded-md border border-border/80 bg-muted/60 px-3.5 py-2.5 focus-within:border-ring focus-within:bg-background">
						<label for="logs-min-level" class="block text-xs font-semibold text-muted-foreground">
							Min log level <span class="text-destructive">*</span>
						</label>
						<input
							id="logs-min-level"
							type="number"
							min="-8"
							max="12"
							required
							bind:value={draftMinLevel}
							class="mt-1 w-full bg-transparent text-sm text-foreground outline-none"
						/>
					</div>
					<p class="text-xs text-muted-foreground">Logs with level below the minimum will be ignored.</p>
					<div class="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
						<span>Default log levels:</span>
						{#each LEVEL_PRESETS as preset (preset.value)}
							{@const active = Number.parseInt(draftMinLevel, 10) === preset.value}
							<button
								type="button"
								onclick={() => (draftMinLevel = String(preset.value))}
								class="cursor-pointer rounded px-1.5 py-0.5 font-mono text-[11px] transition {active
									? 'bg-foreground text-background font-semibold'
									: 'bg-muted text-foreground hover:bg-accent'}"
							>
								{preset.tag}
							</button>
						{/each}
					</div>
				</div>

				<!-- Max data size (bytes) -->
				<div class="space-y-1.5">
					<div class="rounded-md border border-border/80 bg-muted/60 px-3.5 py-2.5 focus-within:border-ring focus-within:bg-background">
						<label for="logs-max-bytes" class="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
							<span>Max data size <span class="font-normal">(bytes)</span></span>
							<span title="Maximum allowed size in bytes for log data fields. Leave empty or 0 for default (~16KB).">
								<Info class="h-3.5 w-3.5 text-muted-foreground" />
							</span>
						</label>
						<input
							id="logs-max-bytes"
							type="number"
							min="0"
							max="1048576"
							placeholder="Default to ~16KB"
							bind:value={draftMaxBytes}
							class="mt-1 w-full bg-transparent text-sm text-foreground placeholder:text-muted-foreground/70 outline-none"
						/>
					</div>
				</div>

				<!-- Toggles -->
				<div class="space-y-3 pt-1">
					<div class="flex items-center">
						<Switch bind:checked={draftLogIP} label="Enable IP logging" />
					</div>
					<div class="flex items-center">
						<Switch bind:checked={draftLogAuthID} label="Enable Auth Id logging" />
					</div>
				</div>
			</div>

			<div class="flex items-center justify-between border-t border-border bg-card px-6 py-4">
				<button
					type="button"
					onclick={() => (settingsOpen = false)}
					class="cursor-pointer rounded-md px-3 py-2 text-xs font-semibold text-foreground transition hover:bg-muted"
				>
					Close
				</button>
				<div class="flex items-center gap-3">
					<button
						type="button"
						onclick={handleClearLogs}
						disabled={clearingLogs}
						title="Delete all logs"
						class="cursor-pointer rounded-md p-2 text-muted-foreground transition hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
					>
						<Trash2 class="h-4 w-4" />
					</button>
					<button
						type="submit"
						disabled={savingSettings || !hasSettingsChanges}
						class="cursor-pointer rounded-md bg-foreground px-4 py-2 text-xs font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground"
					>
						{savingSettings ? 'Saving...' : 'Save changes'}
					</button>
				</div>
			</div>
		</form>
	</div>
{/if}

<!-- PocketBase-style Log details Slide-over Drawer -->
<Drawer
	open={selectedLog !== null}
	maxWidth="max-w-xl"
	onClose={() => (selectedLog = null)}
>
	{#if selectedLog}
		{@const lvl = logLevelMeta(resolveLevel(selectedLog))}
		{@const authCol = (selectedLog.auth && selectedLog.auth !== '_guest' ? selectedLog.auth : '') || (selectedLog.auth_role === 'superuser' ? '_superusers' : selectedLog.auth_role ? 'users' : '')}
		<div class="flex h-full flex-col bg-card">
			<div class="flex items-center justify-between border-b border-border px-6 py-4">
				<h2 class="text-base font-semibold text-foreground">Log details</h2>
				<button
					type="button"
					aria-label="Close log details"
					onclick={() => (selectedLog = null)}
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<div class="flex-1 overflow-y-auto p-6">
				<div class="overflow-hidden rounded-md border border-border bg-card text-sm">
					<table class="w-full border-collapse text-left">
						<tbody class="divide-y divide-border">
							<tr>
								<td class="w-40 px-4 py-3 font-medium text-muted-foreground">id</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.id ?? '—'}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">level</td>
								<td class="px-4 py-3">
									<span class="inline-flex items-center gap-1.5 rounded px-2.5 py-0.5 text-xs font-semibold {lvl.badgeClass}">
										<span class="h-2 w-2 rounded-full {lvl.dotClass}"></span>
										{lvl.label}
									</span>
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">created</td>
								<td class="px-4 py-3">
									<div class="font-mono text-foreground">{formatTimestamp(selectedLog.timestamp, 'local')}</div>
									<div class="font-mono text-xs text-muted-foreground">{new Date(selectedLog.timestamp).toISOString()}</div>
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.execTime</td>
								<td class="px-4 py-3 font-mono text-foreground">{formatExecTime(selectedLog)}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.type</td>
								<td class="px-4 py-3 font-mono text-foreground">request</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.auth</td>
								<td class="px-4 py-3 font-mono {authCol ? 'text-foreground' : 'text-muted-foreground/60'}">{authCol || 'N/A'}</td>
							</tr>
							{#if selectedLog.auth_email}
								<tr>
									<td class="px-4 py-3 font-medium text-muted-foreground">data.authEmail</td>
									<td class="px-4 py-3 font-mono text-foreground">{selectedLog.auth_email}</td>
								</tr>
							{/if}
							{#if selectedLog.auth_id}
								<tr>
									<td class="px-4 py-3 font-medium text-muted-foreground">data.authId</td>
									<td class="px-4 py-3 font-mono text-foreground">{selectedLog.auth_id}</td>
								</tr>
							{/if}
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.status</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.status}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.method</td>
								<td class="px-4 py-3 font-mono text-foreground">{selectedLog.method}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.url</td>
								<td class="px-4 py-3 font-mono break-all text-foreground">{selectedLog.url || selectedLog.path}</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.referer</td>
								<td class="px-4 py-3 font-mono break-all {selectedLog.referer ? 'text-foreground' : 'text-muted-foreground/60'}">
									{selectedLog.referer || 'N/A'}
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.remoteIP</td>
								<td class="px-4 py-3 font-mono {selectedLog.remote_ip || selectedLog.ip ? 'text-foreground' : 'text-muted-foreground/60'}">
									{selectedLog.remote_ip || selectedLog.ip || 'N/A'}
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.userIP</td>
								<td class="px-4 py-3 font-mono {selectedLog.ip ? 'text-foreground' : 'text-muted-foreground/60'}">
									{selectedLog.ip || 'N/A'}
								</td>
							</tr>
							<tr>
								<td class="px-4 py-3 font-medium text-muted-foreground">data.userAgent</td>
								<td class="px-4 py-3 font-mono break-all {selectedLog.user_agent ? 'text-foreground' : 'text-muted-foreground/60'}">
									{selectedLog.user_agent || 'N/A'}
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</div>

			<div class="flex shrink-0 items-center justify-between border-t border-border bg-card px-6 py-4">
				<button
					type="button"
					onclick={() => (selectedLog = null)}
					class="cursor-pointer rounded-md px-4 py-2 text-xs font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					Close
				</button>
				<button
					type="button"
					onclick={() => selectedLog && downloadLogJson(selectedLog)}
					class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-2 text-xs font-semibold text-background transition hover:opacity-90"
				>
					<Download class="h-3.5 w-3.5" />
					Download JSON
				</button>
			</div>
		</div>
	{/if}
</Drawer>
