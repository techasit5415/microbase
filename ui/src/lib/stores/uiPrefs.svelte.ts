export type TableDensity = 'compact' | 'normal' | 'comfortable';
export type BorderRadiusPreset = 'sharp' | 'default' | 'rounded';

export interface UiPrefs {
	fontSize: number; // Base root UI size (14..20px, default 17)
	sidebarFontSize: number; // Collections & Settings sidebar items (12..19px, default 14)
	tableFontSize: number; // Records table cells & headers (12..20px, default 14)
	density: TableDensity;
	radius: BorderRadiusPreset;
}

const STORAGE_KEY = 'microbase_ui_prefs';

export const DEFAULT_UI_PREFS: UiPrefs = {
	fontSize: 17,
	sidebarFontSize: 14,
	tableFontSize: 14,
	density: 'normal',
	radius: 'default'
};

const RADIUS_VALUES: Record<BorderRadiusPreset, string> = {
	sharp: '2px',
	default: '5px',
	rounded: '10px'
};

function clamp(val: unknown, min: number, max: number, fallback: number): number {
	const n = Number(val);
	if (Number.isNaN(n)) return fallback;
	return Math.max(min, Math.min(max, n));
}

class UiPrefsStore {
	prefs = $state<UiPrefs>({ ...DEFAULT_UI_PREFS });

	apply(next: UiPrefs = this.prefs) {
		if (typeof document === 'undefined') return;
		const root = document.documentElement;
		root.style.fontSize = `${clamp(next.fontSize, 14, 20, DEFAULT_UI_PREFS.fontSize)}px`;
		root.style.setProperty('--ui-sidebar-size', `${clamp(next.sidebarFontSize, 12, 19, DEFAULT_UI_PREFS.sidebarFontSize)}px`);
		root.style.setProperty('--ui-table-size', `${clamp(next.tableFontSize, 12, 20, DEFAULT_UI_PREFS.tableFontSize)}px`);
		root.style.setProperty('--radius', RADIUS_VALUES[next.radius] ?? RADIUS_VALUES.default);
		root.dataset.density = next.density;
	}

	init() {
		if (typeof window === 'undefined') return;
		try {
			const raw = localStorage.getItem(STORAGE_KEY);
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
							: DEFAULT_UI_PREFS.radius
				};
			}
		} catch {
			this.prefs = { ...DEFAULT_UI_PREFS };
		}
		this.apply(this.prefs);
	}

	update(patch: Partial<UiPrefs>) {
		this.prefs = { ...this.prefs, ...patch };
		this.apply(this.prefs);
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(this.prefs));
		} catch {
			// Ignore storage errors
		}
	}

	reset() {
		this.prefs = { ...DEFAULT_UI_PREFS };
		this.apply(this.prefs);
		try {
			localStorage.removeItem(STORAGE_KEY);
		} catch {
			// Ignore
		}
	}
}

export const uiPrefs = new UiPrefsStore();
