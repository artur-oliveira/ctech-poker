import {beforeEach, describe, expect, test, vi} from 'vitest';
import {avatarJPEG, deleteAvatar, uploadAvatar, uploadAvatarJPEG} from './avatar';

const {post, deleteRequest} = vi.hoisted(() => ({post: vi.fn(), deleteRequest: vi.fn()}));
vi.mock('./api/client', () => ({apiClient: {post, delete: deleteRequest}}));

describe('avatar upload', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    post.mockReset();
    deleteRequest.mockReset();
    vi.unstubAllGlobals();
  });
  
  test('posts every signed field before the file without authorization headers', async () => {
    post.mockResolvedValueOnce({
      data: {
        url: 'https://bucket.s3.dualstack.us-east-1.amazonaws.com',
        fields: {key: 'up/u/1.jpg', policy: 'signed'}, version: 1
      }
    })
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
      width: number;
      height: number;
      
      constructor(width: number, height: number) {
        this.width = width;
        this.height = height;
      }
      
      getContext() {
        return {drawImage};
      }
      
      async convertToBlob(options: { type: string }) {
        expect(options.type).toBe('image/jpeg');
        return output;
      }
    }
    
    vi.stubGlobal('OffscreenCanvas', Canvas);
    expect(await avatarJPEG(new File(['source'], 'photo.png', {type: 'image/png'}))).toBe(output);
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 100, 0, 200, 200, 0, 0, 192, 192);
    expect(close).toHaveBeenCalledOnce();
  });

  test('uses the DOM canvas fallback for tall images and always closes the bitmap', async () => {
    const close = vi.fn();
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({width: 200, height: 400, close})));
    vi.stubGlobal('OffscreenCanvas', undefined);
    const drawImage = vi.fn();
    const output = new Blob(['fallback'], {type: 'image/jpeg'});
    const canvas = document.createElement('canvas');
    vi.spyOn(document, 'createElement').mockReturnValue(canvas);
    vi.spyOn(canvas, 'getContext').mockReturnValue({drawImage} as unknown as CanvasRenderingContext2D);
    vi.spyOn(canvas, 'toBlob').mockImplementation(callback => callback(output));

    await expect(avatarJPEG(new File(['source'], 'portrait.png'))).resolves.toBe(output);
    expect(canvas.width).toBe(192);
    expect(canvas.height).toBe(192);
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 0, 100, 200, 200, 0, 0, 192, 192);
    expect(close).toHaveBeenCalledOnce();
  });

  test('reports unavailable canvases and failed DOM canvas encoding', async () => {
    const close = vi.fn();
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({width: 200, height: 200, close})));

    class MissingContextCanvas {
      getContext() { return null; }
    }
    vi.stubGlobal('OffscreenCanvas', MissingContextCanvas);
    await expect(avatarJPEG(new File(['source'], 'photo.png'))).rejects.toThrow('canvas indisponível');
    expect(close).toHaveBeenCalledOnce();

    vi.stubGlobal('OffscreenCanvas', undefined);
    const canvas = document.createElement('canvas');
    vi.spyOn(document, 'createElement').mockReturnValue(canvas);
    vi.spyOn(canvas, 'getContext').mockReturnValue({drawImage: vi.fn()} as unknown as CanvasRenderingContext2D);
    vi.spyOn(canvas, 'toBlob').mockImplementation(callback => callback(null));
    await expect(avatarJPEG(new File(['source'], 'photo.png'))).rejects
      .toThrow('não foi possível preparar a imagem');
    expect(close).toHaveBeenCalledTimes(2);
  });

  test('rejects a failed object-store upload before confirming the avatar', async () => {
    post.mockResolvedValueOnce({data: {url: 'https://bucket.invalid', fields: {}, version: 2}});
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, {status: 503})));

    await expect(uploadAvatarJPEG(new Blob(['jpeg']))).rejects.toThrow('S3 upload failed: 503');
    expect(post).toHaveBeenCalledOnce();
  });

  test('processes an original file through upload and deletes an avatar', async () => {
    const close = vi.fn();
    const jpeg = new Blob(['jpeg'], {type: 'image/jpeg'});
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({width: 192, height: 192, close})));
    vi.stubGlobal('OffscreenCanvas', class {
      getContext() { return {drawImage: vi.fn()}; }
      async convertToBlob() { return jpeg; }
    });
    post.mockResolvedValueOnce({data: {url: 'https://bucket.invalid', fields: {}, version: 3}})
      .mockResolvedValueOnce({data: {user_id: 'u', avatar_url: '/avatars/u/3.jpg'}});
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, {status: 204})));

    await expect(uploadAvatar(new File(['source'], 'photo.png'))).resolves
      .toMatchObject({avatar_url: '/avatars/u/3.jpg'});
    deleteRequest.mockResolvedValueOnce({data: {user_id: 'u'}});
    await expect(deleteAvatar()).resolves.toMatchObject({user_id: 'u'});
  });
});
