import React, { ReactNode, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Button, Drawer, Field, IconButton, Input, Stack, TagsInput, Text, useStyles2 } from '@grafana/ui';
import { HealthStatus } from '../../api/modelCatalogs';
import { CatalogHealthBadge } from './CatalogHealthBadge';
import { useWrite, WriteError } from './FormModal';
import { useFieldErrors } from './useFieldErrors';
import { TargetFields } from './TargetFields';
import {
  CatalogChange,
  fallbackSuggestions,
  MAX_FALLBACKS,
  modelDraft,
  ModelDraft,
  modelErrors,
  modelFieldValues,
  modelFromDraft,
  targetField,
  targetLabel,
  withModel,
} from './catalogSpec';
import { Spec, Target } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

const MAX_ALIAS_LENGTH = 128;

interface Props {
  spec: Spec;
  id: string;
  fallbackHealth?: HealthStatus[];
  readOnly: boolean;
  onSave: (change: CatalogChange) => Promise<void>;
  onClose: () => void;
}

/**
 * ModelDrawer shows a model's details: the provider model it calls, how
 * applications request it, and its fallbacks. A writer edits and saves them
 * together, including the model's ID.
 */
export function ModelDrawer({ spec, id, fallbackHealth, readOnly, onSave, onClose }: Props) {
  const s = useStyles2(getStyles);
  const stored = spec.models[id];
  const [draft, setDraft] = useState<ModelDraft>(() => modelDraft(id, stored));
  const checks = useFieldErrors(
    modelErrors(
      draft,
      Object.keys(spec.models).filter((other) => other !== id)
    ),
    modelFieldValues(draft)
  );
  const { errors } = checks;
  const { saving, error, submit } = useWrite(async () => {
    if (checks.submit()) {
      await onSave({ spec: withModel(spec, draft.id, modelFromDraft(draft, stored), id), secure: {} });
    }
  });
  const { fallback } = draft;
  const setFallback = (next: Target[]) => setDraft({ ...draft, fallback: next });
  const move = (from: number, to: number) => {
    const next = [...fallback];
    next.splice(to, 0, ...next.splice(from, 1));
    setFallback(next);
  };
  const suggestions = readOnly || fallback.length >= MAX_FALLBACKS ? [] : fallbackSuggestions(spec, draft, id);

  return (
    <Drawer title={id} subtitle={stored.name === id ? undefined : stored.name} size="md" onClose={onClose}>
      <form
        className={s.form}
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <Section title="Calls" description="The provider model this model serves.">
          <TargetFields
            title="Primary"
            field={targetField(0)}
            target={draft.primary}
            providers={spec.providers}
            errors={errors}
            disabled={readOnly}
            onChange={(primary) => setDraft({ ...draft, primary })}
            onLeave={checks.leave}
          />
        </Section>

        <Section title="Applications" description="How applications request and list this model.">
          <Field
            label="Model ID"
            description="Applications request the model by this ID. Changing it breaks applications that use the old one."
            invalid={errors.id !== undefined}
            error={errors.id}
          >
            <Input
              id="model-id"
              value={draft.id}
              disabled={readOnly}
              onChange={(event) => setDraft({ ...draft, id: event.currentTarget.value })}
              onBlur={() => checks.leave('id')}
            />
          </Field>
          <Field label="Display name" description="Shown where applications list models. Defaults to the model ID.">
            <Input
              id="model-name"
              value={draft.name}
              placeholder={draft.id}
              disabled={readOnly}
              onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })}
            />
          </Field>
          <Field label="Description">
            <Input
              id="model-description"
              value={draft.description}
              placeholder="Optional"
              disabled={readOnly}
              onChange={(event) => setDraft({ ...draft, description: event.currentTarget.value })}
            />
          </Field>
          <Field
            label="Aliases"
            description="Other IDs applications can request the model by."
            invalid={errors.aliases !== undefined}
            error={errors.aliases}
          >
            <TagsInput
              id="model-aliases"
              tags={draft.aliases}
              placeholder="Add an alias and press Enter"
              maxLength={MAX_ALIAS_LENGTH}
              disabled={readOnly}
              addOnBlur
              onChange={(aliases) => {
                setDraft({ ...draft, aliases });
                checks.leave('aliases');
              }}
            />
          </Field>
        </Section>

        <Section title="Fallbacks" description="If a call fails, the Gateway tries each fallback in order.">
          <Stack direction="column" gap={2}>
            {fallback.length === 0 && <Text color="secondary">No fallbacks.</Text>}
            {fallback.map((target, index) => {
              const title = `Fallback ${index + 1}`;
              const saved = stored.fallback?.[index];
              const status =
                saved?.provider === target.provider && saved.model === target.model ? fallbackHealth?.[index] : undefined;
              return (
                <TargetFields
                  key={index}
                  title={title}
                  field={targetField(index + 1)}
                  target={target}
                  providers={spec.providers}
                  errors={errors}
                  disabled={readOnly}
                  onChange={(next) => setFallback(fallback.map((current, i) => (i === index ? next : current)))}
                  onLeave={checks.leave}
                  actions={
                    <>
                      <CatalogHealthBadge status={status} />
                      {!readOnly && (
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
                      )}
                    </>
                  }
                />
              );
            })}
            {!readOnly && (
              <Stack gap={1} wrap="wrap">
                {suggestions.map((target) => (
                  <Button
                    key={`${target.provider}/${target.model}`}
                    variant="secondary"
                    size="sm"
                    icon="plus"
                    onClick={() => setFallback([...fallback, target])}
                  >
                    {targetLabel(target)}
                  </Button>
                ))}
                <Button
                  variant="secondary"
                  fill="outline"
                  size="sm"
                  icon="plus"
                  disabled={fallback.length >= MAX_FALLBACKS}
                  tooltip={
                    fallback.length >= MAX_FALLBACKS
                      ? `A model can have at most ${MAX_FALLBACKS} fallbacks.`
                      : undefined
                  }
                  onClick={() => setFallback([...fallback, { provider: '', model: '' }])}
                >
                  Another provider model
                </Button>
              </Stack>
            )}
          </Stack>
        </Section>

        <WriteError error={error} />
        {!readOnly && (
          <Stack gap={1}>
            <Button type="submit" disabled={saving}>
              {saving ? 'Saving...' : 'Save model'}
            </Button>
            <Button variant="secondary" fill="outline" onClick={onClose}>
              Cancel
            </Button>
          </Stack>
        )}
      </form>
    </Drawer>
  );
}

function Section({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <section>
      <Stack direction="column" gap={1}>
        <div>
          <Text element="h3" variant="h5">
            {title}
          </Text>
          <Text color="secondary" variant="bodySmall">
            {description}
          </Text>
        </div>
        <div>{children}</div>
      </Stack>
    </section>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  form: css({
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(3),
  }),
});
