<script lang="ts">
	import { onMount } from 'svelte';
	import { Gauge, Plus, Save, Trash2 } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface RateLimitRule {
		label: string;
		max_requests: number;
		duration_sec: number;
	}

	let rules = $state<RateLimitRule[]>([]);
	let saving = $state(false);

	onMount(async () => {
		try {
			const res = await apiRequest<{ rules: RateLimitRule[] }>('/api/settings/ratelimits');
			rules = res.rules ?? [];
		} catch {
			// Ignore
		}
	});

	function addRule() {
		rules = [...rules, { label: '/api/', max_requests: 60, duration_sec: 60 }];
	}

	function removeRule(idx: number) {
		rules = rules.filter((_, i) => i !== idx);
	}

	async function saveRules() {
		saving = true;
		try {
			await apiRequest('/api/settings/ratelimits', {
				method: 'PUT',
				body: JSON.stringify({ rules })
			});
			toast.success('Updated per-route rate limit rules (live)');
		} catch (err) {
			toast.error(err);
		} finally {
			saving = false;
		}
	}
</script>

<section class="space-y-4 rounded-lg border border-border bg-card p-5">
	<div class="flex items-center justify-between gap-3">
		<div>
			<h2 class="flex items-center gap-2 text-sm font-semibold">
				<Gauge class="h-4 w-4 text-muted-foreground" />
				Rate Limit Rules
			</h2>
			<p class="mt-0.5 text-xs text-muted-foreground">Configure per-route request limits applied live via CacheStore.</p>
		</div>
		<button type="button" onclick={addRule} class="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border bg-background px-3 py-1.5 text-xs font-medium hover:bg-muted">
			<Plus class="h-3.5 w-3.5" /> Add rule
		</button>
	</div>

	<div class="space-y-2">
		{#each rules as rule, i (i)}
			<div class="grid grid-cols-1 items-center gap-2 rounded-md border border-border bg-background/60 p-2.5 sm:grid-cols-12">
				<input type="text" bind:value={rule.label} placeholder="/api/auth/login" class="rounded border border-input bg-card px-2.5 py-1.5 font-mono text-xs outline-none focus:border-ring sm:col-span-6" />
				<input type="number" min="1" bind:value={rule.max_requests} placeholder="Max req" class="rounded border border-input bg-card px-2.5 py-1.5 font-mono text-xs outline-none focus:border-ring sm:col-span-2" />
				<input type="number" min="1" bind:value={rule.duration_sec} placeholder="Sec" class="rounded border border-input bg-card px-2.5 py-1.5 font-mono text-xs outline-none focus:border-ring sm:col-span-3" />
				<div class="flex justify-end sm:col-span-1">
					<button type="button" onclick={() => removeRule(i)} class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-destructive/15 hover:text-destructive">
						<Trash2 class="h-3.5 w-3.5" />
					</button>
				</div>
			</div>
		{/each}
	</div>

	<div class="flex justify-end border-t border-border pt-3">
		<button type="button" disabled={saving} onclick={saveRules} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-2 text-xs font-semibold text-background hover:opacity-90 disabled:opacity-50">
			<Save class="h-3.5 w-3.5" />
			{saving ? 'Saving...' : 'Save Rate Limits'}
		</button>
	</div>
</section>
