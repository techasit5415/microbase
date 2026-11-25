<script lang="ts">
	import {
		CalendarCheck,
		Lock,
		LockOpen,
		Mail,
		Plus,
		Settings2,
		ToggleLeft,
		Type,
		X
	} from '@lucide/svelte';
	import { createCollection, updateCollection } from '#lib/api/collections.ts';
	import type { Collection, FieldDef, FieldType, IndexDef } from '#lib/api/types.ts';
	import AuthOptionsTab from '#lib/components/oauth/AuthOptionsTab.svelte';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import { validateCollectionForm } from '#lib/validation/validators.ts';
	import FieldRowInput from './FieldRowInput.svelte';
	import IndexModal from './IndexModal.svelte';
	import NewFieldPicker from './NewFieldPicker.svelte';

	interface Props {
		open: boolean;
		editing?: Collection | null;
		availableCollections?: string[];
		onClose: () => void;
		onSaved: (col: Collection, isEdit: boolean) => void;
	}

	let { open, editing = null, availableCollections = [], onClose, onSaved }: Props = $props();

	let activeTab = $state<'fields' | 'rules' | 'options'>('fields');
	let name = $state('');
	let fields = $state<FieldDef[]>([]);
	let indexes = $state<IndexDef[]>([]);
	let apiRules = $state<Record<string, string>>({});
	let errorMsg = $state('');
	let saving = $state(false);

	let showIndexModal = $state(false);
	let editingIndexPos = $state<number | null>(null);

	let isAuth = $derived(editing?.type === 'auth' || editing?.name === 'users');

	const authSystemFields = [
		{ name: 'id', icon: 'text', badges: ['Required'] },
		{ name: 'password', icon: 'lock', badges: ['Required', 'Hidden'] },
		{ name: 'email', icon: 'mail', badges: ['Required'] },
		{ name: 'email_visibility', icon: 'bool', badges: [] },
		{ name: 'verified', icon: 'bool', badges: [] },
		{ name: 'role', icon: 'text', badges: [] }
	];

	const baseSystemFields = [{ name: 'id', icon: 'text', badges: ['Required'] }];

	let systemFieldsList = $derived(isAuth ? authSystemFields : baseSystemFields);

	let availableIndexCols = $derived.by(() => {
		const userCols = fields.map((f) => f.name.trim()).filter((n) => n !== '');
		const sysCols = isAuth
			? ['id', 'email', 'verified', 'role', 'created_at', 'updated_at']
			: ['id', 'created_at', 'updated_at'];
		return Array.from(new Set([...sysCols, ...userCols]));
	});

	const ruleKeys = [
		{ key: 'list', label: 'List/Search rule' },
		{ key: 'view', label: 'View rule' },
		{ key: 'create', label: 'Create rule' },
		{ key: 'update', label: 'Update rule' },
		{ key: 'delete', label: 'Delete rule' }
	];

	$effect(() => {
		if (open) {
			errorMsg = '';
			activeTab = 'fields';
			if (editing) {
				name = editing.name;
				fields = (editing.fields ?? [])
					.filter((f) => !(editing.name === 'users' && ['email', 'role'].includes(f.name)))
					.map((f) => ({
						...f,
						original_name: f.name,
						required: Boolean(f.required),
						unique: Boolean(f.unique)
					}));
				indexes = (editing.indexes ?? []).map((idx) => ({
					name: idx.name,
					columns: [...idx.columns],
					unique: Boolean(idx.unique)
				}));
				apiRules = { ...(editing.api_rules ?? {}) };
			} else {
				name = '';
				fields = [{ name: 'title', type: 'text', required: true, unique: false }];
				indexes = [];
				apiRules = {};
			}
		}
	});

	function generateDefaultFieldName(base: string): string {
		const existing = new Set([
			...fields.map((f) => f.name.trim().toLowerCase()),
			...systemFieldsList.map((sf) => sf.name)
		]);
		if (!existing.has(base)) return base;
		let n = 2;
		while (existing.has(`${base}${n}`)) n++;
		return `${base}${n}`;
	}

	function addField(type: FieldType) {
		fields = [
			...fields,
			{
				name: generateDefaultFieldName(type),
				type,
				required: false,
				unique: false,
				dimension: type === 'vector' ? 768 : undefined
			}
		];
	}

	function removeField(index: number) {
		fields = fields.filter((_, i) => i !== index);
	}

	function openNewIndex() {
		editingIndexPos = null;
		showIndexModal = true;
	}

	function openEditIndex(pos: number) {
		editingIndexPos = pos;
		showIndexModal = true;
	}

	function handleSaveIndex(idx: IndexDef) {
		if (editingIndexPos !== null) {
			indexes = indexes.map((item, i) => (i === editingIndexPos ? idx : item));
		} else {
			indexes = [...indexes, idx];
		}
	}

	function removeIndex(pos: number) {
		indexes = indexes.filter((_, i) => i !== pos);
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		const validationErr = validateCollectionForm(name, fields, indexes, Boolean(editing));
		if (validationErr) {
			errorMsg = validationErr;
			toast.error(validationErr);
			return;
		}
		saving = true;
		try {
			const cleanFields = fields.map((f) => ({
				name: f.name.trim(),
				original_name: f.original_name,
				type: f.type,
				required: Boolean(f.required),
				unique: Boolean(f.unique),
				presentable: Boolean(f.presentable),
				hidden: Boolean(f.hidden),
				help_text: f.help_text?.trim() || undefined,
				collection_name: f.type === 'relation' ? f.collection_name?.trim() || undefined : undefined,
				cascade_delete: f.type === 'relation' ? Boolean(f.cascade_delete) : undefined,
				max_select:
					f.type === 'file' || f.type === 'relation' || f.type === 'select'
						? Number(f.max_select || 1)
						: undefined,
				values: f.type === 'select' && f.values?.length ? f.values : undefined,
				mime_types: f.type === 'file' && f.mime_types?.length ? f.mime_types : undefined,
				thumbs: f.type === 'file' && f.thumbs?.length ? f.thumbs : undefined,
				max_size:
					(f.type === 'file' || f.type === 'editor' || f.type === 'json') && f.max_size
						? Number(f.max_size)
						: undefined,
				protected: f.type === 'file' ? Boolean(f.protected) : undefined,
				min: f.min !== undefined ? Number(f.min) : undefined,
				max: f.max !== undefined ? Number(f.max) : undefined,
				pattern: f.type === 'text' && f.pattern?.trim() ? f.pattern.trim() : undefined,
				only_int: f.type === 'number' || f.type === 'float' ? Boolean(f.only_int) : undefined,
				only_domains:
					(f.type === 'email' || f.type === 'url') && f.only_domains?.length
						? f.only_domains
						: undefined,
				except_domains:
					(f.type === 'email' || f.type === 'url') && f.except_domains?.length
						? f.except_domains
						: undefined,
				on_create: f.type === 'autodate' ? f.on_create !== false : undefined,
				on_update: f.type === 'autodate' ? Boolean(f.on_update) : undefined,
				dimension: f.type === 'vector' ? Number(f.dimension || 768) : undefined
			}));

			const result = editing
				? await updateCollection(editing.name, cleanFields, apiRules, indexes)
				: await createCollection(name.trim(), cleanFields, indexes, apiRules);

			onSaved(result, Boolean(editing));
			onClose();
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to save collection';
			toast.error(errorMsg);
		} finally {
			saving = false;
		}
	}
</script>

<Drawer {open} {onClose} maxWidth="max-w-2xl">
	<div class="flex items-center justify-between px-6 pt-5 pb-3">
		<h2 class="text-base font-normal text-foreground">
			{editing ? 'Edit ' : 'New '}<strong class="font-semibold">{editing ? editing.name : 'collection'}</strong>{editing ? ' collection' : ''}
		</h2>
		<button
			type="button"
			onclick={onClose}
			class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
		>
			<X class="h-4 w-4" />
		</button>
	</div>

	<div class="px-6 pb-4">
		<div class="flex items-center justify-between rounded-md bg-muted/75 px-3.5 py-2.5">
			<div class="min-w-0 flex-1">
				<label for="col-name" class="block text-xs font-medium text-muted-foreground">
					Name <span class="text-destructive">*</span>
				</label>
				<input
					id="col-name"
					type="text"
					required
					disabled={Boolean(editing)}
					bind:value={name}
					placeholder="e.g. products"
					class="w-full bg-transparent pt-0.5 font-mono text-sm font-medium text-foreground outline-none disabled:opacity-85"
				/>
			</div>
			<span class="ml-3 shrink-0 text-xs font-medium text-muted-foreground">
				Type: <strong class="text-foreground">{isAuth ? 'Auth' : 'Base'}</strong>
			</span>
		</div>
	</div>

	<!-- PocketBase-style Tabs -->
	<div class="grid border-b border-border px-6 {isAuth ? 'grid-cols-3' : 'grid-cols-2'}">
		<button
			type="button"
			onclick={() => (activeTab = 'fields')}
			class="cursor-pointer rounded-t-md border border-b-0 py-2.5 text-xs font-medium transition {activeTab === 'fields'
				? 'border-border bg-card text-foreground'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
		>
			Fields
		</button>
		<button
			type="button"
			onclick={() => (activeTab = 'rules')}
			class="cursor-pointer rounded-t-md border border-b-0 py-2.5 text-xs font-medium transition {activeTab === 'rules'
				? 'border-border bg-card text-foreground'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
		>
			API rules
		</button>
		{#if isAuth}
			<button
				type="button"
				onclick={() => (activeTab = 'options')}
				class="cursor-pointer rounded-t-md border border-b-0 py-2.5 text-xs font-medium transition {activeTab === 'options'
					? 'border-border bg-card text-foreground'
					: 'border-transparent text-muted-foreground hover:text-foreground'}"
			>
				Options
			</button>
		{/if}
	</div>

	<form onsubmit={handleSubmit} class="flex min-h-0 flex-1 flex-col">
		<div class="flex-1 overflow-y-auto px-6 py-5">
			{#if activeTab === 'fields'}
				<!-- PocketBase Stacked Fields Table -->
				<div class="overflow-hidden rounded-md border border-border divide-y divide-border bg-muted/45">
					<!-- Built-in System Fields (top, muted, just like PocketBase) -->
					{#each systemFieldsList as sf (sf.name)}
						<div class="flex items-center justify-between px-3 py-2.5 text-muted-foreground">
							<div class="flex items-center gap-2.5">
								<div class="flex h-6 w-6 items-center justify-center text-muted-foreground/80">
									{#if sf.icon === 'lock'}
										<Lock class="h-4 w-4" />
									{:else if sf.icon === 'mail'}
										<Mail class="h-4 w-4" />
									{:else if sf.icon === 'bool'}
										<ToggleLeft class="h-4 w-4" />
									{:else}
										<Type class="h-4 w-4" />
									{/if}
								</div>
								<span class="font-mono text-xs">{sf.name}</span>
							</div>
							<div class="flex items-center gap-1.5">
								{#each sf.badges as badge (badge)}
									<span
										class="rounded px-2 py-0.5 text-xs font-medium {badge === 'Hidden'
											? 'bg-destructive/12 text-destructive'
											: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}"
									>
										{badge}
									</span>
								{/each}
								<span class="ml-1 inline-flex h-7 w-7 items-center justify-center rounded text-muted-foreground/50" title="System field">
									<Settings2 class="h-3.5 w-3.5" />
								</span>
							</div>
						</div>
					{/each}

					<!-- Editable Custom Fields (for BOTH Auth users & Base collections!) -->
					{#each fields as field, i (i)}
						<FieldRowInput
							bind:field={fields[i]}
							isExisting={Boolean(field.original_name)}
							{availableCollections}
							stacked={true}
							onRemove={() => removeField(i)}
						/>
					{/each}

					<!-- Built-in Timestamps at bottom (just like PocketBase created / updated) -->
					<div class="flex items-center justify-between px-3 py-2.5 text-muted-foreground">
						<div class="flex items-center gap-2.5">
							<div class="flex h-6 w-6 items-center justify-center text-muted-foreground/80">
								<CalendarCheck class="h-4 w-4" />
							</div>
							<span class="font-mono text-xs">created_at</span>
						</div>
						<span class="rounded bg-background/70 px-2.5 py-0.5 text-xs text-muted-foreground">Create</span>
					</div>
					<div class="flex items-center justify-between px-3 py-2.5 text-muted-foreground">
						<div class="flex items-center gap-2.5">
							<div class="flex h-6 w-6 items-center justify-center text-muted-foreground/80">
								<CalendarCheck class="h-4 w-4" />
							</div>
							<span class="font-mono text-xs">updated_at</span>
						</div>
						<span class="rounded bg-background/70 px-2.5 py-0.5 text-xs text-muted-foreground">Create/Update</span>
					</div>
				</div>

				<NewFieldPicker onPick={addField} />

				<!-- Unique constraints and indexes -->
				<div class="mt-6 border-t border-border pt-5">
					<div class="mb-2.5 text-xs font-semibold text-foreground">
						Unique constraints and indexes ({indexes.length})
					</div>
					<div class="flex flex-wrap items-center gap-2">
						{#each indexes as idx, pos (idx.name + pos)}
							<div
								class="inline-flex items-center gap-1 rounded-md border px-2.5 py-1 text-xs transition {idx.unique
									? 'border-emerald-500/30 bg-emerald-500/12 text-emerald-800 dark:text-emerald-300'
									: 'border-border bg-muted/50 text-foreground'}"
							>
								<button
									type="button"
									onclick={() => openEditIndex(pos)}
									class="cursor-pointer text-left hover:underline"
								>
									<span class="font-semibold">{idx.unique ? 'Unique:' : 'Index:'}</span>
									<span class="font-mono"> {idx.columns.join(', ')}</span>
								</button>
								<button
									type="button"
									onclick={() => removeIndex(pos)}
									title="Remove index"
									class="ml-0.5 cursor-pointer rounded p-0.5 opacity-60 transition hover:bg-black/10 hover:opacity-100"
								>
									<X class="h-3 w-3" />
								</button>
							</div>
						{/each}

						<button
							type="button"
							onclick={openNewIndex}
							class="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border bg-muted/60 px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-accent"
						>
							<Plus class="h-3 w-3" />
							New index
						</button>
					</div>
				</div>
			{:else if activeTab === 'rules'}
				<div class="space-y-3">
					<p class="text-xs text-muted-foreground">
						By default, locked rules allow <strong>Superusers only</strong>. Click <strong>Unlock</strong> to allow public access or write a rule like <code class="rounded bg-muted px-1">auth</code>, <code class="rounded bg-muted px-1">admin</code>, or <code class="rounded bg-muted px-1">@request.auth.id != ''</code>.
					</p>
					{#each ruleKeys as r (r.key)}
						{@const unlocked = Object.prototype.hasOwnProperty.call(apiRules, r.key)}
						<div class="rounded-md border border-border bg-muted/35 p-3">
							<div class="flex items-center justify-between gap-2">
								<div class="flex items-center gap-2">
									{#if unlocked}
										<LockOpen class="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
									{:else}
										<Lock class="h-4 w-4 text-muted-foreground" />
									{/if}
									<label for="rule-{r.key}" class="text-xs font-medium text-foreground">{r.label}</label>
								</div>

								{#if unlocked}
									<button
										type="button"
										onclick={() => {
											const next = { ...apiRules };
											delete next[r.key];
											apiRules = next;
										}}
										class="inline-flex cursor-pointer items-center gap-1 rounded border border-border bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground transition hover:bg-accent hover:text-foreground"
									>
										<Lock class="h-3 w-3" />
										Set Superusers only
									</button>
								{:else}
									<button
										type="button"
										onclick={() => {
											apiRules = { ...apiRules, [r.key]: '' };
										}}
										class="inline-flex cursor-pointer items-center gap-1 rounded border border-border bg-card px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-accent"
									>
										<LockOpen class="h-3 w-3" />
										Unlock
									</button>
								{/if}
							</div>

							{#if unlocked}
								<input
									id="rule-{r.key}"
									type="text"
									value={apiRules[r.key] ?? ''}
									oninput={(e) => {
										apiRules = { ...apiRules, [r.key]: e.currentTarget.value };
									}}
									placeholder="Leave empty for anyone, or e.g. @request.auth.id != ''"
									class="mt-2 w-full rounded-md border border-input bg-card px-3 py-2 font-mono text-xs outline-none focus:border-ring"
								/>
							{:else}
								<div class="mt-1.5 text-xs text-muted-foreground">
									Locked — only <code class="rounded bg-muted px-1">_superusers</code> can access this endpoint.
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{:else if activeTab === 'options' && isAuth}
				<AuthOptionsTab />
			{/if}

			{#if errorMsg}
				<p class="mt-4 rounded-md bg-destructive/15 px-3.5 py-2.5 text-xs text-destructive">
					{errorMsg}
				</p>
			{/if}
		</div>

		<div class="flex items-center justify-between border-t border-border bg-card px-6 py-3.5">
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
				{saving ? 'Saving...' : editing ? 'Save changes' : 'Create'}
			</button>
		</div>
	</form>
</Drawer>

<IndexModal
	open={showIndexModal}
	tableName={name}
	availableColumns={availableIndexCols}
	editing={editingIndexPos !== null ? indexes[editingIndexPos] : null}
	onClose={() => (showIndexModal = false)}
	onSave={handleSaveIndex}
	onRemove={editingIndexPos !== null ? () => removeIndex(editingIndexPos!) : undefined}
/>
