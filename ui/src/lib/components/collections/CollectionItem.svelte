<script lang="ts">
	import { Folder, Pin, Table2, Trash2, Users } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';

	interface Props {
		collection: Collection;
		active: boolean;
		pinned?: boolean;
		onSelect: (name: string) => void;
		onTogglePin?: (name: string) => void;
		onDelete?: (name: string) => void;
	}

	let { collection, active, pinned = false, onSelect, onTogglePin, onDelete }: Props = $props();
	let isAuth = $derived(collection.type === 'auth');
	let isView = $derived(collection.type === 'view');
</script>

<div
	class="group relative flex h-9 shrink-0 items-center rounded-md text-[length:var(--ui-sidebar-size)] transition {active
		? 'bg-accent font-medium text-foreground'
		: 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'}"
>
	<button
		type="button"
		onclick={() => onSelect(collection.name)}
		class="flex h-full w-full min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-md px-3 text-left {onDelete && onTogglePin
			? 'group-hover:pr-14'
			: onDelete || onTogglePin
				? 'group-hover:pr-8'
				: ''} {pinned ? 'pr-8' : ''}"
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

	{#if onTogglePin}
		<button
			type="button"
			title={pinned ? 'Unpin collection' : 'Pin collection'}
			onclick={(e) => {
				e.stopPropagation();
				onTogglePin(collection.name);
			}}
			class="absolute top-1/2 inline-flex -translate-y-1/2 cursor-pointer rounded p-1 transition {onDelete
				? 'right-7'
				: 'right-1.5'} {pinned
				? 'text-primary opacity-100 hover:bg-accent'
				: 'pointer-events-none text-muted-foreground opacity-0 hover:bg-accent hover:text-foreground group-hover:pointer-events-auto group-hover:opacity-100'}"
		>
			<Pin class="h-3.5 w-3.5 {pinned ? 'fill-current' : ''}" />
		</button>
	{/if}

	{#if onDelete}
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
