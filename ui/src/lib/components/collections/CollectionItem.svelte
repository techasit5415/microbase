<script lang="ts">
	import { Table2, Trash2 } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';

	interface Props {
		collection: Collection;
		active: boolean;
		onSelect: (name: string) => void;
		onDelete: (name: string) => void;
	}

	let { collection, active, onSelect, onDelete }: Props = $props();
</script>

<div
	class="group flex items-center justify-between rounded-md px-2.5 py-2 text-sm transition {active
		? 'bg-accent font-medium text-accent-foreground'
		: 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'}"
>
	<button
		type="button"
		onclick={() => onSelect(collection.name)}
		class="flex min-w-0 flex-1 items-center gap-2 text-left"
	>
		<Table2 class="h-4 w-4 shrink-0" />
		<span class="truncate">{collection.name}</span>
	</button>

	<div class="flex items-center gap-1.5">
		<span class="rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
			{collection.fields.length}f
		</span>
		<button
			type="button"
			title="Delete collection"
			onclick={() => onDelete(collection.name)}
			class="hidden rounded p-1 text-muted-foreground hover:bg-destructive/15 hover:text-destructive group-hover:inline-flex"
		>
			<Trash2 class="h-3.5 w-3.5" />
		</button>
	</div>
</div>