/*
 * The bake pipeline's little server: serves the repo so bake.html can import
 * the preview's modules, and accepts the baked pictures back —
 *
 *   PUT /plate/<location-key>   (body: image/webp)
 *   PUT /props/<location-key>   (body: image/webp)
 *
 * writes web/public/locations/<location-key>/plate.webp and props.webp. That
 * makes the whole bake scriptable end to end: `node tools/bake/serve.mjs`, open
 * http://localhost:8090/tools/bake/bake.html?bake=street, done.
 */

import { createServer } from 'node:http'
import { createWriteStream, existsSync, mkdirSync, readFile } from 'node:fs'
import { dirname, extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = normalize(join(dirname(fileURLToPath(import.meta.url)), '..', '..'))
const port = Number(process.env.PORT || 8090)

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json',
  '.tmj': 'application/json',
  '.webp': 'image/webp',
  '.png': 'image/png',
}

createServer((req, res) => {
  const url = new URL(req.url, `http://localhost:${port}`)

  const put = /^\/(plate|props)\/(.+)$/.exec(url.pathname)
  if (req.method === 'PUT' && put) {
    const [, picture, key] = put
    if (!/^[a-z0-9-]+$/.test(key)) {
      res.writeHead(400).end('not a location key')
      return
    }
    const dir = join(root, 'web', 'public', 'locations', key)
    mkdirSync(dir, { recursive: true })
    const file = join(dir, `${picture}.webp`)
    const out = createWriteStream(file)
    req.pipe(out)
    out.on('finish', () => {
      console.log(`wrote ${file}`)
      res.writeHead(200).end('saved')
    })
    out.on('error', (err) => res.writeHead(500).end(String(err)))
    return
  }

  const path = normalize(join(root, decodeURIComponent(url.pathname)))
  if (!path.startsWith(root) || !existsSync(path)) {
    res.writeHead(404).end('not found')
    return
  }
  readFile(path, (err, data) => {
    if (err) {
      res.writeHead(500).end(String(err))
      return
    }
    res.writeHead(200, { 'Content-Type': TYPES[extname(path)] ?? 'application/octet-stream' })
    res.end(data)
  })
}).listen(port, () => console.log(`bake server on http://localhost:${port}`))
