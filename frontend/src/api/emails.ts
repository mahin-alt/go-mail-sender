import { http } from './client'

export interface SingleEmail {
  to: string
  subject: string
  body: string
}

/** Sends one email right away. Unlike a Bulk Email it is not queued; the call waits for the mail server. */
export async function sendEmail(input: SingleEmail): Promise<string> {
  const { data } = await http.post<{ message: string }>('/api/v1/emails', input, {
    timeout: 60_000,
  })
  return data.message
}
