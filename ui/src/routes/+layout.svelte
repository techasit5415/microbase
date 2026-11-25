<script lang="ts">
	import './layout.css';
	import { cubicOut } from 'svelte/easing';
	import { fly } from 'svelte/transition';
	import { page } from '$app/state';
	import IconRail from '#lib/components/layout/IconRail.svelte';
	import SuperuserGate from '#lib/components/layout/SuperuserGate.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
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
			{#key page.url.pathname}
				<div
					in:fly={{ y: 8, duration: 200, easing: cubicOut }}
					class="flex min-w-0 flex-1 overflow-hidden"
				>
					{@render children()}
				</div>
			{/key}
		</div>
	</div>
</SuperuserGate>

<ConfirmDialog />
<Toaster />
