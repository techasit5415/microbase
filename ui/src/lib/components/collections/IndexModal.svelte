<script lang="ts">
	import { Trash2, X } from '@lucide/svelte';
	import type { IndexDef } from '#lib/api/types.ts';
	import Switch from '#lib/components/ui/Switch.svelte';

	interface Props {
		open: boolean;
		tableName: string;
		availableColumns: string[];
		editing?: IndexDef | null;
		onClose: () => void;
		onSave: (index: IndexDef) => void;
		onRemove?: () => void;
	}

	let {
		open,
		tableName,
		availableColumns,
		editing = null,
		onClose,
		onSave,
		onRemove
	}: Props = $props();

	let indexName = $state('');
	let unique = $state(false);
	let selectedColumns = $state<string[]>([]);

	function randomSuffix(): string {
		const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
		let out = '';
		for (let i = 0; i < 10; i++) {
			out += chars[Math.floor(Math.random() * chars.length)];
		}
		return out;
	}

	$effect(() => {
		if (open) {
			if (editing) {
				indexName = editing.name;
				unique = Boolean(editing.unique);
				selectedColumns = [...editing.columns];
			} else {
				indexName = `idx_${randomSuffix()}`;
				unique = false;
				selectedColumns = [];
			}
		}
	});

	function toggleColumn(col: string) {
		if (selectedColumns.includes(col)) {
			selectedColumns = selectedColumns.filter((c) => c !== col);
		} else {
			selectedColumns = [...selectedColumns, col];
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') {
			e.preventDefault();
			onClose();
		}
	}

	function handleSave() {
		const cleanName = indexName.trim().toLowerCase().replace(/[^a-z0-9_]/g, '_');
		if (!cleanName || selectedColumns.length === 0) return;
		onSave({
			name: cleanName,
			columns: [...selectedColumns],
			unique
		});
		onClose();
	}

	let displayTable = $derived(tableName.trim() || 'collection');
	let colsPreview = $derived(selectedColumns.map((c) => `"${c}"`).join(', '));
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div class="fixed inset-0 z-[60] flex items-center justify-center p-4">
		<button
			type="button"
			aria-label="Close index modal"
			onclick={onClose}
			class="fixed inset-0 bg-black/45 backdrop-blur-[2px]"
		></button>

		<div
			role="dialog"
			aria-modal="true"
			class="relative z-10 w-full max-w-lg rounded-lg border border-border bg-card p-6 shadow-xl"
		>
			<div class="mb-5 flex items-center justify-between">
				<h3 class="text-base font-semibold text-foreground">
					{editing ? 'Edit index' : 'Create index'}
				</h3>
				<button
					type="button"
					onclick={onClose}
					class="cursor-pointer rounded-md p-1 text-muted-foreground transition hover:bg-accent hover:text-foreground"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<div class="mb-4 flex items-center justify-between gap-4">
				<div class="flex items-center gap-2.5">
					<Switch checked={unique} onChange={(v) => (unique = v)} />
					<span class="text-xs font-medium text-foreground">Unique</span>
				</div>

				<div class="flex items-center gap-2">
					<label for="idx-name" class="text-[11px] text-muted-foreground">Name</label>
					<input
						id="idx-name"
						type="text"
						value={indexName}
						oninput={(e) => (indexName = e.currentTarget.value)}
						class="w-44 rounded-md border border-input bg-background px-2.5 py-1 font-mono text-xs outline-none focus:border-ring"
					/>
				</div>
			</div>

			<!-- SQL Preview -->
			<div class="mb-4 rounded-md border border-border bg-muted/50 px-3.5 py-3 font-mono text-xs leading-relaxed">
				<span class="font-semibold text-foreground">CREATE {unique ? 'UNIQUE ' : ''}INDEX</span>
				<span class="text-emerald-700 dark:text-emerald-400"> "{indexName || 'idx_...'}" </span>
				<span class="font-semibold text-foreground">ON</span>
				<span class="text-emerald-700 dark:text-emerald-400"> "{displayTable}" </span>
				<span class="text-foreground">({colsPreview})</span>
			</div>

			<!-- Presets -->
			<div>
				<div class="mb-2 text-xs text-muted-foreground">Presets:</div>
				<div class="flex flex-wrap gap-1.5">
					{#each availableColumns as col (col)}
						{@const active = selectedColumns.includes(col)}
						<button
							type="button"
							onclick={() => toggleColumn(col)}
							class="cursor-pointer rounded-md border px-2.5 py-1 font-mono text-[11px] transition {active
								? 'border-foreground bg-foreground text-background'
								: 'border-border bg-muted/40 text-foreground hover:bg-accent'}"
						>
							{col}
						</button>
					{/each}
				</div>
			</div>

			<!-- Footer -->
			<div class="mt-6 flex items-center justify-between border-t border-border pt-4">
				<div>
					{#if editing && onRemove}
						<button
							type="button"
							onclick={() => {
								onRemove();
								onClose();
							}}
							class="inline-flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-destructive transition hover:bg-destructive/10"
						>
							<Trash2 class="h-3.5 w-3.5" />
							Remove
						</button>
					{/if}
				</div>

				<div class="flex items-center gap-2">
					<button
						type="button"
						onclick={onClose}
						class="cursor-pointer rounded-md px-3.5 py-1.5 text-xs font-medium text-muted-foreground transition hover:bg-accent hover:text-foreground"
					>
						Close
					</button>
					<button
						type="button"
						disabled={selectedColumns.length === 0 || !indexName.trim()}
						onclick={handleSave}
						class="cursor-pointer rounded-md bg-foreground px-4 py-1.5 text-xs font-medium text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
					>
						Set index
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}
