import {
  answerForHistory,
  isNavigablePath,
  splitAnswer,
  toAction,
} from '../actions';

const Pages = ['/', '/datasets', '/dataset', '/user-setting', '/chat'];

const block = (body: string) => '```bohemia-action\n' + body + '\n```';

describe('isNavigablePath', () => {
  it('accepts in-app pages under the allowed prefixes', () => {
    expect(isNavigablePath('/datasets', Pages)).toBe(true);
    expect(isNavigablePath('/user-setting/model', Pages)).toBe(true);
    expect(isNavigablePath('/dataset//files/abc?tab=1', Pages)).toBe(true);
    expect(isNavigablePath('/', Pages)).toBe(true);
  });

  it('refuses other sites, schemes and unknown pages', () => {
    expect(isNavigablePath('https://example.com', Pages)).toBe(false);
    expect(isNavigablePath('//example.com/datasets', Pages)).toBe(false);
    expect(isNavigablePath('/javascript:alert(1)', Pages)).toBe(false);
    expect(isNavigablePath('/datasetsx', Pages)).toBe(false);
    expect(isNavigablePath('/chats', ['/chat'])).toBe(false);
    expect(isNavigablePath('/admin', Pages)).toBe(false);
    expect(isNavigablePath('/datasets\n', Pages)).toBe(false);
  });
});

describe('toAction', () => {
  it('keeps the four action types with valid fields', () => {
    expect(
      toAction({ type: 'navigate', to: '/datasets', label: ' 打开 ' }, Pages),
    ).toEqual({
      type: 'navigate',
      to: '/datasets',
      label: '打开',
    });
    expect(
      toAction({ type: 'connect_datasets', add: ['a1', 'a1'] }, Pages),
    ).toEqual({
      type: 'connect_datasets',
      add: ['a1'],
      remove: [],
    });
    expect(
      toAction({ type: 'create_dataset', name: ' 读书笔记 ' }, Pages),
    ).toEqual({
      type: 'create_dataset',
      name: '读书笔记',
    });
    expect(
      toAction(
        { type: 'set_default_model', model_type: 'chat', model_id: 'm@i@p' },
        Pages,
      ),
    ).toEqual({
      type: 'set_default_model',
      model_type: 'chat',
      model_id: 'm@i@p',
    });
  });

  it('drops anything outside the whitelist', () => {
    expect(
      toAction({ type: 'delete_dataset', id: 'x' }, Pages),
    ).toBeUndefined();
    expect(
      toAction({ type: 'navigate', to: 'https://x.y' }, Pages),
    ).toBeUndefined();
    expect(
      toAction({ type: 'connect_datasets', add: [] }, Pages),
    ).toBeUndefined();
    expect(
      toAction({ type: 'connect_datasets', add: ['a b'] }, Pages),
    ).toBeUndefined();
    expect(
      toAction({ type: 'create_dataset', name: '' }, Pages),
    ).toBeUndefined();
    expect(
      toAction(
        { type: 'set_default_model', model_type: 'tts', model_id: 'x' },
        Pages,
      ),
    ).toBeUndefined();
    expect(toAction(['navigate'], Pages)).toBeUndefined();
  });
});

describe('splitAnswer', () => {
  it('takes the action blocks out of the words', () => {
    const answer = [
      '去模型页设置默认模型。',
      block(
        '{"type":"navigate","to":"/user-setting/model","label":"打开模型设置"}',
      ),
      block('{"type":"create_dataset","name":"读书笔记"}'),
    ].join('\n\n');
    const { text, actions } = splitAnswer(answer, Pages);
    expect(text).toBe('去模型页设置默认模型。');
    expect(actions.map((action) => action.type)).toEqual([
      'navigate',
      'create_dataset',
    ]);
  });

  it('ignores broken or repeated blocks and keeps at most three', () => {
    const go = block('{"type":"navigate","to":"/datasets"}');
    const answer = [
      'x',
      block('not json'),
      go,
      go,
      block('{"type":"navigate","to":"/chat/1"}'),
      block('{"type":"navigate","to":"/user-setting"}'),
      block('{"type":"navigate","to":"/"}'),
    ].join('\n');
    const { text, actions } = splitAnswer(answer, Pages);
    expect(text).toBe('x');
    expect(actions).toHaveLength(3);
    expect(actions[0]).toEqual({
      type: 'navigate',
      to: '/datasets',
      label: undefined,
    });
  });

  it('holds back a block that is still being written', () => {
    expect(
      splitAnswer('好的。\n```bohemia-action\n{"type":"nav', Pages, true).text,
    ).toBe('好的。');
    expect(splitAnswer('好的。\n``', Pages, true).text).toBe('好的。');
  });
});

describe('answerForHistory', () => {
  it('drops reasoning and citation markers', () => {
    expect(
      answerForHistory('<think>想一想</think>先建库 [ID:0]，再上传 [ID:2]。'),
    ).toBe('先建库 ，再上传 。');
  });
});
