export type ToastKind = 'error' | 'success' | 'info';

export interface Toast {
	id: number;
	kind: ToastKind;
	message: string;
}

let _toasts = $state<Toast[]>([]);
let nextId = 1;

const DEFAULT_DURATION_MS = 5000;

export function getToasts(): Toast[] {
	return _toasts;
}

export function pushToast(kind: ToastKind, message: string, durationMs = DEFAULT_DURATION_MS): number {
	const id = nextId++;
	_toasts = [..._toasts, { id, kind, message }];
	if (durationMs > 0) {
		setTimeout(() => dismissToast(id), durationMs);
	}
	return id;
}

export function dismissToast(id: number): void {
	_toasts = _toasts.filter((t) => t.id !== id);
}

export function toastError(err: unknown, fallback: string): void {
	const message = err instanceof Error ? err.message : fallback;
	pushToast('error', message);
}
