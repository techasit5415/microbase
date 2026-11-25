<script lang="ts">
	import { page } from '$app/state';
	import { Activity, Database, Layers, LogOut, Settings } from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY } from '#lib/api/client.ts';
	import ThemeToggle from './ThemeToggle.svelte';

	let pathname = $derived(page.url.pathname);

	function logout() {
		localStorage.removeItem(ADMIN_TOKEN_KEY);
		window.location.reload();
	}
</script>

<nav class="flex w-14 shrink-0 flex-col items-center justify-between border-r border-border bg-card py-3">
	<div class="flex flex-col items-center gap-2.5">
		<a
			href="/_/collections"
			title="Microbase"
			class="flex h-9 w-9 items-center justify-center rounded-md bg-foreground text-background shadow-xs"
		>
			<Database class="h-4 w-4" />
		</a>

		<div class="my-1 h-px w-6 bg-border"></div>

		<a
			href="/_/collections"
			title="Collections"
			class="flex h-9 w-9 items-center justify-center rounded-md transition {pathname.includes('/collections')
				? 'bg-accent text-foreground'
				: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
		>
			<Layers class="h-4 w-4" />
		</a>

		<a
			href="/_/logs"
			title="Logs"
			class="flex h-9 w-9 items-center justify-center rounded-md transition {pathname.includes('/logs')
				? 'bg-accent text-foreground'
				: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
		>
			<Activity class="h-4 w-4" />
		</a>

		<a
			href="/_/settings"
			title="Settings"
			class="flex h-9 w-9 items-center justify-center rounded-md transition {pathname.includes('/settings')
				? 'bg-accent text-foreground'
				: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
		>
			<Settings class="h-4 w-4" />
		</a>
	</div>

	<div class="flex flex-col items-center gap-2">
		<ThemeToggle />
		<button
			type="button"
			onclick={logout}
			title="Sign out"
			class="flex h-9 w-9 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition hover:bg-destructive/15 hover:text-destructive"
		>
			<LogOut class="h-4 w-4" />
		</button>
	</div>
</nav>
