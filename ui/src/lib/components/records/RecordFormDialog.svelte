<script lang="ts">
	import { Upload, X } from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY, apiRequest } from '#lib/api/client.ts';
	import { createRecord, listRecords, updateRecord } from '#lib/api/records.ts';
	import type { Collection, FieldDef, RecordRow } from '#lib/api/types.ts';
	import { createSuperuser, createUser, listUsers, updateSuperuser, updateUser } from '#lib/api/users.ts';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import { buildAndValidateRecordPayload } from '#lib/validation/validators.ts';

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
	let relationOptions = $state<Record<string, { value: string; label: string }[]>>({});
	let uploadingField = $state<string | null>(null);
	let email = $state('');
	let role = $state('user');
	let password = $state('');
	let confirmPassword = $state('');
	let errorMsg = $state('');
	let saving = $state(false);
	let impersonateToken = $state('');

	let isSuperuser = $derived(collection.name === '_superusers');
	let isAuth = $derived(collection.type === 'auth' || isSuperuser);
	let customFields = $derived(
		(collection.fields ?? []).filter((f) => !(isAuth && ['email', 'role'].includes(f.name)))
	);

	async function loadRelationChoices(fields: FieldDef[]) {
		const next: Record<string, { value: string; label: string }[]> = {};
		for (const f of fields) {
			if (f.type !== 'relation' || !f.collection_name) continue;
			try {
				const res =
					f.collection_name === 'users'
						? await listUsers({ page: 1, perPage: 100 })
						: await listRecords(f.collection_name, { page: 1, perPage: 100 });
				next[f.name] = [
					{ value: '', label: `Select ${f.collection_name}...` },
					...(res.items ?? []).map((row) => {
						const id = String(row.id ?? '');
						const label =
							String(row.title ?? row.name ?? row.email ?? row.username ?? `#${id}`);
						return { value: id, label: `#${id} — ${label}` };
					})
				];
			} catch {
				next[f.name] = [{ value: '', label: `Enter ${f.collection_name} ID` }];
			}
		}
		relationOptions = next;
	}

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
			impersonateToken = '';
			loadRelationChoices(collection.fields ?? []);
		}
	});

	async function handleFileUpload(fieldName: string, file: File | undefined) {
		if (!file) return;
		uploadingField = fieldName;
		try {
			const form = new FormData();
			form.append('file', file);
			const token = localStorage.getItem(ADMIN_TOKEN_KEY);
			const res = await fetch('/api/files', {
				method: 'POST',
				headers: token ? { Authorization: `Bearer ${token}` } : {},
				body: form
			});
			const json = await res.json();
			if (!res.ok) throw new Error(json?.error?.message || 'File upload failed');
			values = { ...values, [fieldName]: json.data?.url || json.url || '' };
			toast.success(`Uploaded "${file.name}" (${json.data?.backend || 'storage'})`);
		} catch (err) {
			toast.error(err, 'Upload failed');
		} finally {
			uploadingField = null;
		}
	}

	async function handleImpersonate() {
		if (!editingRecord?.id) return;
		try {
			const res = await apiRequest<{ token: string }>(`/api/users/${editingRecord.id}/impersonate`, { method: 'POST' });
			impersonateToken = res.token;
			await navigator.clipboard?.writeText(res.token).catch(() => {});
			toast.success('Impersonation JWT generated & copied to clipboard!');
		} catch (err) {
			toast.error(err);
		}
	}

	function isImageUrl(u: string): boolean {
		return /\.(png|jpe?g|gif|webp|svg)$/i.test(u);
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
			impersonateToken = '';
		if (isAuth) {
			if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
				errorMsg = 'Please enter a valid email address.';
				toast.error(errorMsg);
				return;
			}
			if ((!editingRecord || password !== '') && password.length < 8) {
				errorMsg = 'Password must be at least 8 characters.';
				toast.error(errorMsg);
				return;
			}
			if ((password !== '' || !editingRecord) && password !== confirmPassword) {
				errorMsg = 'Passwords do not match.';
				toast.error(errorMsg);
				return;
			}
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
					const { payload: customPayload, error: customErr } = buildAndValidateRecordPayload(
						customFields,
						values,
						boolValues
					);
					if (customErr) {
						errorMsg = customErr;
						toast.error(customErr);
						saving = false;
						return;
					}
					await updateUser(editingRecord.id, {
						...(customPayload ?? {}),
						email: email.trim(),
						role: role.trim() || 'user',
						password: password || undefined
					});
				} else {
					await createUser(email.trim(), password);
				}
			} else {
				const { payload, error } = buildAndValidateRecordPayload(
					collection.fields ?? [],
					values,
					boolValues
				);
				if (error || !payload) {
					errorMsg = error ?? 'Invalid record data';
					toast.error(errorMsg);
					saving = false;
					return;
				}
				if (editingRecord && (typeof editingRecord.id === 'number' || typeof editingRecord.id === 'string')) {
					await updateRecord(collection.name, editingRecord.id, payload);
				} else {
					await createRecord(collection.name, payload);
				}
			}
			toast.success(
				editingRecord
					? `Updated record #${editingRecord.id}`
					: `Created record in "${collection.name}"`
			);
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
		<h2 class="text-base font-semibold">
			{editingRecord ? `Edit ${collection.name} #${editingRecord.id}` : `New ${collection.name} record`}
		</h2>
		<button
			type="button"
			onclick={onClose}
			class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
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
						class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 text-sm outline-none focus:border-ring"
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
							class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 font-mono text-sm outline-none focus:border-ring"
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
							class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 text-sm outline-none focus:border-ring"
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
							class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 text-sm outline-none focus:border-ring"
						/>
					</div>
				</div>
			{/if}

			{#if !isSuperuser && (!isAuth || editingRecord)}
				{#each customFields as field (field.name)}
					{#if field.type === 'boolean'}
						<div class="flex items-center justify-between rounded-md border border-border bg-muted/35 px-3.5 py-2.5">
							<div>
								<div class="text-xs font-medium">
									{field.name}{field.required ? ' *' : ''}
								</div>
								<div class="text-xs text-muted-foreground">{field.help_text || 'boolean'}</div>
							</div>
							<Switch
								checked={Boolean(boolValues[field.name])}
								onChange={(v) => (boolValues[field.name] = v)}
							/>
						</div>
					{:else if field.type === 'file'}
						<div>
							<div class="mb-1.5 flex items-center justify-between text-xs">
								<span class="font-medium">
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</span>
								<span class="font-mono text-muted-foreground">file</span>
							</div>
							<div class="space-y-2 rounded-md border border-border bg-muted/30 p-3">
								{#if values[field.name]}
									<div class="flex items-center justify-between gap-2 rounded bg-card p-2">
										<div class="flex min-w-0 items-center gap-2.5">
											{#if isImageUrl(values[field.name])}
												<img
													src={values[field.name]}
													alt={field.name}
													class="h-10 w-10 rounded border border-border object-cover"
												/>
											{/if}
											<a
												href={values[field.name]}
												target="_blank"
												rel="noreferrer"
												class="truncate font-mono text-xs underline"
											>
												{values[field.name]}
											</a>
										</div>
										<button
											type="button"
											onclick={() => (values = { ...values, [field.name]: '' })}
											class="cursor-pointer rounded p-1 text-muted-foreground hover:bg-muted hover:text-destructive"
										>
											<X class="h-3.5 w-3.5" />
										</button>
									</div>
								{/if}
								<label class="inline-flex cursor-pointer items-center gap-2 rounded-md border border-border bg-card px-3 py-2 text-xs font-medium transition hover:bg-muted">
									<Upload class="h-3.5 w-3.5" />
									{uploadingField === field.name ? 'Uploading...' : 'Upload file (Local / S3)'}
									<input
										type="file"
										class="hidden"
										accept={field.mime_types?.join(',') || undefined}
										onchange={(e) => handleFileUpload(field.name, e.currentTarget.files?.[0])}
									/>
								</label>
							</div>
							{#if field.help_text}
								<p class="mt-1 text-xs text-muted-foreground">{field.help_text}</p>
							{/if}
						</div>
					{:else if field.type === 'select' && field.values?.length}
						<div>
							<div class="mb-1.5 flex items-center justify-between text-xs">
								<span class="font-medium">
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</span>
								<span class="font-mono text-muted-foreground">select</span>
							</div>
							<Select
								bind:value={() => values[field.name] ?? '', (v) => (values = { ...values, [field.name]: v })}
								options={[
									{ value: '', label: 'Select option...' },
									...field.values.map((v) => ({ value: v, label: v }))
								]}
							/>
							{#if field.help_text}
								<p class="mt-1 text-xs text-muted-foreground">{field.help_text}</p>
							{/if}
						</div>
					{:else if field.type === 'relation' && relationOptions[field.name]?.length}
						<div>
							<div class="mb-1.5 flex items-center justify-between text-xs">
								<span class="font-medium">
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</span>
								<span class="font-mono text-muted-foreground">relation &rarr; {field.collection_name}</span>
							</div>
							<Select
								bind:value={() => values[field.name] ?? '', (v) => (values = { ...values, [field.name]: v })}
								options={relationOptions[field.name]}
							/>
							{#if field.help_text}
								<p class="mt-1 text-xs text-muted-foreground">{field.help_text}</p>
							{/if}
						</div>
					{:else}
						<div>
							<label for="f-{field.name}" class="mb-1.5 flex items-center justify-between text-xs">
								<span class="font-medium">
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</span>
								<span class="font-mono text-muted-foreground">
									{field.type}{field.type === 'vector' ? `(${field.dimension})` : ''}
								</span>
							</label>
							{#if field.type === 'editor'}
								<div class="overflow-hidden rounded-md border border-input bg-card focus-within:border-ring">
									<div class="flex flex-wrap items-center gap-1 border-b border-border bg-muted/45 px-2 py-1.5 text-xs">
										{#each [
											{ lbl: 'B', wrap: ['<strong>', '</strong>'] },
											{ lbl: 'I', wrap: ['<em>', '</em>'] },
											{ lbl: 'H2', wrap: ['<h2>', '</h2>'] },
											{ lbl: 'Code', wrap: ['<code>', '</code>'] },
											{ lbl: 'Link', wrap: ['<a href="https://">', '</a>'] },
											{ lbl: 'List', wrap: ['<ul><li>', '</li></ul>'] }
										] as fmt (fmt.lbl)}
											<button
												type="button"
												onclick={() => (values[field.name] = (values[field.name] ?? '') + fmt.wrap[0] + 'text' + fmt.wrap[1])}
												class="cursor-pointer rounded border border-border bg-card px-2 py-0.5 font-mono text-xs font-semibold hover:bg-accent"
											>
												{fmt.lbl}
											</button>
										{/each}
									</div>
									<textarea
										id="f-{field.name}"
										rows="5"
										required={field.required}
										value={values[field.name] ?? ''}
										oninput={(e) => (values[field.name] = e.currentTarget.value)}
										placeholder="<p>Rich HTML or Markdown content...</p>"
										class="w-full bg-transparent px-3.5 py-2.5 text-sm outline-none"
									></textarea>
								</div>
							{:else if field.type === 'geopoint'}
								{@const gp = (() => { try { const o = JSON.parse(values[field.name] || '{}'); return { lat: o.lat ?? 13.7563, lon: o.lon ?? 100.5018 }; } catch { return { lat: 13.7563, lon: 100.5018 }; } })()}
								<div class="space-y-2 rounded-md border border-border bg-muted/30 p-3">
									<div class="grid grid-cols-2 gap-2">
										<div>
											<div class="mb-1 text-xs text-muted-foreground">Latitude</div>
											<input
												type="number"
												step="any"
												value={gp.lat}
												oninput={(e) => (values[field.name] = JSON.stringify({ lat: parseFloat(e.currentTarget.value) || 0, lon: gp.lon }))}
												class="w-full rounded-md border border-input bg-card px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
											/>
										</div>
										<div>
											<div class="mb-1 text-xs text-muted-foreground">Longitude</div>
											<input
												type="number"
												step="any"
												value={gp.lon}
												oninput={(e) => (values[field.name] = JSON.stringify({ lat: gp.lat, lon: parseFloat(e.currentTarget.value) || 0 }))}
												class="w-full rounded-md border border-input bg-card px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
											/>
										</div>
									</div>
									<iframe
										title="Map preview"
										class="h-40 w-full rounded border border-border"
										src="https://www.openstreetmap.org/export/embed.html?bbox={gp.lon - 0.02},{gp.lat - 0.015},{gp.lon + 0.02},{gp.lat + 0.015}&layer=mapnik&marker={gp.lat},{gp.lon}"
									></iframe>
								</div>
							{:else if field.type === 'json' || field.type === 'vector'}
								<textarea
									id="f-{field.name}"
									rows="3"
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									placeholder={field.type === 'vector' ? '[0.12, -0.34, ...]' : '{"key": "value"}'}
									class="w-full rounded-md border border-input bg-card px-3.5 py-2 font-mono text-xs outline-none focus:border-ring"
								></textarea>
							{:else if field.type === 'number' || field.type === 'integer' || field.type === 'float' || field.type === 'relation'}
								<input
									id="f-{field.name}"
									type="number"
									step={field.type === 'integer' || field.type === 'relation' ? '1' : 'any'}
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 font-mono text-sm outline-none focus:border-ring"
								/>
							{:else if field.type === 'email'}
								<input
									id="f-{field.name}"
									type="email"
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 text-sm outline-none focus:border-ring"
								/>
							{:else if field.type === 'url'}
								<input
									id="f-{field.name}"
									type="url"
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									placeholder="https://..."
									class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 text-sm outline-none focus:border-ring"
								/>
							{:else}
								<input
									id="f-{field.name}"
									type="text"
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									class="w-full rounded-md border border-input bg-card px-3.5 py-2.5 text-sm outline-none focus:border-ring"
								/>
							{/if}
							{#if field.help_text}
								<p class="mt-1 text-xs text-muted-foreground">{field.help_text}</p>
							{/if}
						</div>
					{/if}
				{/each}
			{/if}

			{#if errorMsg}
				<p class="rounded-md bg-destructive/15 px-3.5 py-2.5 text-xs text-destructive">{errorMsg}</p>
			{/if}
		</div>

		{#if impersonateToken}
			<div class="border-t border-border bg-emerald-500/10 px-6 py-3 text-xs">
				<div class="mb-1 font-semibold text-emerald-700 dark:text-emerald-400">Impersonation Bearer Token (copied):</div>
				<code class="block max-h-16 overflow-auto break-all rounded bg-card p-2 font-mono text-xs">{impersonateToken}</code>
			</div>
		{/if}
		<div class="flex items-center justify-between gap-2 border-t border-border bg-card px-6 py-3.5">
			<div>
				{#if isAuth && !isSuperuser && editingRecord}
					<button
						type="button"
						onclick={handleImpersonate}
						class="cursor-pointer rounded-md border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground transition hover:bg-muted"
					>
						Impersonate (JWT)
					</button>
				{/if}
			</div>
			<div class="flex items-center gap-2">
			<button
				type="button"
				onclick={onClose}
				class="cursor-pointer rounded-md px-4 py-2 text-xs font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				Cancel
			</button>
			<button
				type="submit"
				disabled={saving}
				class="cursor-pointer rounded-md bg-foreground px-5 py-2 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-50"
			>
				{saving ? 'Saving...' : editingRecord ? 'Save changes' : 'Create'}
			</button>
			</div>
		</div>
	</form>
</Drawer>
