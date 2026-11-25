<script lang="ts">
	import {
		Braces,
		Calendar,
		CalendarCheck,
		FileImage,
		Hash,
		Link2,
		List,
		Mail,
		MapPin,
		PenLine,
		Settings2,
		Share2,
		Sparkles,
		ToggleLeft,
		Trash2,
		Type,
		X
	} from '@lucide/svelte';
	import type { FieldDef } from '#lib/api/types.ts';
	import Select from '#lib/components/ui/Select.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';

	interface Props {
		field: FieldDef;
		isExisting?: boolean;
		onRemove: () => void;
	}

	let { field = $bindable(), isExisting = false, onRemove }: Props = $props();
	let expanded = $state(false);
	let customMime = $state('');

	const mimePresets: Record<string, string[]> = {
		Images: ['image/jpeg', 'image/png', 'image/svg+xml', 'image/gif', 'image/webp'],
		Documents: ['application/pdf', 'application/msword', 'text/plain', 'text/csv'],
		Videos: ['video/mp4', 'video/webm', 'video/quicktime'],
		Audio: ['audio/mpeg', 'audio/wav', 'audio/ogg']
	};

	const selectModeOptions = [
		{ value: '1', label: 'Single' },
		{ value: '99', label: 'Multiple' }
	];

	function addMime(mime: string) {
		const clean = mime.trim().toLowerCase();
		if (!clean) return;
		const current = field.mime_types ?? [];
		if (!current.includes(clean)) {
			field.mime_types = [...current, clean];
		}
	}

	function removeMime(mime: string) {
		field.mime_types = (field.mime_types ?? []).filter((m) => m !== mime);
	}

	function applyPreset(key: string) {
		const current = new Set(field.mime_types ?? []);
		for (const m of mimePresets[key] ?? []) current.add(m);
		field.mime_types = Array.from(current);
	}

	function mimeShortExt(m: string): string {
		const map: Record<string, string> = {
			'image/jpeg': '.jpg',
			'image/png': '.png',
			'image/svg+xml': '.svg',
			'image/gif': '.gif',
			'image/webp': '.webp',
			'application/pdf': '.pdf',
			'video/mp4': '.mp4',
			'audio/mpeg': '.mp3'
		};
		return map[m] ?? '';
	}
</script>

<div class="rounded-md border border-border bg-background/80 transition hover:border-border/90">
	<div class="flex items-center gap-2 px-3 py-2">
		<div
			class="flex h-7 w-7 shrink-0 items-center justify-center rounded bg-muted text-muted-foreground"
			title={field.type}
		>
			{#if field.type === 'editor'}
				<PenLine class="h-3.5 w-3.5" />
			{:else if field.type === 'number' || field.type === 'integer' || field.type === 'float'}
				<Hash class="h-3.5 w-3.5" />
			{:else if field.type === 'boolean'}
				<ToggleLeft class="h-3.5 w-3.5" />
			{:else if field.type === 'email'}
				<Mail class="h-3.5 w-3.5" />
			{:else if field.type === 'url'}
				<Link2 class="h-3.5 w-3.5" />
			{:else if field.type === 'datetime' || field.type === 'timestamptz'}
				<Calendar class="h-3.5 w-3.5" />
			{:else if field.type === 'autodate'}
				<CalendarCheck class="h-3.5 w-3.5" />
			{:else if field.type === 'file'}
				<FileImage class="h-3.5 w-3.5" />
			{:else if field.type === 'relation'}
				<Share2 class="h-3.5 w-3.5" />
			{:else if field.type === 'select'}
				<List class="h-3.5 w-3.5" />
			{:else if field.type === 'json'}
				<Braces class="h-3.5 w-3.5" />
			{:else if field.type === 'geopoint'}
				<MapPin class="h-3.5 w-3.5" />
			{:else if field.type === 'vector'}
				<Sparkles class="h-3.5 w-3.5 text-pink-500" />
			{:else}
				<Type class="h-3.5 w-3.5" />
			{/if}
		</div>

		<input
			type="text"
			bind:value={field.name}
			placeholder="column_name"
			class="min-w-0 flex-1 bg-transparent px-1.5 py-1 font-mono text-xs outline-none focus:rounded focus:bg-muted/50"
		/>

		{#if field.type === 'file'}
			<div class="w-24 shrink-0">
				<Select
					bind:value={
						() => String((field.max_select ?? 1) > 1 ? 99 : 1),
						(v) => (field.max_select = Number(v))
					}
					options={selectModeOptions}
				/>
			</div>
		{:else}
			<span class="rounded bg-muted px-2 py-0.5 font-mono text-[11px] text-muted-foreground">
				{field.type}
			</span>
		{/if}

		{#if field.required}
			<span class="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
				Req
			</span>
		{/if}
		{#if field.hidden}
			<span class="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
				Hidden
			</span>
		{/if}

		<button
			type="button"
			onclick={() => (expanded = !expanded)}
			title="Field options"
			class="cursor-pointer rounded p-1.5 transition {expanded
				? 'bg-accent text-foreground'
				: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
		>
			<Settings2 class="h-3.5 w-3.5" />
		</button>

		<button
			type="button"
			onclick={onRemove}
			title="Remove field"
			class="cursor-pointer rounded p-1.5 text-muted-foreground transition hover:bg-destructive/15 hover:text-destructive"
		>
			<Trash2 class="h-3.5 w-3.5" />
		</button>
	</div>

	{#if expanded || field.type === 'vector'}
		<div class="space-y-3.5 border-t border-border/60 bg-muted/20 px-4 py-3.5">
			{#if field.type === 'file'}
				<!-- Allowed MIME types -->
				<div>
					<div class="mb-1.5 text-[11px] font-medium text-muted-foreground">Allowed mime types</div>
					<div class="flex flex-wrap items-center gap-1.5 rounded-md border border-input bg-background p-2">
						{#each field.mime_types ?? [] as mime (mime)}
							<span class="inline-flex items-center gap-1 rounded border border-border bg-muted/60 px-2 py-0.5 text-[11px]">
								{#if mimeShortExt(mime)}
									<strong class="font-semibold">{mimeShortExt(mime)}</strong>
								{/if}
								<span class="font-mono text-muted-foreground">{mime}</span>
								<button
									type="button"
									onclick={() => removeMime(mime)}
									class="ml-0.5 cursor-pointer rounded p-0.5 hover:bg-black/10"
								>
									<X class="h-2.5 w-2.5" />
								</button>
							</span>
						{/each}
						<input
							type="text"
							value={customMime}
							oninput={(e) => (customMime = e.currentTarget.value)}
							onkeydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									addMime(customMime);
									customMime = '';
								}
							}}
							placeholder="Add mime type + Enter..."
							class="min-w-[140px] flex-1 bg-transparent px-1 py-0.5 font-mono text-[11px] outline-none"
						/>
					</div>
					<div class="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
						<span>Presets:</span>
						{#each Object.keys(mimePresets) as preset (preset)}
							<button
								type="button"
								onclick={() => applyPreset(preset)}
								class="cursor-pointer rounded border border-border bg-background px-2 py-0.5 text-[10px] font-medium text-foreground transition hover:bg-accent"
							>
								+ {preset}
							</button>
						{/each}
						{#if (field.mime_types ?? []).length > 0}
							<button
								type="button"
								onclick={() => (field.mime_types = [])}
								class="cursor-pointer text-[10px] text-muted-foreground underline hover:text-foreground"
							>
								Clear
							</button>
						{/if}
					</div>
				</div>

				<!-- Thumb sizes & Max size -->
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
					<div class="sm:col-span-2">
						<div class="mb-1 text-[11px] font-medium text-muted-foreground">Thumb sizes</div>
						<input
							type="text"
							value={(field.thumbs ?? []).join(', ')}
							oninput={(e) => {
								field.thumbs = e.currentTarget.value
									.split(',')
									.map((t) => t.trim())
									.filter(Boolean);
							}}
							placeholder="e.g. 50x50, 480x720"
							class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
						/>
						<p class="mt-1 text-[10px] text-muted-foreground">Use comma as separator.</p>
					</div>
					<div>
						<div class="mb-1 text-[11px] font-medium text-muted-foreground">Max size</div>
						<input
							type="number"
							min="0"
							value={field.max_size ?? ''}
							oninput={(e) => {
								const n = parseInt(e.currentTarget.value, 10);
								field.max_size = Number.isNaN(n) ? undefined : n;
							}}
							placeholder="~5MB default"
							class="w-full rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus:border-ring"
						/>
						<p class="mt-1 text-[10px] text-muted-foreground">In bytes.</p>
					</div>
				</div>

				<!-- Protected switch -->
				<div class="flex items-center gap-2.5">
					<Switch checked={Boolean(field.protected)} onChange={(v) => (field.protected = v)} />
					<span class="text-xs text-foreground">
						<strong>Protected</strong>
						<span class="text-muted-foreground"> File download requests will need to satisfy the View API rule.</span>
					</span>
				</div>
			{/if}

			{#if field.type === 'vector'}
				<div class="flex items-center gap-2">
					<span class="text-xs font-medium text-muted-foreground">Vector Dimensions</span>
					<input
						type="number"
						min="1"
						max="4096"
						disabled={isExisting}
						bind:value={field.dimension}
						placeholder="768"
						class="w-24 rounded-md border border-input bg-background px-2.5 py-1 font-mono text-xs outline-none focus:border-ring disabled:opacity-50"
					/>
				</div>
			{/if}

			<!-- Help text -->
			<div>
				<div class="mb-1 text-[11px] font-medium text-muted-foreground">Help text</div>
				<input
					type="text"
					value={field.help_text ?? ''}
					oninput={(e) => (field.help_text = e.currentTarget.value)}
					placeholder="Optional helper text shown below the input..."
					class="w-full rounded-md border border-input bg-background px-3 py-1.5 text-xs outline-none focus:border-ring"
				/>
			</div>

			<!-- Bottom flags: Required / Presentable / Hidden / Unique -->
			<div class="flex flex-wrap items-center gap-5 pt-1">
				<Switch
					checked={Boolean(field.required)}
					onChange={(v) => (field.required = v)}
					label="Required"
				/>
				<Switch
					checked={Boolean(field.presentable)}
					onChange={(v) => (field.presentable = v)}
					label="Presentable"
				/>
				<Switch
					checked={Boolean(field.hidden)}
					onChange={(v) => (field.hidden = v)}
					label="Hidden"
				/>
				<Switch
					checked={Boolean(field.unique)}
					onChange={(v) => (field.unique = v)}
					label="Unique"
					disabled={isExisting}
				/>
			</div>
		</div>
	{/if}
</div>
