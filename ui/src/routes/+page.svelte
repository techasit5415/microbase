<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	onMount(() => {
		goto('/_/collections?collection=users', { replaceState: true });
	});
</script>
