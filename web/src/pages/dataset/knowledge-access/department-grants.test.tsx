import { fireEvent, render, screen } from '@testing-library/react';
import { AccessGrants } from './grants';
import { useAccessDepartments } from '@/hooks/use-knowledge-access';
import {
  actor,
  admin,
  Tenant,
} from '@/services/__tests__/knowledge-access-fixtures';
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) =>
      key
        .split('.')
        .reduce(
          (value: any, part) => value?.[part],
          require('@/locales/en').default.translation,
        ) ?? key,
  }),
}));
jest.mock('@/hooks/use-knowledge-access', () => ({
  useAccessDepartments: jest.fn(),
}));
jest.mock('@/utils/register-server', () => ({
  registerNextServer: () => ({ post: jest.fn() }),
}));
const department = 'a'.repeat(32),
  start = jest.fn(),
  query = jest.mocked(useAccessDepartments);
beforeEach(() => {
  jest.clearAllMocks();
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    configurable: true,
    value: () => require('node:crypto').randomUUID(),
  });
  query.mockReturnValue({
    data: {
      items: [
        {
          department_id: department,
          tenant_id: Tenant,
          name: 'Synthetic department A',
          version: 1,
          enabled: true,
          user_ids: null,
          member_count: null,
        },
        {
          department_id: 'b'.repeat(32),
          tenant_id: Tenant,
          name: 'Disabled department',
          version: 1,
          enabled: false,
          user_ids: [],
          member_count: 0,
        },
      ],
      pagination: 'none',
    },
    isFetching: false,
    isError: false,
    error: null,
  } as any);
});
it('loads department names only when selected and submits one exact department grant', () => {
  const module = {
    ...admin(),
    enabled: true,
    policy: { ...admin().policy, enabled: true },
  };
  render(
    <AccessGrants
      actor={actor()}
      module={module}
      command={{ start, locked: false } as any}
    />,
  );
  expect(query).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText('Grant audience'), {
    target: { value: 'department' },
  });
  expect(query).toHaveBeenCalledWith(actor());
  expect(
    screen.queryByRole('option', { name: 'Disabled department' }),
  ).toBeNull();
  fireEvent.change(screen.getByLabelText('Department'), {
    target: { value: department },
  });
  fireEvent.click(
    screen.getByLabelText(
      'I reviewed the department, fixed version, permission and expiry and confirm this grant',
    ),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'Create department grant' }),
  );
  expect(start).toHaveBeenCalledWith({
    operation: 'createGrant',
    body: expect.objectContaining({
      domain_id: module.domain_id,
      module_id: module.module_id,
      expected_module_version: module.version,
      subject: { kind: 'department', id: department },
      mode: 'read',
      delivery: 'web_only',
      confirmed: true,
    }),
  });
  expect(
    screen.queryByText(/member count|user_ids|temporary credential/i),
  ).toBeNull();
});
