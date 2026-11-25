<script lang="ts">
	import {
		CalendarCheck,
		ChevronDown,
		Copy,
		Eraser,
		Folder,
		Lock,
		LockOpen,
		Mail,
		Plus,
		Settings2,
		Table2,
		ToggleLeft,
		Type,
		Users,
		X
	} from '@lucide/svelte';
	import { createCollection, duplicateCollection, truncateCollection, updateCollection } from '#lib/api/collections.ts';
	import type { Collection, FieldDef, FieldType, IndexDef } from '#lib/api/types.ts';
	import AuthOptionsTab from '#lib/components/oauth/AuthOptionsTab.svelte';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
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

	let colType = $state<'base' | 'view' | 'auth'>('base');
	let idType = $state<'text' | 'serial'>('text');
	let idAutogen = $state('[a-z0-9]{15}');
	let typeMenuOpen = $state(false);
	let activeTab = $state<'fields' | 'query' | 'rules' | 'options'>('fields');
	let name = $state('');
	let viewQuery = $state('');
	let fields = $state<FieldDef[]>([]);
	let indexes = $state<IndexDef[]>([]);
	let apiRules = $state<Record<string, string>>({});
	let showRulesInfo = $state(false);
	let expandedSysField = $state<string | null>(null);
	let errorMsg = $state('');
	let saving = $state(false);

	let showIndexModal = $state(false);
	let editingIndexPos = $state<number | null>(null);

	let isAuth = $derived(colType === 'auth');
	let isView = $derived(colType === 'view');

	const typeChoices: { value: 'base' | 'view' | 'auth'; label: string; short: string }[] = [
		{ value: 'base', label: 'Base collection', short: 'Base' },
		{ value: 'view', label: 'View collection', short: 'View' },
		{ value: 'auth', label: 'Auth collection', short: 'Auth' }
	];

	const authSystemFields = [
		{ name: 'id', icon: 'text', badges: ['Required'] },
		{ name: 'password', icon: 'lock', badges: ['Required', 'Hidden'] },
		{ name: 'tokenKey', icon: 'text', badges: ['Required', 'Hidden'] },
		{ name: 'email', icon: 'mail', badges: ['Required'] },
		{ name: 'emailVisibility', icon: 'bool', badges: [] },
		{ name: 'verified', icon: 'bool', badges: [] }
	];

	const baseSystemFields = [{ name: 'id', icon: 'text', badges: ['Required'] }];

	let systemFieldsList = $derived(isAuth ? authSystemFields : baseSystemFields);

	let availableIndexCols = $derived.by(() => {
		const userCols = fields.map((f) => f.name.trim()).filter((n) => n !== '');
		const sysCols = isAuth
			? ['id', 'email', 'token_key', 'email_visibility', 'verified', 'created_at', 'updated_at']
			: ['id', 'created_at', 'updated_at'];
		return Array.from(new Set([...sysCols, ...userCols]));
	});

	const allRuleKeys = [
		{ key: 'list', label: 'List/Search rule' },
		{ key: 'view', label: 'View rule' },
		{ key: 'create', label: 'Create rule' },
		{ key: 'update', label: 'Update rule' },
		{ key: 'delete', label: 'Delete rule' }
	];

	let visibleRuleKeys = $derived(isView ? allRuleKeys.slice(0, 2) : allRuleKeys);

	$effect(() => {
		if (open) {
			errorMsg = '';
			typeMenuOpen = false;
			showRulesInfo = false;
			if (editing) {
				name = editing.name;
				colType =
					editing.name === 'users' || editing.type === 'auth'
						? 'auth'
						: editing.type === 'view'
							? 'view'
							: 'base';
				activeTab = colType === 'view' ? 'query' : 'fields';
				viewQuery = editing.view_query ?? '';
				idType = editing.id_type === 'text' ? 'text' : 'serial';
				idAutogen = editing.id_autogenerate_pattern || '[a-z0-9]{15}';
				fields = (editing.fields ?? [])
					.filter((f) => !(editing.name === 'users' && ['email', 'password', 'tokenKey', 'token_key', 'emailVisibility', 'email_visibility', 'verified'].includes(f.name)))
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
				colType = 'base';
				activeTab = 'fields';
				viewQuery = '';
				idType = 'text';
				idAutogen = '[a-z0-9]{15}';
				fields = [{ name: 'title', type: 'text', required: true, unique: false }];
				indexes = [];
				apiRules = {};
			}
		}
	});

	function selectCollectionType(next: 'base' | 'view' | 'auth') {
		colType = next;
		typeMenuOpen = false;
		if (next === 'view') {
			activeTab = 'query';
		} else if (activeTab === 'query') {
			activeTab = 'fields';
		} else if (next !== 'auth' && activeTab === 'options') {
			activeTab = 'fields';
		}
	}

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

	async function handleTruncate() {
		if (!editing) return;
		const ok = await confirmAction({
			title: `Truncate "${editing.name}"?`,
			description: 'This will permanently delete ALL records inside this collection while keeping the schema intact.',
			confirmLabel: 'Truncate all rows',
			destructive: true
		});
		if (!ok) return;
		try {
			await truncateCollection(editing.name);
			toast.success(`Truncated "${editing.name}"`);
			onSaved(editing, true);
			onClose();
		} catch (err) {
			toast.error(err);
		}
	}

	async function handleDuplicate() {
		if (!editing) return;
		try {
			const cloned = await duplicateCollection(editing.name, `${editing.name}_copy`);
			onSaved(cloned, false);
			onClose();
		} catch (err) {
			toast.error(err);
		}
	}

	function removeIndex(pos: number) {
		indexes = indexes.filter((_, i) => i !== pos);
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';

		if (isView) {
			if (!editing && !/^[a-z][a-z0-9_]{0,62}$/.test(name.trim())) {
				errorMsg = 'View collection name must start with a lowercase letter (a-z, 0-9, _).';
				toast.error(errorMsg);
				return;
			}
			const q = viewQuery.trim();
			if (!q || (!q.toUpperCase().startsWith('SELECT ') && !q.toUpperCase().startsWith('WITH '))) {
				errorMsg = 'View collection requires a valid SQL SELECT query.';
				toast.error(errorMsg);
				return;
			}
		} else {
			const validationErr = validateCollectionForm(name, fields, indexes, Boolean(editing));
			if (validationErr) {
				errorMsg = validationErr;
				toast.error(validationErr);
				return;
			}
		}

		if (editing && !isView) {
			const oldNames = new Set((editing.fields ?? []).map((f) => f.name));
			const keptOrigins = new Set(fields.map((f) => f.original_name).filter(Boolean));
			const dropped = [...oldNames].filter((n) => !keptOrigins.has(n));
			const renamed = fields.filter((f) => f.original_name && f.original_name !== f.name.trim()).map((f) => `${f.original_name} → ${f.name.trim()}`);
			if (dropped.length > 0 || renamed.length > 0) {
				const parts: string[] = [];
				if (dropped.length) parts.push(`Drop columns (data will be lost): ${dropped.join(', ')}`);
				if (renamed.length) parts.push(`Rename columns: ${renamed.join(', ')}`);
				const ok = await confirmAction({
					title: `Confirm schema changes on "${editing.name}"?`,
					description: parts.join(' • '),
					confirmLabel: 'Apply changes',
					destructive: dropped.length > 0
				});
				if (!ok) return;
			}
		}

		saving = true;
		try {
			const cleanFields = isView
				? []
				: fields.map((f) => ({
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
						autogenerate_pattern: f.type === 'text' && f.autogenerate_pattern?.trim() ? f.autogenerate_pattern.trim() : undefined,
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
				? await updateCollection(editing.name, cleanFields, apiRules, indexes, viewQuery)
				: await createCollection(name.trim(), cleanFields, indexes, apiRules, colType, viewQuery, idType, idAutogen.trim() || '[a-z0-9]{15}');

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
			{editing ? 'Edit ' : 'Create '}<strong class="font-semibold">{editing ? editing.name : 'collection'}</strong>{editing ? ' collection' : ''}
		</h2>
		<div class="flex items-center gap-1">
			{#if editing && !isView}
				<button
					type="button"
					onclick={handleDuplicate}
					title="Duplicate collection schema"
					class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
				>
					<Copy class="h-3.5 w-3.5" />
					Duplicate
				</button>
				<button
					type="button"
					onclick={handleTruncate}
					title="Truncate all records"
					class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-destructive transition hover:bg-destructive/10"
				>
					<Eraser class="h-3.5 w-3.5" />
					Truncate
				</button>
			{/if}
			<button
				type="button"
				onclick={onClose}
				class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				<X class="h-4 w-4" />
			</button>
		</div>
	</div>

	<!-- Name + PocketBase Type Dropdown -->
	<div class="px-6 pb-4">
		<div class="flex items-center justify-between gap-3 rounded-md bg-muted/75 px-3.5 py-2.5">
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
					placeholder="e.g. posts"
					class="w-full bg-transparent pt-0.5 font-mono text-sm font-medium text-foreground outline-none disabled:opacity-85"
				/>
			</div>

			<!-- PocketBase Type: Base / View / Auth dropdown -->
			<div class="relative shrink-0">
				<button
					type="button"
					disabled={Boolean(editing)}
					onclick={() => (typeMenuOpen = !typeMenuOpen)}
					class="inline-flex items-center gap-1.5 rounded-md border border-foreground/70 bg-card px-3 py-1.5 text-xs font-semibold text-foreground transition hover:bg-muted disabled:cursor-default disabled:border-border disabled:bg-transparent disabled:text-muted-foreground"
				>
					<span>Type: {typeChoices.find((t) => t.value === colType)?.short}</span>
					{#if !editing}
						<ChevronDown class="h-3.5 w-3.5" />
					{/if}
				</button>

				{#if typeMenuOpen && !editing}
					<div class="animate-modal-in absolute right-0 z-30 mt-1.5 w-48 rounded-lg border border-border bg-card p-1.5 shadow-xl">
						{#each typeChoices as tc (tc.value)}
							<button
								type="button"
								onclick={() => selectCollectionType(tc.value)}
								class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-3 py-2 text-left text-xs font-medium transition {colType === tc.value
									? 'bg-muted text-foreground'
									: 'text-foreground/85 hover:bg-muted/60'}"
							>
								{#if tc.value === 'view'}
									<Table2 class="h-4 w-4 text-muted-foreground" />
								{:else if tc.value === 'auth'}
									<Users class="h-4 w-4 text-muted-foreground" />
								{:else}
									<Folder class="h-4 w-4 text-muted-foreground" />
								{/if}
								<span>{tc.label}</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</div>

	<!-- Dynamic PocketBase Tabs -->
	<div class="grid border-b border-border px-6 {isAuth ? 'grid-cols-3' : 'grid-cols-2'}">
		{#if isView}
			<button
				type="button"
				onclick={() => (activeTab = 'query')}
				class="cursor-pointer rounded-t-md border border-b-0 py-2.5 text-xs font-medium transition {activeTab === 'query'
					? 'border-border bg-card text-foreground'
					: 'border-transparent text-muted-foreground hover:text-foreground'}"
			>
				Query
			</button>
		{:else}
			<button
				type="button"
				onclick={() => (activeTab = 'fields')}
				class="cursor-pointer rounded-t-md border border-b-0 py-2.5 text-xs font-medium transition {activeTab === 'fields'
					? 'border-border bg-card text-foreground'
					: 'border-transparent text-muted-foreground hover:text-foreground'}"
			>
				Fields
			</button>
		{/if}

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
			{#if activeTab === 'query' && isView}
				<div class="space-y-3">
					<div class="flex items-center justify-between">
						<label for="view-sql" class="text-xs font-semibold text-foreground">
							Select query <span class="text-destructive">*</span>
						</label>
						<span class="text-xs text-muted-foreground">PostgreSQL VIEW (read-only)</span>
					</div>
					<textarea
						id="view-sql"
						rows="8"
						value={viewQuery}
						oninput={(e) => (viewQuery = e.currentTarget.value)}
						placeholder={'SELECT id, title, created_at FROM posts WHERE published = true'}
						class="w-full rounded-md border border-input bg-muted/50 p-3.5 font-mono text-xs leading-relaxed outline-none focus:border-ring focus:bg-card"
					></textarea>
					<div class="rounded-md border border-border bg-muted/35 p-3 text-xs text-muted-foreground">
						<div class="mb-1 font-semibold text-foreground">Query caveats:</div>
						<ul class="list-disc space-y-1 pl-4">
							<li>Must be a single <code class="rounded bg-muted px-1">SELECT</code> or <code class="rounded bg-muted px-1">WITH</code> statement without semicolons.</li>
							<li>Include a unique <code class="rounded bg-muted px-1">id</code> column (or use <code class="rounded bg-muted px-1">(ROW_NUMBER() OVER()) AS id</code>).</li>
						</ul>
					</div>
				</div>
			{:else if activeTab === 'fields' && !isView}
				<!-- PocketBase Stacked Fields Table -->
				<div class="divide-y divide-border overflow-hidden rounded-md border border-border bg-muted/45">
					{#each systemFieldsList as sf (sf.name)}
						<div>
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
									<button
										type="button"
										onclick={() => (expandedSysField = expandedSysField === sf.name ? null : sf.name)}
										title="System field info"
										class="ml-1 inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded transition {expandedSysField === sf.name
											? 'bg-accent text-foreground'
											: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
									>
										<Settings2 class="h-3.5 w-3.5" />
									</button>
								</div>
							</div>
							{#if expandedSysField === sf.name}
								{#if sf.name === 'id'}
									<div class="grid grid-cols-1 gap-3 border-t border-border/60 bg-card/85 px-4 py-3 sm:grid-cols-2">
										<div>
											<div class="mb-1 text-xs font-medium text-muted-foreground">Primary Key Type</div>
											<div class="flex gap-1.5">
												<button
													type="button"
													disabled={Boolean(editing)}
													onclick={() => (idType = 'text')}
													class="cursor-pointer rounded border px-2.5 py-1 text-xs font-medium transition disabled:opacity-60 {idType === 'text' ? 'border-foreground bg-foreground text-background' : 'border-border bg-background text-foreground'}"
												>
													Text (PocketBase)
												</button>
												<button
													type="button"
													disabled={Boolean(editing)}
													onclick={() => (idType = 'serial')}
													class="cursor-pointer rounded border px-2.5 py-1 text-xs font-medium transition disabled:opacity-60 {idType === 'serial' ? 'border-foreground bg-foreground text-background' : 'border-border bg-background text-foreground'}"
												>
													Auto-Increment
												</button>
											</div>
										</div>
										{#if idType === 'text'}
											<div>
												<div class="mb-1 text-xs font-medium text-muted-foreground">Autogenerate pattern</div>
												<input
													type="text"
													disabled={Boolean(editing)}
													bind:value={idAutogen}
													placeholder="[a-z0-9]{15} or uuid"
													class="w-full rounded-md border border-input bg-background px-3 py-1 font-mono text-xs outline-none focus:border-ring disabled:opacity-60"
												/>
											</div>
										{/if}
									</div>
								{:else}
									<div class="border-t border-border/60 bg-card/70 px-4 py-2.5 text-xs text-muted-foreground">
										Built-in system field <code class="rounded bg-muted px-1">{sf.name}</code> — managed automatically by Microbase.
									</div>
								{/if}
							{/if}
						</div>
					{/each}

					{#each fields as field, i (i)}
						<FieldRowInput
							bind:field={fields[i]}
							isExisting={Boolean(field.original_name)}
							{availableCollections}
							stacked={true}
							onRemove={() => removeField(i)}
						/>
					{/each}

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
						Unique constraints and indexes ({indexes.length + (isAuth ? 2 : 0)})
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
					<div class="flex items-center justify-between gap-2 text-xs text-muted-foreground">
						<span>All rules follow PocketBase filter syntax.</span>
						<button
							type="button"
							onclick={() => (showRulesInfo = !showRulesInfo)}
							class="cursor-pointer font-semibold text-foreground underline-offset-2 hover:underline"
						>
							{showRulesInfo ? 'Hide available fields' : 'Show available fields'}
						</button>
					</div>

					{#if showRulesInfo}
						<div class="animate-fade-in space-y-2 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs">
							<div class="font-medium text-foreground">Available fields:</div>
							<div class="flex flex-wrap gap-1.5">
								{#each availableIndexCols as col (col)}
									<code class="rounded bg-card px-1.5 py-0.5 font-mono text-xs">{col}</code>
								{/each}
							</div>
							<div class="pt-1 text-muted-foreground">
								Request variables: <code class="rounded bg-card px-1">@request.auth.id</code>, <code class="rounded bg-card px-1">@request.auth.role</code>, <code class="rounded bg-card px-1">auth</code>, <code class="rounded bg-card px-1">admin</code>
							</div>
						</div>
					{/if}

					{#each visibleRuleKeys as r (r.key)}
						{@const unlocked = Object.prototype.hasOwnProperty.call(apiRules, r.key)}
						<div class="rounded-md border border-border bg-card px-4 py-3 transition hover:border-ring/60">
							<div class="flex items-center justify-between gap-2">
								<label for="rule-{r.key}" class="text-xs font-medium text-muted-foreground">
									{r.label}
									{#if !unlocked}
										<span class="font-normal"> (Superusers only)</span>
									{/if}
								</label>

								{#if unlocked}
									<button
										type="button"
										title="Set Superusers only"
										onclick={() => {
											const next = { ...apiRules };
											delete next[r.key];
											apiRules = next;
										}}
										class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
									>
										<LockOpen class="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
									</button>
								{:else}
									<button
										type="button"
										title="Unlock rule"
										onclick={() => {
											apiRules = { ...apiRules, [r.key]: '' };
										}}
										class="cursor-pointer rounded-md p-1.5 text-emerald-600 transition hover:bg-emerald-500/10 dark:text-emerald-400"
									>
										<Lock class="h-4 w-4" />
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
									placeholder="Leave empty to grant everyone access, or e.g. @request.auth.id != ''"
									class="mt-2 w-full rounded-md bg-muted/65 px-3 py-2 font-mono text-xs outline-none focus:bg-accent/70"
								/>
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
				Close
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
