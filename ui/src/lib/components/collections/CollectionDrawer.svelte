<script lang="ts">
	import { Lock, X } from '@lucide/svelte';
	import { createCollection, updateCollection } from '#lib/api/collections.ts';
	import type { Collection, FieldDef, FieldType } from '#lib/api/types.ts';
	import AuthOptionsTab from '#lib/components/oauth/AuthOptionsTab.svelte';
	import Drawer from '#lib/components/ui/Drawer.svelte';
	import FieldRowInput from './FieldRowInput.svelte';
	import NewFieldPicker from './NewFieldPicker.svelte';

	interface Props {
		open: boolean;
		editing?: Collection | null;
		onClose: () => void;
		onSaved: (col: Collection, isEdit: boolean) => void;
	}

	let { open, editing = null, onClose, onSaved }: Props = $props();

	let activeTab = $state<'fields' | 'rules' | 'options'>('fields');
	let name = $state('');
	let fields = $state<FieldDef[]>([]);
	let apiRules = $state<Record<string, string>>({});
	let errorMsg = $state('');
	let saving = $state(false);

	let isAuth = $derived(editing?.type === 'auth');

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
			activeTab = editing?.type === 'auth' ? 'options' : 'fields';
			if (editing) {
				name = editing.name;
				fields = editing.fields.map((f) => ({
					...f,
					original_name: f.name,
					required: Boolean(f.required),
					unique: Boolean(f.unique)
				}));
				apiRules = { ...(editing.api_rules ?? {}) };
			} else {
				name = '';
				fields = [{ name: 'title', type: 'text', required: true, unique: false }];
				apiRules = {};
			}
		}
	});

	function addField(type: FieldType) {
		fields = [
			...fields,
			{
				name: '',
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

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (isAuth) {
			onClose();
			return;
		}
		errorMsg = '';
		saving = true;
		try {
			const cleanFields = fields
				.filter((f) => f.name.trim() !== '')
				.map((f) => ({
					name: f.name.trim(),
					original_name: f.original_name,
					type: f.type,
					required: Boolean(f.required),
					unique: Boolean(f.unique),
					dimension: f.type === 'vector' ? Number(f.dimension || 768) : undefined
				}));

			const result = editing
				? await updateCollection(editing.name, cleanFields, apiRules)
				: await createCollection(name.trim(), cleanFields);

			onSaved(result, Boolean(editing));
			onClose();
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to save collection';
		} finally {
			saving = false;
		}
	}
</script>

<Drawer {open} {onClose} maxWidth="max-w-2xl">
	<div class="flex items-center justify-between px-6 pt-5 pb-3">
		<h2 class="text-base font-semibold">
			{editing ? `Edit ${editing.name} collection` : 'New collection'}
		</h2>
		<button
			type="button"
			onclick={onClose}
			class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition hover:bg-accent hover:text-foreground"
		>
			<X class="h-4 w-4" />
		</button>
	</div>

	<div class="px-6 pb-4">
		<div class="flex items-center justify-between rounded-md border border-border bg-muted/40 px-3.5 py-2">
			<div class="min-w-0 flex-1">
				<label for="col-name" class="block text-[11px] font-medium text-muted-foreground">
					Name <span class="text-destructive">*</span>
				</label>
				<input
					id="col-name"
					type="text"
					required
					disabled={Boolean(editing)}
					bind:value={name}
					placeholder="e.g. products"
					class="w-full bg-transparent pt-0.5 font-mono text-sm font-medium outline-none disabled:opacity-80"
				/>
			</div>
			<span class="ml-3 shrink-0 rounded border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground">
				Type: {isAuth ? 'Auth' : 'Base'}
			</span>
		</div>
	</div>

	<div class="grid border-b border-border px-6 {isAuth ? 'grid-cols-3' : 'grid-cols-2'}">
		<button
			type="button"
			onclick={() => (activeTab = 'fields')}
			class="cursor-pointer border-b-2 py-2.5 text-xs font-medium transition {activeTab === 'fields'
				? 'border-foreground text-foreground'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
		>
			Fields
		</button>
		<button
			type="button"
			onclick={() => (activeTab = 'rules')}
			class="cursor-pointer border-b-2 py-2.5 text-xs font-medium transition {activeTab === 'rules'
				? 'border-foreground text-foreground'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
		>
			API rules
		</button>
		{#if isAuth}
			<button
				type="button"
				onclick={() => (activeTab = 'options')}
				class="cursor-pointer border-b-2 py-2.5 text-xs font-medium transition {activeTab === 'options'
					? 'border-foreground text-foreground'
					: 'border-transparent text-muted-foreground hover:text-foreground'}"
			>
				Options
			</button>
		{/if}
	</div>

	<form onsubmit={handleSubmit} class="flex min-h-0 flex-1 flex-col">
		<div class="flex-1 overflow-y-auto px-6 py-5">
			{#if activeTab === 'fields'}
				{#if isAuth}
					<div class="space-y-2">
						<p class="mb-3 text-xs text-muted-foreground">
							System authentication fields managed by Microbase in <code class="rounded bg-muted px-1">_users</code>.
						</p>
						{#each ['id (bigserial)', 'email (text, unique)', 'role (text)', 'created_at (timestamptz)', 'updated_at (timestamptz)'] as sysField}
							<div class="flex items-center justify-between rounded-md border border-border bg-muted/25 px-3.5 py-2.5 text-xs">
								<span class="font-mono">{sysField}</span>
								<Lock class="h-3.5 w-3.5 text-muted-foreground" />
							</div>
						{/each}
					</div>
				{:else}
					<div class="flex flex-col gap-2">
						{#each fields as field, i (i)}
							<FieldRowInput
								bind:field={fields[i]}
								isExisting={Boolean(field.original_name)}
								onRemove={() => removeField(i)}
							/>
						{/each}
					</div>

					<NewFieldPicker onPick={addField} />
				{/if}
			{:else if activeTab === 'rules'}
				<div class="space-y-3">
					<p class="text-xs text-muted-foreground">
						Leave blank for public access, or set rules like <code class="rounded bg-muted px-1">auth</code> or <code class="rounded bg-muted px-1">admin</code>.
					</p>
					{#each ruleKeys as r (r.key)}
						<div>
							<label for="rule-{r.key}" class="mb-1 block text-xs font-medium">{r.label}</label>
							<input
								id="rule-{r.key}"
								type="text"
								disabled={isAuth}
								bind:value={apiRules[r.key]}
								placeholder="Public (leave empty) or e.g. @request.auth.id != ''"
								class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:border-ring disabled:opacity-60"
							/>
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

		<div class="flex items-center justify-between border-t border-border bg-muted/15 px-6 py-3.5">
			<button
				type="button"
				onclick={onClose}
				class="cursor-pointer rounded-md px-3.5 py-1.5 text-xs font-medium text-muted-foreground transition hover:bg-accent hover:text-foreground"
			>
				Close
			</button>
			{#if !isAuth}
				<button
					type="submit"
					disabled={saving}
					class="cursor-pointer rounded-md bg-foreground px-4 py-1.5 text-xs font-medium text-background transition hover:opacity-90 disabled:opacity-50"
				>
					{saving ? 'Saving...' : editing ? 'Save changes' : 'Create'}
				</button>
			{/if}
		</div>
	</form>
</Drawer>
