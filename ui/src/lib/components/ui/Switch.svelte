<script lang="ts">
	interface Props {
		checked?: boolean;
		label?: string;
		disabled?: boolean;
		onChange?: (next: boolean) => void;
	}

	let { checked = $bindable(false), label = '', disabled = false, onChange }: Props = $props();

	function handleToggle() {
		const next = !checked;
		checked = next;
		onChange?.(next);
	}
</script>

<button
	type="button"
	role="switch"
	aria-checked={Boolean(checked)}
	{disabled}
	onclick={handleToggle}
	class="inline-flex cursor-pointer items-center gap-2.5 select-none disabled:cursor-not-allowed disabled:opacity-50"
>
	<span
		class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors duration-200 {checked
			? 'border-emerald-500/60 bg-emerald-500'
			: 'border-border bg-muted'}"
	>
		<span
			class="pointer-events-none block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform duration-200 {checked
				? 'translate-x-4.5'
				: 'translate-x-0.5'}"
		></span>
	</span>
	{#if label}
		<span class="text-xs font-medium {checked ? 'text-foreground' : 'text-muted-foreground'}">
			{label}
		</span>
	{/if}
</button>
