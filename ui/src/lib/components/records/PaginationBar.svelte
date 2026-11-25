<script lang="ts">
	import { ChevronLeft, ChevronRight } from '@lucide/svelte';

	interface Props {
		page: number;
		perPage: number;
		totalItems: number;
		onPageChange: (nextPage: number) => void;
	}

	let { page, perPage, totalItems, onPageChange }: Props = $props();
	let totalPages = $derived(Math.max(1, Math.ceil(totalItems / perPage)));
</script>

<footer class="flex h-14 shrink-0 items-center justify-between border-t border-border px-5 text-xs text-muted-foreground">
	<span>Total: <strong class="text-foreground">{totalItems}</strong> records</span>

	<div class="flex items-center gap-2">
		<button
			type="button"
			disabled={page <= 1}
			onclick={() => onPageChange(page - 1)}
			class="inline-flex items-center gap-1 rounded-md border border-border px-2.5 py-1 hover:bg-accent disabled:opacity-40"
		>
			<ChevronLeft class="h-3.5 w-3.5" /> Prev
		</button>

		<span>Page {page} of {totalPages}</span>

		<button
			type="button"
			disabled={page >= totalPages}
			onclick={() => onPageChange(page + 1)}
			class="inline-flex items-center gap-1 rounded-md border border-border px-2.5 py-1 hover:bg-accent disabled:opacity-40"
		>
			Next <ChevronRight class="h-3.5 w-3.5" />
		</button>
	</div>
</footer>