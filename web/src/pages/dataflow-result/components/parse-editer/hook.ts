import { useEffect, useRef, useState } from 'react';
import { IJsonContainerProps, IObjContainerProps } from './interface';

export const useParserInit = <
  T extends
    | IJsonContainerProps['initialValue']
    | IObjContainerProps['initialValue'],
>({
  initialValue,
}: {
  initialValue: T;
}) => {
  const [content, setContent] = useState(initialValue);

  useEffect(() => {
    setContent(initialValue);
    console.log('initialValue json parse', initialValue);
  }, [initialValue]);

  const [activeEditIndex, setActiveEditIndex] = useState<number | undefined>(
    undefined,
  );
  const editDivRef = useRef<HTMLDivElement>(null);

  return {
    content,
    setContent,
    activeEditIndex,
    setActiveEditIndex,
    editDivRef,
  };
};
