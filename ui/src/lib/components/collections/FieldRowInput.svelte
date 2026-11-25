<script lang="ts">
	import { Trash2 } from '@lucide/svelte';
	import type { FieldDef, FieldType } from '#lib/api/types.ts';

	interface Props {
		field: FieldDef;
		onRemove: () => void;
	}

	let { field = $bindable(), onRemove }: Props = $props();

	const fieldTypes: FieldType[] = [
		'text',
		'integer',
		'float',
		'boolean',
		'json',
		'timestamptz',
		'vector'
	];
</script>

<div class="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/30 p-2">
	<input
		type="text"
		bind:value={field.name}
		placeholder="column_name"
		class="min-w-32 flex-1 rounded-md border border-input bg-background px-2.5 py-1.5 text-xs outline-none focus:ring-1 focus:ring-ring"
	/>

	<select
		bind:value={field.type}
		class="rounded-md border border-input bg-background px-2 py-1.5 text-xs outline-none focus:ring-1 focus:ring-ring"
	>
		{#each fieldTypes as t}
			<option value={t}>{t}</option>
		{/each}
	</select>

	{#if field.type === 'vector'}
		<input
			type="number"
			min="1"
			max="4096"
			bind:value={field.dimension}
			placeholder="dim (e.g. 768)"
			class="w-24 rounded-md border border-input bg-background px-2 py-1.5 text-xs outline-none focus:ring-1 focus:ring-ring"
		/>
	{/if}

	<label class="flex items-center gap-1 text-xs text-muted-foreground">
		<input type="checkbox" bind:checked={field.required} class="rounded border-input" />
		Req
	</label>

	<button
		type="button"
		onclick={onRemove}
		title="Remove field"
		class="rounded p-1 text-muted-foreground hover:bg-destructive/15 hover:text-destructive"
	>
		<Trash2 class="h-3.5 w-3.5" />
	</button>
</div>