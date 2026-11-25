<script lang="ts">
	import { Plus, Search } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';
	import CollectionItem from './CollectionItem.svelte';

	interface Props {
		collections: Collection[];
		activeName: string | null;
		onSelect: (name: string) => void;
		onDelete: (name: string) => void;
		onOpenCreate: () => void;
	}

	let { collections, activeName, onSelect, onDelete, onOpenCreate }: Props = $props();
	let filter = $state('');

	let filtered = $derived(
		collections.filter((c) => c.name.toLowerCase().includes(filter.trim().toLowerCase()))
	);
</script>

<aside class="flex w-60 shrink-0 flex-col border-r border-border bg-card/50">
	<div class="flex items-center justify-between border-b border-border px-3 py-3">
		<span class="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
			Collections
		</span>
		<button
			type="button"
			onclick={onOpenCreate}
			class="inline-flex items-center gap-1 rounded-md bg-primary px-2 py-1 text-xs font-medium text-primary-foreground transition hover:opacity-90"
		>
			<Plus class="h-3.5 w-3.5" />
			New
		</button>
	</div>

	<div class="p-2.5">
		<div class="relative">
			<Search class="pointer-events-none absolute top-2.5 left-2.5 h-3.5 w-3.5 text-muted-foreground" />
			<input
				type="text"
				bind:value={filter}
				placeholder="Filter tables..."
				class="w-full rounded-md border border-input bg-background py-1.5 pr-2.5 pl-8 text-xs outline-none focus:ring-1 focus:ring-ring"
			/>
		</div>
	</div>

	<div class="flex flex-1 flex-col gap-0.5 overflow-y-auto px-2 pb-3">
		{#if filtered.length === 0}
			<p class="px-2 py-6 text-center text-xs text-muted-foreground">No collections found</p>
		{:else}
			{#each filtered as col (col.id)}
				<CollectionItem
					collection={col}
					active={activeName === col.name}
					{onSelect}
					{onDelete}
				/>
			{/each}
		{/if}
	</div>
</aside>