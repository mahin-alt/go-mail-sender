import { describe, expect, it } from 'vitest'

import { problemKind } from '@/lib/problems'
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

describe('problemKind', () => {
  it.each([
    ['email is empty', 'invalid'],
    ['name is empty', 'invalid'],
    ['invalid email "ben@@x"', 'invalid'],
    ['email has no domain', 'invalid'],
    ['duplicate email "a@b.c"', 'duplicate'],
    ['domain nope.example does not exist or has no mail server', 'domain'],
    ['domain x.example does not accept mail', 'domain'],
  ])('%s is %s', (reason, kind) => {
    expect(problemKind({ reason })).toBe(kind)
  })
})
