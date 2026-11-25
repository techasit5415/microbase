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
		Settings2,
		Share2,
		Sparkles,
		ToggleLeft,
		Trash2,
		Type
	} from '@lucide/svelte';
	import type { FieldDef } from '#lib/api/types.ts';
	import Switch from '#lib/components/ui/Switch.svelte';

	interface Props {
		field: FieldDef;
		isExisting?: boolean;
		onRemove: () => void;
	}

	let { field = $bindable(), isExisting = false, onRemove }: Props = $props();
	let expanded = $state(false);
</script>

<div class="rounded-md border border-border bg-background/80 transition hover:border-border/90">
	<div class="flex items-center gap-2 px-3 py-2">
		<div class="flex h-7 w-7 shrink-0 items-center justify-center rounded bg-muted text-muted-foreground" title={field.type}>
			{#if field.type === 'editor'}
				<PenLine class="h-3.5 w-3.5" />
			{:else if field.type === 'number' || field.type === 'integer' || field.type === 'float'}
				<Hash class="h-3.5 w-3.5" />
			{:else if field.type === 'boolean'}
				<ToggleLeft class="h-3.5 w-3.5" />
			{:else if field.type === 'email'}
				<Mail class="h-3.5 w-3.5" />
			{:else if field.type === 'url'}
				<Link2 class="h-3.5 w-3.5" />
			{:else if field.type === 'datetime' || field.type === 'timestamptz'}
				<Calendar class="h-3.5 w-3.5" />
			{:else if field.type === 'autodate'}
				<CalendarCheck class="h-3.5 w-3.5" />
			{:else if field.type === 'file'}
				<FileImage class="h-3.5 w-3.5" />
			{:else if field.type === 'relation'}
				<Share2 class="h-3.5 w-3.5" />
			{:else if field.type === 'select'}
				<List class="h-3.5 w-3.5" />
			{:else if field.type === 'json'}
				<Braces class="h-3.5 w-3.5" />
			{:else if field.type === 'geopoint'}
				<MapPin class="h-3.5 w-3.5" />
			{:else if field.type === 'vector'}
				<Sparkles class="h-3.5 w-3.5 text-pink-500" />
			{:else}
				<Type class="h-3.5 w-3.5" />
			{/if}
		</div>

		<input
			type="text"
			bind:value={field.name}
			placeholder="column_name"
			class="min-w-0 flex-1 bg-transparent px-1.5 py-1 font-mono text-xs outline-none focus:rounded focus:bg-muted/50"
		/>

		<span class="rounded bg-muted px-2 py-0.5 font-mono text-[11px] text-muted-foreground">
			{field.type}
		</span>

		{#if field.required}
			<span class="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
				Req
			</span>
		{/if}

		<button
			type="button"
			onclick={() => (expanded = !expanded)}
			title="Field options"
			class="cursor-pointer rounded p-1.5 transition {expanded
				? 'bg-accent text-foreground'
				: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
		>
			<Settings2 class="h-3.5 w-3.5" />
		</button>

		<button
			type="button"
			onclick={onRemove}
			title="Remove field"
			class="cursor-pointer rounded p-1.5 text-muted-foreground transition hover:bg-destructive/15 hover:text-destructive"
		>
			<Trash2 class="h-3.5 w-3.5" />
		</button>
	</div>

	{#if expanded || field.type === 'vector'}
		<div class="flex flex-wrap items-center justify-between gap-3 border-t border-border/60 bg-muted/20 px-3 py-2.5">
			<div class="flex items-center gap-5">
				<Switch bind:checked={field.required as boolean} label="Required (NOT NULL)" />
				<Switch bind:checked={field.unique as boolean} label="Unique" disabled={isExisting} />
			</div>

			{#if field.type === 'vector'}
				<div class="flex items-center gap-1.5">
					<span class="text-[11px] text-muted-foreground">Dimensions</span>
					<input
						type="number"
						min="1"
						max="4096"
						disabled={isExisting}
						bind:value={field.dimension}
						placeholder="768"
						class="w-20 rounded-md border border-input bg-background px-2 py-1 font-mono text-xs outline-none focus:border-ring disabled:opacity-50"
					/>
				</div>
			{/if}
		</div>
	{/if}
</div>
