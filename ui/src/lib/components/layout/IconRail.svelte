<script lang="ts">
	import { page } from '$app/state';
	import { Database, LogOut, ScrollText, Settings } from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY } from '#lib/api/client.ts';
	import ThemeToggle from './ThemeToggle.svelte';

	const navItems = [
		{ href: '/_/collections', label: 'Collections', icon: Database, match: '/_/collections' },
		{ href: '/_/logs', label: 'Logs', icon: ScrollText, match: '/_/logs' },
		{ href: '/_/settings', label: 'Settings', icon: Settings, match: '/_/settings' }
	];

	function handleLogout() {
		localStorage.removeItem(ADMIN_TOKEN_KEY);
		window.location.reload();
	}
</script>

<aside class="flex h-full w-[72px] shrink-0 flex-col items-center justify-between border-r border-border bg-card py-4">
	<div class="flex flex-col items-center gap-3">
		<!-- Brand Mark -->
		<a
			href="/_/collections"
			title="Microbase"
			class="mb-3 flex h-11 w-11 items-center justify-center rounded-lg border-2 border-foreground bg-card font-mono text-sm font-bold tracking-tighter text-foreground transition hover:bg-muted"
		>
			MB
		</a>

		<!-- Left Navigation Icons -->
		<nav class="flex flex-col items-center gap-2">
			{#each navItems as item (item.href)}
				{@const active = page.url.pathname.startsWith(item.match)}
				<a
					href={item.href}
					title={item.label}
					class="flex h-11 w-11 items-center justify-center rounded-lg border transition {active
						? 'border-foreground bg-foreground text-background shadow-xs'
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
	</div>
</aside>
