<script lang="ts">
	import { Folder, Table2, Trash2, Users } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';

	interface Props {
		collection: Collection;
		active: boolean;
		onSelect: (name: string) => void;
		onDelete?: (name: string) => void;
	}

	let { collection, active, onSelect, onDelete }: Props = $props();
	let isAuth = $derived(collection.type === 'auth');
	let isView = $derived(collection.type === 'view');
</script>

<div
	class="group relative flex h-9 shrink-0 items-center rounded-md text-[13px] transition {active
		? 'bg-accent font-medium text-foreground'
		: 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'}"
>
	<button
		type="button"
		onclick={() => onSelect(collection.name)}
		class="flex h-full w-full min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-md px-3 text-left {!isAuth && onDelete ? 'group-hover:pr-8' : ''}"
	>
		{#if isAuth}
			<Users class="h-4 w-4 shrink-0" />
		{:else if isView}
			<Table2 class="h-4 w-4 shrink-0" />
		{:else}
			<Folder class="h-4 w-4 shrink-0" />
		{/if}
		<span class="truncate">{collection.name}</span>
	</button>

	{#if !isAuth && onDelete}
		<button
			type="button"
			title="Delete collection"
			onclick={(e) => {
				e.stopPropagation();
				onDelete(collection.name);
			}}
			class="pointer-events-none absolute right-1.5 top-1/2 inline-flex -translate-y-1/2 cursor-pointer rounded p-1 text-muted-foreground opacity-0 transition hover:bg-destructive/15 hover:text-destructive group-hover:pointer-events-auto group-hover:opacity-100"
		>
			<Trash2 class="h-3.5 w-3.5" />
		</button>
	{/if}
</div>
