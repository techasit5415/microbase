<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
		maxWidth?: string;
		children: Snippet;
	}

	let { open, onClose, maxWidth = 'max-w-xl', children }: Props = $props();

	function handleKeydown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') {
			onClose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		onclick={onClose}
		class="fixed inset-0 z-50 flex justify-end bg-black/55 backdrop-blur-[2px] transition-opacity"
	>
		<div
			onclick={(e) => e.stopPropagation()}
			class="flex h-full w-full {maxWidth} flex-col border-l border-border bg-card shadow-2xl"
		>
			{@render children()}
		</div>
	</div>
{/if}
