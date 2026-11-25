<script lang="ts">
	import { Check, ChevronDown } from '@lucide/svelte';

	export interface SelectOption {
		value: string;
		label: string;
	}

	interface Props {
		value: string;
		options: SelectOption[];
		disabled?: boolean;
		title?: string;
	}

	let { value = $bindable(), options, disabled = false, title = '' }: Props = $props();
	let open = $state(false);
	let containerEl = $state<HTMLDivElement | null>(null);

	let activeLabel = $derived(options.find((o) => o.value === value)?.label ?? value);

	function handleWindowClick(e: MouseEvent) {
		if (open && containerEl && !containerEl.contains(e.target as Node)) {
			open = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') {
			open = false;
		}
	}
</script>

<svelte:window onclick={handleWindowClick} onkeydown={handleKeydown} />

<div bind:this={containerEl} class="relative inline-block">
	<button
		type="button"
		{disabled}
		{title}
		onclick={() => (open = !open)}
		class="inline-flex min-w-26 items-center justify-between gap-2 rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-accent/60 focus:border-ring disabled:cursor-not-allowed disabled:opacity-50"
	>
		<span>{activeLabel}</span>
		<ChevronDown class="h-3 w-3 text-muted-foreground transition-transform {open ? 'rotate-180' : ''}" />
	</button>

	{#if open && !disabled}
		<div
			class="absolute right-0 z-50 mt-1.5 min-w-34 overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-xl"
		>
			{#each options as opt (opt.value)}
				<button
					type="button"
					onclick={() => {
						value = opt.value;
						open = false;
					}}
					class="flex w-full items-center justify-between rounded-md px-2.5 py-1.5 text-left text-xs transition hover:bg-accent hover:text-accent-foreground {opt.value ===
					value
						? 'bg-accent/60 font-medium text-foreground'
						: 'text-muted-foreground'}"
				>
					<span>{opt.label}</span>
					{#if opt.value === value}
						<Check class="h-3.5 w-3.5 text-primary" />
					{/if}
				</button>
			{/each}
		</div>
	{/if}
</div>
