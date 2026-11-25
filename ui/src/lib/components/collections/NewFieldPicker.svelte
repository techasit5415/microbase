<script lang="ts">
	import {
		Braces,
		Calendar,
		CalendarCheck,
		FileImage,
		Hash,
		Link2,
		List,
		Mail,
		MapPin,
		PenLine,
		Plus,
		Share2,
		Sparkles,
		ToggleLeft,
		Type
	} from '@lucide/svelte';
	import type { FieldType } from '#lib/api/types.ts';

	interface Props {
		onPick: (type: FieldType) => void;
	}

	let { onPick }: Props = $props();
	let open = $state(false);
	let wrapperEl = $state<HTMLDivElement | null>(null);

	const options: { type: FieldType; label: string }[] = [
		{ type: 'text', label: 'Plain text' },
		{ type: 'editor', label: 'Rich editor' },
		{ type: 'number', label: 'Number' },
		{ type: 'boolean', label: 'Bool' },
		{ type: 'email', label: 'Email' },
		{ type: 'url', label: 'URL' },
		{ type: 'datetime', label: 'Datetime' },
		{ type: 'autodate', label: 'Autodate' },
		{ type: 'file', label: 'File' },
		{ type: 'relation', label: 'Relation' },
		{ type: 'select', label: 'Select' },
		{ type: 'json', label: 'JSON' },
		{ type: 'geopoint', label: 'Geo Point' },
		{ type: 'vector', label: 'Vector' }
	];

	function handleWindowClick(e: MouseEvent) {
		if (open && wrapperEl && !wrapperEl.contains(e.target as Node)) {
			open = false;
		}
	}
</script>

<svelte:window onclick={handleWindowClick} />

<div bind:this={wrapperEl} class="relative mt-3">
	<button
		type="button"
		onclick={() => (open = !open)}
		class="flex w-full cursor-pointer items-center justify-center gap-1.5 rounded-md border-2 border-foreground/80 bg-muted/40 py-2.5 text-xs font-semibold text-foreground transition hover:bg-accent"
	>
		<Plus class="h-4 w-4" />
		New field
	</button>

	{#if open}
		<div
			class="mt-1 grid grid-cols-2 gap-1 rounded-md border-2 border-foreground/80 bg-popover p-2 text-popover-foreground shadow-xl sm:grid-cols-4"
		>
			{#each options as opt (opt.type)}
				<button
					type="button"
					onclick={() => {
						onPick(opt.type);
						open = false;
					}}
					class="flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-2 text-left text-xs font-medium text-foreground transition hover:bg-accent"
				>
					{#if opt.type === 'text'}
						<Type class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'editor'}
						<PenLine class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'number'}
						<Hash class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'boolean'}
						<ToggleLeft class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'email'}
						<Mail class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'url'}
						<Link2 class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'datetime'}
						<Calendar class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'autodate'}
						<CalendarCheck class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'file'}
						<FileImage class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'relation'}
						<Share2 class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'select'}
						<List class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'json'}
						<Braces class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else if opt.type === 'geopoint'}
						<MapPin class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					{:else}
						<Sparkles class="h-3.5 w-3.5 shrink-0 text-pink-500" />
					{/if}
					<span class="truncate">{opt.label}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
