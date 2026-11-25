<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { fly } from 'svelte/transition';
	import { ArrowDown, ArrowUp, ArrowUpDown, Calendar, Check, Hash, KeyRound, Mail, ToggleLeft, Trash2, Type } from '@lucide/svelte';
	import type { Collection, RecordRow } from '#lib/api/types.ts';

	interface Props {
		collection: Collection;
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
		items,
		loading,
		readOnly = false,
		sort = '',
		onSortChange,
		onSelect,
		onDelete,
		onBulkDelete
	}: Props = $props();

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

	let selectedIds = $state<(number | string)[]>([]);

	// Clear selection when collection or items change
	$effect(() => {
		collection.name;
		items;
		selectedIds = [];
	});

	let columns = $derived.by(() => {
		if (collection.type === 'system' || collection.type === 'view') {
			return items.length > 0 ? Object.keys(items[0]) : ['id'];
		}
		if (collection.type === 'auth' || collection.name === 'users') {
			const custom = (collection.fields ?? [])
				.map((f) => f.name)
				.filter((n) => !['id', 'email', 'verified', 'email_visibility', 'created_at', 'updated_at'].includes(n));
			return ['id', 'email', 'verified', ...custom, 'created_at'];
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
		if (col === 'created_at' || col === 'updated_at') return 'date';
		return (collection.fields ?? []).find((f) => f.name === col)?.type ?? 'text';
	}

	function formatCell(val: unknown): string {
		if (val === null || val === undefined) return 'N/A';
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
	<table class="w-full border-collapse text-left text-[13px]">
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
								{#if dir === 'desc'}
									<ArrowDown class="h-3.5 w-3.5 text-primary" />
								{:else if dir === 'asc'}
									<ArrowUp class="h-3.5 w-3.5 text-primary" />
								{:else}
									<ArrowUpDown class="h-3 w-3 opacity-0 transition group-hover/sort:opacity-60" />
								{/if}
							</button>
						{:else}
							<span class="inline-flex items-center gap-1.5 px-2 py-1">
								{#if t === 'id'}
									<KeyRound class="h-3.5 w-3.5" />
								{:else if t === 'email'}
									<Mail class="h-3.5 w-3.5" />
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
					<th class="w-16 px-5 py-3 text-right font-medium"></th>
				{/if}
			</tr>
		</thead>
		<tbody class="divide-y divide-border">
			{#if loading}
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
						class="transition {isChecked ? 'bg-muted/75' : 'bg-card hover:bg-muted/60'} {!readOnly && onSelect ? 'cursor-pointer' : ''}"
					>
						{#if !readOnly && onBulkDelete}
							<td class="px-4 py-3">
								{#if rid !== null}
									<button
										type="button"
										aria-label="Select record"
										onclick={(e) => {
											e.stopPropagation();
											toggleOne(rid);
										}}
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
							{@const val = row[col]}
							<td class="max-w-xs truncate px-5 py-3">
								{#if col === 'id'}
									<span class="rounded bg-muted px-2 py-0.5 font-mono text-xs text-foreground">
										{val}
									</span>
								{:else if isImageFileUrl(val)}
									<img
										src="{val}?thumb=64x64"
										alt={col}
										class="h-8 w-8 rounded border border-border object-cover"
									/>
								{:else if typeof val === 'boolean'}
									<span
										class="rounded px-2 py-0.5 text-xs font-medium {val
											? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
											: 'bg-muted text-muted-foreground'}"
									>
										{val ? 'True' : 'False'}
									</span>
								{:else}
									<span class="text-xs text-foreground">{formatCell(val)}</span>
								{/if}
							</td>
						{/each}
						{#if !readOnly && rid !== null}
							<td class="px-5 py-3 text-right">
								<button
									type="button"
									title="Delete record"
									onclick={(e) => {
										e.stopPropagation();
										onDelete(rid);
									}}
									class="cursor-pointer rounded p-1.5 text-muted-foreground transition hover:bg-destructive/15 hover:text-destructive"
								>
									<Trash2 class="h-3.5 w-3.5" />
								</button>
							</td>
						{/if}
					</tr>
				{/each}
			{/if}
		</tbody>
	</table>

	<!-- Floating PocketBase Bulkbar -->
	{#if !readOnly && onBulkDelete && selectedIds.length > 0}
		<div transition:fly={{ y: 24, duration: 180, easing: cubicOut }} class="sticky bottom-4 z-20 mx-auto mt-4 flex w-fit items-center gap-3 rounded-full border border-border bg-foreground px-5 py-2.5 text-xs text-background shadow-xl">
			<span>Selected <strong>{selectedIds.length}</strong> record{selectedIds.length > 1 ? 's' : ''}</span>
			<button
				type="button"
				onclick={() => (selectedIds = [])}
				class="cursor-pointer rounded-full bg-background/15 px-3 py-1 text-xs font-medium transition hover:bg-background/25"
			>
				Reset
			</button>
			<button
				type="button"
				onclick={() => onBulkDelete([...selectedIds])}
				class="inline-flex cursor-pointer items-center gap-1.5 rounded-full bg-destructive px-3.5 py-1 text-xs font-semibold text-white transition hover:opacity-90"
			>
				<Trash2 class="h-3.5 w-3.5" />
				Delete selected
			</button>
		</div>
	{/if}
</div>
