<script lang="ts">
	import { onMount } from 'svelte';
	import { Moon, Sun } from '@lucide/svelte';

	let isDark = $state(false);

	onMount(() => {
		const saved = localStorage.getItem('microbase-theme');
		isDark = saved ? saved === 'dark' : false;
		applyTheme(isDark);
	});

	function applyTheme(dark: boolean) {
		document.documentElement.classList.toggle('dark', dark);
		document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
	}

	function toggle() {
		isDark = !isDark;
		applyTheme(isDark);
		localStorage.setItem('microbase-theme', isDark ? 'dark' : 'light');
	}
</script>

<button
	type="button"
	onclick={toggle}
	title={isDark ? 'Switch to Light Mode' : 'Switch to Dark Mode'}
	class="flex h-9 w-9 cursor-pointer items-center justify-center rounded-md border border-border bg-background text-foreground transition hover:bg-accent"
>
	{#if isDark}
		<Sun class="h-4 w-4" />
	{:else}
		<Moon class="h-4 w-4" />
	{/if}
</button>
