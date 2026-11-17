import type { FilterValue } from '@/components/list-filter-bar/interface';
import { render, screen } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { DatasetFilter } from '../dataset-filter';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/list-filter-bar', () => ({
  FilterButton: ({ count }: { count: number }) => (
    <output aria-label="Selected filters">{count}</output>
  ),
}));
jest.mock('@/components/list-filter-bar/filter-popover', () => ({
  FilterPopover: ({ children }: PropsWithChildren) => <>{children}</>,
}));
jest.mock('@/components/ui/input', () => ({ SearchInput: () => null }));
jest.mock('@/components/ui/segmented', () => ({ Segmented: () => null }));

it.each<{ value?: FilterValue; count: number }>([
  { value: undefined, count: 0 },
  { value: { status: ['ready'], type: [] }, count: 1 },
  {
    value: { status: ['ready'], author: { team: ['one', 'two'], person: [] } },
    count: 3,
  },
])('counts flat and grouped selected filter values', ({ value, count }) => {
  render(<DatasetFilter value={value} />);
  expect(screen.getByLabelText('Selected filters')).toHaveTextContent(
    String(count),
  );
});
