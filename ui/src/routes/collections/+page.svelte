<script lang="ts">
	import { fade } from 'svelte/transition';
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page as pageState } from '$app/state';
	import { deleteCollection, listCollections } from '#lib/api/collections.ts';
	import { deleteRecord, listRecords } from '#lib/api/records.ts';
	import { deleteSystemRecord, listSystemTable } from '#lib/api/system.ts';
	import { deleteSuperuser, deleteUser, listUsers } from '#lib/api/users.ts';
	import type { Collection, RecordRow } from '#lib/api/types.ts';
	import { confirmAction } from '#lib/stores/confirm.svelte.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import { uiPrefs } from '#lib/stores/uiPrefs.svelte.ts';
	import CollectionDrawer from '#lib/components/collections/CollectionDrawer.svelte';
	import CollectionsOverviewModal from '#lib/components/collections/CollectionsOverviewModal.svelte';
	import CollectionSidebar from '#lib/components/collections/CollectionSidebar.svelte';
	import PaginationBar from '#lib/components/records/PaginationBar.svelte';
	import ApiPreviewDrawer from '#lib/components/records/ApiPreviewDrawer.svelte';
	import RecordFormDialog from '#lib/components/records/RecordFormDialog.svelte';
	import RecordsTable from '#lib/components/records/RecordsTable.svelte';
	import RecordsToolbar from '#lib/components/records/RecordsToolbar.svelte';

	const usersCollection: Collection = {
		id: 0,
		name: 'users',
		type: 'auth',
		fields: [
			{ name: 'email', type: 'email', required: true, unique: true },
			{ name: 'role', type: 'text' }
		],
		created_at: '',
		updated_at: ''
	};

	const systemTables: Collection[] = [
		{ id: -1, name: '_superusers', type: 'system', fields: [], created_at: '', updated_at: '' },
		{
			id: -2,
			name: '_authOrigins',
			type: 'system',
			fields: [
				{ name: 'collection_ref', type: 'text', required: true },
				{ name: 'record_ref', type: 'text', required: true },
				{ name: 'fingerprint', type: 'text', required: true },
				{ name: 'ip', type: 'text' }
			],
			created_at: '',
			updated_at: ''
		},
		{
			id: -3,
			name: '_oauth_accounts',
			type: 'system',
			fields: [
				{ name: 'user_id', type: 'text', required: true },
				{ name: 'provider', type: 'text', required: true },
				{ name: 'provider_user_id', type: 'text', required: true }
			],
			created_at: '',
			updated_at: ''
		},
		{ id: -4, name: '_oauth_providers', type: 'system', fields: [], created_at: '', updated_at: '' },
		{ id: -5, name: '_webhooks', type: 'system', fields: [], created_at: '', updated_at: '' }
	];

	let userCollections = $state<Collection[]>([]);
	let mainCollections = $derived.by(() => {
		const dbUsers = userCollections.find((c) => c.name === 'users');
		const authCol: Collection = dbUsers
			? { ...dbUsers, type: 'auth' }
			: usersCollection;
		const others = userCollections
			.filter((c) => c.name !== 'users')
			.map((c) => ({ ...c, type: (c.type || 'base') as 'base' | 'view' | 'auth' }));
		return [authCol, ...others];
	});
	let allLookup = $derived([...mainCollections, ...systemTables]);

	// Sync active collection directly from URL ?collection=...
	let activeName = $derived(pageState.url.searchParams.get('collection') || 'users');
	let activeCol = $derived(allLookup.find((c) => c.name === activeName) ?? usersCollection);

	let items = $state<RecordRow[]>([]);
	let currentPage = $state(1);
	let perPage = $state(uiPrefs.prefs.defaultPerPage);
	let totalItems = $state(0);
	let search = $state('');
	let sort = $state('');
	let loading = $state(false);

	let showColDrawer = $state(false);
	let editingCol = $state<Collection | null>(null);
	let editingColTab = $state<'fields' | 'query' | 'rules' | 'options' | undefined>(undefined);
	let showOverview = $state(false);
	let showCreateRec = $state(false);
	let editingRec = $state<RecordRow | null>(null);
	let showApiPreview = $state(false);

	async function refreshCollections() {
		try {
			userCollections = (await listCollections()).map((c) => ({
				...c,
				type: c.name === 'users' ? 'auth' : (c.type || 'base')
			}));
		} catch (err) {
			userCollections = [];
			toast.error(err, 'Failed to load collections');
		}
	}

	function isFilterExpression(raw: string): boolean {
		const q = raw.trim();
		if (q === '') return false;
		return /(?:==|!=|>=|<=|!~|\?=|\?!=|\?~|\?!~|\?>|\?<|&&|\|\||(?<![<>!=?~])=(?!=)|(?<![!?])~|(?<![<>?])[><](?!=))/.test(q);
	}

	async function refreshRecords(col: Collection = activeCol) {
		loading = true;
		try {
			const expand = (col.fields ?? [])
				.filter((f) => f.type === 'relation' && f.collection_name)
				.map((f) => f.name)
				.join(',');
			const trimmed = search.trim();
			const useFilter = isFilterExpression(trimmed);
			const res =
				col.type === 'system'
					? await listSystemTable(col.name)
					: await listRecords(col.name, {
							page: currentPage,
							perPage,
							search: useFilter ? undefined : trimmed || undefined,
							filter: useFilter ? trimmed : undefined,
							sort,
							expand
						});
			items = res.items ?? [];
			totalItems = res.total_items ?? items.length;
		} catch (err) {
			items = [];
			totalItems = 0;
			toast.error(err, 'Failed to load records');
		} finally {
			loading = false;
		}
	}

	// Automatically reload records whenever URL ?collection=... changes (including Back/Forward!)
	$effect(() => {
		const target = activeCol;
		void target.name;
		untrack(() => {
			currentPage = 1;
			search = '';
			sort = '';
			refreshRecords(target);
		});
	});

	function handleSelectCollection(name: string) {
		goto(`/_/collections?collection=${encodeURIComponent(name)}`);
	}

	function openCreateCollection() {
		editingCol = null;
		editingColTab = undefined;
		showColDrawer = true;
	}

	function openEditCollection(col: Collection = activeCol, tab?: 'fields' | 'rules') {
		editingCol = col;
		editingColTab = tab;
		showColDrawer = true;
	}

	async function handleDeleteCollection(name: string) {
		const ok = await confirmAction({
			title: `Delete "${name}" collection?`,
			description: 'This will permanently drop the table and all of its records. This action cannot be undone.',
			confirmLabel: 'Delete collection',
			destructive: true
		});
		if (!ok) return;
		try {
			await deleteCollection(name);
			toast.success(`Deleted collection "${name}"`);
			await refreshCollections();
			if (activeName === name) handleSelectCollection('users');
		} catch (err) {
			toast.error(err);
		}
	}

	async function handleDeleteRecord(id: number | string) {
		const deletableSystem = activeCol.name === '_superusers' || activeCol.name === '_authOrigins' || activeCol.name === '_oauth_accounts';
		if (activeCol.type === 'system' && !deletableSystem) return;
		const ok = await confirmAction({
			title: `Delete record #${id}?`,
			description: `Are you sure you want to permanently delete this record from "${activeCol.name}"?`,
			confirmLabel: 'Delete',
			destructive: true
		});
		if (!ok) return;
		try {
			if (activeCol.name === '_superusers') await deleteSuperuser(Number(id));
			else if (activeCol.name === '_authOrigins' || activeCol.name === '_oauth_accounts') await deleteSystemRecord(activeCol.name, id);
			else if (activeCol.type === 'auth') await deleteUser(id);
			else await deleteRecord(activeCol.name, id);
			toast.success('Record deleted');
			await refreshRecords();
		} catch (err) {
			toast.error(err);
		}
	}

	async function handleBulkDelete(ids: (number | string)[]) {
		if (ids.length === 0) return;
		const ok = await confirmAction({
			title: `Delete ${ids.length} selected records?`,
			description: `Permanently delete ${ids.length} records from "${activeCol.name}"? This cannot be undone.`,
			confirmLabel: `Delete ${ids.length} records`,
			destructive: true
		});
		if (!ok) return;
		try {
			for (const id of ids) {
				if (activeCol.name === '_superusers') await deleteSuperuser(Number(id));
				else if (activeCol.name === '_authOrigins' || activeCol.name === '_oauth_accounts') await deleteSystemRecord(activeCol.name, id);
				else if (activeCol.type === 'auth') await deleteUser(id);
				else await deleteRecord(activeCol.name, id);
			}
			toast.success(`Deleted ${ids.length} records`);
			await refreshRecords();
		} catch (err) {
			toast.error(err);
		}
	}

	onMount(() => {
		perPage = uiPrefs.prefs.defaultPerPage;
		refreshCollections();
	});
</script>

<CollectionSidebar
	collections={mainCollections}
	{systemTables}
	{activeName}
	onSelect={handleSelectCollection}
	onDelete={handleDeleteCollection}
	onOpenCreate={openCreateCollection}
	onOpenOverview={() => (showOverview = true)}
/>

<main class="flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
		{#key activeName}
		<div in:fade={{ duration: 150 }} class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
	<RecordsToolbar
		collectionName={activeCol.name}
		collection={activeCol}
		allCollections={mainCollections}
		{search}
		onSearchChange={(v) => {
			search = v;
			currentPage = 1;
			if (!isFilterExpression(v)) {
				refreshRecords();
			}
		}}
		onSearchSubmit={(v) => {
			search = v;
			currentPage = 1;
			refreshRecords();
		}}
		onRefresh={() => refreshRecords()}
		onOpenEditCollection={() => openEditCollection(activeCol)}
		onOpenApiPreview={() => (showApiPreview = true)}
		onOpenCreateRecord={() => {
			if ((activeCol.type !== 'system' && activeCol.type !== 'view') || activeCol.name === '_superusers' || activeCol.name === '_authOrigins' || activeCol.name === '_oauth_accounts') {
				editingRec = null;
				showCreateRec = true;
			}
		}}
	/>

	<RecordsTable
		collection={activeCol}
		allCollections={mainCollections}
		{items}
		{loading}
		readOnly={(activeCol.type === 'system' && activeCol.name !== '_superusers' && activeCol.name !== '_authOrigins' && activeCol.name !== '_oauth_accounts') || activeCol.type === 'view'}
		{sort}
		onSortChange={(next) => {
			sort = next;
			currentPage = 1;
			refreshRecords();
		}}
		onSelect={(row) => {
			if (
				activeCol.type === 'base' ||
				activeCol.type === 'auth' ||
				activeCol.name === '_superusers' ||
				activeCol.name === '_authOrigins' ||
				activeCol.name === '_oauth_accounts'
			) {
				editingRec = row;
				showCreateRec = true;
			}
		}}
		onDelete={handleDeleteRecord}
		onBulkDelete={handleBulkDelete}
	/>

	<PaginationBar
		page={currentPage}
		{perPage}
		{totalItems}
		onPageChange={(next) => {
			currentPage = next;
			refreshRecords();
		}}
		onPerPageChange={(nextPerPage) => {
			perPage = nextPerPage;
			currentPage = 1;
			refreshRecords();
		}}
	/>
		</div>
		{/key}
</main>

<CollectionDrawer
	open={showColDrawer}
	editing={editingCol}
	initialTab={editingColTab}
	availableCollections={mainCollections.map((c) => c.name)}
	allCollections={mainCollections}
	onClose={() => (showColDrawer = false)}
	onSaved={(col, isEdit) => {
		const typed: Collection = { ...col, type: col.name === 'users' ? 'auth' : (col.type || 'base') };
		if (isEdit) {
			userCollections = userCollections.map((c) => (c.name === typed.name ? typed : c));
			toast.success(`Updated collection "${typed.name}"`);
			refreshRecords(typed);
		} else {
			userCollections = [...userCollections, typed];
			toast.success(`Created collection "${typed.name}"`);
			handleSelectCollection(typed.name);
		}
	}}
/>

<CollectionsOverviewModal
	open={showOverview}
	collections={mainCollections}
	{systemTables}
	onClose={() => (showOverview = false)}
	onEditCollection={(col, tab) => openEditCollection(col, tab)}
/>

<RecordFormDialog
	open={showCreateRec}
	collection={activeCol}
	editingRecord={editingRec}
	onClose={() => (showCreateRec = false)}
	onSaved={() => refreshRecords()}
	onDelete={async (id) => {
		await handleDeleteRecord(id);
		showCreateRec = false;
	}}
/>

<ApiPreviewDrawer
	open={showApiPreview}
	collection={activeCol}
	onClose={() => (showApiPreview = false)}
/>
