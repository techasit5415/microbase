<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { slide } from 'svelte/transition';
	import { ChevronUp, Folder, Plus, Search, Users } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';
	import { appSettings } from '#lib/stores/appSettings.svelte.ts';
	import CollectionItem from './CollectionItem.svelte';

	interface Props {
		collections: Collection[];
		systemTables: Collection[];
		activeName: string | null;
		onSelect: (name: string) => void;
		onDelete: (name: string) => void;
		onOpenCreate: () => void;
	}

	let { collections, systemTables, activeName, onSelect, onDelete, onOpenCreate }: Props = $props();
	let filter = $state('');
	let showSystem = $state(false);

	let filtered = $derived(
		collections.filter((c) => c.name.toLowerCase().includes(filter.trim().toLowerCase()))
	);
</script>

<aside class="flex h-full min-h-0 w-60 shrink-0 flex-col overflow-hidden border-r border-border bg-card">
	<div class="shrink-0 border-b border-border p-3">
		<div class="relative">
			<Search class="pointer-events-none absolute top-2.5 left-2.5 h-3.5 w-3.5 text-muted-foreground" />
			<input
				type="text"
				bind:value={filter}
				placeholder="Search collections..."
				class="w-full rounded-md border border-transparent bg-transparent py-1.5 pr-2.5 pl-8 text-xs outline-none placeholder:text-muted-foreground focus:border-border focus:bg-background"
			/>
		</div>
	</div>

	<div class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-2.5">
		{#each filtered as col (col.name)}
			<CollectionItem
				collection={col}
				active={activeName === col.name}
				{onSelect}
				onDelete={col.type === 'auth' || appSettings.data.hide_controls ? undefined : onDelete}
			/>
		{/each}

		<!-- Collapsible System section (PocketBase style) -->
		<div class="mt-4 shrink-0 pt-2">
			<button
				type="button"
				onclick={() => (showSystem = !showSystem)}
				class="flex w-full cursor-pointer items-center gap-1.5 px-3 py-1.5 text-xs font-semibold text-muted-foreground transition hover:text-foreground"
			>
				<span>System</span>
				<ChevronUp class="h-3.5 w-3.5 transition-transform {showSystem ? '' : 'rotate-180'}" />
			</button>

			{#if showSystem}
				<div class="mt-1 flex flex-col gap-0.5">
					{#each systemTables as sys (sys.name)}
						<button
							type="button"
							onclick={() => onSelect(sys.name)}
							class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-3 py-2 text-left text-[13px] transition {activeName ===
							sys.name
								? 'bg-accent font-medium text-foreground'
								: 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'}"
						>
							{#if sys.name === '_superusers'}
								<Users class="h-4 w-4 shrink-0" />
							{:else}
								<Folder class="h-4 w-4 shrink-0" />
							{/if}
							<span class="truncate">{sys.name}</span>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>

	{#if !appSettings.data.hide_controls}
	<div class="flex h-14 shrink-0 items-center border-t border-border px-3">
		<button
			type="button"
			onclick={onOpenCreate}
			class="flex w-full cursor-pointer items-center justify-center gap-1.5 rounded-md border-2 border-primary/80 bg-transparent px-3 py-2 text-xs font-semibold text-primary transition hover:bg-primary hover:text-primary-foreground"
		>
			<Plus class="h-3.5 w-3.5" />
			New collection
		</button>
	</div>
	{/if}
</aside>
