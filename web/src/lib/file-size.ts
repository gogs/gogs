const SIZE_UNITS = ["B", "KiB", "MiB", "GiB"];

// formatFileSize formats a size in bytes with binary units, e.g., "4.6 KiB".
export function formatFileSize(bytes: number) {
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < SIZE_UNITS.length - 1) {
    size /= 1024;
    unit++;
  }
  return `${unit ? size.toFixed(1) : size} ${SIZE_UNITS[unit]}`;
}
