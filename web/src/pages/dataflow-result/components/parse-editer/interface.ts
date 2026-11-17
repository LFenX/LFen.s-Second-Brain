import { CheckedState } from '@radix-ui/react-checkbox';
import { ChunkTextMode } from '../../constant';
import { ComponentParams, IChunk } from '../../interface';
import { parserKeyMap } from './json-parser';

export interface FormatPreserveEditorProps {
  initialValue:
    | IJsonContainerProps['initialValue']
    | IObjContainerProps['initialValue'];
  onSave: (value: FormatPreserveEditorProps['initialValue']) => void;
  className?: string;
  isSelect?: boolean;
  isDelete?: boolean;
  isChunck?: boolean;
  handleCheckboxClick?: (id: string | number, checked: boolean) => void;
  selectedChunkIds?: string[];
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
  newChunkIndex?: number;
}

export type IJsonContainerProps = {
  initialValue: {
    key: keyof typeof parserKeyMap;
    type: string;
    value: {
      [key: string]: unknown;
    }[];
    params: ComponentParams;
  };
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  selectedChunkIds: string[] | undefined;
  unescapeNewlines: (text: string) => string;
  escapeNewlines: (text: string) => string;
  onSave: (data: IJsonContainerProps['initialValue']) => void;
  className?: string;
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
  newChunkIndex?: number;
};

export type IObjContainerProps = {
  initialValue: {
    key: 'text' | 'html' | 'markdown';
    type: string;
    value: string;
    params: ComponentParams;
  };
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  unescapeNewlines: (text: string) => string;
  escapeNewlines: (text: string) => string;
  onSave: (data: IObjContainerProps['initialValue']) => void;
  className?: string;
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
};
