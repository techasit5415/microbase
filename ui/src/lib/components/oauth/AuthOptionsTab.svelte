<script lang="ts">
	import { onMount } from 'svelte';
	import { Check, ChevronDown, Copy, ExternalLink, KeyRound, Lock, Plus, Trash2 } from '@lucide/svelte';
	import {
		listOAuthProviders,
		updateOAuthProvider,
		type OAuthProvider
	} from '#lib/api/oauth.ts';
	import { apiRequest } from '#lib/api/client.ts';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import ProviderIcon from './ProviderIcon.svelte';

	let providers = $state<OAuthProvider[]>([]);
	let openSection = $state<'password' | 'oauth2' | null>('oauth2');
	let editingProvider = $state<OAuthProvider | null>(null);

	let enabled = $state(false);
	let clientId = $state('');
	let clientSecret = $state('');
	let displayName = $state('');
	let issuerUrl = $state('');
	let authUrl = $state('');
	let tokenUrl = $state('');
	let userInfoUrl = $state('');
	let scopes = $state('openid email profile');
	let mappingRows = $state<{ col: string; path: string }[]>([]);
	let saving = $state(false);
	let copied = $state(false);
	let errorMsg = $state('');
	let otpEnabled = $state(true);
	let mfaEnabled = $state(false);

	async function saveAuthOptions(nextOtp: boolean, nextMfa: boolean) {
		otpEnabled = nextOtp;
		mfaEnabled = nextMfa;
		try {
			await apiRequest('/api/settings/auth-options', {
				method: 'PUT',
				body: JSON.stringify({ otp_enabled: nextOtp, mfa_enabled: nextMfa })
			});
			toast.success('Updated Auth options');
		} catch (err) {
			toast.error(err);
		}
	}

	let anyOAuthEnabled = $derived(providers.some((p) => p.enabled && p.client_id && p.has_secret));

	let callbackUrl = $derived(
		editingProvider && typeof window !== 'undefined'
			? `${window.location.origin}/api/auth/oauth/${editingProvider.name}/callback`
			: ''
	);

	onMount(async () => {
		providers = await listOAuthProviders();
			const opts = await apiRequest<{ otp_enabled?: boolean; mfa_enabled?: boolean }>('/api/settings/auth-options').catch(() => ({ otp_enabled: true, mfa_enabled: false }));
			otpEnabled = Boolean(opts.otp_enabled);
			mfaEnabled = Boolean(opts.mfa_enabled);
	});

	function selectProvider(p: OAuthProvider) {
		if (editingProvider?.name === p.name) {
			editingProvider = null;
			return;
		}
		editingProvider = p;
		enabled = p.enabled;
		clientId = p.client_id;
		clientSecret = '';
		displayName = p.display_name ?? '';
		issuerUrl = p.issuer_url ?? '';
		authUrl = p.auth_url ?? '';
		tokenUrl = p.token_url ?? '';
		userInfoUrl = p.userinfo_url ?? '';
		scopes = p.scopes || 'openid email profile';
		mappingRows = Object.entries(p.field_mapping ?? {}).map(([col, path]) => ({ col, path }));
		errorMsg = '';
	}

	async function copyCallback() {
		if (!callbackUrl) return;
		await navigator.clipboard.writeText(callbackUrl);
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}

	async function saveProvider() {
		if (!editingProvider) return;
		saving = true;
		errorMsg = '';
		try {
			const updated = await updateOAuthProvider(editingProvider.name, {
				enabled,
				client_id: clientId,
				client_secret: clientSecret,
				issuer_url: issuerUrl
			});
			providers = providers.map((p) => (p.name === updated.name ? updated : p));
			editingProvider = updated;
			clientSecret = '';
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to save provider';
		} finally {
			saving = false;
		}
	}
</script>

<div class="space-y-4">
	<div class="text-xs font-medium text-muted-foreground">Auth methods</div>

	<div class="divide-y divide-border overflow-hidden rounded-md border border-border bg-background">
		<div>
			<button
				type="button"
				onclick={() => (openSection = openSection === 'password' ? null : 'password')}
				class="flex w-full cursor-pointer items-center justify-between px-4 py-3 text-left transition hover:bg-accent/40"
			>
				<div class="flex items-center gap-2.5 text-sm font-medium">
					<Lock class="h-4 w-4 text-muted-foreground" />
					Identity/Password
				</div>
				<div class="flex items-center gap-2.5">
					<span class="rounded bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
						Enabled
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openSection === 'password' ? 'rotate-180' : ''}" />
				</div>
			</button>

			{#if openSection === 'password'}
				<div class="border-t border-border bg-muted/20 px-4 py-3 text-xs text-muted-foreground">
					Email & Password sign-in using <code class="rounded bg-muted px-1 py-0.5">Argon2id</code> +
					<code class="rounded bg-muted px-1 py-0.5">HS256 JWT</code>.
					<div class="mt-2 font-mono text-xs">
						POST /api/auth/register &middot; POST /api/auth/login &middot; GET /api/auth/me
					</div>
				</div>
			{/if}
		</div>

		<div>
			<button
				type="button"
				onclick={() => (openSection = openSection === 'oauth2' ? null : 'oauth2')}
				class="flex w-full cursor-pointer items-center justify-between px-4 py-3 text-left transition hover:bg-accent/40"
			>
				<div class="flex items-center gap-2.5 text-sm font-medium">
					<KeyRound class="h-4 w-4 text-muted-foreground" />
					OAuth2 / OIDC
				</div>
				<div class="flex items-center gap-2.5">
					<span
						class="rounded px-2 py-0.5 text-xs font-medium {anyOAuthEnabled
							? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
							: 'bg-muted text-muted-foreground'}"
					>
						{anyOAuthEnabled ? 'Enabled' : 'Disabled'}
					</span>
					<ChevronDown class="h-4 w-4 text-muted-foreground transition-transform {openSection === 'oauth2' ? 'rotate-180' : ''}" />
				</div>
			</button>

			{#if openSection === 'oauth2'}
				<div class="space-y-4 border-t border-border bg-muted/15 p-4">
					<div class="grid grid-cols-3 gap-2.5">
						{#each providers as p (p.name)}
							{@const active = p.enabled && Boolean(p.client_id) && p.has_secret}
							<button
								type="button"
								onclick={() => selectProvider(p)}
								class="flex cursor-pointer flex-col items-center gap-2 rounded-md border p-3 text-center transition {editingProvider?.name === p.name
									? 'border-foreground bg-background shadow-xs'
									: 'border-border bg-background/60 hover:bg-accent/50'}"
							>
								<ProviderIcon name={p.name} />
								<div class="text-xs font-medium">{p.display_name}</div>
								<span
									class="rounded px-1.5 py-0.5 text-xs font-medium {active
										? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
										: 'bg-muted text-muted-foreground'}"
								>
									{active ? 'Enabled' : 'Off'}
								</span>
							</button>
						{/each}
					</div>

					{#if editingProvider}
						<div class="space-y-3.5 rounded-md border border-border bg-background p-4">
							<div class="flex items-center justify-between">
								<span class="text-sm font-semibold">{editingProvider.display_name} settings</span>
								<Switch bind:checked={enabled} label={enabled ? 'Enabled' : 'Disabled'} />
							</div>

							<div>
								<div class="mb-1 text-xs font-medium text-muted-foreground">Callback URL</div>
								<div class="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5">
									<code class="truncate text-xs">{callbackUrl}</code>
									<button
										type="button"
										onclick={copyCallback}
										class="inline-flex shrink-0 cursor-pointer items-center gap-1 rounded px-2 py-0.5 text-xs font-medium hover:bg-accent"
									>
										{#if copied}
											<Check class="h-3 w-3 text-emerald-500" /> Copied
										{:else}
											<Copy class="h-3 w-3" /> Copy
										{/if}
									</button>
								</div>
							</div>

							<div class="grid gap-3 sm:grid-cols-2">
								<div>
									<label for="oa-cid" class="mb-1 block text-xs text-muted-foreground">Client ID</label>
									<input
										id="oa-cid"
										type="text"
										bind:value={clientId}
										placeholder="Client ID"
										class="w-full rounded-md border border-input bg-background px-3 py-2 text-xs outline-none focus:border-ring"
									/>
								</div>
								<div>
									<label for="oa-sec" class="mb-1 block text-xs text-muted-foreground">
										Client Secret {editingProvider.has_secret ? '(saved)' : ''}
									</label>
									<input
										id="oa-sec"
										type="password"
										bind:value={clientSecret}
										placeholder={editingProvider.has_secret ? '•••••••• (keep current)' : 'Client Secret'}
										class="w-full rounded-md border border-input bg-background px-3 py-2 text-xs outline-none focus:border-ring"
									/>
								</div>
							</div>

							{#if editingProvider.name === 'oidc'}
								<div class="space-y-3 rounded-md border border-border bg-muted/25 p-3.5">
									<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
										<div>
											<label for="oa-dname" class="mb-1 block text-xs font-medium">Display name</label>
											<input id="oa-dname" type="text" bind:value={displayName} placeholder="e.g. KMITL IAM" class="w-full rounded-md border border-input bg-background px-3 py-1.5 text-xs outline-none focus:border-ring" />
										</div>
										<div>
											<label for="oa-scopes" class="mb-1 block text-xs font-medium">Scopes</label>
											<input id="oa-scopes" type="text" bind:value={scopes} placeholder="openid email profile" class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring" />
										</div>
									</div>
									<div>
										<label for="oa-issuer" class="mb-1 block text-xs font-medium">OIDC Issuer URL (auto-discovery, optional if manual URLs set)</label>
										<input id="oa-issuer" type="url" bind:value={issuerUrl} placeholder="https://api.science.kmitl.ac.th/iam" class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring" />
									</div>
									<div class="grid grid-cols-1 gap-2.5">
										<div>
											<label for="oa-auth" class="mb-1 block text-xs font-medium">Auth URL (manual override)</label>
											<input id="oa-auth" type="url" bind:value={authUrl} placeholder="https://api.science.kmitl.ac.th/iam/oauth/authorize" class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring" />
										</div>
										<div>
											<label for="oa-token" class="mb-1 block text-xs font-medium">Token URL (manual override)</label>
											<input id="oa-token" type="url" bind:value={tokenUrl} placeholder="https://api.science.kmitl.ac.th/iam/oauth/token" class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring" />
										</div>
										<div>
											<label for="oa-userinfo" class="mb-1 block text-xs font-medium">UserInfo / Profile API URL</label>
											<input id="oa-userinfo" type="url" bind:value={userInfoUrl} placeholder="https://api.science.kmitl.ac.th/iam/o/v2/students" class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring" />
										</div>
									</div>

									<!-- IAM Claim -> _users Column Mapping -->
									<div class="border-t border-border pt-3">
										<div class="mb-2 flex items-center justify-between">
											<span class="text-xs font-semibold">Claim / Field Mapping (IAM JSON Path &rarr; _users column)</span>
											<button type="button" onclick={() => (mappingRows = [...mappingRows, { col: '', path: '' }])} class="inline-flex cursor-pointer items-center gap-1 rounded border border-border bg-card px-2 py-0.5 text-xs font-medium hover:bg-muted">
												<Plus class="h-3 w-3" /> Add mapping
											</button>
										</div>
										{#if mappingRows.length === 0}
											<p class="text-xs text-muted-foreground">Example: <code>sub &larr; student_id</code>, <code>name &larr; firstname_th</code>, <code>department &larr; department.name</code></p>
										{:else}
											<div class="space-y-1.5">
												{#each mappingRows as row, idx (idx)}
													<div class="flex items-center gap-2">
														<input type="text" bind:value={row.col} placeholder="_users col (e.g. name, sub)" class="w-1/2 rounded border border-input bg-background px-2.5 py-1 font-mono text-xs outline-none focus:border-ring" />
														<span class="text-xs text-muted-foreground">&larr;</span>
														<input type="text" bind:value={row.path} placeholder="IAM path (e.g. firstname_th)" class="w-1/2 rounded border border-input bg-background px-2.5 py-1 font-mono text-xs outline-none focus:border-ring" />
														<button type="button" onclick={() => (mappingRows = mappingRows.filter((_, i) => i !== idx))} class="cursor-pointer rounded p-1 text-muted-foreground hover:text-destructive">
															<Trash2 class="h-3.5 w-3.5" />
														</button>
													</div>
												{/each}
											</div>
										{/if}
									</div>
								</div>
							{/if}

							{#if errorMsg}
								<p class="rounded-md bg-destructive/15 px-3 py-2 text-xs text-destructive">{errorMsg}</p>
							{/if}

							<div class="flex items-center justify-between pt-1">
								{#if editingProvider.enabled && editingProvider.client_id && editingProvider.has_secret}
									<a
										href="/api/auth/oauth/{editingProvider.name}"
										target="_blank"
										rel="noreferrer"
										class="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
									>
										Test login <ExternalLink class="h-3 w-3" />
									</a>
								{:else}
									<span class="text-xs text-muted-foreground">
										External apps: <code>GET /api/auth/oauth/{editingProvider.name}?redirect_to=...</code>
									</span>
								{/if}

								<button
									type="button"
									disabled={saving}
									onclick={saveProvider}
									class="cursor-pointer rounded-md bg-foreground px-3.5 py-1.5 text-xs font-medium text-background transition hover:opacity-90 disabled:opacity-50"
								>
									{saving ? 'Saving...' : 'Save provider'}
								</button>
							</div>
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>


<div class="mt-4 space-y-3 rounded-lg border border-border bg-muted/25 p-4">
	<div class="flex items-center justify-between gap-3">
		<div>
			<div class="text-xs font-semibold">One-Time Password (Email OTP)</div>
			<div class="text-xs text-muted-foreground">Allow users to sign in with a 6-digit email OTP via <code>/api/auth/request-otp</code>.</div>
		</div>
		<Switch checked={otpEnabled} onChange={(v) => saveAuthOptions(v, mfaEnabled)} />
	</div>
	<div class="flex items-center justify-between gap-3 border-t border-border pt-3">
		<div>
			<div class="text-xs font-semibold">Multi-Factor Authentication (MFA)</div>
			<div class="text-xs text-muted-foreground">Require two distinct auth factors (Password + Email OTP / OAuth2).</div>
		</div>
		<Switch checked={mfaEnabled} onChange={(v) => saveAuthOptions(otpEnabled, v)} />
	</div>
</div>
