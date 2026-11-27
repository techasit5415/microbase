<script lang="ts">
	import { Code2, Plus, RefreshCw, Search, Settings, X } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';
	import FilterAutocomplete from '#lib/components/ui/FilterAutocomplete.svelte';

	interface Props {
		collectionName: string;
		collection?: Collection | null;
		allCollections?: Collection[];
		search: string;
		onSearchChange: (value: string) => void;
		onSearchSubmit?: (value: string) => void;
		onRefresh: () => void;
		onOpenEditCollection: () => void;
		onOpenApiPreview: () => void;
		onOpenCreateRecord: () => void;
	}

	let {
		collectionName,
		collection = null,
		allCollections = [],
		search,
		onSearchChange,
		onSearchSubmit,
		onRefresh,
		onOpenEditCollection,
		onOpenApiPreview,
		onOpenCreateRecord
	}: Props = $props();

	let spinning = $state(false);
	function triggerRefresh() {
		spinning = true;
		onRefresh();
		setTimeout(() => (spinning = false), 450);
	}
</script>

<header class="space-y-3 border-b border-border px-6 py-4">
	<div class="flex items-center justify-between gap-3">
		<div class="flex items-center gap-2 text-sm">
			<span class="text-muted-foreground">Collections</span>
			<span class="text-muted-foreground/60">/</span>
			<h1 class="font-semibold text-foreground">{collectionName}</h1>

			<button
				type="button"
				onclick={onOpenEditCollection}
				title="Edit collection"
				class="ml-1 cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-accent hover:text-foreground"
			>
				<Settings class="h-4 w-4" />
			</button>

			<button
				type="button"
				onclick={triggerRefresh}
				title="Refresh"
				class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-accent hover:text-foreground"
			>
				<RefreshCw class="h-4 w-4 transition-transform {spinning ? 'animate-spin text-foreground' : ''}" />
			</button>
		</div>

		<div class="flex items-center gap-2">
			<button
				type="button"
				onclick={onOpenApiPreview}
				class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-3 py-2 text-xs font-medium text-foreground transition hover:bg-accent"
			>
				<Code2 class="h-3.5 w-3.5" />
				API Preview
			</button>
			<button
				type="button"
				onclick={onOpenCreateRecord}
			class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-3.5 py-2 text-xs font-medium text-background transition hover:opacity-90"
		>
			<Plus class="h-3.5 w-3.5" />
			New record
			</button>
		</div>
	</div>

	<div class="relative flex items-center gap-2">
		<div class="relative min-w-0 flex-1">
			<Search class="pointer-events-none absolute top-2.5 left-3.5 z-10 h-4 w-4 text-muted-foreground" />
			<FilterAutocomplete
				value={search}
				{collection}
				{allCollections}
				includeMacros={false}
				placeholder='Search term or filter, e.g. email.name = "alice@example.com" && status != "draft"'
				onInput={(v) => onSearchChange(v)}
				onSubmit={(v) => (onSearchSubmit ? onSearchSubmit(v) : onSearchChange(v))}
				inputClass="w-full rounded-full border border-transparent bg-muted/80 py-2 pr-9 pl-10 font-mono text-xs outline-none placeholder:font-sans placeholder:text-muted-foreground focus:border-border focus:bg-accent/70"
			/>
			{#if search.trim() !== ''}
				<button
					type="button"
					title="Clear search"
					onclick={() => {
						onSearchChange('');
						onSearchSubmit?.('');
					}}
					class="absolute top-1.5 right-2.5 z-10 cursor-pointer rounded-full p-1 text-muted-foreground transition hover:bg-accent hover:text-foreground"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			{/if}
		</div>
		{#if onSearchSubmit && search.trim() !== ''}
			<button
				type="button"
				onclick={() => onSearchSubmit(search)}
				class="shrink-0 cursor-pointer rounded-full border border-border bg-card px-3.5 py-1.5 text-xs font-medium text-foreground transition hover:bg-accent"
			>
				Search
			</button>
		{/if}
	</div>
</header>
