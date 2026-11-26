export type TableDensity = 'compact' | 'normal' | 'comfortable';
export type BorderRadiusPreset = 'sharp' | 'default' | 'rounded';

export interface UiPrefs {
	fontSize: number; // px, 14..20 (default 17)
	density: TableDensity;
	radius: BorderRadiusPreset;
}

const STORAGE_KEY = 'microbase_ui_prefs';

export const DEFAULT_UI_PREFS: UiPrefs = {
	fontSize: 17,
	density: 'normal',
	radius: 'default'
};

const RADIUS_VALUES: Record<BorderRadiusPreset, string> = {
	sharp: '2px',
	default: '5px',
	rounded: '10px'
};

class UiPrefsStore {
	prefs = $state<UiPrefs>({ ...DEFAULT_UI_PREFS });

	apply(next: UiPrefs = this.prefs) {
		if (typeof document === 'undefined') return;
		const root = document.documentElement;
		const clamped = Math.max(14, Math.min(20, Number(next.fontSize) || DEFAULT_UI_PREFS.fontSize));
		root.style.fontSize = `${clamped}px`;
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
					fontSize:
						typeof parsed.fontSize === 'number' && parsed.fontSize >= 14 && parsed.fontSize <= 20
							? parsed.fontSize
							: DEFAULT_UI_PREFS.fontSize,
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
