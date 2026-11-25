<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { fade, scale } from 'svelte/transition';
	import { Check, Copy, Eye, Search, X } from '@lucide/svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';

	export interface RelationOption {
		id: string;
		preview: string;
	}

	interface Props {
		open: boolean;
		collectionName: string;
		options: RelationOption[];
		selectedIds: string[];
		maxSelect: number;
		onClose: () => void;
		onConfirm: (nextIds: string[]) => void;
	}

	let {
		open,
		collectionName,
		options,
		selectedIds,
		maxSelect,
		onClose,
		onConfirm
	}: Props = $props();

	let search = $state('');
	let draftIds = $state<string[]>([]);

	let isMulti = $derived(maxSelect > 1);

	$effect(() => {
		if (open) {
			draftIds = [...selectedIds];
			search = '';
		}
	});

	let filtered = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return options;
		return options.filter(
			(o) => o.id.toLowerCase().includes(q) || o.preview.toLowerCase().includes(q)
		);
	});

	let optionMap = $derived.by(() => {
		const m: Record<string, RelationOption> = {};
		for (const o of options) m[o.id] = o;
		return m;
	});

	function toggleId(id: string) {
		if (draftIds.includes(id)) {
			draftIds = draftIds.filter((x) => x !== id);
			return;
		}
		if (!isMulti) {
			draftIds = [id];
			return;
		}
		if (draftIds.length >= maxSelect) {
			toast.error(`Maximum ${maxSelect} records allowed`);
			return;
		}
		draftIds = [...draftIds, id];
	}

	function copyId(e: MouseEvent, id: string) {
		e.stopPropagation();
		navigator.clipboard?.writeText(id).catch(() => {});
		toast.success(`Copied ${id}`);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') {
			e.stopPropagation();
			onClose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div
		transition:fade={{ duration: 140 }}
		role="dialog"
		aria-modal="true"
		aria-label="Select {collectionName} records"
		tabindex="-1"
		onclick={(e) => {
			if (e.target === e.currentTarget) onClose();
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') onClose();
		}}
		class="fixed inset-0 z-60 flex items-center justify-center bg-black/45 p-4 backdrop-blur-[1px]"
	>
		<div
			transition:scale={{ start: 0.96, duration: 160, easing: cubicOut }}
			class="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-lg border border-border bg-card shadow-2xl"
		>
			<!-- PocketBase-style Header with Pill Search -->
			<div class="flex items-center gap-3 border-b border-border px-5 py-4">
				<span class="shrink-0 font-mono text-sm font-semibold text-foreground">{collectionName}</span>
				<div class="relative flex-1">
					<Search class="pointer-events-none absolute top-1/2 left-3.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
					<input
						type="text"
						bind:value={search}
						placeholder="Search term or ID..."
						class="w-full rounded-full border border-transparent bg-secondary/80 py-2 pr-4 pl-9 text-xs text-foreground outline-none transition placeholder:text-muted-foreground focus:border-border focus:bg-background"
					/>
				</div>
				<button
					type="button"
					aria-label="Close picker"
					onclick={onClose}
					class="cursor-pointer rounded-full p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<!-- Scrollable Records List -->
			<div class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-5">
				<div class="max-h-72 overflow-y-auto rounded-md border border-border divide-y divide-border bg-background/50">
					{#if filtered.length === 0}
						<div class="py-8 text-center text-xs text-muted-foreground">No records found.</div>
					{:else}
						{#each filtered as opt (opt.id)}
							{@const selected = draftIds.includes(opt.id)}
							<div
								role="button"
								tabindex="0"
								onclick={() => toggleId(opt.id)}
								onkeydown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault();
										toggleId(opt.id);
									}
								}}
								class="flex cursor-pointer items-center gap-3 px-3.5 py-2.5 transition hover:bg-muted/50 {selected ? 'bg-emerald-500/5' : ''}"
							>
								<!-- Circular check indicator -->
								<span
									class="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border transition {selected
										? 'border-emerald-600 bg-emerald-600 text-white'
										: 'border-muted-foreground/45 bg-background'}"
								>
									{#if selected}
										<Check class="h-2.5 w-2.5 stroke-[3]" />
									{/if}
								</span>

								<!-- ID Pill + Copy button -->
								<span class="inline-flex shrink-0 items-center gap-1.5 rounded bg-secondary px-2 py-0.5 font-mono text-[11px] text-foreground">
									<Eye class="h-3 w-3 text-muted-foreground" />
									<button
										type="button"
										title="Copy ID"
										onclick={(e) => copyId(e, opt.id)}
										class="inline-flex cursor-pointer items-center gap-1 rounded bg-background/75 px-1.5 py-0.5 text-[11px] text-foreground transition hover:bg-background"
									>
										<Copy class="h-2.5 w-2.5 text-muted-foreground" />
										<span>{opt.id}</span>
									</button>
								</span>

								<!-- Human-readable preview value -->
								{#if opt.preview}
									<span class="min-w-0 flex-1 truncate text-xs text-foreground/90">{opt.preview}</span>
								{/if}
							</div>
						{/each}
					{/if}
				</div>

				<!-- Selected Summary Section -->
				<div class="space-y-2">
					<div class="text-xs font-semibold text-foreground">
						Selected ({draftIds.length}{#if isMulti} of max {maxSelect}{/if})
					</div>
					{#if draftIds.length === 0}
						<p class="text-xs text-muted-foreground">No records selected.</p>
					{:else}
						<div class="flex flex-wrap gap-2">
							{#each draftIds as id (id)}
								{@const info = optionMap[id]}
								<span class="inline-flex items-center gap-1.5 rounded-md bg-secondary px-2.5 py-1.5 text-xs text-foreground">
									<Eye class="h-3 w-3 text-muted-foreground" />
									<span class="rounded bg-background/80 px-1.5 py-0.5 font-mono text-[11px]">{id}</span>
									{#if info?.preview}
										<span class="max-w-36 truncate text-xs font-medium">{info.preview}</span>
									{/if}
									<button
										type="button"
										aria-label="Remove {id}"
										onclick={() => toggleId(id)}
										class="ml-0.5 cursor-pointer rounded p-0.5 text-muted-foreground transition hover:bg-destructive/15 hover:text-destructive"
									>
										<X class="h-3 w-3" />
									</button>
								</span>
							{/each}
						</div>
					{/if}
				</div>
			</div>

			<!-- PocketBase-style Footer -->
			<div class="flex items-center justify-between border-t border-border bg-card px-5 py-3.5">
				<button
					type="button"
					onclick={onClose}
					class="cursor-pointer rounded-md px-3.5 py-2 text-xs font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					Close
				</button>
				<button
					type="button"
					onclick={() => {
						onConfirm(draftIds);
						onClose();
					}}
					class="cursor-pointer rounded-md bg-foreground px-5 py-2 text-xs font-semibold text-background transition hover:opacity-90"
				>
					Set selection
				</button>
			</div>
		</div>
	</div>
{/if}
