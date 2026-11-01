/** The fields an email template may use. They match the Go `RecipientData` struct. */
export const TEMPLATE_FIELDS = ['Name', 'Email'] as const

export const DEFAULT_SUBJECT = 'Update on your order, {{.Name}}'

export const DEFAULT_BODY = `Hello {{.Name}},

We wanted to let you know that your order is on its way and should arrive within 2 to 3 working days.

If you have any questions, just reply to this email.

Thank you,
MailFlow Support`

export interface TemplateRecipient {
  name: string
  email: string
}

/** Fill `{{.Name}}` and `{{.Email}}` (spaces inside the braces are allowed, like Go templates). */
export function fillTemplate(template: string, recipient: TemplateRecipient): string {
  return template
    .replace(/\{\{\s*\.Name\s*\}\}/g, recipient.name)
    .replace(/\{\{\s*\.Email\s*\}\}/g, recipient.email)
}

/** Field names used in the template that the backend does not know (e.g. `{{.Phone}}`). */
export function unknownFields(template: string): string[] {
  const known: readonly string[] = TEMPLATE_FIELDS
  const found = [...template.matchAll(/\{\{\s*\.(\w+)\s*\}\}/g)].map((m) => m[1] as string)
  return [...new Set(found.filter((name) => !known.includes(name)))]
}
