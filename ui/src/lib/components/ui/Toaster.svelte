<script lang="ts">
	import { AlertCircle, CheckCircle2, X } from '@lucide/svelte';
	import { cubicOut } from 'svelte/easing';
	import { fly } from 'svelte/transition';
	import { toast } from '#lib/stores/toast.svelte.ts';
</script>

{#if toast.items.length > 0}
	<div class="pointer-events-none fixed top-5 left-1/2 z-[70] flex w-full max-w-md -translate-x-1/2 flex-col gap-2 px-4">
		{#each toast.items as item (item.id)}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div
				onmouseenter={() => toast.pause(item.id)}
				onmouseleave={() => toast.resume(item.id)}
				transition:fly={{ y: -24, duration: 220, easing: cubicOut }}
				class="pointer-events-auto flex items-start gap-2.5 rounded-lg border px-4 py-3 text-xs shadow-xl backdrop-blur-md {item.type ===
				'error'
					? 'border-destructive/45 bg-card text-destructive'
					: 'border-emerald-500/45 bg-card text-foreground'}"
			>
				{#if item.type === 'error'}
					<AlertCircle class="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
				{:else}
					<CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
				{/if}
				<span class="min-w-0 flex-1 select-text break-words leading-relaxed">{item.message}</span>
				<button
					type="button"
					onclick={() => toast.dismiss(item.id)}
					class="cursor-pointer rounded p-0.5 opacity-70 hover:opacity-100"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</div>
		{/each}
	</div>
{/if}
