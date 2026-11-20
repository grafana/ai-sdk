import React, { ReactNode } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Combobox, ComboboxOption, Field, Input, useStyles2 } from '@grafana/ui';
import { FieldErrors, providerTypeLabel } from './catalogSpec';
import { Provider, Target } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

interface Props {
  title: string;
  field: string;
  target: Target;
  providers: Record<string, Provider>;
  errors: FieldErrors;
  onChange: (target: Target) => void;
  onLeave?: (field: string) => void;
  disabled?: boolean;
  actions?: ReactNode;
}

/** TargetFields edits one provider and model a model routes calls to. */
export function TargetFields({ title, field, target, providers, errors, onChange, onLeave, disabled, actions }: Props) {
  const s = useStyles2(getStyles);
  const options: Array<ComboboxOption<string>> = Object.entries(providers).map(([name, provider]) => ({
    value: name,
    description: providerTypeLabel(provider.type),
  }));
  if (target.provider !== '' && !Object.hasOwn(providers, target.provider)) {
    options.push({ value: target.provider });
  }
  const providerError = errors[`${field}.provider`];
  const modelError = errors[`${field}.model`];

  return (
    <div className={s.row}>
      <Field
        className={s.provider}
        label={`${title} provider`}
        invalid={providerError !== undefined}
        error={providerError}
        noMargin
      >
        <Combobox
          aria-label={`${title} provider`}
          options={options}
          value={target.provider || null}
          placeholder="Choose a provider"
          invalid={providerError !== undefined}
          disabled={disabled}
          onChange={(option) => onChange({ ...target, provider: option.value })}
          onBlur={() => onLeave?.(`${field}.provider`)}
        />
      </Field>
      <Field
        className={s.model}
        label={`${title} model`}
        invalid={modelError !== undefined}
        error={modelError}
        noMargin
      >
        <Input
          aria-label={`${title} model`}
          value={target.model}
          placeholder="The provider's model ID"
          invalid={modelError !== undefined}
          disabled={disabled}
          onChange={(event) => onChange({ ...target, model: event.currentTarget.value })}
          onBlur={() => onLeave?.(`${field}.model`)}
        />
      </Field>
      {actions && <div className={s.actions}>{actions}</div>}
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  row: css({
    display: 'flex',
    alignItems: 'flex-start',
    gap: theme.spacing(1),
  }),
  provider: css({
    flex: '0 0 35%',
  }),
  model: css({
    flex: 1,
    minWidth: 0,
  }),
  actions: css({
    display: 'flex',
    paddingTop: theme.spacing(3),
  }),
});
