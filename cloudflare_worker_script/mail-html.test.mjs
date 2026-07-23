import assert from 'node:assert/strict';
import test from 'node:test';

import worker from './mail-html.js';

function envReturning(data) {
    return {
        DB: {
            prepare() {
                return {
                    bind() {
                        return {
                            async first(column) {
                                assert.equal(column, 'data');
                                return data;
                            },
                        };
                    },
                };
            },
        },
    };
}

test('mail viewer returns the D1 field with an active-content sandbox', async () => {
    const html = '<html><img src="https://images.example/pixel.png"><script>alert(1)</script></html>';
    const response = await worker.fetch(
        new Request('https://mail.example/mail/8da2531f-6421-4b79-b81f-9fb3f2b05826'),
        envReturning(html),
        {},
    );

    assert.equal(response.status, 200);
    assert.equal(await response.text(), html);
    assert.equal(response.headers.get('Content-Type'), 'text/html; charset=UTF-8');
    assert.equal(response.headers.get('Cache-Control'), 'private, no-store');

    const csp = response.headers.get('Content-Security-Policy');
    assert.match(csp, /(?:^|; )sandbox(?:;|$)/);
    assert.match(csp, /script-src 'none'/);
    assert.match(csp, /img-src https: data:/);
    assert.doesNotMatch(csp, /allow-scripts|allow-same-origin/);
});

test('mail viewer returns 404 when D1 has no matching row', async () => {
    const response = await worker.fetch(
        new Request('https://mail.example/mail/missing'),
        envReturning(null),
        {},
    );

    assert.equal(response.status, 404);
    assert.deepEqual(await response.json(), { success: false, message: 'Mail not found' });
});

test('mail viewer does not expose D1 errors', async () => {
    const env = {
        DB: {
            prepare() {
                throw new Error('private database detail');
            },
        },
    };
    const originalConsoleError = console.error;
    console.error = () => {};

    try {
        const response = await worker.fetch(
            new Request('https://mail.example/mail/failure'),
            env,
            {},
        );

        assert.equal(response.status, 500);
        const body = await response.text();
        assert.doesNotMatch(body, /private database detail/);
        assert.deepEqual(JSON.parse(body), { success: false, message: 'Internal Server Error' });
    } finally {
        console.error = originalConsoleError;
    }
});
