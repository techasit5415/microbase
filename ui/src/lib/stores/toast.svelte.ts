export interface ToastItem {
	id: number;
	type: 'success' | 'error';
	message: string;
}

class ToastStore {
	items = $state<ToastItem[]>([]);
	private nextId = 1;

	show(type: 'success' | 'error', message: string) {
		const id = this.nextId++;
		this.items = [...this.items, { id, type, message }];
		setTimeout(() => this.dismiss(id), 4000);
	}

	success(message: string) {
		this.show('success', message);
	}

	error(err: unknown, fallback = 'Something went wrong') {
		const msg = err instanceof Error ? err.message : typeof err === 'string' ? err : fallback;
		this.show('error', msg);
	}

	dismiss(id: number) {
		this.items = this.items.filter((t) => t.id !== id);
	}
}

export const toast = new ToastStore();
