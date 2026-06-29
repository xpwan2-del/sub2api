import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../PublicTopBar.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('PublicTopBar canvas entry', () => {
  it('renders the Canvas topbar link (entry uncommented)', () => {
    // The canvas <a> block is the only HTML comment in this file;
    // once the entry is restored there must be no `<!--` left.
    expect(componentSource).not.toContain('<!--')
    // The local canvasPath constant must not be left as a `//` comment.
    expect(componentSource).not.toContain('// const canvasPath')
  })
})
