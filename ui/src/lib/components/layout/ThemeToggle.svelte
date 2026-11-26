<script lang="ts">
	import { Moon, Sun } from '@lucide/svelte';
	import { uiPrefs } from '#lib/stores/uiPrefs.svelte.ts';

	function toggle() {
		uiPrefs.update({ theme: uiPrefs.isDark ? 'light' : 'dark' });
	}
</script>

<button
	type="button"
	onclick={toggle}
	title={uiPrefs.isDark ? 'Switch to Light Mode' : 'Switch to Dark Mode'}
	class="flex h-9 w-9 cursor-pointer items-center justify-center rounded-md border border-border bg-background text-foreground transition hover:bg-accent"
>
	{#if uiPrefs.isDark}
		<Sun class="h-4 w-4" />
	{:else}
		<Moon class="h-4 w-4" />
	{/if}
</button>
