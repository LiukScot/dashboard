<script lang="ts">
	import { getToasts, dismissToast, type Toast } from '$lib/stores/toast.svelte';

	const toasts = $derived<Toast[]>(getToasts());
	const polite = $derived(toasts.filter((t) => t.kind !== 'error'));
	const assertive = $derived(toasts.filter((t) => t.kind === 'error'));

	const kindClasses: Record<Toast['kind'], string> = {
		error: 'border-danger/40 bg-danger/15 text-danger',
		success: 'border-success/40 bg-success/15 text-success',
		info: 'border-info/40 bg-info/15 text-info'
	};
</script>

<!--
	Two stacked regions so screen readers handle errors interrupt-style
	while non-error toasts queue politely. Single live-region role on the
	region; toast items themselves carry no role to avoid double-announce.
-->
<div class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-full max-w-sm flex-col gap-2">
	<div role="region" aria-label="Errors" aria-live="assertive" class="flex flex-col gap-2">
		{#each assertive as toast (toast.id)}
			<div
				class="pointer-events-auto flex items-start gap-3 rounded-lg border bg-bg-card px-4 py-3 text-sm shadow-lg {kindClasses[toast.kind]}"
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
	<div role="region" aria-label="Notifications" aria-live="polite" class="flex flex-col gap-2">
		{#each polite as toast (toast.id)}
			<div
				class="pointer-events-auto flex items-start gap-3 rounded-lg border bg-bg-card px-4 py-3 text-sm shadow-lg {kindClasses[toast.kind]}"
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
</div>
