import { TimelineNode } from '@/components/originui/timeline';
import { cn } from '@/lib/utils';
import classNames from 'classnames';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import FormatPreserEditor from './components/parse-editer';
import { TimelineNodeType } from './constant';
import { useChangeChunkTextMode } from './hooks';
import { IChunk, IDslComponent } from './interface';
import { editorValue } from './components/parse-editer/value';
import type { FormatPreserveEditorProps } from './components/parse-editer/interface';
interface IProps {
  isReadonly: boolean;
  step?: TimelineNode;
  data: { value: IDslComponent; key: string };
  clickChunk: (chunk: IChunk) => void;
  summaryInfo: string;
}
const ParserContainer = (props: IProps) => {
  const { step, data, clickChunk, isReadonly, summaryInfo } = props;
  const { t } = useTranslation();
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  const [newChunkIndex, setNewChunkIndex] = useState<number | undefined>();
  const { textMode } = useChangeChunkTextMode();
  const initialValue = useMemo(() => {
    return editorValue(data.value.obj.params);
  }, [data]);

  const [initialText, setInitialText] = useState(initialValue);

  useEffect(() => {
    setInitialText(initialValue);
  }, [initialValue]);
  const handleSave = (
    newContent: FormatPreserveEditorProps['initialValue'],
  ) => {
    console.log('newContent-change-->', newContent, initialValue);
    if (JSON.stringify(newContent) !== JSON.stringify(initialValue)) {
      setInitialText(newContent);
    }
    // Here, the API is called to send newContent to the backend
  };

  const handleCheckboxClick = useCallback(
    (id: string | number, checked: boolean) => {
      setSelectedChunkIds((prev) => {
        if (checked) {
          return [...prev, id.toString()];
        } else {
          return prev.filter((item) => item.toString() !== id.toString());
        }
      });
    },
    [],
  );

  const isChunck =
    step?.type === TimelineNodeType.tokenChunker ||
    step?.type === TimelineNodeType.titleChunker;

  useEffect(() => {
    if (newChunkIndex === undefined) return;
    const timer = setTimeout(() => setNewChunkIndex(undefined), 3000);
    return () => clearTimeout(timer);
  }, [newChunkIndex]);

  return (
    <>
      <div className={classNames('flex flex-col w-full')}>
        {/* <Spin spinning={false} className="" size="large"> */}
        <div className="h-[50px] flex flex-col justify-end pb-[5px]">
          {!isChunck && (
            <div>
              <h2 className="text-[16px]">
                {t('dataflowParser.parseSummary')}
              </h2>
              <div className="text-[12px] text-text-secondary italic ">
                {/* {t('dataflowParser.parseSummaryTip')} */}
                {summaryInfo}
              </div>
            </div>
          )}
          {isChunck && (
            <div>
              <h2 className="text-[16px]">{t('dataflowParser.result')}</h2>
              <div className="text-[12px] text-text-secondary italic">
                {/* {t('chunk.chunkResultTip')} */}
              </div>
            </div>
          )}
        </div>

        <div
          className={cn(
            ' border rounded-lg p-[20px] box-border w-[calc(100%-20px)] overflow-auto scrollbar-auto',
            {
              'h-[calc(100vh-240px)]': isChunck,
              'h-[calc(100vh-180px)]': !isChunck,
            },
          )}
        >
          {initialText && (
            <FormatPreserEditor
              initialValue={initialText}
              onSave={handleSave}
              isReadonly={isReadonly}
              isChunck={isChunck}
              textMode={textMode}
              isDelete={
                step?.type === TimelineNodeType.tokenChunker ||
                step?.type === TimelineNodeType.titleChunker
              }
              clickChunk={clickChunk}
              handleCheckboxClick={handleCheckboxClick}
              selectedChunkIds={selectedChunkIds}
              newChunkIndex={newChunkIndex}
            />
          )}
        </div>
        {/* </Spin> */}
      </div>
    </>
  );
};
export default ParserContainer;
