export const MCPClientConfigKinds = ['json', 'claudeCode'] as const;
export type MCPClientConfigKind = (typeof MCPClientConfigKinds)[number];

/** Client setup for a Streamable HTTP MCP endpoint behind a bearer token. */
export function mcpClientConfig(
  kind: MCPClientConfigKind,
  endpoint: string,
  token: string,
  serverName = 'lfen-second-brain',
): string {
  if (kind === 'claudeCode')
    return `claude mcp add --transport http ${serverName} ${endpoint} --header "Authorization: Bearer ${token}"`;
  return JSON.stringify(
    {
      mcpServers: {
        [serverName]: {
          type: 'http',
          url: endpoint,
          headers: { Authorization: `Bearer ${token}` },
        },
      },
    },
    null,
    2,
  );
}
