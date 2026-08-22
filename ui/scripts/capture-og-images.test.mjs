import {basename, dirname} from 'node:path';
import {describe, expect, test} from 'vitest';
import {capturesForArguments, GUIDE_CAPTURES, OG_CAPTURES} from './capture-og-images.mjs';

describe('capture image targets', () => {
    test('captures every Open Graph and guide image by default', () => {
        expect(capturesForArguments([])).toEqual([...GUIDE_CAPTURES, ...OG_CAPTURES]);
        expect(capturesForArguments(['--all'])).toEqual([...GUIDE_CAPTURES, ...OG_CAPTURES]);
        expect(GUIDE_CAPTURES.map(capture => basename(capture.output))).toEqual([
            'lobby.webp',
            'buyin.webp',
            'create-room.webp',
            'table-preflop.webp',
            'table-flop.webp',
            'table-showdown.webp',
            'hands-live.webp',
            'achievements-live.webp',
            'store-live.webp',
            'profile-live.webp',
            'leaderboard-live.webp'
        ]);
        expect(GUIDE_CAPTURES.find(capture => capture.slug === 'buyin').route).not.toContain('scenario=');
        expect(GUIDE_CAPTURES.filter(capture => capture.slug.startsWith('table-'))
            .every(capture => capture.ready === '.game' && capture.prepare === undefined)).toBe(true);
    });

    test('can limit a run to guide images', () => {
        const captures = capturesForArguments(['--guide']);

        expect(captures).toBe(GUIDE_CAPTURES);
        expect(captures.every(capture => basename(dirname(capture.output)) === 'guide')).toBe(true);
    });

    test('can limit a run to Open Graph images', () => {
        const captures = capturesForArguments(['--og']);

        expect(captures).toBe(OG_CAPTURES);
        expect(captures.every(capture => basename(dirname(capture.output)) === 'og')).toBe(true);
        expect(captures.find(capture => capture.slug === 'table')).toMatchObject({ready: '.game'});
    });
});
