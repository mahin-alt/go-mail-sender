import { computed, shallowRef } from 'vue'
import { useMutation } from '@tanstack/vue-query'

import { sendEmail, type SingleEmail } from '@/api/emails'
import { ApiError } from '@/api/errors'

const EMAIL = /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/

/** What is wrong with each field, in words. Empty when the field is fine. */
export function emailErrors(draft: SingleEmail) {
  const to = draft.to.trim()
  return {
    to: !to
      ? 'Enter the recipient’s email address.'
      : !EMAIL.test(to)
        ? 'That does not look like an email address.'
        : '',
    subject: draft.subject.trim() ? '' : 'Write a subject.',
    body: draft.body.trim() ? '' : 'Write a message.',
  }
}

/** State of the Single Email page: the three fields, what is wrong with them, and the last result. */
export function useSingleEmail() {
  const to = shallowRef('')
  const subject = shallowRef('')
  const body = shallowRef('')
  /** Errors stay hidden until the user first tries to send. */
  const showErrors = shallowRef(false)
  const sentTo = shallowRef('')
  const failure = shallowRef('')

  const send = useMutation({ mutationFn: (input: SingleEmail) => sendEmail(input) })

  const errors = computed(() =>
    emailErrors({ to: to.value, subject: subject.value, body: body.value }),
  )
  const valid = computed(() => !errors.value.to && !errors.value.subject && !errors.value.body)
  const sending = computed(() => send.isPending.value)

  /** A finished send or an old error is stale as soon as the user edits anything. */
  function edited() {
    sentTo.value = ''
    failure.value = ''
  }

  /** Marks the form as submitted. Returns whether it can be sent. */
  function validate(): boolean {
    showErrors.value = true
    return valid.value
  }

  async function submit(): Promise<boolean> {
    failure.value = ''
    sentTo.value = ''
    const recipient = to.value.trim()
    try {
      await send.mutateAsync({ to: recipient, subject: subject.value.trim(), body: body.value })
      sentTo.value = recipient
      return true
    } catch (error) {
      failure.value =
        error instanceof ApiError
          ? error.status === 502
            ? `${error.message}. Check the SMTP settings and the server log.`
            : error.message
          : 'Something went wrong. Nothing was sent.'
      return false
    }
  }

  function reset() {
    to.value = ''
    subject.value = ''
    body.value = ''
    showErrors.value = false
    sentTo.value = ''
    failure.value = ''
  }

  return {
    to,
    subject,
    body,
    errors,
    valid,
    showErrors,
    sentTo,
    failure,
    sending,
    edited,
    validate,
    submit,
    reset,
  }
}
