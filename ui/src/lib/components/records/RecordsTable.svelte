<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { fly } from 'svelte/transition';
	import {
		ArrowDown,
		ArrowRight,
		ArrowUp,
		ArrowUpDown,
		Braces,
		Calendar,
		Check,
		Copy,
		Eye,
		FileImage,
		Hash,
		KeyRound,
		List,
		Mail,
		Share2,
		ToggleLeft,
		Trash2,
		Type
	} from '@lucide/svelte';
	import type { Collection, FieldDef, RecordRow } from '#lib/api/types.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface Props {
		collection: Collection;
		allCollections?: Collection[];
		items: RecordRow[];
		loading: boolean;
		readOnly?: boolean;
		sort?: string;
		onSortChange?: (nextSort: string) => void;
		onSelect?: (row: RecordRow) => void;
		onDelete: (id: number | string) => void;
		onBulkDelete?: (ids: (number | string)[]) => void;
	}

	let {
		collection,
		allCollections = [],
		items,
		loading,
		readOnly = false,
		sort = '',
		onSortChange,
		onSelect,
		onDelete,
		onBulkDelete
	}: Props = $props();

	let selectedIds = $state<(number | string)[]>([]);
	let hoveredTooltip = $state<{ json: string; x: number; y: number } | null>(null);
	let showSlowLoading = $state(false);

	$effect(() => {
		if (!loading) {
			showSlowLoading = false;
			return;
		}
		const timer = setTimeout(() => {
			showSlowLoading = true;
		}, 250);
		return () => clearTimeout(timer);
	});

	$effect(() => {
		collection.name;
		items;
		selectedIds = [];
	});

	let collectionMap = $derived.by(() => {
		const m: Record<string, Collection> = {};
		for (const c of allCollections) m[c.name] = c;
		return m;
	});

	let fieldMap = $derived.by(() => {
		const m: Record<string, FieldDef> = {};
		for (const f of collection.fields ?? []) m[f.name] = f;
		return m;
	});

	let columns = $derived.by(() => {
		if (collection.type === 'system' || collection.type === 'view') {
			return items.length > 0 ? Object.keys(items[0]) : ['id'];
		}
		if (collection.type === 'auth' || collection.name === 'users') {
			const custom = (collection.fields ?? [])
				.map((f) => f.name)
				.filter((n) => !['id', 'email', 'verified', 'email_visibility', 'created_at', 'updated_at'].includes(n));
			return ['id', 'email', 'email_visibility', 'verified', ...custom, 'created_at'];
		}
		return ['id', ...(collection.fields ?? []).map((f) => f.name), 'created_at'];
	});

	let allSelectableIds = $derived(
		items
			.map((r) => r.id)
			.filter((id): id is number | string => typeof id === 'number' || typeof id === 'string')
	);

	let allSelected = $derived(
		allSelectableIds.length > 0 && selectedIds.length === allSelectableIds.length
	);

	function getSortDir(col: string): 'asc' | 'desc' | null {
		if (sort === `-${col}`) return 'desc';
		if (sort === col || sort === `+${col}`) return 'asc';
		return null;
	}

	function cycleSort(col: string) {
		if (!onSortChange) return;
		const dir = getSortDir(col);
		if (dir === null) onSortChange(`-${col}`);
		else if (dir === 'desc') onSortChange(col);
		else onSortChange('');
	}

	function toggleOne(id: number | string) {
		if (selectedIds.includes(id)) {
			selectedIds = selectedIds.filter((x) => x !== id);
		} else {
			selectedIds = [...selectedIds, id];
		}
	}

	function toggleAll() {
		selectedIds = allSelected ? [] : [...allSelectableIds];
	}

	function getFieldType(col: string): string {
		if (col === 'id') return 'id';
		if (col === 'email') return 'email';
		if (col === 'verified' || col === 'email_visibility') return 'boolean';
		if (col === 'created_at' || col === 'updated_at') return 'date';
		return fieldMap[col]?.type ?? 'text';
	}

	function copyText(e: MouseEvent, text: string) {
		e.stopPropagation();
		navigator.clipboard?.writeText(text).catch(() => {});
		toast.success(`Copied ${text}`);
	}

	function showRecordTooltip(e: MouseEvent, rec: RecordRow | undefined) {
		if (!rec) return;
		const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const clean: Record<string, unknown> = {};
		for (const [k, v] of Object.entries(rec)) {
			if (k === 'password_hash' || k === 'token_key') continue;
			clean[k] = v;
		}
		hoveredTooltip = {
			json: JSON.stringify(clean, null, 2),
			x: Math.min(window.innerWidth - 340, Math.max(16, rect.left)),
			y: Math.min(window.innerHeight - 280, rect.bottom + 8)
		};
	}

	function hideRecordTooltip() {
		hoveredTooltip = null;
	}

	// PocketBase-style relation display label:
	// 1. Target collection's presentable=true fields
	// 2. Fallback common label fields: name, title, email, username
	// 3. Otherwise null (shows copyable ID pill instead)
	function getRelationDisplayLabel(targetColName: string | undefined, rec: RecordRow | undefined): string | null {
		if (!rec) return null;
		const targetCol = targetColName ? collectionMap[targetColName] : undefined;
		if (targetCol) {
			const presentable = (targetCol.fields ?? [])
				.filter((f) => f.presentable)
				.map((f) => rec[f.name])
				.filter((v) => v !== null && v !== undefined && String(v).trim() !== '')
				.map((v) => String(v).trim());
			if (presentable.length > 0) {
				return presentable.join(', ');
			}
		}
		for (const key of ['name', 'title', 'email', 'username']) {
			const v = rec[key];
			if (typeof v === 'string' && v.trim() !== '') {
				return v.trim();
			}
		}
		return null;
	}

	function getRelationPills(
		row: RecordRow,
		col: string
	): Array<{ id: string; label: string | null; record?: RecordRow }> {
		const raw = row[col];
		if (raw === null || raw === undefined || raw === '') return [];
		const targetColName = fieldMap[col]?.collection_name;
		const expandObj = (row.expand as Record<string, unknown> | undefined)?.[col];

		const ids: string[] = Array.isArray(raw)
			? raw.map((x) => String(x)).filter(Boolean)
			: [String(raw)];

		const expandedById: Record<string, RecordRow> = {};
		if (Array.isArray(expandObj)) {
			for (const item of expandObj) {
				if (item && typeof item === 'object' && 'id' in item) {
					expandedById[String((item as RecordRow).id)] = item as RecordRow;
				}
			}
		} else if (expandObj && typeof expandObj === 'object' && 'id' in expandObj) {
			expandedById[String((expandObj as RecordRow).id)] = expandObj as RecordRow;
		}

		return ids.map((id) => {
			const rec = expandedById[id];
			return {
				id,
				label: getRelationDisplayLabel(targetColName, rec),
				record: rec
			};
		});
	}

	function getSelectPills(val: unknown): string[] {
		if (val === null || val === undefined || val === '') return [];
		if (Array.isArray(val)) return val.map((v) => String(v)).filter(Boolean);
		return [String(val)];
	}

	function formatCell(val: unknown): string {
		if (val === null || val === undefined || val === '') return 'N/A';
		if (typeof val === 'object') return JSON.stringify(val);
		const str = String(val);
		const iso = str.match(/^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}:\d{2})(?:\.(\d+))?(?:Z|[+-]\d{2}:\d{2})$/);
		if (iso) {
			const ms = iso[3] ? `.${iso[3].slice(0, 3).padEnd(3, '0')}` : '';
			return `${iso[1]} ${iso[2]}${ms}`;
		}
		return str;
	}

	function isImageFileUrl(val: unknown): val is string {
		return typeof val === 'string' && val.startsWith('/api/files/') && /\.(png|jpe?g|gif|webp|svg)$/i.test(val);
	}
</script>

<div class="relative flex-1 overflow-auto">
	<table class="w-full border-collapse text-left text-sm">
		<thead class="sticky top-0 z-10 border-b border-border bg-card/95 text-xs text-muted-foreground backdrop-blur-xs">
			<tr>
				{#if !readOnly && onBulkDelete}
					<th class="w-11 px-4 py-3">
						<button
							type="button"
							aria-label="Select all records"
							onclick={toggleAll}
							class="flex h-4 w-4 cursor-pointer items-center justify-center rounded border transition {allSelected
								? 'border-foreground bg-foreground text-background'
								: 'border-border bg-background hover:border-foreground/60'}"
						>
							{#if allSelected}
								<Check class="h-3 w-3" />
							{/if}
						</button>
					</th>
				{/if}
				{#each columns as col}
					{@const t = getFieldType(col)}
					{@const dir = getSortDir(col)}
					<th class="px-3 py-2 font-semibold whitespace-nowrap">
						{#if onSortChange && collection.type !== 'system'}
							<button
								type="button"
								onclick={() => cycleSort(col)}
								title={dir === 'desc' ? `Sorted ${col} DESC (click for ASC)` : dir === 'asc' ? `Sorted ${col} ASC (click to clear)` : `Sort by ${col} DESC`}
								class="group/sort inline-flex cursor-pointer items-center gap-1.5 rounded px-2 py-1 transition hover:bg-muted {dir ? 'text-foreground' : 'text-muted-foreground hover:text-foreground'}"
							>
								{#if t === 'id'}
									<KeyRound class="h-3.5 w-3.5" />
								{:else if t === 'email'}
									<Mail class="h-3.5 w-3.5" />
								{:else if t === 'relation'}
									<Share2 class="h-3.5 w-3.5" />
								{:else if t === 'select'}
									<List class="h-3.5 w-3.5" />
								{:else if t === 'file'}
									<FileImage class="h-3.5 w-3.5" />
								{:else if t === 'json'}
									<Braces class="h-3.5 w-3.5" />
								{:else if t === 'integer' || t === 'float' || t === 'number'}
									<Hash class="h-3.5 w-3.5" />
								{:else if t === 'boolean'}
									<ToggleLeft class="h-3.5 w-3.5" />
								{:else if t === 'date' || t === 'timestamptz' || t === 'datetime' || t === 'autodate'}
									<Calendar class="h-3.5 w-3.5" />
								{:else}
									<Type class="h-3.5 w-3.5" />
								{/if}
								<span>{col}</span>
								<span class="inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center">
									{#if dir === 'desc'}
										<ArrowDown class="h-3.5 w-3.5 text-primary" />
									{:else if dir === 'asc'}
										<ArrowUp class="h-3.5 w-3.5 text-primary" />
									{:else}
										<ArrowUpDown class="h-3 w-3 opacity-0 transition group-hover/sort:opacity-60" />
									{/if}
								</span>
							</button>
						{:else}
							<span class="inline-flex items-center gap-1.5 px-2 py-1">
								{#if t === 'id'}
									<KeyRound class="h-3.5 w-3.5" />
								{:else if t === 'email'}
									<Mail class="h-3.5 w-3.5" />
								{:else if t === 'relation'}
									<Share2 class="h-3.5 w-3.5" />
								{:else if t === 'select'}
									<List class="h-3.5 w-3.5" />
								{:else if t === 'file'}
									<FileImage class="h-3.5 w-3.5" />
								{:else if t === 'json'}
									<Braces class="h-3.5 w-3.5" />
								{:else if t === 'integer' || t === 'float' || t === 'number'}
									<Hash class="h-3.5 w-3.5" />
								{:else if t === 'boolean'}
									<ToggleLeft class="h-3.5 w-3.5" />
								{:else if t === 'date' || t === 'timestamptz' || t === 'datetime' || t === 'autodate'}
									<Calendar class="h-3.5 w-3.5" />
								{:else}
									<Type class="h-3.5 w-3.5" />
								{/if}
								{col}
							</span>
						{/if}
					</th>
				{/each}
				{#if !readOnly}
					<th class="w-10 px-3 py-3 text-right font-medium"></th>
				{/if}
			</tr>
		</thead>
		<tbody class="divide-y divide-border transition-opacity duration-150 {showSlowLoading && items.length > 0 ? 'opacity-65' : 'opacity-100'}">
			{#if loading && items.length === 0}
				<tr>
					<td colspan={columns.length + 2} class="px-5 py-12 text-center text-xs text-muted-foreground">
						Loading records...
					</td>
				</tr>
			{:else if items.length === 0}
				<tr>
					<td colspan={columns.length + 2} class="px-5 py-12 text-center text-xs text-muted-foreground">
						No records found.
					</td>
				</tr>
			{:else}
				{#each items as row, idx (row.id ?? idx)}
					{@const rid = typeof row.id === 'number' || typeof row.id === 'string' ? row.id : null}
					{@const isChecked = rid !== null && selectedIds.includes(rid)}
					<tr
						onclick={() => !readOnly && onSelect?.(row)}
						class="group transition {isChecked ? 'bg-muted/75' : 'bg-card hover:bg-muted/50'} {!readOnly && onSelect ? 'cursor-pointer' : ''}"
					>
						{#if !readOnly && onBulkDelete}
							<td class="w-11 px-4 py-[var(--row-py)]" onclick={(e) => e.stopPropagation()}>
								{#if rid !== null}
									<button
										type="button"
										aria-label="Select record {rid}"
										onclick={() => toggleOne(rid)}
										class="flex h-4 w-4 cursor-pointer items-center justify-center rounded border transition {isChecked
											? 'border-foreground bg-foreground text-background'
											: 'border-border bg-background hover:border-foreground/60'}"
									>
										{#if isChecked}
											<Check class="h-3 w-3" />
										{/if}
									</button>
								{/if}
							</td>
						{/if}

						{#each columns as col}
							{@const t = getFieldType(col)}
							{@const val = row[col]}
							<td class="max-w-xs min-w-28 px-5 py-[var(--row-py)] whitespace-nowrap">
								{#if t === 'id' && val !== null && val !== undefined}
									<button
										type="button"
										title="Copy ID"
										onclick={(e) => copyText(e, String(val))}
										class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-secondary px-2 py-1 font-mono text-xs text-foreground transition hover:bg-secondary/80"
									>
										<Copy class="h-3 w-3 text-muted-foreground" />
										<span>{String(val)}</span>
									</button>
								{:else if t === 'boolean'}
									{#if val === true || val === 'true'}
										<span class="inline-flex rounded-md bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">
											True
										</span>
									{:else if val === false || val === 'false'}
										<span class="inline-flex rounded-md bg-secondary px-2 py-0.5 text-xs font-medium text-muted-foreground">
											False
										</span>
									{:else}
										<span class="text-muted-foreground/60">N/A</span>
									{/if}
								{:else if t === 'select'}
									{@const pills = getSelectPills(val)}
									{#if pills.length === 0}
										<span class="text-muted-foreground/60">N/A</span>
									{:else}
										<div class="inline-flex flex-wrap items-center gap-1">
											{#each pills as pill (pill)}
												<span class="inline-flex rounded-md bg-secondary px-2 py-0.5 text-xs font-medium text-foreground">
													{pill}
												</span>
											{/each}
										</div>
									{/if}
								{:else if t === 'relation'}
									{@const rels = getRelationPills(row, col)}
									{#if rels.length === 0}
										<span class="text-muted-foreground/60">N/A</span>
									{:else}
										<div class="inline-flex flex-wrap items-center gap-1.5">
											{#each rels.slice(0, 3) as rel (rel.id)}
												<span
													onclick={(e) => e.stopPropagation()}
													role="presentation"
													class="inline-flex items-center gap-1.5 rounded-md bg-secondary px-2 py-1 text-xs text-foreground"
												>
													<span
														role="presentation"
														onmouseenter={(e) => showRecordTooltip(e, rel.record)}
														onmouseleave={hideRecordTooltip}
														class="inline-flex cursor-help items-center text-muted-foreground transition hover:text-foreground"
													>
														<Eye class="h-3.5 w-3.5" />
													</span>
													{#if rel.label}
														<span class="max-w-44 truncate font-medium">{rel.label}</span>
													{:else}
														<button
															type="button"
															title="Copy ID"
															onclick={(e) => copyText(e, rel.id)}
															class="inline-flex cursor-pointer items-center gap-1 rounded bg-background/80 px-1.5 py-0.5 font-mono text-xs text-foreground transition hover:bg-background"
														>
															<Copy class="h-3 w-3 text-muted-foreground" />
															<span>{rel.id}</span>
														</button>
													{/if}
												</span>
											{/each}
											{#if rels.length > 3}
												<span class="rounded-md bg-secondary px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
													+{rels.length - 3}
												</span>
											{/if}
										</div>
									{/if}
								{:else if isImageFileUrl(val)}
									<img src={val} alt={col} class="h-8 w-8 rounded border border-border object-cover" />
								{:else if val === null || val === undefined || val === ''}
									<span class="text-muted-foreground/60">N/A</span>
								{:else}
									<span class="block max-w-xs truncate {t === 'json' || t === 'date' || t === 'timestamptz' || t === 'datetime' || t === 'autodate' ? 'font-mono text-xs' : ''}">
										{formatCell(val)}
									</span>
								{/if}
							</td>
						{/each}

						{#if !readOnly}
							<td class="w-10 px-3 py-[var(--row-py)] text-right">
								<span class="inline-flex items-center justify-center text-muted-foreground/60 transition group-hover:translate-x-0.5 group-hover:text-foreground">
									<ArrowRight class="h-4 w-4" />
								</span>
							</td>
						{/if}
					</tr>
				{/each}
			{/if}
		</tbody>
	</table>

	<!-- PocketBase-style floating bulk actions bar -->
	{#if selectedIds.length > 0 && onBulkDelete}
		<div
			transition:fly={{ y: 20, duration: 180, easing: cubicOut }}
			class="sticky bottom-4 left-1/2 z-20 mx-auto flex w-fit items-center gap-4 rounded-full border border-border bg-card px-5 py-2.5 text-xs shadow-lg"
		>
			<span>
				Selected <strong class="text-foreground">{selectedIds.length}</strong>
				{selectedIds.length === 1 ? 'record' : 'records'}
			</span>
			<button
				type="button"
				onclick={() => (selectedIds = [])}
				class="cursor-pointer rounded-full px-2.5 py-1 text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				Reset
			</button>
			<button
				type="button"
				onclick={() => {
					const toDelete = [...selectedIds];
					selectedIds = [];
					onBulkDelete(toDelete);
				}}
				class="inline-flex cursor-pointer items-center gap-1.5 rounded-full bg-destructive px-3.5 py-1 font-semibold text-white transition hover:opacity-90"
			>
				<Trash2 class="h-3.5 w-3.5" />
				Delete selected
			</button>
		</div>
	{/if}
</div>

{#if hoveredTooltip}
	<div
		style="left: {hoveredTooltip.x}px; top: {hoveredTooltip.y}px;"
		class="pointer-events-none fixed z-70 max-h-72 w-80 overflow-hidden rounded-lg border border-zinc-700 bg-zinc-900 p-3 text-zinc-100 shadow-2xl"
	>
		<pre class="overflow-auto font-mono text-xs leading-relaxed whitespace-pre-wrap break-all">{hoveredTooltip.json}</pre>
	</div>
{/if}
