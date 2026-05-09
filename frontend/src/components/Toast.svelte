<script lang="ts">
	import { getToasts, dismissToast, type Toast } from '$lib/stores/toast.svelte';

	const toasts = $derived<Toast[]>(getToasts());

	const kindClasses: Record<Toast['kind'], string> = {
		error: 'border-danger/40 bg-danger/15 text-danger',
		success: 'border-success/40 bg-success/15 text-success',
		info: 'border-info/40 bg-info/15 text-info'
	};
</script>

<div
	class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-full max-w-sm flex-col gap-2"
	role="region"
	aria-label="Notifications"
	aria-live="polite"
>
	{#each toasts as toast (toast.id)}
		<div
			class="pointer-events-auto flex items-start gap-3 rounded-lg border bg-bg-card px-4 py-3 text-sm shadow-lg {kindClasses[toast.kind]}"
			role="status"
		>
			<span class="flex-1 break-words">{toast.message}</span>
			<button
				type="button"
				class="text-current opacity-70 transition hover:opacity-100 cursor-pointer"
				onclick={() => dismissToast(toast.id)}
				aria-label="Dismiss notification"
			>
				✕
			</button>
		</div>
	{/each}
</div>
