<script lang="ts">
	import './layout.css';
	import IconRail from '#lib/components/layout/IconRail.svelte';
	import SuperuserGate from '#lib/components/layout/SuperuserGate.svelte';
	import Toaster from '#lib/components/ui/Toaster.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<title>Microbase Admin</title>
</svelte:head>

<SuperuserGate>
	<div class="flex h-screen w-screen overflow-hidden bg-background text-foreground">
		<IconRail />
		<div class="flex min-w-0 flex-1 overflow-hidden">
			{@render children()}
		</div>
	</div>
</SuperuserGate>

<Toaster />
