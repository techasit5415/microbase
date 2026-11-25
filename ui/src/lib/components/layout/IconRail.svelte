<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { Database, LogOut, ScrollText, Settings, Terminal } from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY } from '#lib/api/client.ts';
	import ThemeToggle from './ThemeToggle.svelte';

	const navItems = [
		{ href: '/_/collections', label: 'Collections', icon: Database, match: '/_/collections' },
		{ href: '/_/logs', label: 'Logs', icon: ScrollText, match: '/_/logs' },
		{ href: '/_/sql', label: 'SQL Console & ERD', icon: Terminal, match: '/_/sql' },
		{ href: '/_/settings', label: 'Settings', icon: Settings, match: '/_/settings' }
	];

	let version = $state('');

	onMount(async () => {
		try {
			const res = await fetch('/health');
			const json = await res.json();
			version = json?.data?.version || json?.version || '';
		} catch {
			// ignore
		}
	});

	function handleLogout() {
		localStorage.removeItem(ADMIN_TOKEN_KEY);
		window.location.reload();
	}
</script>

<aside class="flex h-full w-[72px] shrink-0 flex-col items-center justify-between border-r border-border bg-card py-4">
	<div class="flex flex-col items-center gap-3">
		<!-- Left Navigation Icons -->
		<nav class="flex flex-col items-center gap-2">
			{#each navItems as item (item.href)}
				{@const active = page.url.pathname.startsWith(item.match)}
				<a
					href={item.href}
					title={item.label}
					class="flex h-11 w-11 items-center justify-center rounded-lg border transition {active
						? 'border-primary bg-primary text-primary-foreground shadow-xs'
						: 'border-transparent text-muted-foreground hover:bg-muted hover:text-foreground'}"
				>
					<item.icon class="h-5 w-5" />
				</a>
			{/each}
		</nav>
	</div>

	<!-- Bottom Utilities -->
	<div class="flex flex-col items-center gap-2">
		<ThemeToggle />
		<button
			type="button"
			onclick={handleLogout}
			title="Sign out"
			class="flex h-10 w-10 cursor-pointer items-center justify-center rounded-lg text-muted-foreground transition hover:bg-destructive/12 hover:text-destructive"
		>
			<LogOut class="h-4.5 w-4.5" />
		</button>
		{#if version}
			<span
				title="Microbase {version}"
				class="mt-1 rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground"
			>
				{version}
			</span>
		{/if}
	</div>
</aside>
