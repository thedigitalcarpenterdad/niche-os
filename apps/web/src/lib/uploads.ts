import type { Upload } from "./types";

export function uploadURL(upload: Upload): string {
  return `/api/uploads/${encodeURIComponent(upload.id)}`;
}

export function isImageUpload(upload: Upload): boolean {
  return upload.content_type.startsWith("image/");
}

export function isVideoUpload(upload: Upload): boolean {
  return upload.content_type.startsWith("video/");
}

export function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

// Cache of upload_id -> blob URL to avoid re-fetching
const blobURLCache = new Map<string, string>();

export async function fetchUploadBlobURL(uploadId: string): Promise<string> {
  const cached = blobURLCache.get(uploadId);
  if (cached) return cached;
  
  const response = await fetch(`/api/uploads/${encodeURIComponent(uploadId)}`, {
    credentials: "include",
  });
  if (!response.ok) throw new Error(`Upload fetch failed: ${response.status}`);
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  blobURLCache.set(uploadId, url);
  return url;
}
