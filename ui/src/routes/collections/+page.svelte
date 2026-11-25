<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page as pageState } from '$app/state';
	import { deleteCollection, listCollections } from '#lib/api/collections.ts';
	import { deleteRecord, listRecords } from '#lib/api/records.ts';
	import { listSystemTable } from '#lib/api/system.ts';
	import { deleteUser, listUsers } from '#lib/api/users.ts';
	import type { Collection, RecordRow } from '#lib/api/types.ts';
	import { toast } from '#lib/stores/toast.svelte.ts';
	import CollectionDrawer from '#lib/components/collections/CollectionDrawer.svelte';
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
		{ id: -2, name: '_oauth_accounts', type: 'system', fields: [], created_at: '', updated_at: '' },
		{ id: -3, name: '_oauth_providers', type: 'system', fields: [], created_at: '', updated_at: '' },
		{ id: -4, name: '_webhooks', type: 'system', fields: [], created_at: '', updated_at: '' },
		{ id: -5, name: '_collections', type: 'system', fields: [], created_at: '', updated_at: '' }
	];

	let userCollections = $state<Collection[]>([]);
	let mainCollections = $derived([usersCollection, ...userCollections]);
	let allLookup = $derived([...mainCollections, ...systemTables]);

	// Sync active collection directly from URL ?collection=...
	let activeName = $derived(pageState.url.searchParams.get('collection') || 'users');
	let activeCol = $derived(allLookup.find((c) => c.name === activeName) ?? usersCollection);

	let items = $state<RecordRow[]>([]);
	let currentPage = $state(1);
	let perPage = $state(20);
	let totalItems = $state(0);
	let search = $state('');
	let loading = $state(false);

	let showColDrawer = $state(false);
	let editingCol = $state<Collection | null>(null);
	let showCreateRec = $state(false);
	let editingRec = $state<RecordRow | null>(null);
	let showApiPreview = $state(false);

	async function refreshCollections() {
		try {
			userCollections = (await listCollections()).map((c) => ({ ...c, type: 'base' as const }));
		} catch (err) {
			userCollections = [];
			toast.error(err, 'Failed to load collections');
		}
	}

	async function refreshRecords(col: Collection = activeCol) {
		loading = true;
		try {
			const res =
				col.type === 'system'
					? await listSystemTable(col.name)
					: col.type === 'auth'
						? await listUsers({ page: currentPage, perPage, search })
						: await listRecords(col.name, { page: currentPage, perPage, search });
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
		currentPage = 1;
		search = '';
		refreshRecords(target);
	});

	function handleSelectCollection(name: string) {
		goto(`/_/collections?collection=${encodeURIComponent(name)}`);
	}

	function openCreateCollection() {
		editingCol = null;
		showColDrawer = true;
	}

	function openEditCollection() {
		if (activeCol.type === 'system') return;
		editingCol = activeCol;
		showColDrawer = true;
	}

	async function handleDeleteCollection(name: string) {
		if (!confirm(`Delete collection "${name}" and all its data?`)) return;
		try {
			await deleteCollection(name);
			toast.success(`Deleted collection "${name}"`);
			await refreshCollections();
			if (activeName === name) handleSelectCollection('users');
		} catch (err) {
			toast.error(err);
		}
	}

	async function handleDeleteRecord(id: number) {
		if (activeCol.type === 'system') return;
		try {
			if (activeCol.type === 'auth') await deleteUser(id);
			else await deleteRecord(activeCol.name, id);
			toast.success('Record deleted');
			await refreshRecords();
		} catch (err) {
			toast.error(err);
		}
	}

	onMount(() => {
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
/>

<main class="flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
	<RecordsToolbar
		collectionName={activeCol.name}
		{search}
		onSearchChange={(v) => {
			search = v;
			currentPage = 1;
			refreshRecords();
		}}
		onRefresh={() => refreshRecords()}
		onOpenEditCollection={openEditCollection}
		onOpenApiPreview={() => (showApiPreview = true)}
		onOpenCreateRecord={() => {
			if (activeCol.type !== 'system') {
				editingRec = null;
				showCreateRec = true;
			}
		}}
	/>

	<RecordsTable
		collection={activeCol}
		{items}
		{loading}
		readOnly={activeCol.type === 'system'}
		onSelect={(row) => {
			if (activeCol.type === 'base') {
				editingRec = row;
				showCreateRec = true;
			}
		}}
		onDelete={handleDeleteRecord}
	/>

	<PaginationBar
		page={currentPage}
		{perPage}
		{totalItems}
		onPageChange={(next) => {
			currentPage = next;
			refreshRecords();
		}}
	/>
</main>

<CollectionDrawer
	open={showColDrawer}
	editing={editingCol}
	onClose={() => (showColDrawer = false)}
	onSaved={(col, isEdit) => {
		const typed: Collection = { ...col, type: 'base' };
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

<RecordFormDialog
	open={showCreateRec}
	collection={activeCol}
	editingRecord={editingRec}
	onClose={() => (showCreateRec = false)}
	onSaved={() => refreshRecords()}
/>

<ApiPreviewDrawer
	open={showApiPreview}
	collection={activeCol}
	onClose={() => (showApiPreview = false)}
/>
