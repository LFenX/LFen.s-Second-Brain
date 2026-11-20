import {
  MaxActionsPerAnswer,
  SettableModelType,
  SettableModelTypes,
} from './constants';

/*
 * The actions 波西米亚 may offer under an answer. The model writes each one as
 * a fenced block tagged `bohemia-action` holding one JSON object; the panel
 * turns valid ones into cards. Navigation runs on a click; every other action
 * waits for an explicit confirmation and then runs as the signed-in person
 * through the existing APIs, so it can do nothing that person cannot.
 */

export type NavigateAction = { type: 'navigate'; to: string; label?: string };
export type ConnectDatasetsAction = {
  type: 'connect_datasets';
  add: string[];
  remove: string[];
};
export type CreateDatasetAction = { type: 'create_dataset'; name: string };
export type SetDefaultModelAction = {
  type: 'set_default_model';
  model_type: SettableModelType;
  model_id: string;
};
export type AssistantAction =
  | NavigateAction
  | ConnectDatasetsAction
  | CreateDatasetAction
  | SetDefaultModelAction;

export const ActionFence = 'bohemia-action';
const BlockPattern = /```[ \t]*bohemia-action[ \t]*\r?\n([\s\S]*?)```/g;
/** A block still being written at the end of a streaming answer. */
const OpenBlockPattern = /```[ \t]*bohemia-action[\s\S]*$/;
/** A fence that has only begun to arrive, e.g. "``" or "```bohem". */
const PartialFencePattern = /`{1,3}[a-z-]*$/;

const MaxNameLength = 128;
const MaxIds = 20;

/** Whether `path` is an in-app page under one of the allowed prefixes. */
export function isNavigablePath(path: string, prefixes: string[]) {
  if (!path.startsWith('/') || path.startsWith('//') || path.includes('\\')) {
    return false;
  }
  const control = [...path].some((char) => char.charCodeAt(0) < 0x20);
  if (control || /^\/[a-z]+:/i.test(path)) return false;
  const pathname = path.split(/[?#]/)[0];
  return prefixes.some((prefix) =>
    prefix === '/'
      ? pathname === '/'
      : pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}

const ids = (value: unknown): string[] | undefined => {
  if (value === undefined) return [];
  if (!Array.isArray(value) || value.length > MaxIds) return undefined;
  const list = value.filter(
    (item): item is string =>
      typeof item === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(item),
  );
  return list.length === value.length ? [...new Set(list)] : undefined;
};

/** One parsed JSON object as an action, or undefined when it is not one. */
export function toAction(
  value: unknown,
  navigable: string[],
): AssistantAction | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return;
  const item = value as Record<string, unknown>;
  switch (item.type) {
    case 'navigate': {
      if (typeof item.to !== 'string' || !isNavigablePath(item.to, navigable))
        return;
      const label =
        typeof item.label === 'string' && item.label.trim()
          ? item.label.trim().slice(0, 40)
          : undefined;
      return { type: 'navigate', to: item.to, label };
    }
    case 'connect_datasets': {
      const add = ids(item.add);
      const remove = ids(item.remove);
      if (!add || !remove || add.length + remove.length === 0) return;
      return { type: 'connect_datasets', add, remove };
    }
    case 'create_dataset': {
      const name = typeof item.name === 'string' ? item.name.trim() : '';
      if (!name || [...name].length > MaxNameLength) return;
      return { type: 'create_dataset', name };
    }
    case 'set_default_model': {
      const modelType = item.model_type as SettableModelType;
      const modelId = typeof item.model_id === 'string' ? item.model_id : '';
      if (!SettableModelTypes.includes(modelType)) return;
      if (!modelId || modelId.length > 256) return;
      return {
        type: 'set_default_model',
        model_type: modelType,
        model_id: modelId,
      };
    }
  }
}

export interface SplitAnswer {
  /** What the reader sees: the answer without any action block. */
  text: string;
  actions: AssistantAction[];
}

/**
 * Separates the action blocks from the words. While an answer is still
 * streaming, an unfinished block (or the start of its fence) is held back
 * rather than flashing as code.
 */
export function splitAnswer(
  answer: string,
  navigable: string[],
  streaming = false,
): SplitAnswer {
  const actions: AssistantAction[] = [];
  const seen = new Set<string>();
  let text = answer.replace(BlockPattern, (_, body: string) => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(body.trim());
    } catch {
      return '';
    }
    const action = toAction(parsed, navigable);
    const key = action && JSON.stringify(action);
    if (
      action &&
      key &&
      !seen.has(key) &&
      actions.length < MaxActionsPerAnswer
    ) {
      seen.add(key);
      actions.push(action);
    }
    return '';
  });
  text = text.replace(OpenBlockPattern, '');
  if (streaming) text = text.replace(PartialFencePattern, '');
  return { text: text.replace(/\n{3,}/g, '\n\n').trimEnd(), actions };
}

/** The answer as earlier history for the model: words and actions, no citations. */
export function answerForHistory(answer: string) {
  return answer
    .replace(/<think>[\s\S]*?<\/think>/g, '')
    .replace(/\[ID:\s*\d+\]/g, '')
    .trim();
}
