import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  LucideBookText,
  LucideFolderOpen,
  LucideLogs,
  LucideSettings,
  LucideTextSearch,
} from 'lucide-react';

import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { useSecondPathName } from '@/hooks/route-hook';
import { cn, formatBytes } from '@/lib/utils';
import { Routes } from '@/routes';
import { formatPureDate } from '@/utils/date';
import { KnowledgeAccessPaths } from '@/utils/knowledge-access-navigation';

import { IDataset } from '@/interfaces/database/dataset';
import { useParams } from 'react-router';

type PropType = {
  refreshCount?: number;
  dataset: IDataset;
};

export function SideBar({ dataset: data }: PropType) {
  const pathName = useSecondPathName();
  const { id } = useParams();
  const { t } = useTranslation();
  const [navigationOpen, setNavigationOpen] = useState(false);

  const items = useMemo(() => {
    const list = [
      {
        icon: <LucideBookText className="size-[1em]" />,
        label: t('knowledgeCatalogue.title'),
        key: Routes.KnowledgeCatalogue,
      },
      {
        icon: <LucideBookText className="size-[1em]" />,
        label: t('knowledgeAccess.ownerTitle'),
        key: KnowledgeAccessPaths.ownerKey,
      },
      {
        icon: <LucideFolderOpen className="size-[1em]" />,
        label: t(`knowledgeDetails.subbarFiles`),
        key: Routes.Files,
      },
      {
        icon: <LucideTextSearch className="size-[1em]" />,
        label: t(`knowledgeDetails.testing`),
        key: Routes.DatasetTesting,
      },
      {
        icon: <LucideLogs className="size-[1em]" />,
        label: t(`knowledgeDetails.overview`),
        key: Routes.DataSetOverview,
      },
      {
        icon: <LucideSettings className="size-[1em]" />,
        label: t(`knowledgeDetails.configuration`),
        key: Routes.DataSetSetting,
      },
      {
        icon: <LucideBookText className="size-[1em]" />,
        label: 'Artifacts',
        key: Routes.Compilation,
      },
    ];

    return list;
  }, [t]);

  return (
    <Collapsible asChild open={navigationOpen} onOpenChange={setNavigationOpen}>
      <aside className="relative flex min-h-0 min-w-0 flex-col xl:w-64">
        <CollapsibleTrigger asChild>
          <Button
            variant="outline"
            className="mx-2 mb-2 min-w-0 justify-start xl:hidden"
          >
            <LucideFolderOpen aria-hidden="true" />
            <span className="min-w-0 truncate">
              {t('header.knowledgeBase')}: {data.name}
            </span>
            <span>
              {t(
                navigationOpen
                  ? 'knowledgeDocument.collapse'
                  : 'knowledgeDocument.expand',
              )}
            </span>
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent
          forceMount
          className="min-h-0 max-h-[45dvh] overflow-auto data-[state=closed]:hidden xl:max-h-none xl:data-[state=closed]:block"
        >
          <header
            className="px-5 pb-4 grid grid-cols-[auto_minmax(0,1fr)] grid-rows-[auto_auto] gap-x-3"
            style={{
              gridTemplateAreas: '"avatar title" "avatar stats"',
            }}
          >
            <RAGFlowAvatar
              avatar={data.avatar}
              name={data.name}
              className="size-16"
              style={{ gridArea: 'avatar' }}
            />

            <h3
              className="text-lg font-semibold line-clamp-1 text-text-primary text-ellipsis overflow-hidden"
              style={{ gridArea: 'title' }}
            >
              {data.name}
            </h3>

            <div
              className="self-end text-text-secondary text-xs overflow-hidden"
              style={{ gridArea: 'stats' }}
            >
              <div className="flex justify-between">
                <span>
                  {data.document_count} {t('knowledgeDetails.files')}
                </span>
                <span>{data.size ? formatBytes(data.size) : ''}</span>
              </div>

              <div className="mt-0.5">
                {t('knowledgeDetails.created')}{' '}
                {formatPureDate(data.create_time)}
              </div>
            </div>
          </header>

          <nav
            aria-label={t('header.knowledgeBase')}
            className="px-5 pt-1 pb-5 overflow-y-auto"
          >
            <ul className="space-y-5">
              {items.map((item) => {
                const active = '/' + pathName === item.key;

                return (
                  <li key={item.key}>
                    <Button
                      asLink
                      block
                      variant="ghost"
                      className={cn(
                        'justify-start gap-2.5 px-3 relative h-10 text-base',
                        active && 'bg-bg-card text-text-primary',
                      )}
                      to={`${Routes.DatasetBase}${item.key}/${id}`}
                    >
                      {item.icon}
                      <span>{item.label}</span>
                    </Button>
                  </li>
                );
              })}
            </ul>
          </nav>
        </CollapsibleContent>
      </aside>
    </Collapsible>
  );
}
