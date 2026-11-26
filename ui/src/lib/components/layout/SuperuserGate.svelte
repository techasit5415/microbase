<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { Database, Eye, EyeOff, KeyRound, Lock, LogIn, User } from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY, AUTH_EXPIRED_EVENT, apiRequest } from '#lib/api/client.ts';
	import { appSettings } from '#lib/stores/appSettings.svelte.ts';
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
	let otp = $state('');
	let showPassword = $state(false);
	let submitting = $state(false);
	let errorMsg = $state('');

	onMount(() => {
		const handleAuthExpired = () => {
			authenticated = false;
			errorMsg = 'Session expired. Please sign in again.';
		};
		window.addEventListener(AUTH_EXPIRED_EVENT, handleAuthExpired);

		(async () => {
			try {
				const status = await apiRequest<{ has_superuser: boolean }>('/api/superusers/status');
				hasSuperuser = status.has_superuser;
				const token = localStorage.getItem(ADMIN_TOKEN_KEY);
				if (hasSuperuser && token) {
					const verify = await fetch('/api/settings/app', {
						headers: { Authorization: `Bearer ${token}` }
					});
					if (verify.status === 401) {
						localStorage.removeItem(ADMIN_TOKEN_KEY);
						authenticated = false;
					} else {
						authenticated = true;
					}
				} else {
					authenticated = false;
				}
			} catch {
				authenticated = true;
			} finally {
				checking = false;
			}
		})();

		return () => {
			window.removeEventListener(AUTH_EXPIRED_EVENT, handleAuthExpired);
		};
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		const form = e.currentTarget as HTMLFormElement | null;
		if (form) {
			const fd = new FormData(form);
			const fEmail = String(fd.get('email') ?? '');
			const fPass = String(fd.get('password') ?? '');
			if (fEmail) email = fEmail;
			if (fPass) password = fPass;
		}

		if (!hasSuperuser && password !== confirmPassword) {
			errorMsg = 'Passwords do not match';
			return;
		}

		submitting = true;
		try {
			const endpoint = hasSuperuser ? '/api/superusers/login' : '/api/superusers/setup';
			const payload = hasSuperuser
				? { email: email.trim(), password }
				: { email: email.trim(), password, otp: otp.trim() };

			const res = await apiRequest<{ token: string }>(endpoint, {
				method: 'POST',
				body: JSON.stringify(payload)
			});
			localStorage.setItem(ADMIN_TOKEN_KEY, res.token);
			const wasSetup = !hasSuperuser;
			hasSuperuser = true;
			authenticated = true;
			appSettings.load();
			toast.success(wasSetup ? 'Superuser account created' : 'Signed in as Superuser');
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
	<div class="flex min-h-screen w-screen items-center justify-center bg-muted/25 p-6">
		<div class="grid w-full max-w-5xl grid-cols-1 items-center gap-12 lg:grid-cols-2 lg:gap-16">
			<!-- Left Column: Brand Showcase -->
			<div class="flex flex-col items-start px-2">
				<div class="mb-5 flex h-12 w-12 items-center justify-center rounded-lg bg-foreground text-background shadow-sm">
					<Database class="h-6 w-6" />
				</div>
				<span class="text-xs font-semibold tracking-wide text-muted-foreground">
					PostgreSQL &middot; pgvector &middot; Redis
				</span>
				<h1 class="mt-1.5 text-3xl font-bold tracking-tight text-foreground">
					Microbase
				</h1>
				<p class="mt-3 max-w-sm text-sm leading-relaxed text-muted-foreground">
					Manage collections, auth providers, outbound webhooks, and vector search from one lightweight binary.
				</p>
			</div>

			<!-- Right Column: Clean Elevated Login / Setup Card -->
			<div class="w-full max-w-md justify-self-center rounded-lg border border-border bg-card p-8 shadow-sm lg:justify-self-end">
				<div class="mb-6">
					<h2 class="text-xl font-bold tracking-tight text-foreground">
						{hasSuperuser ? 'Sign in' : 'First-time setup'}
					</h2>
					<p class="mt-1 text-xs text-muted-foreground">
						{hasSuperuser
							? 'Sign in with your Superuser account to open the dashboard.'
							: 'Create your first Superuser account using the 8-digit OTP from the server log.'}
					</p>
				</div>

				<form onsubmit={handleSubmit} class="space-y-4">
					<div>
						<label for="su-email" class="mb-1.5 block text-xs font-medium text-foreground">
							Email
						</label>
						<div class="relative">
							<User class="pointer-events-none absolute top-1/2 left-3.5 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
							<input
								id="su-email"
								name="email"
								autocomplete="username"
								type="email"
								required
								value={email}
								oninput={(e) => (email = e.currentTarget.value)}
								placeholder="admin@microbase.dev"
								class="w-full rounded-md border border-input bg-muted/30 py-2.5 pr-3.5 pl-10 text-sm outline-none transition focus:border-ring focus:bg-background"
							/>
						</div>
					</div>

					<div>
						<label for="su-pass" class="mb-1.5 block text-xs font-medium text-foreground">
							Password
						</label>
						<div class="relative">
							<Lock class="pointer-events-none absolute top-1/2 left-3.5 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
							<input
								id="su-pass"
								name="password"
								autocomplete={hasSuperuser ? 'current-password' : 'new-password'}
								type={showPassword ? 'text' : 'password'}
								required
								minlength="8"
								value={password}
								oninput={(e) => (password = e.currentTarget.value)}
								placeholder="••••••••"
								class="w-full rounded-md border border-input bg-muted/30 py-2.5 pr-10 pl-10 text-sm outline-none transition focus:border-ring focus:bg-background"
							/>
							<button
								type="button"
								onclick={() => (showPassword = !showPassword)}
								title={showPassword ? 'Hide password' : 'Show password'}
								class="absolute top-1/2 right-2.5 -translate-y-1/2 cursor-pointer rounded p-1 text-muted-foreground hover:text-foreground"
							>
								{#if showPassword}
									<EyeOff class="h-4 w-4" />
								{:else}
									<Eye class="h-4 w-4" />
								{/if}
							</button>
						</div>
					</div>

					{#if !hasSuperuser}
						<div>
							<label for="su-confirm" class="mb-1.5 block text-xs font-medium text-foreground">
								Confirm password
							</label>
							<div class="relative">
								<Lock class="pointer-events-none absolute top-1/2 left-3.5 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
								<input
									id="su-confirm"
									type="password"
									required
									minlength="8"
									value={confirmPassword}
									oninput={(e) => (confirmPassword = e.currentTarget.value)}
									placeholder="••••••••"
									class="w-full rounded-md border border-input bg-muted/30 py-2.5 pr-3.5 pl-10 text-sm outline-none transition focus:border-ring focus:bg-background"
								/>
							</div>
						</div>

						<div>
							<label for="su-otp" class="mb-1.5 block text-xs font-medium text-foreground">
								8-Digit Setup OTP <span class="text-muted-foreground">(valid 5m)</span>
							</label>
							<div class="relative">
								<KeyRound class="pointer-events-none absolute top-1/2 left-3.5 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
								<input
									id="su-otp"
									type="text"
									inputmode="numeric"
									required
									minlength="8"
									maxlength="8"
									value={otp}
									oninput={(e) => (otp = e.currentTarget.value.replace(/\D/g, '').slice(0, 8))}
									placeholder="12345678"
									class="w-full rounded-md border border-input bg-muted/30 py-2.5 pr-3.5 pl-10 font-mono text-sm tracking-[0.25em] outline-none transition focus:border-ring focus:bg-background"
								/>
							</div>
						</div>
					{/if}

					{#if errorMsg}
						<p class="rounded-md bg-destructive/15 px-3 py-2 text-xs text-destructive">{errorMsg}</p>
					{/if}

					<button
						type="submit"
						disabled={submitting}
						class="mt-2 flex w-full cursor-pointer items-center justify-center gap-2 rounded-md bg-foreground py-2.5 text-sm font-medium text-background shadow-xs transition hover:opacity-90 disabled:opacity-50"
					>
						<LogIn class="h-4 w-4" />
						{submitting ? 'Please wait...' : hasSuperuser ? 'Sign in' : 'Create & Sign in'}
					</button>
				</form>
			</div>
		</div>
	</div>
{:else}
	{@render children()}
{/if}
