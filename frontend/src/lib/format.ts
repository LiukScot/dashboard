export const BYTES_PER_KB = 1024;

export function formatBytes(bytes: number): string {
	if (bytes <= 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	const i = Math.min(Math.floor(Math.log(bytes) / Math.log(BYTES_PER_KB)), units.length - 1);
	return `${(bytes / Math.pow(BYTES_PER_KB, i)).toFixed(1)} ${units[i]}`;
}

/** Round to one decimal place. */
export function round1dp(x: number): number {
	return Math.round(x * 10) / 10;
}
