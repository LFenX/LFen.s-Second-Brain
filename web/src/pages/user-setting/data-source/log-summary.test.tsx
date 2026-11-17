import { RunningStatus, RunningStatusOld } from '@/constants/knowledge';
import { render, screen } from '@testing-library/react';
import type { IDataSourceLog } from './interface';
import { dataSourceLogStatus, getSummary } from './log-summary';

function row(status: IDataSourceLog['status']): IDataSourceLog {
  return {
    connector_id: 'connector',
    error_count: 1,
    error_msg: 'Sync failed',
    id: 'task',
    kb_id: 'dataset',
    kb_name: 'Dataset',
    new_docs_indexed: 2,
    status,
    update_date: '',
    task_type: 'sync',
    total_docs_indexed: 5,
    time_started: '2026-11-18T00:00:00Z',
    refresh_freq: 2,
  };
}

it.each([
  [RunningStatusOld.UNSTART, RunningStatus.UNSTART],
  [RunningStatusOld.RUNNING, RunningStatus.RUNNING],
  [RunningStatusOld.CANCEL, RunningStatus.CANCEL],
  [RunningStatusOld.DONE, RunningStatus.DONE],
  [RunningStatusOld.FAIL, RunningStatus.FAIL],
  [RunningStatusOld.SCHEDULE, RunningStatus.SCHEDULE],
])(
  'interprets Go status %s as %s for both the badge and summary',
  (wire, canonical) => {
    expect(dataSourceLogStatus(wire)).toBe(canonical);
    expect(dataSourceLogStatus(canonical)).toBe(canonical);
    const wireView = render(<>{getSummary(row(wire), 0)}</>);
    const canonicalView = render(<>{getSummary(row(canonical), 0)}</>);
    expect(wireView.container.textContent).toBe(
      canonicalView.container.textContent,
    );
  },
);

it('renders the numeric scheduled status as a countdown instead of completed totals', () => {
  render(
    <>
      {getSummary(
        row(RunningStatusOld.SCHEDULE),
        Date.parse('2026-11-18T00:01:00Z'),
      )}
    </>,
  );
  expect(screen.getByText('Task starts in 1m 0s')).toBeTruthy();
  expect(getSummary(row(RunningStatusOld.RUNNING), 0)).toBe('Sync in progress');
  expect(getSummary(row(RunningStatusOld.FAIL), 0)).toBe('Sync failed');
  expect(getSummary(row(RunningStatusOld.CANCEL), 0)).toBe('');
});
