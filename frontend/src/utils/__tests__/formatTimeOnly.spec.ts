import { describe, expect, it } from 'vitest'
import { formatTimeOnly } from '../format'

describe('formatTimeOnly', () => {
  it('returns empty string for nullish input', () => {
    expect(formatTimeOnly(null)).toBe('')
    expect(formatTimeOnly(undefined)).toBe('')
  })

  it('returns empty string for invalid date', () => {
    expect(formatTimeOnly('not-a-date')).toBe('')
  })

  it('formats time as HH:mm:ss (24-hour) with seconds', () => {
    expect(formatTimeOnly('2026-07-15T14:30:45', 'en-US')).toBe('14:30:45')
  })
})
