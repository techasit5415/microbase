<script lang="ts">
	import { Calendar, Hash, KeyRound, Mail, ToggleLeft, Trash2, Type } from '@lucide/svelte';
	import type { Collection, RecordRow } from '#lib/api/types.ts';

	interface Props {
		collection: Collection;
		items: RecordRow[];
		loading: boolean;
		readOnly?: boolean;
		onSelect?: (row: RecordRow) => void;
		onDelete: (id: number) => void;
	}

	let { collection, items, loading, readOnly = false, onSelect, onDelete }: Props = $props();

	let columns = $derived.by(() => {
		if (collection.type === 'system') {
			return items.length > 0 ? Object.keys(items[0]) : ['id'];
		}
		return ['id', ...(collection.fields ?? []).map((f) => f.name), 'created_at'];
	});

	function getFieldType(col: string): string {
		if (col === 'id') return 'id';
		if (col === 'email') return 'email';
		if (col === 'created_at' || col === 'updated_at') return 'date';
		return (collection.fields ?? []).find((f) => f.name === col)?.type ?? 'text';
	}

	function formatCell(val: unknown): string {
		if (val === null || val === undefined) return 'N/A';
		if (typeof val === 'object') return JSON.stringify(val);
		return String(val);
	}
</script>

<div class="flex-1 overflow-auto">
	<table class="w-full border-collapse text-left text-[13px]">
		<thead class="sticky top-0 border-b border-border bg-background/95 text-xs text-muted-foreground backdrop-blur-xs">
			<tr>
				{#each columns as col}
					{@const t = getFieldType(col)}
					<th class="px-5 py-3 font-medium whitespace-nowrap">
						<span class="inline-flex items-center gap-1.5">
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
					<td colspan={columns.length + 1} class="px-5 py-12 text-center text-xs text-muted-foreground">
						Loading records...
					</td>
				</tr>
			{:else if items.length === 0}
				<tr>
					<td colspan={columns.length + 1} class="px-5 py-12 text-center text-xs text-muted-foreground">
						No records found.
					</td>
				</tr>
			{:else}
				{#each items as row, idx (row.id ?? idx)}
					<tr onclick={() => !readOnly && onSelect?.(row)} class="transition hover:bg-muted/40 {!readOnly && onSelect ? 'cursor-pointer' : ''}">
						{#each columns as col}
							{@const val = row[col]}
							<td class="max-w-xs truncate px-5 py-3">
								{#if col === 'id'}
									<span class="rounded bg-muted px-2 py-0.5 font-mono text-xs text-foreground">
										{val}
									</span>
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
						{#if !readOnly && typeof row.id === 'number'}
							<td class="px-5 py-3 text-right">
								<button
									type="button"
									title="Delete record"
									onclick={(e) => { e.stopPropagation(); onDelete(row.id as number); }}
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
</div>
