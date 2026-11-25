<script lang="ts">
	import { AlertTriangle, X } from '@lucide/svelte';
	import { cubicOut } from 'svelte/easing';
	import { fade, scale } from 'svelte/transition';
	import { confirmStore } from '#lib/stores/confirm.svelte.ts';

	function handleKeydown(e: KeyboardEvent) {
		if (confirmStore.state.open && e.key === 'Escape') {
			e.preventDefault();
			confirmStore.respond(false);
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if confirmStore.state.open}
	<div class="fixed inset-0 z-[60] flex items-center justify-center p-4">
		<!-- Backdrop -->
		<button
			type="button"
			aria-label="Cancel dialog"
			transition:fade={{ duration: 150 }}
			onclick={() => confirmStore.respond(false)}
			class="fixed inset-0 bg-black/45 backdrop-blur-[2px]"
		></button>

		<!-- Centered Modal Card -->
		<div
			role="alertdialog"
			aria-modal="true"
			transition:scale={{ start: 0.94, duration: 180, easing: cubicOut }}
			class="relative z-10 w-full max-w-md rounded-lg border border-border bg-card p-6 shadow-xl"
		>
			<div class="flex items-start gap-3.5">
				{#if confirmStore.state.options.destructive}
					<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-destructive/12 text-destructive">
						<AlertTriangle class="h-5 w-5" />
					</div>
				{/if}
				<div class="min-w-0 flex-1">
					<h3 class="text-base font-semibold tracking-tight text-foreground">
						{confirmStore.state.options.title}
					</h3>
					{#if confirmStore.state.options.description}
						<p class="mt-1.5 text-xs leading-relaxed text-muted-foreground">
							{confirmStore.state.options.description}
						</p>
					{/if}
				</div>
				<button
					type="button"
					onclick={() => confirmStore.respond(false)}
					class="cursor-pointer rounded-md p-1 text-muted-foreground transition hover:bg-accent hover:text-foreground"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<div class="mt-6 flex items-center justify-end gap-2">
				<button
					type="button"
					onclick={() => confirmStore.respond(false)}
					class="cursor-pointer rounded-md border border-border bg-background px-4 py-2 text-xs font-medium text-foreground transition hover:bg-accent"
				>
					{confirmStore.state.options.cancelLabel ?? 'Cancel'}
				</button>
				<button
					type="button"
					onclick={() => confirmStore.respond(true)}
					class="cursor-pointer rounded-md px-4 py-2 text-xs font-medium transition hover:opacity-90 {confirmStore.state.options.destructive
						? 'bg-destructive text-white'
						: 'bg-foreground text-background'}"
				>
					{confirmStore.state.options.confirmLabel ?? 'Confirm'}
				</button>
			</div>
		</div>
	</div>
{/if}
