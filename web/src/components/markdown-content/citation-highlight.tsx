import {
  PropsWithChildren,
  createContext,
  useContext,
  useMemo,
  useState,
} from 'react';

interface CitationHighlight {
  /** The document whose chips and source card are lit together. */
  activeDocumentId: string | null;
  setActiveDocumentId: (documentId: string | null) => void;
}

const CitationHighlightContext = createContext<CitationHighlight>({
  activeDocumentId: null,
  setActiveDocumentId: () => {},
});

/**
 * Scopes chip ↔ source-card highlighting to one message. Only the chips and
 * cards read it, so hovering never re-renders the markdown around them.
 */
export function CitationHighlightProvider({ children }: PropsWithChildren) {
  const [activeDocumentId, setActiveDocumentId] = useState<string | null>(null);
  const value = useMemo(
    () => ({ activeDocumentId, setActiveDocumentId }),
    [activeDocumentId],
  );
  return (
    <CitationHighlightContext.Provider value={value}>
      {children}
    </CitationHighlightContext.Provider>
  );
}

export const useCitationHighlight = () => useContext(CitationHighlightContext);
