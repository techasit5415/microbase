<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { Database, ShieldCheck } from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY, apiRequest } from '#lib/api/client.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	let checking = $state(true);
	let hasSuperuser = $state(true);
	let authenticated = $state(false);

	let email = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let submitting = $state(false);
	let errorMsg = $state('');

	onMount(async () => {
		try {
			const status = await apiRequest<{ has_superuser: boolean }>('/api/superusers/status');
			hasSuperuser = status.has_superuser;
			const token = localStorage.getItem(ADMIN_TOKEN_KEY);
			authenticated = hasSuperuser && Boolean(token);
		} catch {
			// If DB is not connected yet, allow viewing UI
			authenticated = true;
		} finally {
			checking = false;
		}
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';

		if (!hasSuperuser && password !== confirmPassword) {
			errorMsg = 'Passwords do not match';
			return;
		}

		submitting = true;
		try {
			const endpoint = hasSuperuser ? '/api/superusers/login' : '/api/superusers/setup';
			const res = await apiRequest<{ token: string }>(endpoint, {
				method: 'POST',
				body: JSON.stringify({ email: email.trim(), password })
			});
			localStorage.setItem(ADMIN_TOKEN_KEY, res.token);
			hasSuperuser = true;
			authenticated = true;
			toast.success(hasSuperuser ? 'Signed in as Superuser' : 'Superuser account created');
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Authentication failed';
		} finally {
			submitting = false;
		}
	}
</script>

{#if checking}
	<div class="flex h-screen w-screen items-center justify-center bg-background text-sm text-muted-foreground">
		Loading Microbase...
	</div>
{:else if !authenticated}
	<div class="flex h-screen w-screen items-center justify-center bg-muted/30 p-4">
		<div class="w-full max-w-md rounded-md border border-border bg-card p-7 shadow-lg">
			<div class="mb-6 flex flex-col items-center text-center">
				<div class="mb-3 flex h-11 w-11 items-center justify-center rounded-md bg-foreground text-background">
					{#if hasSuperuser}
						<Database class="h-5 w-5" />
					{:else}
						<ShieldCheck class="h-5 w-5" />
					{/if}
				</div>
				<h1 class="text-lg font-semibold">
					{hasSuperuser ? 'Superuser login' : 'Create your first Superuser'}
				</h1>
				<p class="mt-1 text-xs text-muted-foreground">
					{hasSuperuser
						? 'Sign in to manage Microbase collections, auth, and settings.'
						: 'First-time setup: create an admin account to secure Microbase.'}
				</p>
			</div>

			<form onsubmit={handleSubmit} class="space-y-4">
				<div>
					<label for="su-email" class="mb-1.5 block text-xs font-medium">Email</label>
					<input
						id="su-email"
						type="email"
						required
						value={email}
						oninput={(e) => (email = e.currentTarget.value)}
						placeholder="admin@microbase.dev"
						class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
					/>
				</div>

				<div>
					<label for="su-pass" class="mb-1.5 block text-xs font-medium">Password</label>
					<input
						id="su-pass"
						type="password"
						required
						minlength="8"
						value={password}
						oninput={(e) => (password = e.currentTarget.value)}
						placeholder="Minimum 8 characters"
						class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
					/>
				</div>

				{#if !hasSuperuser}
					<div>
						<label for="su-confirm" class="mb-1.5 block text-xs font-medium">Confirm password</label>
						<input
							id="su-confirm"
							type="password"
							required
							minlength="8"
							value={confirmPassword}
							oninput={(e) => (confirmPassword = e.currentTarget.value)}
							placeholder="Re-enter password"
							class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
						/>
					</div>
				{/if}

				{#if errorMsg}
					<p class="rounded-md bg-destructive/15 px-3 py-2 text-xs text-destructive">{errorMsg}</p>
				{/if}

				<button
					type="submit"
					disabled={submitting}
					class="w-full cursor-pointer rounded-md bg-foreground py-2.5 text-sm font-medium text-background transition hover:opacity-90 disabled:opacity-50"
				>
					{submitting ? 'Please wait...' : hasSuperuser ? 'Login' : 'Create & Login'}
				</button>
			</form>
		</div>
	</div>
{:else}
	{@render children()}
{/if}
