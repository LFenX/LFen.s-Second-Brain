import { Routes } from '@/routes';
import { KnowledgeAccessPaths } from '@/utils/knowledge-access-navigation';

/** Pages an action may lead to: same-origin paths under these prefixes. */
export const NavigablePrefixes: string[] = [
  Routes.Root,
  Routes.Datasets,
  Routes.DatasetBase,
  Routes.Chats,
  Routes.Chat,
  Routes.Searches,
  Routes.Agents,
  Routes.Memories,
  Routes.Memory,
  Routes.Files,
  Routes.UserSetting,
  KnowledgeAccessPaths.shared,
];

/** The pages named in the prompt, in the words the navigation shows. */
export const NamedPages: Array<{ path: string; name: string }> = [
  { path: Routes.Root, name: '首页' },
  { path: Routes.Datasets, name: '知识库列表' },
  { path: Routes.Chats, name: '聊天列表' },
  { path: Routes.Searches, name: '搜索' },
  { path: Routes.Agents, name: '智能体' },
  { path: Routes.Memories, name: '记忆列表' },
  { path: Routes.Files, name: '文件管理' },
  { path: KnowledgeAccessPaths.shared, name: '共享给我' },
  {
    path: `${Routes.UserSetting}${Routes.Model}`,
    name: '用户设置 → 模型提供商（含设置默认模型）',
  },
  { path: `${Routes.UserSetting}${Routes.Team}`, name: '用户设置 → 团队' },
  {
    path: `${Routes.UserSetting}${Routes.Api}`,
    name: '用户设置 → API（MCP 接入、API 密钥）',
  },
  {
    path: `${Routes.UserSetting}${Routes.Mcp}`,
    name: '用户设置 → MCP（给智能体加外部 MCP 服务器）',
  },
  {
    path: `${Routes.UserSetting}${Routes.DataSource}`,
    name: '用户设置 → 数据源',
  },
  { path: `${Routes.UserSetting}/profile`, name: '用户设置 → 个人资料' },
];

export const datasetPagePath = (id: string) => `${Routes.Dataset}/${id}`;
export const chatPagePath = (id: string) => `${Routes.Chat}/${id}`;

/** Where the person is, in the names they see. */
export function describePage(pathname: string, title?: string) {
  const exact = NamedPages.find((page) => page.path === pathname);
  if (exact) return `${exact.name}（${pathname}）`;
  const rules: Array<[string, string]> = [
    [`${Routes.Dataset}/`, '某个知识库的文件页'],
    [`${Routes.DatasetBase}${Routes.DatasetTesting}/`, '某个知识库的检索测试'],
    [`${Routes.DatasetBase}${Routes.DataSetSetting}/`, '某个知识库的配置'],
    [`${Routes.DatasetBase}/knowledge-access/`, '某个知识域的共享管理'],
    [
      `${Routes.DatasetBase}${Routes.KnowledgeCatalogue}/`,
      '某个知识域的知识目录',
    ],
    [`${Routes.DatasetBase}${Routes.KnowledgeImport}/`, '某个知识域的导入页'],
    [`${Routes.Chat}/`, '某个聊天'],
    [`${Routes.Memory}`, '某个记忆的详情'],
    [`${Routes.UserSetting}`, '用户设置'],
  ];
  const rule = rules.find(([prefix]) => pathname.startsWith(prefix));
  const name = rule ? rule[1] : title || '某一页';
  return `${name}（${pathname}）`;
}
