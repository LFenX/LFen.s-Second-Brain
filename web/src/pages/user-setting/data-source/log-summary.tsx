import { RunningStatus, RunningStatusOld } from '@/constants/knowledge';
import { IDataSourceLog } from './interface';

export function dataSourceLogStatus(
  status: IDataSourceLog['status'],
): RunningStatus {
  switch (status) {
    case RunningStatusOld.UNSTART:
      return RunningStatus.UNSTART;
    case RunningStatusOld.RUNNING:
      return RunningStatus.RUNNING;
    case RunningStatusOld.CANCEL:
      return RunningStatus.CANCEL;
    case RunningStatusOld.DONE:
      return RunningStatus.DONE;
    case RunningStatusOld.FAIL:
      return RunningStatus.FAIL;
    case RunningStatusOld.SCHEDULE:
      return RunningStatus.SCHEDULE;
    default:
      return status;
  }
}

export const formatDuration = (seconds: number) => {
  const safeSeconds = Math.max(0, seconds);
  const hours = Math.floor(safeSeconds / 3600);
  const minutes = Math.floor((safeSeconds % 3600) / 60);
  const remainingSeconds = safeSeconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m ${remainingSeconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${remainingSeconds}s`;
  }
  return `${remainingSeconds}s`;
};

export const getTaskCountdownSeconds = (row: IDataSourceLog, now: number) => {
  const freqMinutes =
    row.task_type === 'prune'
      ? Number(row.prune_freq || 0)
      : Number(row.refresh_freq || 0);
  const scheduledAt = row.time_started
    ? new Date(row.time_started).getTime()
    : 0;

  if (!freqMinutes || !scheduledAt) {
    return null;
  }

  const nextRunAt = scheduledAt + freqMinutes * 60 * 1000;
  return Math.ceil((nextRunAt - now) / 1000);
};

export const TaskCountdown = ({
  row,
  now,
}: {
  row: IDataSourceLog;
  now: number;
}) => {
  const remainingSeconds = getTaskCountdownSeconds(row, now);

  if (remainingSeconds === null) {
    return '';
  }

  return (
    <span className="tabular-nums">
      Task starts in {formatDuration(remainingSeconds)}
    </span>
  );
};

export const getSummary = (row: IDataSourceLog, now: number) => {
  const status = dataSourceLogStatus(row.status);
  if (status === RunningStatus.SCHEDULE) {
    return <TaskCountdown row={row} now={now} />;
  }

  if (status === RunningStatus.RUNNING) {
    return row.task_type === 'prune' ? 'Prune in progress' : 'Sync in progress';
  }

  if (status === RunningStatus.FAIL) {
    return row.error_msg || 'Task failed';
  }

  if (status === RunningStatus.CANCEL) {
    return '';
  }

  if (row.task_type === 'prune') {
    return `deleted=${row.docs_removed_from_index || 0}, error=${row.error_count || 0}`;
  }

  return `total=${row.total_docs_indexed || 0}, added=${row.new_docs_indexed || 0}, updated=${Math.max(
    0,
    (row.total_docs_indexed || 0) - (row.new_docs_indexed || 0),
  )}, error=${row.error_count || 0}`;
};
