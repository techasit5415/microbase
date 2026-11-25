<script lang="ts">
	import { Code2, Plus, RefreshCw, Search, Settings } from '@lucide/svelte';

	interface Props {
		collectionName: string;
		search: string;
		onSearchChange: (value: string) => void;
		onRefresh: () => void;
		onOpenEditCollection: () => void;
		onOpenApiPreview: () => void;
		onOpenCreateRecord: () => void;
	}

	let {
		collectionName,
		search,
		onSearchChange,
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

	<div class="relative">
		<Search class="pointer-events-none absolute top-2.5 left-3.5 h-4 w-4 text-muted-foreground" />
		<input
			type="text"
			value={search}
			oninput={(e) => onSearchChange(e.currentTarget.value)}
			placeholder="Search term or filter..."
			class="w-full rounded-full border border-border bg-muted/40 py-2 pr-4 pl-10 text-xs outline-none placeholder:text-muted-foreground focus:border-ring focus:bg-background"
		/>
	</div>
</header>
