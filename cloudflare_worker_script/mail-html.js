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
                return new Response(JSON.stringify({ success: false, message: 'Unauthorized' }), { status: 401, headers: { 'Content-Type': 'application/json' } });
            }

            const body = await request.text();
            const uuid = self.crypto.randomUUID();
            const createTime = Date.now();

            const db = env.DB; // 从环境变量中获取 D1 数据库实例

            try {
                await db.prepare(`INSERT INTO mail_data (id, data, createTime)
                    VALUES (?1, ?2, ?3)`).bind(uuid, body, createTime).run();

                return new Response(JSON.stringify({ uuid, success: true }), { status: 200, headers: { 'Content-Type': 'application/json' } });
            } catch (error) {
                return new Response(JSON.stringify({ success: false, message: error.message }), { status: 500, headers: { 'Content-Type': 'application/json' } });
            }
        } else if (pathname.startsWith('/mail/') && request.method === 'GET') {
            const uuid = pathname.split('/')[2];

            if (!uuid) {
                return new Response(JSON.stringify({ success: false, message: 'UUID is required' }), { status: 400, headers: { 'Content-Type': 'application/json' } });
            }

            const db = env.DB; // 从环境变量中获取 D1 数据库实例

            try {
                const result = await db.prepare(`
                    SELECT data FROM mail_data
                    WHERE id = ?
                `,).bind(uuid).first('data');

                if (result.length === 0) {
                    return new Response(JSON.stringify({ success: false, message: 'Mail not found' }), { status: 404, headers: { 'Content-Type': 'application/json' } });
                }

                const data = result[0].data;
                return new Response(data, { status: 200, headers: { 'Content-Type': 'text/html' } });
            } catch (error) {
                return new Response(JSON.stringify({ success: false, message: error.message }), { status: 500, headers: { 'Content-Type': 'application/json' } });
            }
        }

        return new Response(JSON.stringify({ success: false, message: 'Not Found' }), { status: 404, headers: { 'Content-Type': 'application/json' } });
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