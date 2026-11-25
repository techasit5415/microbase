<script lang="ts">
	import { Plus, X } from '@lucide/svelte';
	import { createCollection } from '#lib/api/collections.ts';
	import type { Collection, FieldDef } from '#lib/api/types.ts';
	import FieldRowInput from './FieldRowInput.svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
		onCreated: (col: Collection) => void;
	}

	let { open, onClose, onCreated }: Props = $props();

	let name = $state('');
	let fields = $state<FieldDef[]>([{ name: 'title', type: 'text', required: true }]);
	let errorMsg = $state('');
	let saving = $state(false);

	function addField() {
		fields = [...fields, { name: '', type: 'text', required: false }];
	}

	function removeField(index: number) {
		fields = fields.filter((_, i) => i !== index);
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		saving = true;
		try {
			const cleanFields = fields
				.filter((f) => f.name.trim() !== '')
				.map((f) => ({
					...f,
					name: f.name.trim(),
					dimension: f.type === 'vector' ? Number(f.dimension || 768) : undefined
				}));
			const created = await createCollection(name.trim(), cleanFields);
			name = '';
			fields = [{ name: 'title', type: 'text', required: true }];
			onCreated(created);
			onClose();
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to create collection';
		} finally {
			saving = false;
		}
	}
</script>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs">
		<div class="w-full max-w-lg rounded-xl border border-border bg-card p-5 shadow-xl">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-semibold">New Collection</h2>
				<button type="button" onclick={onClose} class="rounded p-1 text-muted-foreground hover:bg-accent">
					<X class="h-4 w-4" />
				</button>
			</div>

			<form onsubmit={handleSubmit} class="flex flex-col gap-4">
				<div>
					<label for="col-name" class="mb-1 block text-xs font-medium text-muted-foreground">
						Table Name (lowercase, a-z, 0-9, _)
					</label>
					<input
						id="col-name"
						type="text"
						required
						bind:value={name}
						placeholder="e.g. products"
						class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
					/>
				</div>

				<div>
					<div class="mb-2 flex items-center justify-between">
						<span class="text-xs font-medium text-muted-foreground">Fields</span>
						<button
							type="button"
							onclick={addField}
							class="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs hover:bg-accent"
						>
							<Plus class="h-3 w-3" /> Add Field
						</button>
					</div>

					<div class="flex max-h-60 flex-col gap-2 overflow-y-auto">
						{#each fields as _, i}
							<FieldRowInput bind:field={fields[i]} onRemove={() => removeField(i)} />
						{/each}
					</div>
				</div>

				{#if errorMsg}
					<p class="rounded-md bg-destructive/15 px-3 py-2 text-xs text-destructive">{errorMsg}</p>
				{/if}

				<div class="flex justify-end gap-2 pt-2">
					<button
						type="button"
						onclick={onClose}
						class="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent"
					>
						Cancel
					</button>
					<button
						type="submit"
						disabled={saving}
						class="rounded-md bg-primary px-4 py-1.5 text-xs font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
					>
						{saving ? 'Creating...' : 'Create Collection'}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}