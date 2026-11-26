<script lang="ts">
	import { GraduationCap, KeyRound } from '@lucide/svelte';

	interface Props {
		name: string;
	}

	let { name }: Props = $props();
</script>

{#if name === 'google'}
	<div class="flex h-10 w-10 items-center justify-center rounded-md border border-border bg-white shadow-xs">
		<svg class="h-5 w-5" viewBox="0 0 24 24">
			<path fill="#4285F4" d="M23.49 12.27c0-.79-.07-1.55-.2-2.27H12v4.3h6.45a5.52 5.52 0 0 1-2.39 3.62v3h3.87c2.26-2.08 3.56-5.15 3.56-8.65z" />
			<path fill="#34A853" d="M12 24c3.24 0 5.95-1.07 7.93-2.9l-3.87-3a7.17 7.17 0 0 1-10.67-3.77H1.4v3.09A12 12 0 0 0 12 24z" />
			<path fill="#FBBC05" d="M5.39 14.33a7.2 7.2 0 0 1 0-4.66V6.58H1.4a12 12 0 0 0 0 10.84l3.99-3.09z" />
			<path fill="#EA4335" d="M12 4.77c1.76 0 3.34.61 4.59 1.8l3.44-3.44C17.95 1.19 15.24 0 12 0 7.31 0 3.25 2.69 1.4 6.58l3.99 3.09A7.15 7.15 0 0 1 12 4.77z" />
		</svg>
	</div>
{:else if name === 'discord'}
	<div class="flex h-10 w-10 items-center justify-center rounded-md bg-[#5865F2] text-white shadow-xs">
		<svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
			<path d="M20.317 4.37a19.79 19.79 0 0 0-4.885-1.515.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0 12.64 12.64 0 0 0-.617-1.25.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057 19.9 19.9 0 0 0 5.993 3.03.078.078 0 0 0 .084-.028c.462-.63.874-1.295 1.226-1.994a.076.076 0 0 0-.041-.106 13.107 13.107 0 0 1-1.872-.892.077.077 0 0 1-.008-.128c.126-.094.252-.192.372-.292a.074.074 0 0 1 .077-.01c3.928 1.793 8.18 1.793 12.061 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127 12.299 12.299 0 0 1-1.873.892.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028 19.839 19.839 0 0 0 6.002-3.03.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.028zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z" />
		</svg>
	</div>
{:else if name === 'scikmitl'}
	<div class="flex h-10 w-10 items-center justify-center rounded-md bg-orange-500 text-white shadow-xs">
		<GraduationCap class="h-5 w-5" />
	</div>
{:else}
	<div class="flex h-10 w-10 items-center justify-center rounded-md border border-border bg-accent text-foreground shadow-xs">
		<KeyRound class="h-5 w-5" />
	</div>
{/if}
