<script lang="ts">
	import { X } from '@lucide/svelte';
	import { createRecord, updateRecord } from '#lib/api/records.ts';
	import { createSuperuser, createUser, updateSuperuser, updateUser } from '#lib/api/users.ts';
	import type { Collection, RecordRow } from '#lib/api/types.ts';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';

	interface Props {
		open: boolean;
		collection: Collection;
		editingRecord?: RecordRow | null;
		onClose: () => void;
		onSaved: () => void;
	}

	let { open, collection, editingRecord = null, onClose, onSaved }: Props = $props();

	let values = $state<Record<string, string>>({});
	let boolValues = $state<Record<string, boolean>>({});
	let email = $state('');
	let role = $state('user');
	let password = $state('');
	let confirmPassword = $state('');
	let errorMsg = $state('');
	let saving = $state(false);

	let isSuperuser = $derived(collection.name === '_superusers');
	let isAuth = $derived(collection.type === 'auth' || isSuperuser);

	$effect(() => {
		if (open) {
			const nextVals: Record<string, string> = {};
			const nextBools: Record<string, boolean> = {};
			for (const f of collection.fields ?? []) {
				const existing = editingRecord ? editingRecord[f.name] : undefined;
				if (f.type === 'boolean') nextBools[f.name] = Boolean(existing);
				else if (existing === null || existing === undefined) nextVals[f.name] = '';
				else if (typeof existing === 'object') nextVals[f.name] = JSON.stringify(existing);
				else nextVals[f.name] = String(existing);
			}
			values = nextVals;
			boolValues = nextBools;
			email = editingRecord && editingRecord.email ? String(editingRecord.email) : '';
			role = editingRecord && editingRecord.role ? String(editingRecord.role) : 'user';
			password = '';
			confirmPassword = '';
			errorMsg = '';
		}
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		if (isAuth && (password !== '' || !editingRecord) && password !== confirmPassword) {
			errorMsg = 'Passwords do not match';
			return;
		}
		saving = true;

		try {
			if (isSuperuser) {
				if (editingRecord && typeof editingRecord.id === 'number') {
					await updateSuperuser(editingRecord.id, {
						email: email.trim(),
						password: password || undefined
					});
				} else {
					await createSuperuser(email.trim(), password);
				}
			} else if (isAuth) {
				if (editingRecord && typeof editingRecord.id === 'number') {
					await updateUser(editingRecord.id, {
						email: email.trim(),
						role: role.trim() || 'user',
						password: password || undefined
					});
				} else {
					await createUser(email.trim(), password);
				}
			} else {
				const payload: Record<string, unknown> = {};
				for (const field of collection.fields ?? []) {
					if (field.type === 'boolean') {
						payload[field.name] = Boolean(boolValues[field.name]);
						continue;
					}
					const raw = values[field.name];
					if (raw === undefined || raw === '') continue;

					if (field.type === 'integer' || field.type === 'relation') {
						payload[field.name] = parseInt(raw, 10);
					} else if (field.type === 'float' || field.type === 'number') {
						payload[field.name] = parseFloat(raw);
					} else if (field.type === 'json' || field.type === 'geopoint' || field.type === 'vector') {
						payload[field.name] = JSON.parse(raw);
					} else {
						payload[field.name] = raw;
					}
				}
				if (editingRecord && typeof editingRecord.id === 'number') {
					await updateRecord(collection.name, editingRecord.id, payload);
				} else {
					await createRecord(collection.name, payload);
				}
			}
			toast.success(editingRecord ? `Updated record #${editingRecord.id}` : `Created record in "${collection.name}"`);
			onSaved();
			onClose();
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to save record';
			toast.error(errorMsg);
		} finally {
			saving = false;
		}
	}
</script>

<Drawer {open} {onClose} maxWidth="max-w-xl">
	<div class="flex items-center justify-between border-b border-border px-6 py-4">
		<h2 class="text-base font-semibold">{editingRecord ? `Edit ${collection.name} #${editingRecord.id}` : `New ${collection.name} record`}</h2>
		<button
			type="button"
			onclick={onClose}
			class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-accent hover:text-foreground"
		>
			<X class="h-4 w-4" />
		</button>
	</div>

	<form onsubmit={handleSubmit} class="flex min-h-0 flex-1 flex-col">
		<div class="flex-1 space-y-4 overflow-y-auto px-6 py-5">
			{#if isAuth}
				<div>
					<label for="u-email" class="mb-1.5 block text-xs font-medium">
						email <span class="text-destructive">*</span>
					</label>
					<input
						id="u-email"
						type="email"
						required
						value={email}
						oninput={(e) => (email = e.currentTarget.value)}
						placeholder="user@example.com"
						class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
					/>
				</div>
				{#if !isSuperuser && editingRecord}
					<div>
						<label for="u-role" class="mb-1.5 block text-xs font-medium">role</label>
						<input
							id="u-role"
							type="text"
							value={role}
							oninput={(e) => (role = e.currentTarget.value)}
							placeholder="user or admin"
							class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 font-mono text-sm outline-none focus:border-ring"
						/>
					</div>
				{/if}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					<div>
						<label for="u-pass" class="mb-1.5 block text-xs font-medium">
							{editingRecord ? 'new password' : 'password'}
							{#if !editingRecord}<span class="text-destructive">*</span>{/if}
						</label>
						<input
							id="u-pass"
							type="password"
							required={!editingRecord}
							minlength={password ? 8 : undefined}
							value={password}
							oninput={(e) => (password = e.currentTarget.value)}
							placeholder={editingRecord ? 'Leave blank to keep' : 'Minimum 8 characters'}
							class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
						/>
					</div>
					<div>
						<label for="u-pass-confirm" class="mb-1.5 block text-xs font-medium">
							retype password
							{#if !editingRecord || password}<span class="text-destructive">*</span>{/if}
						</label>
						<input
							id="u-pass-confirm"
							type="password"
							required={!editingRecord || Boolean(password)}
							minlength={password ? 8 : undefined}
							value={confirmPassword}
							oninput={(e) => (confirmPassword = e.currentTarget.value)}
							placeholder={editingRecord ? 'Leave blank to keep' : 'Retype password'}
							class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
						/>
					</div>
				</div>
			{:else}
				{#each collection.fields ?? [] as field (field.name)}
					{#if field.type === 'boolean'}
						<div class="flex items-center justify-between rounded-md border border-border bg-muted/20 px-3.5 py-2.5">
							<div>
								<div class="text-xs font-medium">
									{field.name}{field.required ? ' *' : ''}
								</div>
								<div class="text-[11px] text-muted-foreground">boolean</div>
							</div>
							<Switch
								checked={Boolean(boolValues[field.name])}
								onChange={(v) => (boolValues[field.name] = v)}
							/>
						</div>
					{:else}
						<div>
							<label for="f-{field.name}" class="mb-1.5 flex items-center justify-between text-xs">
								<span class="font-medium">
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</span>
								<span class="font-mono text-[11px] text-muted-foreground">{field.type}</span>
							</label>

							{#if field.type === 'json' || field.type === 'geopoint' || field.type === 'editor'}
								<textarea
									id="f-{field.name}"
									rows="3"
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									placeholder={field.type === 'geopoint' ? '{"lon": 100.5, "lat": 13.75}' : field.type === 'json' ? '{"key": "value"}' : `Enter ${field.name}...`}
									class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 font-mono text-sm outline-none focus:border-ring"
								></textarea>
							{:else if field.type === 'integer' || field.type === 'relation' || field.type === 'float' || field.type === 'number'}
								<input
									id="f-{field.name}"
									type="number"
									step={field.type === 'float' || field.type === 'number' ? 'any' : '1'}
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									placeholder={`Enter ${field.name}...`}
									class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
								/>
							{:else}
								<input
									id="f-{field.name}"
									type="text"
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									placeholder={field.type === 'vector' ? '[0.1, 0.2, 0.3]' : `Enter ${field.name}...`}
									class="w-full rounded-md border border-input bg-background px-3.5 py-2.5 text-sm outline-none focus:border-ring"
								/>
							{/if}
						</div>
					{/if}
				{/each}
			{/if}

			{#if errorMsg}
				<p class="rounded-md bg-destructive/15 px-3 py-2 text-xs text-destructive">{errorMsg}</p>
			{/if}
		</div>

		<div class="flex items-center justify-end gap-2 border-t border-border bg-muted/20 px-6 py-3.5">
			<button
				type="button"
				onclick={onClose}
				class="cursor-pointer rounded-md border border-border px-3.5 py-1.5 text-xs font-medium hover:bg-accent"
			>
				Cancel
			</button>
			<button
				type="submit"
				disabled={saving}
				class="cursor-pointer rounded-md bg-foreground px-4 py-1.5 text-xs font-medium text-background hover:opacity-90 disabled:opacity-50"
			>
				{saving ? 'Saving...' : editingRecord ? 'Save changes' : 'Create'}
			</button>
		</div>
	</form>
</Drawer>
