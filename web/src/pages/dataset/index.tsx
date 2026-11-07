import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { OwnerTenantIdContext } from '@/pages/agent/context';
import { KnowledgeBaseProvider } from '@/pages/dataset/contexts/knowledge-base-context';

import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  const { data, loading } = useFetchKnowledgeBaseConfiguration();

  return (
    <OwnerTenantIdContext.Provider value={data?.tenant_id ?? ''}>
      <KnowledgeBaseProvider knowledgeBase={data} loading={loading}>
        <article className="grid size-full min-h-0 min-w-0 grid-cols-1 grid-rows-[auto_minmax(0,1fr)] pt-3 xl:grid-cols-[auto_minmax(0,1fr)] xl:grid-rows-1">
          <SideBar dataset={data} />

          <Outlet />
        </article>
      </KnowledgeBaseProvider>
    </OwnerTenantIdContext.Provider>
  );
}
