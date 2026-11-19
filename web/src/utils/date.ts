/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import dayjs from 'dayjs';

export function formatDate(date: any, format?: string) {
  const thisFormat = format || 'DD/MM/YYYY HH:mm:ss';
  if (!date) {
    return '';
  }
  return dayjs(fromServerUtc(date)).format(thisFormat);
}

/**
 * The API writes timestamps such as "2026-11-18 18:49:04" in UTC without a
 * zone marker; mark them as UTC so they display in the viewer's time zone.
 */
export function fromServerUtc<T>(value: T): T | string {
  return typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(\.\d+)?$/.test(value)
    ? value.replace(' ', 'T') + 'Z'
    : value;
}

export function formatTime(date: any) {
  if (!date) {
    return '';
  }
  return dayjs(date).format('HH:mm:ss');
}

export function today() {
  return formatDate(dayjs());
}

export function lastDay() {
  return formatDate(dayjs().subtract(1, 'days'));
}

export function lastWeek() {
  return formatDate(dayjs().subtract(1, 'weeks'));
}

export function formatPureDate(date: any) {
  if (!date) {
    return '';
  }
  return dayjs(date).format('DD/MM/YYYY');
}

export function formatStandardDate(date: any) {
  if (!date) {
    return '';
  }
  const parsedDate = dayjs(date);
  if (!parsedDate.isValid()) {
    return '';
  }
  return parsedDate.format('YYYY-MM-DD');
}

export function formatSecondsToHumanReadable(seconds: number): string {
  if (isNaN(seconds) || seconds < 0) {
    return '0s';
  }

  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  // const s = toFixed(seconds % 60, 3);
  const s = seconds % 60;
  const formattedSeconds = s === 0 ? '0' : s.toFixed(3).replace(/\.?0+$/, '');
  const parts = [];
  if (h > 0) parts.push(`${h}h`);
  if (m > 0) parts.push(`${m}m`);
  if (s || parts.length === 0) parts.push(`${formattedSeconds}s`);

  return parts.join(' ');
}
