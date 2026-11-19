import { mcpClientConfig } from '@/utils/mcp-client-config';
import { formatAccessTime } from '../temporary-mcp';

const endpoint = 'https://kb.example.test/api/v1/knowledge/access/mcp';

describe('mcpClientConfig', () => {
  it('writes a Streamable HTTP server entry with a bearer header', () => {
    expect(JSON.parse(mcpClientConfig('json', endpoint, 'tok'))).toEqual({
      mcpServers: {
        'lfen-second-brain': {
          type: 'http',
          url: endpoint,
          headers: { Authorization: 'Bearer tok' },
        },
      },
    });
  });

  it('writes the matching Claude Code command', () => {
    expect(mcpClientConfig('claudeCode', endpoint, 'tok')).toBe(
      `claude mcp add --transport http lfen-second-brain ${endpoint} --header "Authorization: Bearer tok"`,
    );
  });
});

describe('formatAccessTime', () => {
  it('shows a readable local time and a dash when absent', () => {
    expect(formatAccessTime(null)).toBe('—');
    expect(formatAccessTime('not a time')).toBe('not a time');
    expect(formatAccessTime('2026-11-19T08:05:00')).toBe('2026-11-19 08:05');
  });
});
