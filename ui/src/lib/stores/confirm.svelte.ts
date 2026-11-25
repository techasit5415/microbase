export interface ConfirmOptions {
	title: string;
	description?: string;
	confirmLabel?: string;
	cancelLabel?: string;
	destructive?: boolean;
}

interface ConfirmState {
	open: boolean;
	options: ConfirmOptions;
	resolve: ((value: boolean) => void) | null;
}

class ConfirmStore {
	state = $state<ConfirmState>({
		open: false,
		options: { title: '' },
		resolve: null
	});

	ask(options: ConfirmOptions): Promise<boolean> {
		if (this.state.resolve) {
			this.state.resolve(false);
		}
		return new Promise<boolean>((resolve) => {
			this.state = {
				open: true,
				options: {
					confirmLabel: 'Confirm',
					cancelLabel: 'Cancel',
					destructive: false,
					...options
				},
				resolve
			};
		});
	}

	respond(confirmed: boolean) {
		const resolver = this.state.resolve;
		this.state = {
			open: false,
			options: { title: '' },
			resolve: null
		};
		resolver?.(confirmed);
	}
}

export const confirmStore = new ConfirmStore();

export function confirmAction(options: ConfirmOptions): Promise<boolean> {
	return confirmStore.ask(options);
}
