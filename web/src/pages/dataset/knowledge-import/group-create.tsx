import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useRootProjectCreate } from '@/hooks/use-knowledge-foundation-entry';
import {
  getKnowledgeSessionScope,
  useKnowledgeDomain,
} from '@/hooks/use-knowledge-foundation';
import type {
  ComponentSide,
  KnowledgeGroup,
} from '@/interfaces/database/knowledge-foundation';

const Sides: ComponentSide[] = [
  'frontend',
  'backend',
  'shared',
  'cli',
  'non_code',
];
interface GroupCreateProps {
  domainId: string;
  disabled?: boolean;
  createDisabled?: boolean;
  groups: KnowledgeGroup[];
  onSelect: (group: KnowledgeGroup) => void;
  onLockChange: (locked: boolean) => void;
  onRefresh: () => void;
  onMore: () => void;
  more: boolean;
  reading: boolean;
}

function RecoveryChoice({
  group,
  disabled,
  onCheck,
}: {
  group: KnowledgeGroup;
  disabled: boolean;
  onCheck: (id: string) => void;
}) {
  const { t } = useTranslation();
  const handleCheck = () => onCheck(group.id);
  return (
    <li className="space-y-1 rounded-md border border-border-button p-2">
      <p>
        {group.name} · {group.id.slice(-8)}
      </p>
      <details className="text-xs text-text-secondary">
        <summary>{t('groupCreate.identity')}</summary>
        <code>{group.id}</code>
      </details>
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        onClick={handleCheck}
      >
        {t('groupCreate.checkChoice')}
      </Button>
    </li>
  );
}

function GroupCreatePanel({
  domainId,
  disabled = false,
  createDisabled = false,
  groups,
  onSelect,
  onLockChange,
  onRefresh,
  onMore,
  more,
  reading,
}: GroupCreateProps) {
  const { t } = useTranslation();
  const domain = useKnowledgeDomain(domainId);
  const ownerId =
    domain.data?.domain_id === domainId ? domain.data.owner_id : '';
  const userLoading = !domain.isSuccess || domain.isFetching;
  const container = useRef<HTMLElement>(null);
  const canCreate = () => {
    if (disabled || createDisabled) return false;
    let parent = container.current?.parentElement;
    while (parent) {
      if (parent instanceof HTMLFieldSetElement && parent.disabled)
        return false;
      parent = parent.parentElement;
    }
    return true;
  };
  const flow = useRootProjectCreate(ownerId, domainId, canCreate);
  const [name, setName] = useState('');
  const [side, setSide] = useState<ComponentSide>('shared');
  const epoch = getKnowledgeSessionScope();
  useLayoutEffect(() => {
    setName('');
    setSide('shared');
  }, [ownerId, domainId, epoch]);
  const detachedForm = useId();
  useEffect(() => {
    onLockChange(flow.locked);
  }, [flow.locked, onLockChange]);
  useEffect(() => () => onLockChange(false), [onLockChange]);
  const handleName = (event: ChangeEvent<HTMLInputElement>) =>
    setName(event.target.value);
  const handleSide = (event: ChangeEvent<HTMLSelectElement>) =>
    setSide(event.target.value as ComponentSide);
  const handleEnter = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.stopPropagation();
      if (!event.nativeEvent.isComposing) event.preventDefault();
    }
  };
  const handleCreate = () => {
    if (!canCreate() || flow.locked || flow.receipt || !ownerId || userLoading)
      return;
    onLockChange(true);
    void flow.create(name, side);
  };
  const handleSelect = () => {
    if (!disabled && (flow.origin === 'existing' || canCreate()))
      void flow.select(onSelect);
  };
  const handleCheck = (id: string) => {
    if (!disabled) void flow.reconcile(id);
  };
  return (
    <section
      ref={container}
      className="mt-3 space-y-3"
      aria-label={t('groupCreate.title')}
    >
      <p className="text-sm text-text-secondary">{t('groupCreate.hint')}</p>
      {!ownerId || userLoading ? (
        <p role="status">{t('knowledgeDomain.identityPending')}</p>
      ) : (
        <>
          <fieldset
            disabled={
              disabled || createDisabled || flow.locked || !!flow.receipt
            }
            className="space-y-2"
          >
            <label className="block space-y-1">
              <span>{t('groupCreate.name')}</span>
              <Input
                form={detachedForm}
                value={name}
                onChange={handleName}
                onKeyDown={handleEnter}
              />
            </label>
            <label className="block space-y-1">
              <span>{t('knowledgeImport.componentSide')}</span>
              <select
                form={detachedForm}
                value={side}
                onChange={handleSide}
                className="block w-full rounded-md border border-border-button bg-bg-base p-2"
              >
                {Sides.map((value) => (
                  <option key={value} value={value}>
                    {t(`knowledgeCatalogue.${value}`)}
                  </option>
                ))}
              </select>
            </label>
            <Button type="button" onClick={handleCreate}>
              {t('groupCreate.create')}
            </Button>
          </fieldset>
          {flow.busy && <p role="status">{t('knowledgeDomain.working')}</p>}
          {flow.issue && (
            <div role="alert" className="space-y-1">
              <p>
                {t(
                  flow.issue.kind === 'unknown'
                    ? 'groupCreate.unknown'
                    : 'knowledgeDomain.rejected',
                )}
              </p>
              <details className="text-xs text-text-secondary">
                <summary>{t('knowledgeDomain.details')}</summary>
                <p>
                  {flow.issue.code} {flow.issue.requestId}
                </p>
              </details>
            </div>
          )}
          {flow.attempt && !flow.receipt && (
            <div className="space-y-2">
              <p className="text-sm">{t('groupCreate.recoveryHint')}</p>
              <Button
                type="button"
                variant="outline"
                disabled={disabled || flow.busy || reading}
                onClick={onRefresh}
              >
                {t('knowledgeDomain.refresh')}
              </Button>
              <ul className="space-y-2">
                {groups
                  .filter(
                    (group) =>
                      group.kind === 'project' &&
                      group.parent_id === null &&
                      group.domain_id === domainId,
                  )
                  .map((group) => (
                    <RecoveryChoice
                      key={group.id}
                      group={group}
                      disabled={disabled || flow.busy}
                      onCheck={handleCheck}
                    />
                  ))}
              </ul>
              {more && (
                <Button
                  type="button"
                  variant="outline"
                  disabled={disabled || flow.busy || reading}
                  onClick={onMore}
                >
                  {t('knowledgeDomain.more')}
                </Button>
              )}
            </div>
          )}
          {flow.receipt && (
            <div className="space-y-2">
              <p role="status">
                {t(
                  flow.origin === 'created'
                    ? 'groupCreate.created'
                    : 'groupCreate.found',
                )}
                : {flow.receipt.name}
              </p>
              <details className="text-xs text-text-secondary">
                <summary>{t('groupCreate.identity')}</summary>
                <code>{flow.receipt.id}</code>
              </details>
              <Button
                type="button"
                disabled={
                  disabled ||
                  flow.busy ||
                  (createDisabled && flow.origin === 'created')
                }
                onClick={handleSelect}
              >
                {t('groupCreate.select')}
              </Button>
            </div>
          )}
        </>
      )}
    </section>
  );
}

export function GroupCreate(props: GroupCreateProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [locked, setLocked] = useState(false);
  const handleToggle = () => {
    if (!locked) {
      setOpen(!open);
    }
  };
  const parentLockChange = props.onLockChange;
  const handleLockChange = useCallback(
    (next: boolean) => {
      setLocked(next);
      parentLockChange(next);
    },
    [parentLockChange],
  );
  return (
    <div className="rounded-md border border-border-button p-2">
      <Button
        type="button"
        variant="outline"
        disabled={props.disabled || props.createDisabled || locked}
        onClick={handleToggle}
        aria-expanded={open}
      >
        {t('groupCreate.title')}
      </Button>
      {open && (
        <div>
          <GroupCreatePanel {...props} onLockChange={handleLockChange} />
        </div>
      )}
    </div>
  );
}
