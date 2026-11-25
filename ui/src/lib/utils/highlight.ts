export type HighlightLang = 'js' | 'ts' | 'curl' | 'go' | 'json';

function escapeHtml(s: string): string {
	return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

const JS_TS_KEYWORDS = new Set([
	'const', 'let', 'var', 'await', 'async', 'function', 'return', 'import', 'export',
	'from', 'new', 'if', 'else', 'try', 'catch', 'true', 'false', 'null', 'undefined',
	'interface', 'type', 'extends', 'as', 'readonly'
]);

const JS_TS_TYPES = new Set([
	'string', 'number', 'boolean', 'unknown', 'any', 'void', 'never',
	'Promise', 'Record', 'Array', 'EventSource', 'URLSearchParams', 'Headers', 'Response', 'JSON'
]);

const GO_KEYWORDS = new Set([
	'package', 'import', 'func', 'var', 'const', 'type', 'struct', 'interface',
	'return', 'if', 'else', 'for', 'range', 'defer', 'go', 'nil', 'true', 'false', 'make', 'map'
]);

const GO_TYPES = new Set([
	'string', 'int', 'int64', 'float64', 'bool', 'byte', 'any', 'error', 'http', 'bytes', 'strings', 'io', 'fmt'
]);

export function highlightCode(code: string, lang: HighlightLang): string {
	if (lang === 'json') {
		return highlightJSON(code);
	}
	if (lang === 'curl') {
		return highlightCurl(code);
	}
	return highlightCLike(code, lang === 'go' ? GO_KEYWORDS : JS_TS_KEYWORDS, lang === 'go' ? GO_TYPES : JS_TS_TYPES);
}

function highlightJSON(code: string): string {
	const escaped = escapeHtml(code);
	return escaped.replace(
		/("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?)/g,
		(match, str, colon, kw, num) => {
			if (str) {
				if (colon) {
					return `<span class="text-sky-800 dark:text-sky-300">${str}</span>${colon}`;
				}
				return `<span class="text-emerald-700 dark:text-emerald-400">${str}</span>`;
			}
			if (kw) {
				return `<span class="font-medium text-purple-700 dark:text-purple-400">${kw}</span>`;
			}
			if (num) {
				return `<span class="text-amber-700 dark:text-amber-400">${num}</span>`;
			}
			return match;
		}
	);
}

function highlightCurl(code: string): string {
	const escaped = escapeHtml(code);
	return escaped.replace(
		/(#[^\n]*)|('(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*")|(\s-[a-zA-Z]+)|\b(curl)\b/g,
		(match, comment, str, flag, cmd) => {
			if (comment) return `<span class="italic text-slate-400 dark:text-slate-500">${comment}</span>`;
			if (str) return `<span class="text-emerald-700 dark:text-emerald-400">${str}</span>`;
			if (flag) return `<span class="font-medium text-purple-700 dark:text-purple-400">${flag}</span>`;
			if (cmd) return `<span class="font-semibold text-blue-700 dark:text-blue-400">${cmd}</span>`;
			return match;
		}
	);
}

function highlightCLike(code: string, keywords: Set<string>, types: Set<string>): string {
	const escaped = escapeHtml(code);
	return escaped.replace(
		/(\/\/[^\n]*)|('(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*"|`(?:\\.|[^`\\])*`)|\b([A-Za-z_][A-Za-z0-9_]*)\b(\s*\()?|(\b\d+(?:\.\d+)?\b)/g,
		(match, comment, str, ident, paren, num) => {
			if (comment) return `<span class="italic text-slate-400 dark:text-slate-500">${comment}</span>`;
			if (str) return `<span class="text-emerald-700 dark:text-emerald-400">${str}</span>`;
			if (num) return `<span class="text-amber-700 dark:text-amber-400">${num}</span>`;
			if (ident) {
				if (keywords.has(ident)) {
					return `<span class="font-medium text-purple-700 dark:text-purple-400">${ident}</span>${paren ?? ''}`;
				}
				if (types.has(ident) || (/^[A-Z][A-Za-z0-9_]*$/.test(ident) && !paren)) {
					return `<span class="text-sky-700 dark:text-sky-400">${ident}</span>${paren ?? ''}`;
				}
				if (paren) {
					return `<span class="text-blue-700 dark:text-blue-400">${ident}</span>${paren}`;
				}
			}
			return match;
		}
	);
}
