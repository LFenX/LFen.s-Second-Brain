import { CopyValue, Panel } from '@/components/form-fields';
import { Button } from '@/components/ui/button';
import {
  MCPClientConfigKinds,
  mcpClientConfig,
} from '@/utils/mcp-client-config';
import { KnowledgeAccessPaths } from '@/utils/knowledge-access-navigation';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

const ToolNames = [
  'ragflow_retrieval',
  'ragflow_list_datasets',
  'ragflow_list_chats',
] as const;
const KeyPlaceholder = '<API_KEY>';

/** How to reach every dataset this account can read from an MCP client. */
export function McpAccessPanel({ onManageKeys }: { onManageKeys: () => void }) {
  const { t } = useTranslation();
  const endpoint = `${window.location.origin}/api/v1/mcp`;
  return (
    <Panel
      title={t('setting.mcpAccess.title')}
      description={t('setting.mcpAccess.description')}
      actions={
        <Button variant="outline" size="sm" onClick={onManageKeys}>
          {t('setting.mcpAccess.manageKeys')}
        </Button>
      }
      className="mb-4"
    >
      <dl className="grid gap-3 text-sm sm:grid-cols-[auto_1fr] sm:gap-x-4">
        <dt className="text-text-secondary">
          {t('setting.mcpAccess.endpoint')}
        </dt>
        <dd className="min-w-0">
          <CopyValue value={endpoint} />
        </dd>
        <dt className="text-text-secondary">{t('setting.mcpAccess.tools')}</dt>
        <dd className="flex flex-wrap gap-1.5">
          {ToolNames.map((name) => (
            <code
              key={name}
              className="rounded-md bg-bg-card px-1.5 py-0.5 font-mono text-xs text-text-primary"
            >
              {name}
            </code>
          ))}
        </dd>
      </dl>
      <div className="grid gap-3 lg:grid-cols-2">
        {MCPClientConfigKinds.map((kind) => {
          const config = mcpClientConfig(kind, endpoint, KeyPlaceholder);
          return (
            <div key={kind} className="min-w-0 space-y-1.5">
              <p className="text-sm font-medium text-text-primary">
                {t(`knowledgeAccess.mcpConfig.${kind}`)}
              </p>
              <CopyValue value={config} />
            </div>
          );
        })}
      </div>
      <ul className="list-disc space-y-1 ps-5 text-sm leading-relaxed text-text-secondary">
        <li>{t('setting.mcpAccess.keyHint')}</li>
        <li>{t('setting.mcpAccess.teamHint')}</li>
        <li>
          {t('setting.mcpAccess.grantHint')}{' '}
          <Link
            to={KnowledgeAccessPaths.shared}
            className="text-accent-primary hover:underline"
          >
            {t('knowledgeAccess.sharedWithMe')}
          </Link>
          {t('setting.mcpAccess.grantHintSuffix')}
        </li>
      </ul>
    </Panel>
  );
}
