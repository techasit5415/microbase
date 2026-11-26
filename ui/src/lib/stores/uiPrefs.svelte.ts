export type TableDensity = 'compact' | 'normal' | 'comfortable';
export type BorderRadiusPreset = 'sharp' | 'default' | 'rounded';
export type FontFamilyPreset = 'ibm-plex-thai' | 'inter' | 'system';
export type DateFormatMode = 'local' | 'utc' | 'relative';
export type ThemeMode = 'light' | 'dark' | 'system';

export interface UiPrefs {
	fontSize: number; // Base root UI size (14..20px, default 17)
	sidebarFontSize: number; // Collections & Settings sidebar items (12..19px, default 14)
	tableFontSize: number; // Records table cells & headers (12..20px, default 14)
	density: TableDensity;
	radius: BorderRadiusPreset;
	fontFamily: FontFamilyPreset;
	dateFormat: DateFormatMode;
	defaultPerPage: number;
	theme: ThemeMode;
}

const STORAGE_KEY = 'microbase_ui_prefs';
const LEGACY_THEME_KEY = 'microbase-theme';

export const DEFAULT_UI_PREFS: UiPrefs = {
	fontSize: 17,
	sidebarFontSize: 14,
	tableFontSize: 14,
	density: 'normal',
	radius: 'default',
	fontFamily: 'ibm-plex-thai',
	dateFormat: 'local',
	defaultPerPage: 20,
	theme: 'light'
};

const RADIUS_VALUES: Record<BorderRadiusPreset, string> = {
	sharp: '2px',
	default: '5px',
	rounded: '10px'
};

const FONT_STACKS: Record<FontFamilyPreset, string> = {
	'ibm-plex-thai': "'IBM Plex Sans Thai', 'IBM Plex Sans Variable', 'IBM Plex Sans', system-ui, sans-serif",
	inter: "'Inter', 'IBM Plex Sans Thai', system-ui, sans-serif",
	system: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"
};

const VALID_PER_PAGE = [10, 20, 30, 50, 100];

function clamp(val: unknown, min: number, max: number, fallback: number): number {
	const n = Number(val);
	if (Number.isNaN(n)) return fallback;
	return Math.max(min, Math.min(max, n));
}

function pad2(n: number): string {
	return String(n).padStart(2, '0');
}

export function formatTimestamp(raw: string, mode: DateFormatMode = uiPrefs.prefs.dateFormat): string {
	const d = new Date(raw);
	if (Number.isNaN(d.getTime())) return raw;

	if (mode === 'relative') {
		const diffSec = Math.round((Date.now() - d.getTime()) / 1000);
		const abs = Math.abs(diffSec);
		const suffix = diffSec >= 0 ? 'ago' : 'from now';
		if (abs < 45) return diffSec >= 0 ? 'just now' : 'in a moment';
		if (abs < 3600) return `${Math.round(abs / 60)}m ${suffix}`;
		if (abs < 86400) return `${Math.round(abs / 3600)}h ${suffix}`;
		if (abs < 86400 * 30) return `${Math.round(abs / 86400)}d ${suffix}`;
		if (abs < 86400 * 365) return `${Math.round(abs / (86400 * 30))}mo ${suffix}`;
		return `${Math.round(abs / (86400 * 365))}y ${suffix}`;
	}

	if (mode === 'utc') {
		return `${d.getUTCFullYear()}-${pad2(d.getUTCMonth() + 1)}-${pad2(d.getUTCDate())} ${pad2(d.getUTCHours())}:${pad2(d.getUTCMinutes())}:${pad2(d.getUTCSeconds())} UTC`;
	}

	// local
	return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`;
}

class UiPrefsStore {
	prefs = $state<UiPrefs>({ ...DEFAULT_UI_PREFS });
	isDark = $state(false);
	private mediaQuery: MediaQueryList | null = null;
	private mediaListenerBound = false;

	private resolveDark(mode: ThemeMode): boolean {
		if (mode === 'dark') return true;
		if (mode === 'light') return false;
		if (typeof window !== 'undefined' && window.matchMedia) {
			return window.matchMedia('(prefers-color-scheme: dark)').matches;
		}
		return false;
	}

	apply(next: UiPrefs = this.prefs) {
		if (typeof document === 'undefined') return;
		const root = document.documentElement;
		root.style.fontSize = `${clamp(next.fontSize, 14, 20, DEFAULT_UI_PREFS.fontSize)}px`;
		root.style.setProperty('--ui-sidebar-size', `${clamp(next.sidebarFontSize, 12, 19, DEFAULT_UI_PREFS.sidebarFontSize)}px`);
		root.style.setProperty('--ui-table-size', `${clamp(next.tableFontSize, 12, 20, DEFAULT_UI_PREFS.tableFontSize)}px`);
		root.style.setProperty('--radius', RADIUS_VALUES[next.radius] ?? RADIUS_VALUES.default);
		root.style.setProperty('--ui-font-family', FONT_STACKS[next.fontFamily] ?? FONT_STACKS['ibm-plex-thai']);
		root.dataset.density = next.density;

		const dark = this.resolveDark(next.theme);
		this.isDark = dark;
		root.classList.toggle('dark', dark);
		root.style.colorScheme = dark ? 'dark' : 'light';
	}

	init() {
		if (typeof window === 'undefined') return;
		try {
			const raw = localStorage.getItem(STORAGE_KEY);
			const legacyTheme = localStorage.getItem(LEGACY_THEME_KEY);
			if (raw) {
				const parsed = JSON.parse(raw) as Partial<UiPrefs>;
				this.prefs = {
					fontSize: clamp(parsed.fontSize, 14, 20, DEFAULT_UI_PREFS.fontSize),
					sidebarFontSize: clamp(parsed.sidebarFontSize, 12, 19, DEFAULT_UI_PREFS.sidebarFontSize),
					tableFontSize: clamp(parsed.tableFontSize, 12, 20, DEFAULT_UI_PREFS.tableFontSize),
					density:
						parsed.density === 'compact' || parsed.density === 'normal' || parsed.density === 'comfortable'
							? parsed.density
							: DEFAULT_UI_PREFS.density,
					radius:
						parsed.radius === 'sharp' || parsed.radius === 'default' || parsed.radius === 'rounded'
							? parsed.radius
							: DEFAULT_UI_PREFS.radius,
					fontFamily:
						parsed.fontFamily === 'ibm-plex-thai' || parsed.fontFamily === 'inter' || parsed.fontFamily === 'system'
							? parsed.fontFamily
							: DEFAULT_UI_PREFS.fontFamily,
					dateFormat:
						parsed.dateFormat === 'local' || parsed.dateFormat === 'utc' || parsed.dateFormat === 'relative'
							? parsed.dateFormat
							: DEFAULT_UI_PREFS.dateFormat,
					defaultPerPage: VALID_PER_PAGE.includes(Number(parsed.defaultPerPage))
						? Number(parsed.defaultPerPage)
						: DEFAULT_UI_PREFS.defaultPerPage,
					theme:
						parsed.theme === 'light' || parsed.theme === 'dark' || parsed.theme === 'system'
							? parsed.theme
							: legacyTheme === 'dark'
								? 'dark'
								: DEFAULT_UI_PREFS.theme
				};
			} else if (legacyTheme === 'dark') {
				this.prefs = { ...DEFAULT_UI_PREFS, theme: 'dark' };
			}
		} catch {
			this.prefs = { ...DEFAULT_UI_PREFS };
		}

		if (!this.mediaListenerBound && window.matchMedia) {
			this.mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
			this.mediaQuery.addEventListener('change', () => {
				if (this.prefs.theme === 'system') this.apply(this.prefs);
			});
			this.mediaListenerBound = true;
		}
		this.apply(this.prefs);
	}

	update(patch: Partial<UiPrefs>) {
		this.prefs = { ...this.prefs, ...patch };
		this.apply(this.prefs);
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(this.prefs));
			if (patch.theme) {
				localStorage.setItem(LEGACY_THEME_KEY, this.isDark ? 'dark' : 'light');
			}
		} catch {
			// Ignore storage errors
		}
	}

	reset() {
		this.prefs = { ...DEFAULT_UI_PREFS };
		this.apply(this.prefs);
		try {
			localStorage.removeItem(STORAGE_KEY);
			localStorage.removeItem(LEGACY_THEME_KEY);
		} catch {
			// Ignore
		}
	}
}

export const uiPrefs = new UiPrefsStore();
