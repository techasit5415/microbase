export interface ToastItem {
	id: number;
	type: 'success' | 'error';
	message: string;
}

class ToastStore {
	items = $state<ToastItem[]>([]);
	private nextId = 1;
	private timers = new Map<number, ReturnType<typeof setTimeout>>();
	private remaining = new Map<number, number>();
	private startedAt = new Map<number, number>();

	show(type: 'success' | 'error', message: string, durationMs?: number) {
		const id = this.nextId++;
		const ms = durationMs ?? (type === 'error' ? 10000 : 6000);
		this.items = [...this.items, { id, type, message }];
		this.remaining.set(id, ms);
		this.startTimer(id, ms);
	}

	private startTimer(id: number, ms: number) {
		this.startedAt.set(id, Date.now());
		const t = setTimeout(() => this.dismiss(id), ms);
		this.timers.set(id, t);
	}

	pause(id: number) {
		const t = this.timers.get(id);
		if (!t) return;
		clearTimeout(t);
		this.timers.delete(id);
		const elapsed = Date.now() - (this.startedAt.get(id) ?? Date.now());
		const rem = Math.max(1500, (this.remaining.get(id) ?? 6000) - elapsed);
		this.remaining.set(id, rem);
	}

	resume(id: number) {
		if (this.timers.has(id)) return;
		const rem = this.remaining.get(id);
		if (rem !== undefined) {
			this.startTimer(id, rem);
		}
	}

	success(message: string, durationMs?: number) {
		this.show('success', message, durationMs);
	}

	error(err: unknown, fallback = 'Something went wrong', durationMs?: number) {
		const msg = err instanceof Error ? err.message : typeof err === 'string' ? err : fallback;
		this.show('error', msg, durationMs);
	}

	dismiss(id: number) {
		const t = this.timers.get(id);
		if (t) clearTimeout(t);
		this.timers.delete(id);
		this.remaining.delete(id);
		this.startedAt.delete(id);
		this.items = this.items.filter((item) => item.id !== id);
	}
}

export const toast = new ToastStore();
