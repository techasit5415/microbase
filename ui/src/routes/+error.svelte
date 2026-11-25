<script lang="ts">
	import { page } from '$app/state';
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
	<div class="text-3xl font-bold text-destructive">{page.status}</div>
	<p class="max-w-md text-sm text-muted-foreground">{page.error?.message ?? 'Unexpected error'}</p>
	<button
		type="button"
		onclick={() => window.location.reload()}
		class="mt-2 cursor-pointer rounded-md bg-foreground px-4 py-1.5 text-xs font-medium text-background"
	>
		Reload
	</button>
</div>
