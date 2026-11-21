package parser

import "ragflow/internal/utility"

// ParserCapability describes registration, not runtime readiness or extraction quality.
// Model-assisted ingestion and knowledge representation eligibility are checked by
// their owning services rather than inferred from a file suffix.
type ParserCapability struct {
	FileType   utility.FileType `json:"file_type"`
	BaseResult string           `json:"base_result"`
}

type parserRegistration struct {
	capability ParserCapability
	// create builds the producer. textOnly asks for readable text without
	// embedded media (previews, attachments, agent uploads, CLI parsing).
	create func(textOnly bool) (ParseResultProducer, error)
}

var parserRegistrations = []parserRegistration{
	{ParserCapability{utility.FileTypePPTX, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p := NewPPTXParser()
		p.textOnly = textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypePPT, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p := NewPPTParser()
		p.pptx.textOnly = textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypeXLSX, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p, err := NewXLSXParser("")
		if err != nil {
			return nil, err
		}
		p.textOnly = textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypeXLS, "structured"}, func(bool) (ParseResultProducer, error) { return NewXLSParser("") }},
	{ParserCapability{utility.FileTypeCSV, "structured"}, func(bool) (ParseResultProducer, error) { return NewCSVParser(), nil }},
	{ParserCapability{utility.FileTypeDOCX, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p := NewDOCXParser()
		p.textOnly = textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypeDOC, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p := NewDOCParser()
		p.textOnly = textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypePDF, "structured"}, func(bool) (ParseResultProducer, error) { return NewPDFParser(), nil }},
	{ParserCapability{utility.FileTypeHTML, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p := NewHTMLParser()
		p.textOnly = textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypeMarkdown, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p, err := NewMarkdownParser(GoMarkdown)
		if err != nil {
			return nil, err
		}
		p.textOnly = textOnly
		p.mediaRefs = !textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypeTXT, "structured"}, func(bool) (ParseResultProducer, error) { return NewTextParser(), nil }},
	{ParserCapability{utility.FileTypeEPUB, "structured"}, func(textOnly bool) (ParseResultProducer, error) {
		p := NewEPUBParser()
		p.textOnly = textOnly
		return p, nil
	}},
	{ParserCapability{utility.FileTypeJSON, "structured"}, func(bool) (ParseResultProducer, error) { return NewJSONParser(), nil }},
	{ParserCapability{utility.FileTypeEMAIL, "structured"}, func(bool) (ParseResultProducer, error) { return NewEmailParser(), nil }},
	{ParserCapability{utility.FileTypeVISUAL, "image_payload"}, func(bool) (ParseResultProducer, error) { return NewPictureParser(), nil }},
	{ParserCapability{utility.FileTypeAURAL, "configuration_only"}, func(bool) (ParseResultProducer, error) { return NewAudioParser(), nil }},
}

// RegisteredParserCapabilities returns an independent ordered copy of the same
// registry used by GetParser. A structured result may be empty, unsupported or
// dependent on native libraries; callers must still inspect ParseResult.
func RegisteredParserCapabilities() []ParserCapability {
	capabilities := make([]ParserCapability, len(parserRegistrations))
	for i, registration := range parserRegistrations {
		capabilities[i] = registration.capability
	}
	return capabilities
}
