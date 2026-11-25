<script lang="ts">
	import { Database, Download, Layers } from '@lucide/svelte';
    import { getSchemaExportURL } from '#lib/api/collections.ts';
	import ThemeToggle from './ThemeToggle.svelte';
</script>

<nav
	class="flex w-14 shrink-0 flex-col items-center justify-between border-r border-border bg-card py-3"
>
	<div class="flex flex-col items-center gap-3">
		<div
			title="Microbase"
			class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm"
		>
			<Database class="h-4 w-4" />
		</div>

		<div class="my-1 h-px w-6 bg-border"></div>

		<button
			type="button"
			title="Collections"
			class="flex h-9 w-9 items-center justify-center rounded-lg bg-accent text-accent-foreground"
		>
			<Layers class="h-4 w-4" />
		</button>

		<a
			href={getSchemaExportURL()}
			target="_blank"
			rel="noreferrer"
			title="Export Schema JSON"
			class="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground transition hover:bg-accent hover:text-accent-foreground"
		>
			<Download class="h-4 w-4" />
		</a>
	</div>

	<ThemeToggle />
</nav>