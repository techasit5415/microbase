<script lang="ts">
	import { fade, scale } from 'svelte/transition';
	import { Folder, Table2, Users, X } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';
	import ErdCanvas from '#lib/components/erd/ErdCanvas.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';

	interface Props {
		open: boolean;
		collections: Collection[];
		systemTables: Collection[];
		onClose: () => void;
		onEditCollection: (col: Collection, tab?: 'fields' | 'rules') => void;
	}

	let { open, collections, systemTables, onClose, onEditCollection }: Props = $props();

	let activeMainTab = $state<'fields' | 'rules'>('rules');
	let showSystem = $state(false);
	let activeRuleKey = $state<'list' | 'view' | 'create' | 'update' | 'delete'>('list');

	const ruleTabs: Array<{ key: 'list' | 'view' | 'create' | 'update' | 'delete'; label: string }> = [
		{ key: 'list', label: 'List/Search rule' },
		{ key: 'view', label: 'View rule' },
		{ key: 'create', label: 'Create rule' },
		{ key: 'update', label: 'Update rule' },
		{ key: 'delete', label: 'Delete rule' }
	];

	let displayedCollections = $derived(showSystem ? [...collections, ...systemTables] : collections);

	type RuleToken = { text: string; kind: 'string' | 'plain' };

	function tokenizeRule(expr: string): RuleToken[] {
		const tokens: RuleToken[] = [];
		const re = /"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'/g;
		let lastIndex = 0;
		let match: RegExpExecArray | null;
		while ((match = re.exec(expr)) !== null) {
			if (match.index > lastIndex) {
				tokens.push({ text: expr.slice(lastIndex, match.index), kind: 'plain' });
			}
			tokens.push({ text: match[0], kind: 'string' });
			lastIndex = match.index + match[0].length;
		}
		if (lastIndex < expr.length) {
			tokens.push({ text: expr.slice(lastIndex), kind: 'plain' });
		}
		return tokens;
	}

	function getRuleState(
		col: Collection,
		key: 'list' | 'view' | 'create' | 'update' | 'delete'
	): { kind: 'na' | 'superuser' | 'public' | 'expr'; expr?: string } {
		if (col.type === 'view' && (key === 'create' || key === 'update' || key === 'delete')) {
			return { kind: 'na' };
		}
		if (col.type === 'system') {
			return { kind: 'superuser' };
		}
		const rules = col.api_rules ?? {};
		if (!Object.prototype.hasOwnProperty.call(rules, key)) {
			return { kind: 'superuser' };
		}
		const val = (rules[key] ?? '').trim();
		if (val === '') {
			return { kind: 'public' };
		}
		return { kind: 'expr', expr: rules[key] };
	}

	function handleKeydown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') {
			onClose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div
		transition:fade={{ duration: 150 }}
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-[1px]"
		role="dialog"
		aria-modal="true"
		aria-label="Collections overview"
		tabindex="-1"
	>
		<button
			type="button"
			aria-label="Close collections overview"
			onclick={onClose}
			class="fixed inset-0 cursor-default bg-transparent"
		></button>
		<div
			transition:scale={{ duration: 150, start: 0.98 }}
			class="relative z-10 flex h-[88vh] w-full max-w-6xl flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
		>
			<!-- Header -->
			<header class="flex shrink-0 items-center justify-between gap-4 border-b border-border bg-muted/35 px-6 py-4">
				<h2 class="text-base font-semibold text-foreground">Collections overview</h2>
				<div class="flex items-center gap-4">
					<Switch bind:checked={showSystem} label="System collections" />
					<button
						type="button"
						onclick={onClose}
						title="Close"
						class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-accent hover:text-foreground"
					>
						<X class="h-4 w-4" />
					</button>
				</div>
			</header>

			<!-- Top-level 2 tabs (Fields and relations | Rules) -->
			<div class="grid shrink-0 grid-cols-2 border-b border-border bg-muted/25">
				<button
					type="button"
					onclick={() => (activeMainTab = 'fields')}
					class="cursor-pointer border-r border-border py-3 text-center text-sm font-medium transition {activeMainTab ===
					'fields'
						? 'bg-card text-foreground shadow-[inset_0_-2px_0_0_var(--color-foreground)]'
						: 'text-muted-foreground hover:bg-muted/40 hover:text-foreground'}"
				>
					Fields and relations
				</button>
				<button
					type="button"
					onclick={() => (activeMainTab = 'rules')}
					class="cursor-pointer py-3 text-center text-sm font-medium transition {activeMainTab === 'rules'
						? 'bg-card text-foreground shadow-[inset_0_-2px_0_0_var(--color-foreground)]'
						: 'text-muted-foreground hover:bg-muted/40 hover:text-foreground'}"
				>
					Rules
				</button>
			</div>

			<!-- Tab body -->
			{#if activeMainTab === 'fields'}
				<div class="min-h-0 flex-1 overflow-hidden p-4">
					<ErdCanvas collections={displayedCollections} />
				</div>
			{:else}
				<div class="flex min-h-0 flex-1 flex-col overflow-hidden p-5">
					<div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
						<!-- Rule sub-tabs -->
						<div class="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-4 py-3">
							{#each ruleTabs as tab (tab.key)}
								<button
									type="button"
									onclick={() => (activeRuleKey = tab.key)}
									class="cursor-pointer rounded-md border px-4 py-1.5 text-xs font-medium transition {activeRuleKey ===
									tab.key
										? 'border-foreground bg-background font-semibold text-foreground shadow-2xs'
										: 'border-transparent text-muted-foreground hover:bg-muted/60 hover:text-foreground'}"
								>
									{tab.label}
								</button>
							{/each}
						</div>

						<!-- Collections rules matrix -->
						<div class="min-h-0 flex-1 divide-y divide-border overflow-y-auto">
							{#each displayedCollections as col (col.name)}
								{@const state = getRuleState(col, activeRuleKey)}
								<div class="flex flex-col gap-2 px-5 py-3.5 transition hover:bg-muted/25 sm:flex-row sm:items-center sm:gap-4">
									<button
										type="button"
										onclick={() => {
											onClose();
											onEditCollection(col, col.type === 'system' ? 'fields' : 'rules');
										}}
										title="Open {col.name} API rules"
										class="flex w-full shrink-0 cursor-pointer items-center gap-2.5 text-left text-sm text-foreground hover:underline sm:w-52"
									>
										{#if col.type === 'auth' || col.name === 'users' || col.name === '_superusers'}
											<Users class="h-4 w-4 shrink-0 text-muted-foreground" />
										{:else if col.type === 'view'}
											<Table2 class="h-4 w-4 shrink-0 text-muted-foreground" />
										{:else}
											<Folder class="h-4 w-4 shrink-0 text-muted-foreground" />
										{/if}
										<span class="truncate font-medium">{col.name}</span>
									</button>

									<div class="min-w-0 flex-1">
										{#if state.kind === 'na'}
											<span class="text-xs text-muted-foreground/70 italic">N/A (View collection)</span>
										{:else if state.kind === 'superuser'}
											<button
												type="button"
												disabled={col.type === 'system'}
												onclick={() => {
													onClose();
													onEditCollection(col, 'rules');
												}}
												class="inline-flex cursor-pointer items-center rounded-md bg-emerald-500/15 px-2.5 py-1 text-xs font-medium text-emerald-700 transition hover:bg-emerald-500/25 disabled:cursor-default disabled:hover:bg-emerald-500/15 dark:text-emerald-300"
											>
												Superusers only
											</button>
										{:else if state.kind === 'public'}
											<button
												type="button"
												onclick={() => {
													onClose();
													onEditCollection(col, 'rules');
												}}
												class="inline-flex cursor-pointer items-center rounded-md bg-sky-500/15 px-2.5 py-1 text-xs font-medium text-sky-700 transition hover:bg-sky-500/25 dark:text-sky-300"
											>
												Public
											</button>
										{:else}
											<button
												type="button"
												onclick={() => {
													onClose();
													onEditCollection(col, 'rules');
												}}
												title="Click to edit rule"
												class="block w-full cursor-pointer rounded-md border border-transparent bg-muted/70 px-3.5 py-2 text-left font-mono text-xs leading-relaxed break-words whitespace-pre-wrap text-foreground transition hover:border-border hover:bg-muted"
											>
												{#each tokenizeRule(state.expr ?? '') as tok, i (i)}
													{#if tok.kind === 'string'}
														<span class="text-emerald-600 dark:text-emerald-400">{tok.text}</span>
													{:else}
														<span>{tok.text}</span>
													{/if}
												{/each}
											</button>
										{/if}
									</div>
								</div>
							{:else}
								<div class="px-5 py-10 text-center text-xs text-muted-foreground">
									No collections found.
								</div>
							{/each}
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}
