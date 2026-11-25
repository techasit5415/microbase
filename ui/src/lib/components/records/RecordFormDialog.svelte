<script lang="ts">
	import { untrack } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { slide } from 'svelte/transition';
	import {
		Braces,
		Calendar,
		CloudUpload,
		Eye,
		EyeOff,
		FileImage,
		Hash,
		KeyRound,
		Link2,
		List,
		Lock,
		Mail,
		MapPin,
		PenLine,
		Search,
		Share2,
		Sparkles,
		Type,
		X
	} from '@lucide/svelte';
	import { ADMIN_TOKEN_KEY, apiRequest } from '#lib/api/client.ts';
	import { createRecord, listRecords, updateRecord } from '#lib/api/records.ts';
	import type { Collection, FieldDef, RecordRow } from '#lib/api/types.ts';
	import { createSuperuser, createUser, listUsers, updateSuperuser, updateUser } from '#lib/api/users.ts';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
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

	let recordId = $state('');
	let values = $state<Record<string, string>>({});
	let boolValues = $state<Record<string, boolean>>({});
	let relationOptions = $state<Record<string, { value: string; label: string }[]>>({});
	let openRelationPicker = $state<string | null>(null);
	let relationSearch = $state('');
	let uploadingField = $state<string | null>(null);
	let dragOverField = $state<string | null>(null);
	let email = $state('');
	let verified = $state(false);
	let emailVisibility = $state(false);
	let password = $state('');
	let confirmPassword = $state('');
	let showPassword = $state(false);
	let errorMsg = $state('');
	let saving = $state(false);
	let impersonateToken = $state('');
	let initialSnapshot = $state('');

	let isSuperuser = $derived(collection.name === '_superusers');
	let isAuth = $derived(collection.type === 'auth' || isSuperuser);
	let customFields = $derived(
		(collection.fields ?? []).filter(
			(f) =>
				!(
					isAuth &&
					['email', 'password', 'tokenKey', 'token_key', 'verified', 'email_visibility', 'emailVisibility'].includes(
						f.name
					)
				)
		)
	);

	function buildSnapshot(): string {
		return JSON.stringify({
			recordId,
			values,
			boolValues,
			email,
			verified,
			emailVisibility,
			password,
			confirmPassword
		});
	}

	let hasUnsavedChanges = $derived(open && initialSnapshot !== '' && buildSnapshot() !== initialSnapshot);

	async function requestClose() {
		if (hasUnsavedChanges) {
			const discard = await confirmAction({
				title: 'Unsaved changes',
				description: 'You have unsaved changes. Do you really want to close the panel?',
				confirmLabel: 'Discard changes',
				cancelLabel: 'Keep editing',
				destructive: true
			});
			if (!discard) return;
		}
		onClose();
	}

	async function loadRelationChoices(fields: FieldDef[]) {
		const next: Record<string, { value: string; label: string }[]> = {};
		for (const f of fields) {
			if (f.type !== 'relation' || !f.collection_name) continue;
			try {
				const res =
					f.collection_name === 'users'
						? await listUsers({ page: 1, perPage: 100 })
						: await listRecords(f.collection_name, { page: 1, perPage: 100 });
				next[f.name] = (res.items ?? []).map((row) => {
					const id = String(row.id ?? '');
					const label = String(row.title ?? row.name ?? row.email ?? row.username ?? `#${id}`);
					return { value: id, label: `${id} — ${label}` };
				});
			} catch {
				next[f.name] = [];
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
			recordId = editingRecord && editingRecord.id !== undefined ? String(editingRecord.id) : '';
			values = nextVals;
			boolValues = nextBools;
			email = editingRecord && editingRecord.email ? String(editingRecord.email) : '';
			verified = Boolean(editingRecord?.verified);
			emailVisibility = Boolean(editingRecord?.email_visibility);
			password = '';
			confirmPassword = '';
			showPassword = false;
			openRelationPicker = null;
			relationSearch = '';
			errorMsg = '';
			impersonateToken = '';
			initialSnapshot = untrack(() => buildSnapshot());
			loadRelationChoices(collection.fields ?? []);
		}
	});

	function generateRandomPassword() {
		const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*';
		let out = '';
		const arr = new Uint32Array(16);
		crypto.getRandomValues(arr);
		for (let i = 0; i < arr.length; i++) {
			out += chars[arr[i] % chars.length];
		}
		password = out;
		confirmPassword = out;
		showPassword = true;
		navigator.clipboard?.writeText(out).catch(() => {});
		toast.success('Random password generated & copied to clipboard');
	}

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

	function handleFileDrop(fieldName: string, e: DragEvent) {
		e.preventDefault();
		dragOverField = null;
		const file = e.dataTransfer?.files?.[0];
		if (file) handleFileUpload(fieldName, file);
	}

	async function handleImpersonate() {
		if (!editingRecord?.id) return;
		try {
			const res = await apiRequest<{ token: string }>(`/api/users/${editingRecord.id}/impersonate`, {
				method: 'POST'
			});
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
		saving = true;
		try {
			if (isSuperuser) {
				if (!email.trim().includes('@')) {
					errorMsg = 'Please enter a valid email address.';
					toast.error(errorMsg);
					saving = false;
					return;
				}
				if (!editingRecord || password.length > 0) {
					if (password.length < 8) {
						errorMsg = 'Password must be at least 8 characters.';
						toast.error(errorMsg);
						saving = false;
						return;
					}
					if (password !== confirmPassword) {
						errorMsg = 'Passwords do not match.';
						toast.error(errorMsg);
						saving = false;
						return;
					}
				}
				if (editingRecord && typeof editingRecord.id === 'number') {
					await updateSuperuser(editingRecord.id, {
						email: email.trim(),
						password: password || undefined
					});
				} else {
					await createSuperuser(email.trim(), password);
				}
			} else if (isAuth) {
				if (!email.trim().includes('@')) {
					errorMsg = 'Please enter a valid email address.';
					toast.error(errorMsg);
					saving = false;
					return;
				}
				if (!editingRecord || password.length > 0) {
					if (password.length < 8) {
						errorMsg = 'Password must be at least 8 characters.';
						toast.error(errorMsg);
						saving = false;
						return;
					}
					if (password !== confirmPassword) {
						errorMsg = 'Passwords do not match.';
						toast.error(errorMsg);
						saving = false;
						return;
					}
				}
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
				if (editingRecord && typeof editingRecord.id === 'number') {
					await updateUser(editingRecord.id, {
						...(customPayload ?? {}),
						email: email.trim(),
						verified,
						email_visibility: emailVisibility,
						password: password || undefined
					});
				} else {
					await createUser({
						...(customPayload ?? {}),
						email: email.trim(),
						password,
						verified,
						email_visibility: emailVisibility
					});
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
				if (!editingRecord && collection.id_type === 'text' && recordId.trim()) {
					payload.id = recordId.trim();
				}
				if (
					editingRecord &&
					(typeof editingRecord.id === 'number' || typeof editingRecord.id === 'string')
				) {
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

<Drawer {open} onClose={requestClose} maxWidth="max-w-xl">
	<!-- PocketBase Clean Header -->
	<div class="flex items-center justify-between px-6 pt-5 pb-3">
		<h2 class="text-base font-normal text-foreground">
			{editingRecord ? 'Edit ' : 'Create '}<strong class="font-semibold">{collection.name}</strong> record
		</h2>
		<div class="flex items-center gap-2">
			{#if editingRecord && isAuth && !isSuperuser}
				<button
					type="button"
					onclick={handleImpersonate}
					class="cursor-pointer rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-accent"
				>
					Impersonate
				</button>
			{/if}
			<button
				type="button"
				onclick={requestClose}
				class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				<X class="h-4 w-4" />
			</button>
		</div>
	</div>

	<form onsubmit={handleSubmit} class="flex min-h-0 flex-1 flex-col">
		<div class="flex-1 space-y-3.5 overflow-y-auto px-6 py-3">
			{#if impersonateToken}
				<div
					transition:slide={{ duration: 160, easing: cubicOut }}
					class="rounded-md border border-emerald-500/30 bg-emerald-500/10 px-3.5 py-2.5 text-xs"
				>
					<div class="mb-1 font-semibold text-emerald-700 dark:text-emerald-400">
						Impersonation JWT (copied):
					</div>
					<code class="block break-all font-mono text-[11px]">{impersonateToken}</code>
				</div>
			{/if}

			<!-- id field (PocketBase style filled box) -->
			<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5 transition focus-within:bg-secondary focus-within:ring-2 focus-within:ring-foreground/15">
				<label for="rec-id" class="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
					<KeyRound class="h-3.5 w-3.5" />
					<span>id</span>
				</label>
				<input
					id="rec-id"
					type="text"
					readonly={Boolean(editingRecord) || isAuth || collection.id_type !== 'text'}
					value={recordId}
					oninput={(e) => (recordId = e.currentTarget.value)}
					placeholder="Leave empty to autogenerate..."
					class="w-full bg-transparent font-mono text-sm text-foreground outline-none placeholder:font-sans placeholder:text-muted-foreground/70 disabled:opacity-60"
				/>
			</div>

			{#if isAuth}
				<!-- email box with inline Public: Off/On toggle (PocketBase style) -->
				<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5 transition focus-within:bg-secondary focus-within:ring-2 focus-within:ring-foreground/15">
					<div class="mb-1 flex items-center justify-between">
						<label for="u-email" class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
							<Mail class="h-3.5 w-3.5" />
							<span>email <span class="text-destructive">*</span></span>
						</label>
						{#if !isSuperuser}
							<button
								type="button"
								onclick={() => (emailVisibility = !emailVisibility)}
								class="cursor-pointer rounded px-2 py-0.5 text-xs font-medium transition {emailVisibility
									? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
									: 'text-muted-foreground hover:bg-background/60 hover:text-foreground'}"
							>
								Public: {emailVisibility ? 'On' : 'Off'}
							</button>
						{/if}
					</div>
					<input
						id="u-email"
						type="email"
						required
						value={email}
						oninput={(e) => (email = e.currentTarget.value)}
						placeholder="user@example.com"
						class="w-full bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground/60"
					/>
				</div>

				<!-- Unified Split Password + Confirm box (PocketBase style) -->
				<div>
					<div class="grid grid-cols-1 divide-y divide-border/60 rounded-[5px] bg-secondary/75 transition focus-within:bg-secondary focus-within:ring-2 focus-within:ring-foreground/15 sm:grid-cols-2 sm:divide-x sm:divide-y-0">
						<div class="px-3.5 py-2.5">
							<label for="u-pass" class="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
								<Lock class="h-3.5 w-3.5" />
								<span>
									{editingRecord ? 'New password' : 'Password'}
									{#if !editingRecord}<span class="text-destructive">*</span>{/if}
								</span>
							</label>
							<div class="flex items-center gap-1.5">
								<input
									id="u-pass"
									type={showPassword ? 'text' : 'password'}
									required={!editingRecord}
									minlength={password ? 8 : undefined}
									value={password}
									oninput={(e) => (password = e.currentTarget.value)}
									placeholder={editingRecord ? 'Leave blank to keep' : 'Min 8 characters'}
									class="min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground/60"
								/>
								<button
									type="button"
									onclick={() => (showPassword = !showPassword)}
									title={showPassword ? 'Hide password' : 'Show password'}
									class="cursor-pointer rounded p-1 text-muted-foreground transition hover:bg-background/60 hover:text-foreground"
								>
									{#if showPassword}
										<EyeOff class="h-4 w-4" />
									{:else}
										<Eye class="h-4 w-4" />
									{/if}
								</button>
							</div>
						</div>
						<div class="px-3.5 py-2.5">
							<label for="u-pass-confirm" class="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
								<Lock class="h-3.5 w-3.5" />
								<span>
									Confirm
									{#if !editingRecord || password}<span class="text-destructive">*</span>{/if}
								</span>
							</label>
							<div class="flex items-center gap-1.5">
								<input
									id="u-pass-confirm"
									type={showPassword ? 'text' : 'password'}
									required={!editingRecord || Boolean(password)}
									minlength={password ? 8 : undefined}
									value={confirmPassword}
									oninput={(e) => (confirmPassword = e.currentTarget.value)}
									placeholder={editingRecord ? 'Leave blank to keep' : 'Retype password'}
									class="min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground/60"
								/>
								<button
									type="button"
									onclick={() => (showPassword = !showPassword)}
									title={showPassword ? 'Hide password' : 'Show password'}
									class="cursor-pointer rounded p-1 text-muted-foreground transition hover:bg-background/60 hover:text-foreground"
								>
									{#if showPassword}
										<EyeOff class="h-4 w-4" />
									{:else}
										<Eye class="h-4 w-4" />
									{/if}
								</button>
							</div>
						</div>
					</div>
					<div class="mt-1.5 flex items-center justify-between px-0.5">
						<button
							type="button"
							onclick={generateRandomPassword}
							class="cursor-pointer text-xs text-muted-foreground transition hover:text-foreground hover:underline"
						>
							Generate and set random password
						</button>

					</div>
				</div>

				<!-- Clean standalone verified toggle + divider (PocketBase style) -->
				{#if !isSuperuser}
					<div class="flex items-center gap-3 pt-1 pb-2">
						<Switch checked={verified} onChange={(v) => (verified = v)} />
						<span class="text-sm text-foreground">verified</span>
					</div>
					{#if customFields.length > 0}
						<hr class="border-border/70" />
					{/if}
				{/if}
			{/if}

			{#if !isSuperuser}
				{#each customFields as field (field.name)}
					{#if field.type === 'boolean'}
						<div class="flex items-center gap-3 py-1">
							<Switch
								checked={Boolean(boolValues[field.name])}
								onChange={(v) => (boolValues[field.name] = v)}
							/>
							<div>
								<div class="text-sm text-foreground">
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</div>
								{#if field.help_text}
									<div class="text-xs text-muted-foreground">{field.help_text}</div>
								{/if}
							</div>
						</div>
					{:else if field.type === 'file'}
						<!-- PocketBase File Dropzone Box -->
						<div class="overflow-hidden rounded-[5px] bg-secondary/75">
							<div class="flex items-center gap-1.5 border-b border-border/60 px-3.5 py-2 text-xs font-medium text-muted-foreground">
								<FileImage class="h-3.5 w-3.5" />
								<span>{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}</span>
							</div>
							{#if values[field.name]}
								<div class="flex items-center justify-between gap-2 border-b border-border/50 bg-card/70 px-3.5 py-2">
									<div class="flex min-w-0 items-center gap-2.5">
										{#if isImageUrl(values[field.name])}
											<img
												src={values[field.name]}
												alt={field.name}
												class="h-9 w-9 rounded border border-border object-cover"
											/>
										{/if}
										<a
											href={values[field.name]}
											target="_blank"
											rel="noreferrer"
											class="truncate font-mono text-xs text-foreground underline"
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
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<label
								ondragover={(e) => {
									e.preventDefault();
									dragOverField = field.name;
								}}
								ondragleave={() => (dragOverField = null)}
								ondrop={(e) => handleFileDrop(field.name, e)}
								class="flex w-full cursor-pointer items-center justify-center gap-2 px-3.5 py-3 text-xs font-medium text-foreground transition hover:bg-secondary {dragOverField ===
								field.name
									? 'bg-accent ring-2 ring-inset ring-foreground/30'
									: ''}"
							>
								<CloudUpload class="h-4 w-4 text-muted-foreground" />
								<span>{uploadingField === field.name ? 'Uploading...' : 'Upload or drop new file'}</span>
								<input
									type="file"
									class="hidden"
									accept={field.mime_types?.join(',') || undefined}
									onchange={(e) => handleFileUpload(field.name, e.currentTarget.files?.[0])}
								/>
							</label>
						</div>
					{:else if field.type === 'relation'}
						<!-- PocketBase Relation Picker Box -->
						<div class="overflow-hidden rounded-[5px] bg-secondary/75">
							<div class="flex items-center justify-between border-b border-border/60 px-3.5 py-2 text-xs font-medium text-muted-foreground">
								<span class="flex items-center gap-1.5">
									<Share2 class="h-3.5 w-3.5" />
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</span>
								{#if field.collection_name}
									<span class="font-mono text-[11px]">&rarr; {field.collection_name}</span>
								{/if}
							</div>
							{#if values[field.name]}
								{@const matched = (relationOptions[field.name] ?? []).find((o) => o.value === values[field.name])}
								<div class="flex items-center justify-between border-b border-border/50 bg-card/70 px-3.5 py-2 text-xs">
									<span class="truncate font-mono text-foreground">{matched?.label ?? values[field.name]}</span>
									<button
										type="button"
										onclick={() => (values = { ...values, [field.name]: '' })}
										class="cursor-pointer rounded p-0.5 text-muted-foreground hover:text-destructive"
									>
										<X class="h-3.5 w-3.5" />
									</button>
								</div>
							{/if}
							<button
								type="button"
								onclick={() => {
									openRelationPicker = openRelationPicker === field.name ? null : field.name;
									relationSearch = '';
								}}
								class="flex w-full cursor-pointer items-center justify-center gap-2 px-3.5 py-3 text-xs font-medium text-foreground transition hover:bg-secondary"
							>
								<Search class="h-3.5 w-3.5 text-muted-foreground" />
								<span>{openRelationPicker === field.name ? 'Close records picker' : 'Open records picker'}</span>
							</button>
							{#if openRelationPicker === field.name}
								{@const filteredRel = (relationOptions[field.name] ?? []).filter((o) =>
									o.label.toLowerCase().includes(relationSearch.trim().toLowerCase())
								)}
								<div transition:slide={{ duration: 160, easing: cubicOut }} class="border-t border-border/60 bg-card p-2.5">
									<input
										type="text"
										bind:value={relationSearch}
										placeholder="Search records or type ID..."
										class="mb-2 w-full rounded border border-input bg-background px-2.5 py-1.5 text-xs outline-none focus:border-ring"
									/>
									<div class="max-h-40 space-y-0.5 overflow-y-auto">
										{#if filteredRel.length === 0}
											<div class="px-2 py-2 text-center text-xs text-muted-foreground">No records found.</div>
										{:else}
											{#each filteredRel as opt (opt.value)}
												<button
													type="button"
													onclick={() => {
														values = { ...values, [field.name]: opt.value };
														openRelationPicker = null;
													}}
													class="flex w-full cursor-pointer items-center justify-between rounded px-2.5 py-1.5 text-left font-mono text-xs transition hover:bg-accent {values[
														field.name
													] === opt.value
														? 'bg-accent font-semibold text-foreground'
														: 'text-muted-foreground'}"
												>
													<span class="truncate">{opt.label}</span>
												</button>
											{/each}
										{/if}
									</div>
								</div>
							{/if}
						</div>
					{:else if field.type === 'select'}
						<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5">
							<div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
								<List class="h-3.5 w-3.5" />
								<span>{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}</span>
							</div>
							<Select
								bind:value={() => values[field.name] ?? '', (v) => (values = { ...values, [field.name]: v })}
								options={[
									{ value: '', label: 'Select option...' },
									...(field.values ?? []).map((v) => ({ value: v, label: v }))
								]}
							/>
						</div>
					{:else if field.type === 'editor'}
						<div class="overflow-hidden rounded-[5px] bg-secondary/75 focus-within:ring-2 focus-within:ring-foreground/15">
							<div class="flex items-center justify-between border-b border-border/60 px-3.5 py-2">
								<span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
									<PenLine class="h-3.5 w-3.5" />
									{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}
								</span>
								<div class="flex flex-wrap items-center gap-1">
									{#each [
										{ lbl: 'B', wrap: ['<strong>', '</strong>'] },
										{ lbl: 'I', wrap: ['<em>', '</em>'] },
										{ lbl: 'H2', wrap: ['<h2>', '</h2>'] },
										{ lbl: 'Code', wrap: ['<code>', '</code>'] },
										{ lbl: 'Link', wrap: ['<a href="https://">', '</a>'] }
									] as fmt (fmt.lbl)}
										<button
											type="button"
											onclick={() =>
												(values[field.name] = (values[field.name] ?? '') + fmt.wrap[0] + 'text' + fmt.wrap[1])}
											class="cursor-pointer rounded bg-card px-1.5 py-0.5 font-mono text-[11px] font-semibold hover:bg-accent"
										>
											{fmt.lbl}
										</button>
									{/each}
								</div>
							</div>
							<textarea
								id="f-{field.name}"
								rows="4"
								required={field.required}
								value={values[field.name] ?? ''}
								oninput={(e) => (values[field.name] = e.currentTarget.value)}
								placeholder="<p>Rich HTML or Markdown content...</p>"
								class="w-full bg-transparent px-3.5 py-2.5 text-sm outline-none"
							></textarea>
						</div>
					{:else if field.type === 'geopoint'}
						{@const gp = (() => {
							try {
								const o = JSON.parse(values[field.name] || '{}');
								return { lat: o.lat ?? 13.7563, lon: o.lon ?? 100.5018 };
							} catch {
								return { lat: 13.7563, lon: 100.5018 };
							}
						})()}
						<div class="space-y-2 rounded-[5px] bg-secondary/75 p-3.5">
							<div class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
								<MapPin class="h-3.5 w-3.5" />
								<span>{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}</span>
							</div>
							<div class="grid grid-cols-2 gap-2">
								<input
									type="number"
									step="any"
									value={gp.lat}
									placeholder="Latitude"
									oninput={(e) =>
										(values[field.name] = JSON.stringify({
											lat: parseFloat(e.currentTarget.value) || 0,
											lon: gp.lon
										}))}
									class="w-full rounded bg-card px-3 py-1.5 font-mono text-xs outline-none"
								/>
								<input
									type="number"
									step="any"
									value={gp.lon}
									placeholder="Longitude"
									oninput={(e) =>
										(values[field.name] = JSON.stringify({
											lat: gp.lat,
											lon: parseFloat(e.currentTarget.value) || 0
										}))}
									class="w-full rounded bg-card px-3 py-1.5 font-mono text-xs outline-none"
								/>
							</div>
							<iframe
								title="Map preview"
								class="h-36 w-full rounded border border-border"
								src="https://www.openstreetmap.org/export/embed.html?bbox={gp.lon - 0.02},{gp.lat - 0.015},{gp.lon + 0.02},{gp.lat + 0.015}&layer=mapnik&marker={gp.lat},{gp.lon}"
							></iframe>
						</div>
					{:else if field.type === 'json' || field.type === 'vector'}
						<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5 transition focus-within:bg-secondary focus-within:ring-2 focus-within:ring-foreground/15">
							<label for="f-{field.name}" class="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
								{#if field.type === 'vector'}
									<Sparkles class="h-3.5 w-3.5" />
								{:else}
									<Braces class="h-3.5 w-3.5" />
								{/if}
								<span>{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}</span>
							</label>
							<textarea
								id="f-{field.name}"
								rows="3"
								required={field.required}
								value={values[field.name] ?? ''}
								oninput={(e) => (values[field.name] = e.currentTarget.value)}
								placeholder={field.type === 'vector' ? '[0.12, -0.34, ...]' : '{"key": "value"}'}
								class="w-full bg-transparent font-mono text-xs text-foreground outline-none placeholder:text-muted-foreground/60"
							></textarea>
						</div>
					{:else}
						<!-- Standard PocketBase Soft-Gray Input Box (text, email, url, number, datetime) -->
						<div class="rounded-[5px] bg-secondary/75 px-3.5 py-2.5 transition focus-within:bg-secondary focus-within:ring-2 focus-within:ring-foreground/15">
							<label for="f-{field.name}" class="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
								{#if field.type === 'email'}
									<Mail class="h-3.5 w-3.5" />
								{:else if field.type === 'url'}
									<Link2 class="h-3.5 w-3.5" />
								{:else if field.type === 'number' || field.type === 'integer' || field.type === 'float'}
									<Hash class="h-3.5 w-3.5" />
								{:else if field.type === 'datetime' || field.type === 'autodate'}
									<Calendar class="h-3.5 w-3.5" />
								{:else}
									<Type class="h-3.5 w-3.5" />
								{/if}
								<span>{field.name}{#if field.required}<span class="text-destructive"> *</span>{/if}</span>
							</label>
							{#if field.type === 'number' || field.type === 'integer' || field.type === 'float'}
								<input
									id="f-{field.name}"
									type="number"
									step={field.only_int || field.type === 'integer' ? '1' : 'any'}
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									class="w-full bg-transparent text-sm text-foreground outline-none"
								/>
							{:else if field.type === 'datetime'}
								<input
									id="f-{field.name}"
									type="datetime-local"
									required={field.required}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									class="w-full bg-transparent text-sm text-foreground outline-none"
								/>
							{:else}
								<input
									id="f-{field.name}"
									type={field.type === 'email' ? 'email' : field.type === 'url' ? 'url' : 'text'}
									required={field.required && !field.autogenerate_pattern}
									value={values[field.name] ?? ''}
									oninput={(e) => (values[field.name] = e.currentTarget.value)}
									placeholder={field.autogenerate_pattern
										? 'Leave empty to autogenerate...'
										: field.help_text || ''}
									class="w-full bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground/70"
								/>
							{/if}
						</div>
					{/if}
				{/each}
			{/if}

			{#if errorMsg}
				<p class="rounded-md bg-destructive/15 px-3.5 py-2.5 text-xs text-destructive">{errorMsg}</p>
			{/if}
		</div>

		<!-- PocketBase Clean Footer -->
		<div class="flex items-center justify-between border-t border-border bg-card px-6 py-3.5">
			<div class="flex items-center gap-3">
				<button
					type="button"
					onclick={requestClose}
					class="cursor-pointer rounded-md px-4 py-2 text-xs font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					Close
				</button>
				{#if hasUnsavedChanges}
					<span class="inline-flex items-center gap-1.5 text-xs font-medium text-amber-600 dark:text-amber-400">
						<span class="h-1.5 w-1.5 rounded-full bg-amber-500"></span>
						Unsaved changes
					</span>
				{/if}
			</div>
			<button
				type="submit"
				disabled={saving}
				class="cursor-pointer rounded-md bg-foreground px-6 py-2 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-50"
			>
				{saving ? 'Saving...' : editingRecord ? 'Save changes' : 'Create'}
			</button>
		</div>
	</form>
</Drawer>
