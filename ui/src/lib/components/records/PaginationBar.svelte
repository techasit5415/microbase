<script lang="ts">
	import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from '@lucide/svelte';

	interface Props {
		page: number;
		perPage: number;
		totalItems: number;
		onPageChange: (nextPage: number) => void;
		onPerPageChange?: (nextPerPage: number) => void;
	}

	let { page, perPage, totalItems, onPageChange, onPerPageChange }: Props = $props();
	let totalPages = $derived(Math.max(1, Math.ceil(totalItems / perPage)));
	let rangeStart = $derived(totalItems === 0 ? 0 : (page - 1) * perPage + 1);
	let rangeEnd = $derived(Math.min(totalItems, page * perPage));

	let pageNumbers = $derived.by<Array<number | 'ellipsis'>>(() => {
		if (totalPages <= 7) {
			return Array.from({ length: totalPages }, (_, i) => i + 1);
		}
		const items: Array<number | 'ellipsis'> = [1];
		const start = Math.max(2, page - 1);
		const end = Math.min(totalPages - 1, page + 1);
		if (start > 2) items.push('ellipsis');
		for (let p = start; p <= end; p++) items.push(p);
		if (end < totalPages - 1) items.push('ellipsis');
		items.push(totalPages);
		return items;
	});

	const perPageOptions = [10, 20, 30, 50, 100];
</script>

<footer class="flex min-h-14 shrink-0 flex-wrap items-center justify-between gap-3 border-t border-border bg-card/60 px-5 py-2 text-xs text-muted-foreground">
	<div class="flex items-center gap-3">
		{#if totalItems > 0}
			<span>
				Showing <strong class="text-foreground">{rangeStart}&ndash;{rangeEnd}</strong> of <strong class="text-foreground">{totalItems}</strong> records
			</span>
		{:else}
			<span>Total: <strong class="text-foreground">0</strong> records</span>
		{/if}

		{#if onPerPageChange}
			<label class="inline-flex items-center gap-1.5">
				<span class="hidden sm:inline">Per page:</span>
				<select
					aria-label="Rows per page"
					value={perPage}
					onchange={(e) => onPerPageChange?.(Number((e.currentTarget as HTMLSelectElement).value))}
					class="cursor-pointer rounded-md border border-border bg-background px-2 py-1 font-mono text-xs text-foreground outline-none focus:border-ring"
				>
					{#each perPageOptions as opt (opt)}
						<option value={opt}>{opt}</option>
					{/each}
				</select>
			</label>
		{/if}
	</div>

	<div class="flex items-center gap-1">
		<button
			type="button"
			aria-label="First page"
			title="First page"
			disabled={page <= 1}
			onclick={() => onPageChange(1)}
			class="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md border border-border transition hover:bg-accent disabled:cursor-not-allowed disabled:opacity-40"
		>
			<ChevronsLeft class="h-3.5 w-3.5" />
		</button>

		<button
			type="button"
			disabled={page <= 1}
			onclick={() => onPageChange(page - 1)}
			class="inline-flex h-7 cursor-pointer items-center gap-1 rounded-md border border-border px-2.5 transition hover:bg-accent disabled:cursor-not-allowed disabled:opacity-40"
		>
			<ChevronLeft class="h-3.5 w-3.5" /> Prev
		</button>

		<div class="mx-1 flex items-center gap-1">
			{#each pageNumbers as item, idx (`${item}-${idx}`)}
				{#if item === 'ellipsis'}
					<span class="px-1 text-muted-foreground">&hellip;</span>
				{:else}
					<button
						type="button"
						aria-label={`Page ${item}`}
						aria-current={item === page ? 'page' : undefined}
						onclick={() => {
							if (item !== page) onPageChange(item);
						}}
						class="inline-flex h-7 min-w-7 cursor-pointer items-center justify-center rounded-md px-2 font-mono text-xs font-medium transition {item === page
							? 'bg-primary text-primary-foreground shadow-xs'
							: 'border border-border bg-background text-foreground hover:bg-accent'}"
					>
						{item}
					</button>
				{/if}
			{/each}
		</div>

		<button
			type="button"
			disabled={page >= totalPages}
			onclick={() => onPageChange(page + 1)}
			class="inline-flex h-7 cursor-pointer items-center gap-1 rounded-md border border-border px-2.5 transition hover:bg-accent disabled:cursor-not-allowed disabled:opacity-40"
		>
			Next <ChevronRight class="h-3.5 w-3.5" />
		</button>

		<button
			type="button"
			aria-label="Last page"
			title="Last page"
			disabled={page >= totalPages}
			onclick={() => onPageChange(totalPages)}
			class="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md border border-border transition hover:bg-accent disabled:cursor-not-allowed disabled:opacity-40"
		>
			<ChevronsRight class="h-3.5 w-3.5" />
		</button>
	</div>
</footer>
