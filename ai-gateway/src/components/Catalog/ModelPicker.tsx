import React, { KeyboardEvent, useEffect, useId, useRef, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { IconButton, Input, useStyles2 } from '@grafana/ui';
import { ProviderModelListing } from '../../api/modelCatalogs';

type Listing = { kind: 'loading' } | { kind: 'ready'; result: ProviderModelListing } | { kind: 'failed' };

const MESSAGES: Record<ProviderModelListing['status'], string> = {
  ok: 'The provider returned no models.',
  partial: 'The provider listing is incomplete. You can still enter any model ID.',
  invalidKey: 'The provider rejected the saved key.',
  unreachable: 'The provider could not be reached.',
  refused: 'The provider did not return a usable model listing.',
  notChecked: 'This provider listing is unavailable.',
};

interface Props {
  label: string;
  value: string;
  onChange: (id: string) => void;
  onBlur: () => void;
  listModels: () => Promise<ProviderModelListing>;
}

/** ModelPicker accepts an exact provider model ID and suggests matches from the saved account. */
export function ModelPicker({ label, value, onChange, onBlur, listModels }: Props) {
  const s = useStyles2(getStyles);
  const listId = useId();
  const input = useRef<HTMLInputElement>(null);
  const activeOption = useRef<HTMLLIElement>(null);
  const generation = useRef(0);
  const [listing, setListing] = useState<Listing>();
  const [highlight, setHighlight] = useState(-1);

  const open = listing !== undefined;
  const result = listing?.kind === 'ready' ? listing.result : undefined;
  const ids = result?.modelIds ?? [];
  const query = value.toLowerCase();
  const matches = ids.filter((id) => id.toLowerCase().includes(query));
  const active = matches[highlight];
  const message =
    listing?.kind === 'loading'
      ? 'Loading provider models…'
      : listing?.kind === 'failed'
        ? 'Could not list models. You can still enter a model ID.'
        : result && (result.status !== 'ok' || ids.length === 0)
          ? MESSAGES[result.status]
          : undefined;

  useEffect(() => {
    if (active !== undefined) {
      activeOption.current?.scrollIntoView?.({ block: 'nearest' });
    }
  }, [active]);

  const show = async () => {
    if (open) {
      return;
    }
    const current = ++generation.current;
    setListing({ kind: 'loading' });
    try {
      const result = await listModels();
      if (current === generation.current) {
        setListing({ kind: 'ready', result });
      }
    } catch {
      if (current === generation.current) {
        setListing({ kind: 'failed' });
      }
    }
  };

  const close = () => {
    generation.current++;
    setListing(undefined);
    setHighlight(-1);
  };

  const select = (id: string) => {
    onChange(id);
    close();
  };

  const keyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.nativeEvent.isComposing) {
      return;
    }
    switch (event.key) {
      case 'ArrowDown':
      case 'ArrowUp':
        event.preventDefault();
        if (!open) {
          void show();
        }
        if (event.key === 'ArrowDown') {
          setHighlight(open ? Math.min(highlight + 1, matches.length - 1) : 0);
        } else {
          setHighlight(highlight < 0 ? matches.length - 1 : Math.max(0, highlight - 1));
        }
        break;
      case 'Enter':
        if (active !== undefined) {
          event.preventDefault();
          select(active);
        }
        break;
      case 'Escape':
        if (open) {
          event.preventDefault();
          close();
        }
        break;
    }
  };

  return (
    <div className={s.anchor}>
      <Input
        ref={input}
        role="combobox"
        aria-label={label}
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={active === undefined ? undefined : `${listId}-${highlight}`}
        placeholder="Search or enter a model ID"
        autoComplete="off"
        value={value}
        onClick={() => void show()}
        onChange={(event) => {
          onChange(event.currentTarget.value);
          setHighlight(-1);
          void show();
        }}
        onBlur={() => {
          close();
          onBlur();
        }}
        onKeyDown={keyDown}
        suffix={
          <IconButton
            type="button"
            name={open ? 'angle-up' : 'angle-down'}
            aria-label={open ? 'Hide model suggestions' : 'Show model suggestions'}
            tabIndex={-1}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => {
              input.current?.focus();
              if (open) {
                close();
              } else {
                void show();
              }
            }}
          />
        }
      />
      {open && (
        <div className={s.popup}>
          {message && (
            <div className={s.message} role="status">
              {message}
            </div>
          )}
          {ids.length > 0 && matches.length === 0 && (
            <div className={s.message} role="status">
              No matching models. You can add the ID you typed.
            </div>
          )}
          <ul
            className={s.list}
            id={listId}
            role="listbox"
            aria-label="Available provider models"
            aria-busy={listing.kind === 'loading'}
          >
            {matches.map((id, index) => (
              <li
                className={s.option}
                key={id}
                id={`${listId}-${index}`}
                ref={index === highlight ? activeOption : undefined}
                role="option"
                aria-selected={index === highlight}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => select(id)}
              >
                {id}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  anchor: css({ position: 'relative' }),
  popup: css({
    position: 'absolute',
    top: '100%',
    left: 0,
    right: 0,
    zIndex: theme.zIndex.dropdown,
    marginTop: theme.spacing(0.5),
    border: `1px solid ${theme.colors.border.medium}`,
    borderRadius: theme.shape.radius.default,
    background: theme.colors.background.primary,
    boxShadow: theme.shadows.z2,
  }),
  list: css({ listStyle: 'none', margin: 0, padding: 0, maxHeight: theme.spacing(30), overflowY: 'auto' }),
  option: css({
    cursor: 'pointer',
    overflowWrap: 'anywhere',
    padding: theme.spacing(0.75, 1.5),
    '&:hover, &[aria-selected=true]': { background: theme.colors.action.hover },
  }),
  message: css({ padding: theme.spacing(1, 1.5), color: theme.colors.text.secondary }),
});
