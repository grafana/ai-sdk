import { act, renderHook } from '@testing-library/react';
import { useFieldErrors } from './useFieldErrors';
import { FieldErrors } from './catalogSpec';

interface Props {
  errors: FieldErrors;
  values: Record<string, string>;
}

function render(initial: Props) {
  return renderHook(({ errors, values }: Props) => useFieldErrors(errors, values), { initialProps: initial });
}

describe('components/Catalog/useFieldErrors', () => {
  test('shows a format error once the field is left, and clears it as soon as the value is valid', () => {
    const hook = render({ errors: { name: 'bad format' }, values: { name: 'Bad' } });
    expect(hook.result.current.errors).toEqual({});

    act(() => hook.result.current.leave('name'));
    expect(hook.result.current.errors).toEqual({ name: 'bad format' });

    hook.rerender({ errors: {}, values: { name: 'good' } });
    expect(hook.result.current.errors).toEqual({});
    hook.rerender({ errors: { name: 'bad format' }, values: { name: 'Bad again' } });
    expect(hook.result.current.errors).toEqual({ name: 'bad format' });
  });

  test('reports a missing value only on submit', () => {
    const hook = render({ errors: { key: 'missing' }, values: { key: '' } });
    act(() => hook.result.current.leave('key'));
    expect(hook.result.current.errors).toEqual({});

    let sendable = true;
    act(() => {
      sendable = hook.result.current.submit();
    });
    expect(sendable).toBe(false);
    expect(hook.result.current.errors).toEqual({ key: 'missing' });
  });

  test('shows every error after a submit, and forgets them on reset', () => {
    const hook = render({ errors: { a: 'a error', b: 'b error' }, values: { a: 'x', b: '' } });
    act(() => {
      hook.result.current.submit();
    });
    expect(hook.result.current.errors).toEqual({ a: 'a error', b: 'b error' });

    act(() => hook.result.current.reset());
    expect(hook.result.current.errors).toEqual({});
  });

  test('lets a form without errors be sent', () => {
    const hook = render({ errors: {}, values: { a: 'x' } });
    let sendable = false;
    act(() => {
      sendable = hook.result.current.submit();
    });
    expect(sendable).toBe(true);
  });
});
