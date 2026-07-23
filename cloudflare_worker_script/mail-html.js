const JSON_HEADERS = {
    'Content-Type': 'application/json; charset=UTF-8',
    'Cache-Control': 'no-store',
    'X-Content-Type-Options': 'nosniff',
};

const MAIL_HEADERS = {
    'Content-Type': 'text/html; charset=UTF-8',
    'Cache-Control': 'private, no-store',
    'Content-Security-Policy': [
        'sandbox',
        "default-src 'none'",
        "script-src 'none'",
        "style-src 'unsafe-inline'",
        'img-src https: data:',
        'font-src https: data:',
        'media-src https: data:',
        "connect-src 'none'",
        "frame-src 'none'",
        "object-src 'none'",
        "worker-src 'none'",
        "base-uri 'none'",
        "form-action 'none'",
        "frame-ancestors 'none'",
    ].join('; '),
    'Referrer-Policy': 'no-referrer',
    'X-Content-Type-Options': 'nosniff',
    'X-Frame-Options': 'DENY',
    'Cross-Origin-Opener-Policy': 'same-origin',
    'Permissions-Policy': 'camera=(), geolocation=(), microphone=(), payment=(), usb=()',
};

function jsonResponse(payload, status) {
    return new Response(JSON.stringify(payload), { status, headers: JSON_HEADERS });
}

export default {
    async scheduled(event, env, ctx) {
        ctx.waitUntil(handleScheduled(event, env));
    },
    async fetch(request, env, ctx) {
        const { pathname, searchParams } = new URL(request.url);

        if (pathname === '/upload' && request.method === 'POST') {
            const token = searchParams.get('token');
            const expectedToken = env.TOKEN; // 从环境变量中获取 token

            if (token !== expectedToken) {
                return jsonResponse({ success: false, message: 'Unauthorized' }, 401);
            }

            const body = await request.text();
            const uuid = self.crypto.randomUUID();
            const createTime = Date.now();

            const db = env.DB; // 从环境变量中获取 D1 数据库实例

            try {
                await db.prepare(`INSERT INTO mail_data (id, data, createTime)
                    VALUES (?1, ?2, ?3)`).bind(uuid, body, createTime).run();

                return jsonResponse({ uuid, success: true }, 200);
            } catch (error) {
                console.error('mail upload failed:', error);
                return jsonResponse({ success: false, message: 'Internal Server Error' }, 500);
            }
        } else if (pathname.startsWith('/mail/') && request.method === 'GET') {
            const uuid = pathname.split('/')[2];

            if (!uuid) {
                return jsonResponse({ success: false, message: 'UUID is required' }, 400);
            }

            const db = env.DB; // 从环境变量中获取 D1 数据库实例

            try {
                const data = await db.prepare(`
                    SELECT data FROM mail_data
                    WHERE id = ?
                `,).bind(uuid).first('data');

                if (data === null || data === undefined) {
                    return jsonResponse({ success: false, message: 'Mail not found' }, 404);
                }

                return new Response(data, { status: 200, headers: MAIL_HEADERS });
            } catch (error) {
                console.error('mail lookup failed:', error);
                return jsonResponse({ success: false, message: 'Internal Server Error' }, 500);
            }
        }

        return jsonResponse({ success: false, message: 'Not Found' }, 404);
    }
};


async function handleScheduled(event, env) {
    const db = env.DB; // 从环境变量中获取 D1 数据库实例

    try {
        let delTime = Date.now() - (7 * 24 * 60 * 60 * 1000);
        // 将数据插入到 D1 数据库中
        await db.prepare(`DELETE FROM mail_data where createTime < ?1 `).bind(delTime).run();

        console.log('delete expire mail successful at', new Date().toISOString());
    } catch (error) {
        console.error('delete expire mail failed:', error);
    }
}
