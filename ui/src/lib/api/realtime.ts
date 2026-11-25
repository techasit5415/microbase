import type { RealtimeEvent } from './types';

/** Subscribes to Microbase SSE stream and returns an unsubscribe cleanup function. */
export function subscribeRealtime(
	onEvent: (evt: RealtimeEvent) => void,
	onStatus?: (connected: boolean) => void
): () => void {
	const es = new EventSource('/api/realtime');
	es.addEventListener('connected', () => onStatus?.(true));
	es.onerror = () => onStatus?.(false);

	(['create', 'update', 'delete'] as const).forEach((action) => {
		es.addEventListener(action, (e) => {
			onEvent(JSON.parse((e as MessageEvent).data) as RealtimeEvent);
		});
	});

	return () => es.close();
}