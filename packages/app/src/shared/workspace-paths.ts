/**
 * Utilities for cross-platform workspace path validation and normalization.
 */

/**
 * Checks whether a given path string is an absolute directory path on POSIX or Windows.
 * - POSIX: starts with /
 * - Windows drive letter: starts with [A-Za-z]:\ or [A-Za-z]:/ (e.g. D:\Mender, C:/project)
 * - Windows UNC: starts with \\server\share or //server/share
 */
export function isAbsoluteWorkspacePath(path: string): boolean {
  const trimmed = path.trim();
  if (!trimmed) return false;
  if (trimmed.startsWith("/")) return true;
  if (/^[a-zA-Z]:[\\/]/.test(trimmed)) return true;
  if (trimmed.startsWith("\\\\") || trimmed.startsWith("//")) {
    const rest = trimmed.slice(2);
    const sepIdx = rest.search(/[\\/]/);
    if (sepIdx > 0 && sepIdx < rest.length - 1) return true;
  }
  return false;
}

/**
 * Normalizes a workspace path for duplicate comparison across platforms.
 * - Replaces backslashes with forward slashes
 * - Strips trailing slashes
 * - Lowers case for Windows drive-letter paths (case-insensitive NTFS/FAT filesystems)
 */
export function comparableWorkspacePath(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  let normalized = trimmed.replace(/\\+/g, "/").replace(/\/+$/u, "");
  if (/^[a-zA-Z]:(\/|$)/.test(normalized) || normalized.startsWith("//")) {
    normalized = normalized.toLowerCase();
  }
  return normalized || "/";
}

/**
 * Returns a platform-appropriate placeholder example path for workspace directory inputs.
 */
export function defaultWorkspacePlaceholder(): string {
  if (typeof navigator !== "undefined" && /win/i.test(navigator.userAgent || (navigator as { platform?: string }).platform || "")) {
    return "C:\\Users\\example\\project";
  }
  return "/Users/example/project";
}
