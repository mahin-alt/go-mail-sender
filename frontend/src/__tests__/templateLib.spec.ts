import { describe, expect, it } from 'vitest'

import { reasonText, tabCount } from '@/lib/problems'
import type { Batch } from '@/types/api'
import { DEFAULT_BODY, DEFAULT_SUBJECT, fillTemplate, unknownFields } from '@/lib/template'

const aisha = { name: 'Aisha', email: 'aisha@shop.example' }

describe('fillTemplate', () => {
  it('fills name and email, with or without spaces in the braces', () => {
    expect(fillTemplate('Hi {{.Name}} <{{ .Email }}>', aisha)).toBe('Hi Aisha <aisha@shop.example>')
  })
  it('fills every occurrence and leaves other text alone', () => {
    expect(fillTemplate('{{.Name}} {{.Name}} {{.Phone}}', aisha)).toBe('Aisha Aisha {{.Phone}}')
  })
  it('keeps Bangla names intact', () => {
    expect(fillTemplate('Hello {{.Name}}', { name: 'রহিম উদ্দিন', email: 'r@x.example' })).toBe(
      'Hello রহিম উদ্দিন',
    )
  })
  it('does not treat the name as HTML', () => {
    expect(fillTemplate('{{.Name}}', { name: '<b>x</b>', email: '' })).toBe('<b>x</b>')
  })
})

describe('unknownFields', () => {
  it('is empty for the default message', () => {
    expect(unknownFields(`${DEFAULT_SUBJECT}\n${DEFAULT_BODY}`)).toEqual([])
  })
  it('lists unknown fields once each', () => {
    expect(unknownFields('{{.Phone}} {{.Name}} {{ .Phone }} {{.City}}')).toEqual(['Phone', 'City'])
  })
})

describe('tabCount', () => {
  const batch = {
    invalid_count: 7,
    unverified_count: 2,
    invalid_counts: { empty_email: 1, empty_name: 2, invalid_email: 3, bad_domain: 1 },
  } as Batch

  it.each([
    ['all', 7],
    ['invalid', 6], // empty email + empty name + invalid email
    ['domain', 1],
    ['unverified', 2],
  ] as const)('%s holds %i rows', (tab, n) => {
    expect(tabCount(tab, batch)).toBe(n)
  })
})

describe('reasonText', () => {
  it.each([
    ['empty_email', 'email is empty'],
    ['empty_name', 'name is empty'],
    ['invalid_email', 'invalid email'],
    ['bad_domain', 'domain does not accept mail'],
    [null, 'could not verify domain'],
  ] as const)('%s -> %s', (reason, text) => {
    expect(reasonText({ validation_reason: reason })).toBe(text)
  })
})
