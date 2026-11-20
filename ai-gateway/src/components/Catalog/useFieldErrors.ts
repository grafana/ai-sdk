import { useState } from 'react';
import { FieldErrors, hasErrors } from './catalogSpec';

/**
 * useFieldErrors decides which of a form's current errors to show. Before the
 * first submit, a field shows its error once the writer leaves it, unless its
 * value in values is blank: a missing value is reported only on submit. After a submit,
 * every error shows. The errors are passed in on each render, so a shown
 * error clears as soon as its value becomes valid.
 */
export function useFieldErrors(errors: FieldErrors, values: Record<string, string>) {
  const [submitted, setSubmitted] = useState(false);
  const [left, setLeft] = useState<string[]>([]);

  const shown: FieldErrors = submitted
    ? errors
    : Object.fromEntries(
        Object.entries(errors).filter(([field]) => left.includes(field) && values[field]?.trim() !== '')
      );

  return {
    errors: shown,
    /** leave marks a field as left by the writer, so its format errors show. */
    leave: (field: string) => setLeft((current) => (current.includes(field) ? current : [...current, field])),
    /** submit shows every error and says whether the form can be sent. */
    submit: () => {
      setSubmitted(true);
      return !hasErrors(errors);
    },
    /** reset forgets left fields and the submit, for a form that starts over. */
    reset: () => {
      setSubmitted(false);
      setLeft([]);
    },
  };
}
