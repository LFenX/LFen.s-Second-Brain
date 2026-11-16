import { fireEvent, render, screen } from '@testing-library/react';
import En from '@/locales/en';
import KnowledgeDepartments from './knowledge-departments';
import {
  useAccessIdentity,
  useAccessDepartments,
  useAccessDepartmentMembers,
} from '@/hooks/use-knowledge-access';
import { useAccessCommand } from '@/hooks/use-knowledge-access-command';
import {
  Tenant,
  User,
  actor,
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
  useAccessIdentity: jest.fn(),
  useAccessDepartments: jest.fn(),
  useAccessDepartmentMembers: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-access-command', () => ({
  useAccessCommand: jest.fn(),
}));
jest.mock('@/services/knowledge-access-service', () => {
  const actual = jest.requireActual('@/services/knowledge-access-service');
  return { __esModule: true, ...actual };
});
jest.mock('@/utils/register-server', () => ({
  registerNextServer: () => ({ post: jest.fn() }),
}));
const Recipient = 'b'.repeat(32),
  Department = 'a'.repeat(32),
  api = {
    identity: jest.mocked(useAccessIdentity),
    departments: jest.mocked(useAccessDepartments),
    members: jest.mocked(useAccessDepartmentMembers),
    command: jest.mocked(useAccessCommand),
  },
  start = jest.fn();
beforeEach(() => {
  jest.clearAllMocks();
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    configurable: true,
    value: () => require('node:crypto').randomUUID(),
  });
  api.identity.mockReturnValue({
    data: {
      user: User,
      name: 'Owner',
      tenants: [{ id: Tenant, name: 'Synthetic tenant', role: 'owner' }],
    },
    session: actor().session,
    isFetching: false,
    isError: false,
    error: null,
  } as any);
  api.departments.mockReturnValue({
    data: { items: [], pagination: 'none' },
    isFetching: false,
    isError: false,
    error: null,
    refetch: jest.fn(),
  } as any);
  api.members.mockReturnValue({
    data: [
      { id: User, name: 'Owner' },
      { id: Recipient, name: 'Synthetic recipient' },
    ],
    isFetching: false,
    isError: false,
    error: null,
    refetch: jest.fn(),
  } as any);
  api.command.mockReturnValue({
    start,
    locked: false,
    busy: false,
    pending: null,
    error: null,
    completed: false,
    needsReview: false,
    storageBlocked: false,
  } as any);
  start.mockResolvedValue(true);
});

it('confirms exact tenant roster replacement and never invents an impact count', () => {
  render(<KnowledgeDepartments />);
  expect(
    screen.getByText(En.translation.knowledgeAccess.departmentRosterWarning),
  ).toBeVisible();
  expect(api.command).toHaveBeenCalledWith(
    expect.objectContaining({ user: User, tenant: Tenant }),
    'tenant:' + Tenant,
  );
  const save = screen.getByRole('button', { name: 'Save department' });
  expect(save).toBeDisabled();
  fireEvent.change(screen.getByLabelText('Department name'), {
    target: { value: 'Synthetic department A' },
  });
  fireEvent.click(screen.getByLabelText('Synthetic recipient'));
  fireEvent.click(
    screen.getByLabelText(
      'I reviewed the complete roster and confirm this change',
    ),
  );
  fireEvent.click(save);
  expect(start).toHaveBeenCalledWith({
    operation: 'setDepartment',
    body: expect.objectContaining({
      tenant_id: Tenant,
      department_id: null,
      expected_version: 0,
      name: 'Synthetic department A',
      user_ids: [Recipient],
      confirmed: true,
    }),
  });
  expect(screen.queryByText(/affected credentials|impact count/i)).toBeNull();
});

it('never asks for a roster when the native identity owns no current tenant', () => {
  api.identity.mockReturnValue({
    data: {
      user: Tenant,
      name: 'Ordinary member',
      tenants: [{ id: Tenant, name: 'Synthetic tenant', role: 'normal' }],
    },
    session: actor().session,
    isFetching: false,
    isError: false,
    error: null,
  } as any);
  render(<KnowledgeDepartments />);
  expect(
    screen.getByText('Only the current team owner can manage departments.'),
  ).toBeVisible();
  expect(api.departments).not.toHaveBeenCalled();
  expect(api.members).not.toHaveBeenCalled();
  expect(screen.queryByRole('button', { name: 'Save department' })).toBeNull();
});

it('requires explicit removal of inactive saved members and preserves the version CAS', () => {
  const stale = 'c'.repeat(32);
  api.departments.mockReturnValue({
    data: {
      items: [
        {
          department_id: Department,
          tenant_id: Tenant,
          name: 'Existing department',
          version: 2,
          enabled: true,
          user_ids: [Recipient, stale],
          member_count: 2,
        },
      ],
      pagination: 'none',
    },
    isFetching: false,
    isError: false,
    error: null,
    refetch: jest.fn(),
  } as any);
  render(<KnowledgeDepartments />);
  fireEvent.change(screen.getByLabelText('Department'), {
    target: { value: Department },
  });
  const confirm = screen.getByLabelText(
    'I reviewed the complete roster and confirm this change',
  );
  expect(confirm).toBeDisabled();
  fireEvent.click(
    screen.getByLabelText('No longer an active team member: ' + stale),
  );
  fireEvent.click(confirm);
  fireEvent.click(screen.getByRole('button', { name: 'Save department' }));
  expect(start).toHaveBeenCalledWith({
    operation: 'setDepartment',
    body: expect.objectContaining({
      department_id: Department,
      expected_version: 2,
      user_ids: [Recipient],
    }),
  });
});

it('unmounts the old roster editor when the identity loses ownership', () => {
  const view = render(<KnowledgeDepartments />);
  fireEvent.change(screen.getByLabelText('Department name'), {
    target: { value: 'Synthetic unsaved roster draft' },
  });
  api.identity.mockReturnValue({
    data: { user: Recipient, name: 'Recipient', tenants: [] },
    session: actor().session,
    isFetching: false,
    isError: false,
    error: null,
  } as any);
  view.rerender(<KnowledgeDepartments />);
  expect(
    screen.queryByDisplayValue('Synthetic unsaved roster draft'),
  ).toBeNull();
  expect(screen.queryByLabelText('Department name')).toBeNull();
});
