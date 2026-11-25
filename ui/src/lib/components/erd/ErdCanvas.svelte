<script lang="ts">
	import { Database, Maximize2, RotateCcw, ZoomIn, ZoomOut } from '@lucide/svelte';
	import type { Collection } from '#lib/api/types.ts';

	interface Props {
		collections: Collection[];
	}

	let { collections }: Props = $props();

	const CARD_W = 228;
	const HEADER_H = 30;
	const ROW_H = 22;
	const FOOTER_H = 22;
	const GAP_X = 72;
	const GAP_Y = 36;

	interface NodePos {
		x: number;
		y: number;
	}

	interface Edge {
		id: string;
		fromCol: string;
		fromField: string;
		fromRowIndex: number;
		toCol: string;
		multi: boolean;
	}

	let positions = $state<Record<string, NodePos>>({});
	let zoom = $state(0.85);
	let pan = $state({ x: 32, y: 28 });
	let selectedCol = $state<string | null>(null);
	let hoveredCol = $state<string | null>(null);
	let hoveredEdgeId = $state<string | null>(null);

	let containerEl = $state<HTMLDivElement | null>(null);
	let draggingCol = $state<string | null>(null);
	let dragOffset = { x: 0, y: 0 };
	let panning = $state(false);
	let panStart = { x: 0, y: 0, panX: 0, panY: 0 };

	function getRowsForCollection(col: Collection) {
		const rows: Array<{
			name: string;
			type: string;
			pk?: boolean;
			fkTarget?: string;
			fkMulti?: boolean;
			required?: boolean;
			system?: boolean;
		}> = [
			{
				name: 'id',
				type: col.id_type === 'text' ? 'PK text' : 'PK bigserial',
				pk: true
			}
		];
		if (col.type === 'auth' || col.name === 'users') {
			rows.push({ name: 'email', type: 'text', system: true });
			rows.push({ name: 'role', type: 'text', system: true });
		}
		for (const f of col.fields ?? []) {
			rows.push({
				name: f.name,
				type: f.type,
				required: f.required,
				fkTarget: f.type === 'relation' && f.collection_name ? f.collection_name : undefined,
				fkMulti: f.type === 'relation' && (f.max_select ?? 0) > 1
			});
		}
		return rows;
	}

	function cardHeight(col: Collection): number {
		const rowCount = getRowsForCollection(col).length;
		return HEADER_H + rowCount * ROW_H + FOOTER_H;
	}

	let colMap = $derived.by(() => {
		const m: Record<string, Collection> = {};
		for (const c of collections) m[c.name] = c;
		return m;
	});

	let edges = $derived.by<Edge[]>(() => {
		const list: Edge[] = [];
		for (const c of collections) {
			const rows = getRowsForCollection(c);
			rows.forEach((r, idx) => {
				if (r.fkTarget && colMap[r.fkTarget]) {
					list.push({
						id: `${c.name}.${r.name}->${r.fkTarget}`,
						fromCol: c.name,
						fromField: r.name,
						fromRowIndex: idx,
						toCol: r.fkTarget,
						multi: Boolean(r.fkMulti)
					});
				}
			});
		}
		return list;
	});

	function computeAutoLayout(cols: Collection[]): Record<string, NodePos> {
		if (cols.length === 0) return {};
		// Sort most-referenced tables first so hub tables sit near top-left.
		const refCount: Record<string, number> = {};
		for (const c of cols) {
			for (const f of c.fields ?? []) {
				if (f.type === 'relation' && f.collection_name) {
					refCount[f.collection_name] = (refCount[f.collection_name] ?? 0) + 1;
				}
			}
		}
		const sorted = [...cols].sort((a, b) => {
			const diff = (refCount[b.name] ?? 0) - (refCount[a.name] ?? 0);
			return diff !== 0 ? diff : a.name.localeCompare(b.name);
		});

		const numCols = Math.max(3, Math.min(6, Math.ceil(Math.sqrt(sorted.length * 1.25))));
		const colHeights = new Array<number>(numCols).fill(0);
		const pos: Record<string, NodePos> = {};

		for (const c of sorted) {
			// Place in shortest column (masonry layout) to avoid giant empty gaps.
			let bestCol = 0;
			for (let i = 1; i < numCols; i++) {
				if (colHeights[i] < colHeights[bestCol]) bestCol = i;
			}
			pos[c.name] = {
				x: bestCol * (CARD_W + GAP_X),
				y: colHeights[bestCol]
			};
			colHeights[bestCol] += cardHeight(c) + GAP_Y;
		}
		return pos;
	}

	$effect(() => {
		if (collections.length > 0) {
			const missing = collections.some((c) => !positions[c.name]);
			if (missing) {
				positions = computeAutoLayout(collections);
			}
		}
	});

	function resetLayout() {
		positions = computeAutoLayout(collections);
		fitToView();
	}

	function fitToView() {
		if (!containerEl || collections.length === 0) return;
		let minX = Infinity,
			minY = Infinity,
			maxX = -Infinity,
			maxY = -Infinity;
		for (const c of collections) {
			const p = positions[c.name];
			if (!p) continue;
			minX = Math.min(minX, p.x);
			minY = Math.min(minY, p.y);
			maxX = Math.max(maxX, p.x + CARD_W);
			maxY = Math.max(maxY, p.y + cardHeight(c));
		}
		if (!isFinite(minX)) return;
		const rect = containerEl.getBoundingClientRect();
		const contentW = maxX - minX + 80;
		const contentH = maxY - minY + 80;
		const nextZoom = Math.max(0.3, Math.min(1.0, Math.min(rect.width / contentW, rect.height / contentH)));
		zoom = Number(nextZoom.toFixed(2));
		pan = {
			x: Math.round((rect.width - (maxX - minX) * zoom) / 2 - minX * zoom),
			y: Math.round(Math.max(24, (rect.height - (maxY - minY) * zoom) / 2 - minY * zoom))
		};
	}

	function startCardDrag(e: PointerEvent, name: string) {
		e.stopPropagation();
		const pos = positions[name];
		if (!pos) return;
		draggingCol = name;
		selectedCol = name;
		dragOffset = {
			x: e.clientX / zoom - pos.x,
			y: e.clientY / zoom - pos.y
		};
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}

	function onCardMove(e: PointerEvent) {
		if (!draggingCol) return;
		positions = {
			...positions,
			[draggingCol]: {
				x: Math.round(e.clientX / zoom - dragOffset.x),
				y: Math.round(e.clientY / zoom - dragOffset.y)
			}
		};
	}

	function endCardDrag() {
		draggingCol = null;
	}

	function startPan(e: PointerEvent) {
		if (draggingCol) return;
		selectedCol = null;
		panning = true;
		panStart = { x: e.clientX, y: e.clientY, panX: pan.x, panY: pan.y };
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}

	function onPanMove(e: PointerEvent) {
		if (!panning) return;
		pan = {
			x: panStart.panX + (e.clientX - panStart.x),
			y: panStart.panY + (e.clientY - panStart.y)
		};
	}

	function endPan() {
		panning = false;
	}

	function onWheel(e: WheelEvent) {
		e.preventDefault();
		const factor = e.deltaY < 0 ? 1.08 : 0.92;
		const nextZoom = Math.max(0.25, Math.min(1.6, Number((zoom * factor).toFixed(3))));
		if (!containerEl) {
			zoom = nextZoom;
			return;
		}
		const rect = containerEl.getBoundingClientRect();
		const mx = e.clientX - rect.left;
		const my = e.clientY - rect.top;
		const ratio = nextZoom / zoom;
		pan = {
			x: mx - (mx - pan.x) * ratio,
			y: my - (my - pan.y) * ratio
		};
		zoom = nextZoom;
	}

	function edgePath(edge: Edge): { d: string; x1: number; y1: number; x2: number; y2: number; mx: number; my: number } | null {
		const fromPos = positions[edge.fromCol];
		const toPos = positions[edge.toCol];
		if (!fromPos || !toPos) return null;

		const y1 = fromPos.y + HEADER_H + edge.fromRowIndex * ROW_H + ROW_H / 2;
		const y2 = toPos.y + HEADER_H + ROW_H / 2; // target "id" row

		if (edge.fromCol === edge.toCol) {
			// Self-relation loop on right side
			const x = fromPos.x + CARD_W;
			const loop = 42;
			const d = `M ${x} ${y1} C ${x + loop} ${y1}, ${x + loop} ${y2}, ${x} ${y2}`;
			return { d, x1: x, y1, x2: x, y2, mx: x + loop * 0.75, my: (y1 + y2) / 2 };
		}

		const fromCenterX = fromPos.x + CARD_W / 2;
		const toCenterX = toPos.x + CARD_W / 2;
		const goRight = toCenterX >= fromCenterX;

		const x1 = goRight ? fromPos.x + CARD_W : fromPos.x;
		const x2 = goRight ? toPos.x : toPos.x + CARD_W;
		const dx = Math.max(44, Math.min(160, Math.abs(x2 - x1) * 0.45));
		const c1x = goRight ? x1 + dx : x1 - dx;
		const c2x = goRight ? x2 - dx : x2 + dx;

		const d = `M ${x1} ${y1} C ${c1x} ${y1}, ${c2x} ${y2}, ${x2} ${y2}`;
		return { d, x1, y1, x2, y2, mx: (x1 + x2) / 2, my: (y1 + y2) / 2 };
	}

	let focusCol = $derived(hoveredCol ?? selectedCol);

	function isEdgeActive(edge: Edge): boolean {
		if (hoveredEdgeId) return edge.id === hoveredEdgeId;
		if (!focusCol) return false;
		return edge.fromCol === focusCol || edge.toCol === focusCol;
	}

	let connectedToFocus = $derived.by(() => {
		const set = new Set<string>();
		if (!focusCol) return set;
		set.add(focusCol);
		for (const e of edges) {
			if (e.fromCol === focusCol) set.add(e.toCol);
			if (e.toCol === focusCol) set.add(e.fromCol);
		}
		return set;
	});

	let hoveredEdge = $derived(edges.find((e) => e.id === hoveredEdgeId) ?? null);
</script>

<div class="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-muted/25">
	<!-- Compact floating toolbar -->
	<div class="pointer-events-none absolute top-3 right-3 left-3 z-20 flex flex-wrap items-center justify-between gap-2">
		<div class="pointer-events-auto flex items-center gap-2 rounded-md border border-border bg-card/95 px-2.5 py-1 text-[11px] text-muted-foreground shadow-xs backdrop-blur-xs">
			<span><strong class="text-foreground">{collections.length}</strong> tables</span>
			<span>·</span>
			<span><strong class="text-foreground">{edges.length}</strong> relations</span>
			{#if hoveredEdge}
				<span>·</span>
				<span class="font-mono font-medium text-emerald-600 dark:text-emerald-400">
					{hoveredEdge.fromCol}.{hoveredEdge.fromField} &rarr; {hoveredEdge.toCol}.id{hoveredEdge.multi ? ' []' : ''}
				</span>
			{:else if focusCol}
				<span>·</span>
				<span class="font-mono text-foreground">{focusCol}</span>
			{:else}
				<span class="hidden sm:inline">· Hover/click table or line to highlight · Drag cards to move</span>
			{/if}
		</div>

		<div class="pointer-events-auto flex items-center gap-1 rounded-md border border-border bg-card/95 p-1 shadow-xs backdrop-blur-xs">
			<button
				type="button"
				title="Zoom out"
				onclick={() => (zoom = Math.max(0.25, Number((zoom - 0.1).toFixed(2))))}
				class="cursor-pointer rounded p-1 text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				<ZoomOut class="h-3.5 w-3.5" />
			</button>
			<span class="min-w-10 text-center font-mono text-[11px] text-muted-foreground">{Math.round(zoom * 100)}%</span>
			<button
				type="button"
				title="Zoom in"
				onclick={() => (zoom = Math.min(1.6, Number((zoom + 0.1).toFixed(2))))}
				class="cursor-pointer rounded p-1 text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				<ZoomIn class="h-3.5 w-3.5" />
			</button>
			<div class="mx-0.5 h-4 w-px bg-border"></div>
			<button
				type="button"
				title="Fit to screen"
				onclick={fitToView}
				class="inline-flex cursor-pointer items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				<Maximize2 class="h-3 w-3" /> Fit
			</button>
			<button
				type="button"
				title="Reset layout"
				onclick={resetLayout}
				class="inline-flex cursor-pointer items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
			>
				<RotateCcw class="h-3 w-3" /> Reset
			</button>
		</div>
	</div>

	<!-- Interactive Pan/Zoom Canvas -->
	<div
		bind:this={containerEl}
		role="application"
		aria-label="Entity Relationship Diagram"
		onpointerdown={startPan}
		onpointermove={onPanMove}
		onpointerup={endPan}
		onwheel={onWheel}
		class="relative min-h-0 flex-1 overflow-hidden select-none {panning ? 'cursor-grabbing' : 'cursor-grab'}"
		style="background-image: radial-gradient(circle, color-mix(in oklab, var(--border) 75%, transparent) 1px, transparent 1px); background-size: 20px 20px;"
	>
		<div
			class="absolute top-0 left-0 origin-top-left"
			style="transform: translate({pan.x}px, {pan.y}px) scale({zoom});"
		>
			<!-- SVG Relation Connectors Layer -->
			<svg class="pointer-events-none overflow-visible" width="1" height="1" style="position: absolute; top: 0; left: 0; z-index: 1;">
				<defs>
					<marker id="erd-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
						<path d="M 0 1.5 L 9 5 L 0 8.5 z" fill="currentColor" />
					</marker>
				</defs>
				{#each edges as edge (edge.id)}
					{@const geom = edgePath(edge)}
					{#if geom}
						{@const active = isEdgeActive(edge)}
						{@const dimmed = (focusCol !== null || hoveredEdgeId !== null) && !active}
						<g
							class={active
								? 'text-emerald-600 dark:text-emerald-400'
								: 'text-muted-foreground/55'}
							style="opacity: {dimmed ? 0.12 : active ? 1 : 0.72}; transition: opacity 120ms;"
						>
							<!-- Wider invisible hit path for easy hover -->
							<path
								d={geom.d}
								fill="none"
								stroke="transparent"
								stroke-width="12"
								class="pointer-events-auto cursor-pointer"
								role="presentation"
								onpointerenter={() => (hoveredEdgeId = edge.id)}
								onpointerleave={() => (hoveredEdgeId = null)}
							/>
							<path
								d={geom.d}
								fill="none"
								stroke="currentColor"
								stroke-width={active ? 2.2 : 1.3}
								stroke-dasharray={edge.multi ? '5 3' : undefined}
								marker-end="url(#erd-arrow)"
							/>
							<circle cx={geom.x1} cy={geom.y1} r={active ? 3.5 : 2.5} fill="currentColor" />
						</g>
					{/if}
				{/each}
			</svg>

			<!-- Compact Table Cards Layer -->
			{#each collections as col (col.name)}
				{@const pos = positions[col.name]}
				{#if pos}
					{@const rows = getRowsForCollection(col)}
					{@const isDimmed = focusCol !== null && !connectedToFocus.has(col.name)}
					{@const isSelected = selectedCol === col.name || hoveredCol === col.name}
					<div
						role="button"
						tabindex="0"
						onpointerdown={(e) => startCardDrag(e, col.name)}
						onpointermove={onCardMove}
						onpointerup={endCardDrag}
						onpointerenter={() => (hoveredCol = col.name)}
						onpointerleave={() => (hoveredCol = null)}
						style="left: {pos.x}px; top: {pos.y}px; width: {CARD_W}px; opacity: {isDimmed ? 0.35 : 1}; z-index: {isSelected ? 10 : 2};"
						class="absolute overflow-hidden rounded-md border bg-card shadow-xs transition-opacity {isSelected
							? 'border-emerald-500 ring-1 ring-emerald-500/40'
							: 'border-border'}"
					>
						<div
							style="height: {HEADER_H}px;"
							class="flex cursor-move items-center justify-between gap-1.5 border-b border-border bg-muted/65 px-2.5"
						>
							<div class="flex min-w-0 items-center gap-1.5">
								<Database class="h-3 w-3 shrink-0 text-muted-foreground" />
								<span class="truncate font-mono text-[11px] font-semibold text-foreground">{col.name}</span>
							</div>
							<span class="shrink-0 rounded border border-border bg-card px-1 py-0.5 text-[9px] font-medium uppercase text-muted-foreground">
								{col.type || 'base'}
							</span>
						</div>

						<div class="divide-y divide-border/60 font-mono text-[10px]">
							{#each rows as r (r.name)}
								{@const rowEdgeId = r.fkTarget ? `${col.name}.${r.name}->${r.fkTarget}` : null}
								<div
									style="height: {ROW_H}px;"
									role="presentation"
									onpointerenter={() => {
										if (rowEdgeId) hoveredEdgeId = rowEdgeId;
									}}
									onpointerleave={() => {
										if (rowEdgeId && hoveredEdgeId === rowEdgeId) hoveredEdgeId = null;
									}}
									class="flex items-center justify-between gap-1.5 px-2.5 {rowEdgeId && hoveredEdgeId === rowEdgeId
										? 'bg-emerald-500/15'
										: ''}"
								>
									<span class="truncate {r.pk ? 'font-semibold text-foreground' : r.system ? 'text-muted-foreground' : 'text-foreground/90'}">
										{r.name}{#if r.required}<span class="text-destructive">*</span>{/if}
									</span>
									{#if r.pk}
										<span class="shrink-0 rounded bg-amber-500/15 px-1 text-[9px] text-amber-700 dark:text-amber-400">
											{r.type}
										</span>
									{:else if r.fkTarget}
										<span class="shrink-0 rounded bg-emerald-500/15 px-1 text-[9px] text-emerald-700 dark:text-emerald-400">
											&rarr; {r.fkTarget}{r.fkMulti ? '[]' : ''}
										</span>
									{:else}
										<span class="shrink-0 text-[9px] text-muted-foreground">{r.type}</span>
									{/if}
								</div>
							{/each}
							<div
								style="height: {FOOTER_H}px;"
								class="flex items-center justify-between px-2.5 text-[9px] text-muted-foreground/80"
							>
								<span>created_at / updated_at</span>
								<span>timestamptz</span>
							</div>
						</div>
					</div>
				{/if}
			{/each}
		</div>
	</div>
</div>
