<script lang="ts">
	import { onMount } from 'svelte';
	import { Mail, Save, Send } from '@lucide/svelte';
	import { apiRequest } from '#lib/api/client.ts';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';

	let enabled = $state(false);
	let host = $state('');
	let port = $state(587);
	let username = $state('');
	let password = $state('');
	let hasPassword = $state(false);
	let senderName = $state('Microbase');
	let senderAddress = $state('noreply@microbase.dev');
	let testTo = $state('');
	let saving = $state(false);
	let testing = $state(false);

	onMount(async () => {
		try {
			const res = await apiRequest<{
				enabled: boolean;
				host: string;
				port: number;
				username: string;
				has_password: boolean;
				sender_name: string;
				sender_address: string;
			}>('/api/settings/mail');
			enabled = Boolean(res.enabled);
			host = res.host ?? '';
			port = res.port || 587;
			username = res.username ?? '';
			hasPassword = Boolean(res.has_password);
			senderName = res.sender_name || 'Microbase';
			senderAddress = res.sender_address || '';
		} catch {
			// Ignore
		}
	});

	async function handleSave() {
		saving = true;
		try {
			const res = await apiRequest<{ has_password: boolean }>('/api/settings/mail', {
				method: 'PUT',
				body: JSON.stringify({
					enabled,
					host: host.trim(),
					port: Number(port) || 587,
					username: username.trim(),
					password: password.trim(),
					sender_name: senderName.trim(),
					sender_address: senderAddress.trim()
				})
			});
			hasPassword = res.has_password;
			password = '';
			toast.success('Saved SMTP Mail settings');
		} catch (err) {
			toast.error(err);
		} finally {
			saving = false;
		}
	}

	async function handleTest() {
		if (!testTo.includes('@')) return toast.error('Enter a valid test recipient email.');
		testing = true;
		try {
			await apiRequest('/api/settings/mail/test', {
				method: 'POST',
				body: JSON.stringify({ to: testTo.trim() })
			});
			toast.success(`Test email sent to ${testTo.trim()}`);
		} catch (err) {
			toast.error(err);
		} finally {
			testing = false;
		}
	}
</script>

<section class="space-y-4 rounded-lg border border-border bg-card p-5">
	<div class="flex items-center justify-between gap-3">
		<div>
			<h2 class="flex items-center gap-2 text-sm font-semibold">
				<Mail class="h-4 w-4 text-muted-foreground" />
				Mail Settings (SMTP)
			</h2>
			<p class="mt-0.5 text-xs text-muted-foreground">
				Configure SMTP server for verification, password reset, and alert emails.
			</p>
		</div>
		<Switch checked={enabled} onChange={(v) => (enabled = v)} />
	</div>

	{#if enabled}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
			<div>
				<label for="smtp-host" class="mb-1 block text-xs font-medium">SMTP Host</label>
				<input id="smtp-host" type="text" bind:value={host} placeholder="smtp.gmail.com" class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
			</div>
			<div>
				<label for="smtp-port" class="mb-1 block text-xs font-medium">Port</label>
				<input id="smtp-port" type="number" bind:value={port} placeholder="587" class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
			</div>
			<div>
				<label for="smtp-user" class="mb-1 block text-xs font-medium">Username</label>
				<input id="smtp-user" type="text" bind:value={username} placeholder="user@example.com" class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
			</div>
			<div>
				<label for="smtp-pass" class="mb-1 block text-xs font-medium">
					Password {#if hasPassword}<span class="text-muted-foreground">(saved)</span>{/if}
				</label>
				<input id="smtp-pass" type="password" bind:value={password} placeholder={hasPassword ? 'Leave blank to keep' : 'App password'} class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
			</div>
			<div>
				<label for="smtp-sname" class="mb-1 block text-xs font-medium">Sender Name</label>
				<input id="smtp-sname" type="text" bind:value={senderName} class="w-full rounded-md border border-input bg-background px-3 py-2 text-xs outline-none focus:border-ring" />
			</div>
			<div>
				<label for="smtp-saddr" class="mb-1 block text-xs font-medium">Sender Address</label>
				<input id="smtp-saddr" type="email" bind:value={senderAddress} class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring" />
			</div>
		</div>

		<div class="flex flex-wrap items-center gap-2 border-t border-border pt-3">
			<input type="email" bind:value={testTo} placeholder="Send test email to..." class="min-w-[200px] flex-1 rounded-md border border-input bg-background px-3 py-2 text-xs outline-none focus:border-ring" />
			<button type="button" disabled={testing} onclick={handleTest} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-3.5 py-2 text-xs font-medium hover:bg-muted disabled:opacity-50">
				<Send class="h-3.5 w-3.5" />
				{testing ? 'Sending...' : 'Send test'}
			</button>
		</div>
	{/if}

	<div class="flex justify-end border-t border-border pt-3">
		<button type="button" disabled={saving} onclick={handleSave} class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-foreground px-4 py-2 text-xs font-semibold text-background hover:opacity-90 disabled:opacity-50">
			<Save class="h-3.5 w-3.5" />
			{saving ? 'Saving...' : 'Save Mail Settings'}
		</button>
	</div>
</section>
