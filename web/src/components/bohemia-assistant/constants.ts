export const BohemiaName = '波西米亚';
export const BohemiaSubtitle = '第二大脑分身';
/** The chat that answers for 波西米亚; found again by this exact name. */
export const BohemiaChatName = '波西米亚 · 第二大脑分身';
/** 波西米亚's own small knowledge base: how to use this platform. */
export const BohemiaGuideDatasetName = '波西米亚 · 平台使用说明';
export const BohemiaGuideDatasetDescription =
  '波西米亚的平台使用说明，由助手自动维护。';

export const BohemiaStorageKeys = {
  open: 'bohemia-assistant-open',
  turns: 'bohemia-assistant-turns',
  frame: 'bohemia-assistant-frame',
} as const;

/** How many earlier turns go back to the model with a new question. */
export const HistoryTurns = 6;
/** How many turns are kept for this browser tab. */
export const KeptTurns = 20;
/** Waiting for the guide files to be parsed on first open. */
export const GuideParseTimeoutMs = 4 * 60 * 1000;
export const GuideParsePollMs = 2000;
/** At most this many actions are offered under one answer. */
export const MaxActionsPerAnswer = 3;

/** The model types 波西米亚 may set as the default. */
export const SettableModelTypes = ['chat', 'embedding', 'rerank'] as const;
export type SettableModelType = (typeof SettableModelTypes)[number];
export const ModelTypeNames: Record<SettableModelType, string> = {
  chat: 'LLM',
  embedding: 'Embedding',
  rerank: 'Rerank',
};
