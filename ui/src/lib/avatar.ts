import {apiClient} from './api/client';
import type {PlayerProfile} from './api/player';

const AVATAR_SIZE = 192;

export async function avatarJPEG(file: File): Promise<Blob> {
  const bitmap = await createImageBitmap(file, {imageOrientation: 'from-image'});
  try {
    const side = Math.min(bitmap.width, bitmap.height);
    const sx = Math.floor((bitmap.width - side) / 2);
    const sy = Math.floor((bitmap.height - side) / 2);
    if (typeof OffscreenCanvas !== 'undefined') {
      const canvas = new OffscreenCanvas(AVATAR_SIZE, AVATAR_SIZE);
      const context = canvas.getContext('2d');
      if (!context) throw new Error('canvas indisponível');
      context.drawImage(bitmap, sx, sy, side, side, 0, 0, AVATAR_SIZE, AVATAR_SIZE);
      return canvas.convertToBlob({type: 'image/jpeg', quality: .85});
    }
    const canvas = document.createElement('canvas');
    canvas.width = AVATAR_SIZE;
    canvas.height = AVATAR_SIZE;
    const context = canvas.getContext('2d');
    if (!context) throw new Error('canvas indisponível');
    context.drawImage(bitmap, sx, sy, side, side, 0, 0, AVATAR_SIZE, AVATAR_SIZE);
    return await new Promise<Blob>((resolve, reject) => canvas.toBlob(
      blob => blob ? resolve(blob) : reject(new Error('não foi possível preparar a imagem')),
      'image/jpeg', .85
    ));
  } finally {
    bitmap.close();
  }
}

export async function uploadAvatar(file: File): Promise<PlayerProfile> {
  const jpeg = await avatarJPEG(file);
  return uploadAvatarJPEG(jpeg);
}

export async function uploadAvatarJPEG(jpeg: Blob): Promise<PlayerProfile> {
  const presign = (await apiClient.post<{url: string; fields: Record<string, string>; version: number}>(
    '/v1.0/players/me/avatar/upload-url', {}, {silentError: false}
  )).data;
  const form = new FormData();
  Object.entries(presign.fields).forEach(([key, value]) => form.append(key, value));
  form.append('file', jpeg, 'avatar.jpg');
  const response = await fetch(presign.url, {method: 'POST', body: form});
  if (!response.ok) throw new Error(`S3 upload failed: ${response.status}`);
  return (await apiClient.post<PlayerProfile>('/v1.0/players/me/avatar/confirm',
    {version: presign.version}, {silentError: false})).data;
}

export async function deleteAvatar(): Promise<PlayerProfile> {
  return (await apiClient.delete<PlayerProfile>('/v1.0/players/me/avatar', {silentError: false})).data;
}
