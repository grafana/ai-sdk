import React from 'react';
import { Button, Field, IconButton, Input, Stack, TagsInput, Text } from '@grafana/ui';
import { FieldErrors, MAX_FALLBACKS, ModelDraft, targetField } from './catalogSpec';
import { TargetFields } from './TargetFields';
import { Provider, Target } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

const MAX_ALIAS_LENGTH = 128;

interface Props {
  draft: ModelDraft;
  errors: FieldErrors;
  onChange: (draft: ModelDraft) => void;
  idPrefix: string;
}

interface IdentityProps extends Props {
  editing: boolean;
  optional: boolean;
}

/**
 * ModelIdentityFields edits what applications see of a model: its ID, which
 * is fixed once saved, its display name and, when optional is set, its
 * description and aliases.
 */
export function ModelIdentityFields({ draft, errors, onChange, idPrefix, editing, optional }: IdentityProps) {
  return (
    <Stack direction="column" gap={0}>
      <Field
        label="Model ID"
        description={
          editing
            ? "Applications request the model by this ID, so it can't be changed."
            : 'Applications request the model by this ID. Letters, digits and . _ : / -'
        }
        invalid={errors.id !== undefined}
        error={errors.id}
      >
        <Input
          id={`${idPrefix}-id`}
          value={draft.id}
          disabled={editing}
          onChange={(event) => onChange({ ...draft, id: event.currentTarget.value })}
        />
      </Field>
      <Field
        label="Display name"
        description="Shown where applications list models."
        invalid={errors.name !== undefined}
        error={errors.name}
      >
        <Input
          id={`${idPrefix}-name`}
          value={draft.name}
          onChange={(event) => onChange({ ...draft, name: event.currentTarget.value })}
        />
      </Field>
      {optional && (
        <>
          <Field label="Description">
            <Input
              id={`${idPrefix}-description`}
              value={draft.description}
              placeholder="Optional"
              onChange={(event) => onChange({ ...draft, description: event.currentTarget.value })}
            />
          </Field>
          <Field label="Aliases" description="Other IDs applications can request the model by. Press Enter to add one.">
            <TagsInput
              id={`${idPrefix}-aliases`}
              tags={draft.aliases}
              placeholder="Add an alias"
              maxLength={MAX_ALIAS_LENGTH}
              addOnBlur
              onChange={(aliases) => onChange({ ...draft, aliases })}
            />
          </Field>
        </>
      )}
    </Stack>
  );
}

interface RoutingProps extends Omit<Props, 'idPrefix'> {
  providers: Record<string, Provider>;
}

/** RoutingFields edits the provider and model a model calls first, and its fallbacks in order. */
export function RoutingFields({ draft, errors, onChange, providers }: RoutingProps) {
  const { fallback } = draft;
  const setFallback = (next: Target[]) => onChange({ ...draft, fallback: next });
  const move = (from: number, to: number) => {
    const next = [...fallback];
    next.splice(to, 0, ...next.splice(from, 1));
    setFallback(next);
  };

  return (
    <Stack direction="column" gap={2}>
      <TargetFields
        title="Primary"
        field={targetField(0)}
        target={draft.primary}
        providers={providers}
        errors={errors}
        onChange={(primary) => onChange({ ...draft, primary })}
      />
      {fallback.length > 0 && (
        <Text color="secondary" variant="bodySmall">
          If a call fails, the Gateway tries each fallback in order.
        </Text>
      )}
      {fallback.map((target, index) => {
        const title = `Fallback ${index + 1}`;
        return (
          <TargetFields
            key={index}
            title={title}
            field={targetField(index + 1)}
            target={target}
            providers={providers}
            errors={errors}
            onChange={(next) => setFallback(fallback.map((current, i) => (i === index ? next : current)))}
            actions={
              <>
                <IconButton
                  name="arrow-up"
                  tooltip={`Move ${title.toLowerCase()} up`}
                  disabled={index === 0}
                  onClick={() => move(index, index - 1)}
                />
                <IconButton
                  name="arrow-down"
                  tooltip={`Move ${title.toLowerCase()} down`}
                  disabled={index === fallback.length - 1}
                  onClick={() => move(index, index + 1)}
                />
                <IconButton
                  name="trash-alt"
                  tooltip={`Remove ${title.toLowerCase()}`}
                  onClick={() => setFallback(fallback.filter((_, i) => i !== index))}
                />
              </>
            }
          />
        );
      })}
      <div>
        <Button
          variant="secondary"
          size="sm"
          icon="plus"
          disabled={fallback.length >= MAX_FALLBACKS}
          tooltip={
            fallback.length >= MAX_FALLBACKS ? `A model can have at most ${MAX_FALLBACKS} fallbacks.` : undefined
          }
          onClick={() => setFallback([...fallback, { provider: '', model: '' }])}
        >
          Add fallback
        </Button>
      </div>
    </Stack>
  );
}
