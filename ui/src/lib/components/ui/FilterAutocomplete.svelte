<script lang="ts">
	import type { Collection, FieldDef } from '#lib/api/types.ts';

	interface Suggestion {
		value: string;
		label: string;
		hint: string;
		kind: 'field' | 'relation' | 'operator' | 'macro';
	}

	interface Props {
		value: string;
		collection?: Collection | null;
		allCollections?: Collection[];
		includeMacros?: boolean;
		placeholder?: string;
		multiline?: boolean;
		id?: string;
		inputClass?: string;
		onInput?: (next: string) => void;
		onSubmit?: (val: string) => void;
	}

	let {
		value = $bindable(''),
		collection = null,
		allCollections = [],
		includeMacros = false,
		placeholder = '',
		multiline = false,
		id,
		inputClass = '',
		onInput,
		onSubmit
	}: Props = $props();

	let inputEl = $state<HTMLInputElement | HTMLTextAreaElement | null>(null);
	let backdropEl = $state<HTMLDivElement | null>(null);
	let focused = $state(false);
	let activeIndex = $state(0);
	let cursorPos = $state(0);
	let dismissed = $state(false);

	type HighlightKind = 'string' | 'macro' | 'operator' | 'number' | 'keyword' | 'punct' | 'plain';
	interface HighlightToken {
		text: string;
		kind: HighlightKind;
	}

	function tokenizeForHighlight(src: string): HighlightToken[] {
		if (!src) return [];
		const out: HighlightToken[] = [];
		const re =
			/("(?:[^"\\]|\\.)*"?|'(?:[^'\\]|\\.)*'?|@[a-zA-Z0-9_.]+|\?!=|\?!~|\?>=|\?<=|==|!=|>=|<=|!~|\?=|\?~|\?>|\?<|&&|\|\||[=<>~]|\b(?:true|false|null)\b|\b\d+(?:\.\d+)?\b|[()])/gi;
		let last = 0;
		let m: RegExpExecArray | null;
		while ((m = re.exec(src)) !== null) {
			if (m.index > last) {
				out.push({ text: src.slice(last, m.index), kind: 'plain' });
			}
			const tok = m[0];
			let kind: HighlightKind = 'plain';
			if (tok.startsWith('"') || tok.startsWith("'")) kind = 'string';
			else if (tok.startsWith('@')) kind = 'macro';
			else if (tok === '(' || tok === ')') kind = 'punct';
			else if (/^(?:true|false|null)$/i.test(tok)) kind = 'keyword';
			else if (/^\d/.test(tok)) kind = 'number';
			else kind = 'operator';
			out.push({ text: tok, kind });
			last = m.index + tok.length;
		}
		if (last < src.length) {
			out.push({ text: src.slice(last), kind: 'plain' });
		}
		return out;
	}

	let highlightedTokens = $derived(tokenizeForHighlight(value ?? ''));

	function syncScroll() {
		if (inputEl && backdropEl) {
			backdropEl.scrollLeft = inputEl.scrollLeft;
			backdropEl.scrollTop = inputEl.scrollTop;
		}
	}

	const operators: Suggestion[] = [
		{ value: '= ', label: '=', hint: 'equals', kind: 'operator' },
		{ value: '!= ', label: '!=', hint: 'not equals', kind: 'operator' },
		{ value: '~ ', label: '~', hint: 'contains / LIKE', kind: 'operator' },
		{ value: '!~ ', label: '!~', hint: 'does not contain', kind: 'operator' },
		{ value: '> ', label: '>', hint: 'greater than', kind: 'operator' },
		{ value: '>= ', label: '>=', hint: 'greater or equal', kind: 'operator' },
		{ value: '< ', label: '<', hint: 'less than', kind: 'operator' },
		{ value: '<= ', label: '<=', hint: 'less or equal', kind: 'operator' },
		{ value: '?= ', label: '?=', hint: 'any array/relation equals', kind: 'operator' },
		{ value: '&& ', label: '&&', hint: 'AND', kind: 'operator' },
		{ value: '|| ', label: '||', hint: 'OR', kind: 'operator' }
	];

	const macros: Suggestion[] = [
		{ value: '@request.auth.id', label: '@request.auth.id', hint: 'current user ID', kind: 'macro' },
		{ value: '@request.auth.email', label: '@request.auth.email', hint: 'current user email', kind: 'macro' },
		{ value: '@request.auth.role', label: '@request.auth.role', hint: 'current user role', kind: 'macro' },
		{ value: '@request.auth.collectionName', label: '@request.auth.collectionName', hint: 'auth collection name', kind: 'macro' },
		{ value: '@now', label: '@now', hint: 'current UTC timestamp', kind: 'macro' }
	];

	function fieldsForCollection(col: Collection | null | undefined): Array<{ name: string; type: string; target?: string }> {
		if (!col) return [];
		const list: Array<{ name: string; type: string; target?: string }> = [{ name: 'id', type: 'id' }];
		const seen = new Set<string>(['id']);
		if (col.type === 'auth' || col.name === 'users') {
			for (const sys of [
				{ name: 'email', type: 'email' },
				{ name: 'role', type: 'text' },
				{ name: 'verified', type: 'boolean' },
				{ name: 'email_visibility', type: 'boolean' }
			]) {
				if (!seen.has(sys.name)) {
					list.push(sys);
					seen.add(sys.name);
				}
			}
		}
		for (const f of (col.fields ?? []) as FieldDef[]) {
			if (!seen.has(f.name)) {
				list.push({ name: f.name, type: f.type, target: f.collection_name });
				seen.add(f.name);
			}
		}
		for (const ts of ['created_at', 'updated_at']) {
			if (!seen.has(ts)) {
				list.push({ name: ts, type: 'date' });
				seen.add(ts);
			}
		}
		return list;
	}

	let colMap = $derived.by(() => {
		const m: Record<string, Collection> = {};
		for (const c of allCollections) m[c.name] = c;
		if (collection) m[collection.name] = collection;
		return m;
	});

	let tokenContext = $derived.by(() => {
		const text = value ?? '';
		const pos = Math.min(cursorPos, text.length);
		const before = text.slice(0, pos);
		// Find the active token right before the cursor (letters, digits, _, ., @)
		const match = before.match(/([@a-zA-Z0-9_.]+)$/);
		const token = match ? match[1] : '';
		const start = match ? pos - token.length : pos;
		const beforeToken = before.slice(0, start).trimEnd();
		return { before, token, start, end: pos, beforeToken };
	});

	let suggestions = $derived.by<Suggestion[]>(() => {
		const { token, beforeToken, before } = tokenContext;

		// Inside an open quote -> don't interrupt typing string values
		const singleQuotes = (before.match(/'/g) ?? []).length;
		const doubleQuotes = (before.match(/"/g) ?? []).length;
		if (singleQuotes % 2 === 1 || doubleQuotes % 2 === 1) {
			return [];
		}

		// Case 1: Typing a macro (@...)
		if (token.startsWith('@')) {
			const pool = includeMacros ? macros : macros.filter((m) => m.value === '@now');
			const q = token.toLowerCase();
			return pool.filter((m) => m.value.toLowerCase().includes(q));
		}

		// Case 2: Dot-notation relation traversal (e.g. "email." or "email.na")
		if (token.includes('.')) {
			const parts = token.split('.');
			const prefixParts = parts.slice(0, -1);
			const partial = parts[parts.length - 1].toLowerCase();

			let curCol: Collection | null | undefined = collection;
			for (const seg of prefixParts) {
				if (!curCol) break;
				const f: FieldDef | undefined = (curCol.fields ?? []).find(
					(x: FieldDef) => x.name === seg && x.type === 'relation' && Boolean(x.collection_name)
				);
				curCol = f?.collection_name ? colMap[f.collection_name] : null;
			}
			if (!curCol) return [];
			const prefix = prefixParts.join('.') + '.';
			return fieldsForCollection(curCol)
				.filter((f) => f.name.toLowerCase().includes(partial))
				.map((f) => ({
					value: prefix + f.name,
					label: prefix + f.name,
					hint: f.target ? `relation → ${f.target}` : f.type,
					kind: f.target ? 'relation' : 'field'
				}));
		}

		// Case 3: Just finished typing a field/value followed by whitespace -> suggest operators
		if (token === '' && beforeToken !== '' && !/(?:&&|\|\||\(|\b(?:=|==|!=|~|!~|>|>=|<|<=|\?=|\?!=|\?~|\?!~))$/.test(beforeToken)) {
			// After a string/number literal, prefer logical combinators first
			if (/["'\d)]$/.test(beforeToken)) {
				return operators.filter((o) => o.label === '&&' || o.label === '||');
			}
			return operators;
		}

		// Case 4: Suggest current collection fields (+ macros when enabled)
		const q = token.toLowerCase();
		const fieldSuggestions: Suggestion[] = fieldsForCollection(collection)
			.filter((f) => f.name.toLowerCase().includes(q))
			.map((f) => ({
				value: f.target ? `${f.name}.` : f.name,
				label: f.name,
				hint: f.target ? `relation → ${f.target}` : f.type,
				kind: f.target ? 'relation' : 'field'
			}));

		const macroSuggestions = (includeMacros ? macros : macros.filter((m) => m.value === '@now')).filter(
			(m) => q === '' || m.value.toLowerCase().includes(q)
		);

		return [...fieldSuggestions, ...macroSuggestions].slice(0, 12);
	});

	let showMenu = $derived(focused && !dismissed && suggestions.length > 0);

	$effect(() => {
		void suggestions.length;
		if (activeIndex >= suggestions.length) activeIndex = 0;
	});

	function updateCursor() {
		if (inputEl) {
			cursorPos = inputEl.selectionStart ?? value.length;
		}
	}

	function handleInput(e: Event) {
		const el = e.currentTarget as HTMLInputElement | HTMLTextAreaElement;
		value = el.value;
		cursorPos = el.selectionStart ?? el.value.length;
		dismissed = false;
		activeIndex = 0;
		onInput?.(value);
	}

	function applySuggestion(s: Suggestion) {
		const { start, end } = tokenContext;
		const next = value.slice(0, start) + s.value + value.slice(end);
		const nextCursor = start + s.value.length;
		value = next;
		cursorPos = nextCursor;
		dismissed = false;
		activeIndex = 0;
		onInput?.(next);
		queueMicrotask(() => {
			if (inputEl) {
				inputEl.focus();
				inputEl.setSelectionRange(nextCursor, nextCursor);
			}
		});
	}

	function handleKeydown(e: KeyboardEvent) {
		if (showMenu) {
			if (e.key === 'ArrowDown') {
				e.preventDefault();
				activeIndex = (activeIndex + 1) % suggestions.length;
				return;
			}
			if (e.key === 'ArrowUp') {
				e.preventDefault();
				activeIndex = (activeIndex - 1 + suggestions.length) % suggestions.length;
				return;
			}
			if (e.key === 'Tab' || (e.key === 'Enter' && !multiline && !e.shiftKey && suggestions[activeIndex])) {
				// For single-line Search bar: Tab always inserts suggestion; Enter submits if user already typed an operator+value
				if (e.key === 'Tab' || tokenContext.token !== '' || suggestions[activeIndex]?.kind === 'operator') {
					e.preventDefault();
					applySuggestion(suggestions[activeIndex]);
					return;
				}
			}
			if (e.key === 'Escape') {
				e.preventDefault();
				dismissed = true;
				return;
			}
		}
		if (e.key === 'Enter' && !multiline) {
			dismissed = true;
			onSubmit?.(value);
		}
	}
</script>

<div class="relative w-full">
	<!-- Syntax-highlighted overlay layer (sits above input with pointer-events-none so input bg never washes out colors) -->
	{#if value}
		<div
			bind:this={backdropEl}
			aria-hidden="true"
			class="pointer-events-none absolute inset-0 z-2 overflow-hidden border-transparent! bg-transparent! leading-relaxed {multiline
				? 'break-words whitespace-pre-wrap'
				: 'whitespace-pre'} {inputClass}"
		>
			{#each highlightedTokens as tok, i (i)}
				{#if tok.kind === 'string'}
					<span class="text-emerald-600 dark:text-emerald-400">{tok.text}</span>
				{:else if tok.kind === 'operator'}
					<span class="font-semibold text-amber-600 dark:text-amber-400">{tok.text}</span>
				{:else if tok.kind === 'macro'}
					<span class="text-purple-600 dark:text-purple-400">{tok.text}</span>
				{:else if tok.kind === 'number' || tok.kind === 'keyword'}
					<span class="text-sky-600 dark:text-sky-400">{tok.text}</span>
				{:else if tok.kind === 'punct'}
					<span class="text-muted-foreground">{tok.text}</span>
				{:else}
					<span class="text-foreground">{tok.text}</span>
				{/if}
			{/each}
			{#if multiline && value.endsWith('\n')}<br />{/if}
		</div>
	{/if}

	{#if multiline}
		<textarea
			bind:this={inputEl}
			{id}
			rows={Math.max(2, Math.min(8, (value ?? '').split('\n').length))}
			{value}
			{placeholder}
			spellcheck="false"
			oninput={(e) => {
				handleInput(e);
				syncScroll();
			}}
			onscroll={syncScroll}
			onkeydown={handleKeydown}
			onclick={updateCursor}
			onkeyup={updateCursor}
			onfocus={() => {
				focused = true;
				dismissed = false;
				updateCursor();
			}}
			onblur={() => {
				setTimeout(() => (focused = false), 150);
			}}
			class="relative z-1 block resize-none leading-relaxed {value ? 'text-transparent! caret-foreground selection:bg-primary/25' : ''} {inputClass}"
		></textarea>
	{:else}
		<input
			bind:this={inputEl}
			{id}
			type="text"
			{value}
			{placeholder}
			spellcheck="false"
			oninput={(e) => {
				handleInput(e);
				syncScroll();
			}}
			onscroll={syncScroll}
			onkeydown={handleKeydown}
			onclick={updateCursor}
			onkeyup={updateCursor}
			onfocus={() => {
				focused = true;
				dismissed = false;
				updateCursor();
			}}
			onblur={() => {
				setTimeout(() => (focused = false), 150);
			}}
			class="relative z-1 block leading-relaxed {value ? 'text-transparent! caret-foreground selection:bg-primary/25' : ''} {inputClass}"
		/>
	{/if}

	{#if showMenu}
		<div
			class="absolute top-full right-0 left-0 z-50 mt-1 max-h-56 overflow-y-auto rounded-lg border border-border bg-card p-1 shadow-lg"
			role="listbox"
		>
			{#each suggestions as s, idx (s.label + s.kind)}
				<button
					type="button"
					role="option"
					aria-selected={idx === activeIndex}
					onmousedown={(e) => {
						e.preventDefault();
						applySuggestion(s);
					}}
					class="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-left text-xs transition {idx ===
					activeIndex
						? 'bg-accent text-foreground'
						: 'text-foreground hover:bg-muted/70'}"
				>
					<span class="truncate font-mono font-medium">{s.label}</span>
					<span
						class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium {s.kind === 'relation'
							? 'bg-sky-500/15 text-sky-700 dark:text-sky-300'
							: s.kind === 'macro'
								? 'bg-amber-500/15 text-amber-700 dark:text-amber-300'
								: s.kind === 'operator'
									? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
									: 'bg-muted text-muted-foreground'}"
					>
						{s.hint}
					</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
