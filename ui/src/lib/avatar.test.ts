import {beforeEach, describe, expect, test, vi} from 'vitest';

const {post} = vi.hoisted(() => ({post: vi.fn()}));
vi.mock('./api/client', () => ({apiClient: {post, delete: vi.fn()}}));

import {avatarJPEG, uploadAvatarJPEG} from './avatar';

describe('avatar upload', () => {
  beforeEach(() => { post.mockReset(); vi.unstubAllGlobals(); });

  test('posts every signed field before the file without authorization headers', async () => {
    post.mockResolvedValueOnce({data: {url: 'https://bucket.s3.dualstack.us-east-1.amazonaws.com',
      fields: {key: 'up/u/1.jpg', policy: 'signed'}, version: 1}})
      .mockResolvedValueOnce({data: {user_id: 'u', avatar_url: '/avatars/u/1.jpg'}});
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      expect(init?.headers).toBeUndefined();
      expect([...((init?.body as FormData).keys())]).toEqual(['key', 'policy', 'file']);
      return new Response(null, {status: 204});
    });
    vi.stubGlobal('fetch', fetchMock);
    const result = await uploadAvatarJPEG(new Blob(['jpeg'], {type: 'image/jpeg'}));
    expect(result.avatar_url).toBe('/avatars/u/1.jpg');
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  test('center-crops to a 192 square JPEG', async () => {
    const close = vi.fn();
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({width: 400, height: 200, close})));
    const drawImage = vi.fn();
    const output = new Blob(['jpeg'], {type: 'image/jpeg'});
    class Canvas {
      width: number; height: number;
      constructor(width: number, height: number) { this.width = width; this.height = height; }
      getContext() { return {drawImage}; }
      async convertToBlob(options: {type: string}) { expect(options.type).toBe('image/jpeg'); return output; }
    }
    vi.stubGlobal('OffscreenCanvas', Canvas);
    expect(await avatarJPEG(new File(['source'], 'photo.png', {type: 'image/png'}))).toBe(output);
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 100, 0, 200, 200, 0, 0, 192, 192);
    expect(close).toHaveBeenCalledOnce();
  });
});
