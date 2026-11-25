<script lang="ts">
	import { AlertCircle, CheckCircle2, X } from '@lucide/svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';
</script>

{#if toast.items.length > 0}
	<div class="fixed right-5 bottom-5 z-50 flex w-80 flex-col gap-2">
		{#each toast.items as item (item.id)}
			<div
				class="flex items-start gap-2.5 rounded-md border px-3.5 py-2.5 text-xs shadow-lg backdrop-blur-sm {item.type ===
				'error'
					? 'border-destructive/40 bg-destructive/15 text-destructive'
					: 'border-emerald-500/40 bg-card text-foreground'}"
			>
				{#if item.type === 'error'}
					<AlertCircle class="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
				{:else}
					<CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
				{/if}
				<span class="min-w-0 flex-1 break-words leading-relaxed">{item.message}</span>
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
